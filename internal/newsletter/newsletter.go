// Package newsletter is what a subscription means and when an issue may go out.
//
// The rules, in one place:
//
//   - An address is subscribed only after it clicks the link in the confirmation email
//     (double opt-in). The click's time is the proof of consent.
//   - Asking to subscribe never reveals whether an address is already on the list: every
//     request gets the same answer.
//   - A confirmation link works for ConfirmWindow. An address that never confirms is deleted
//     after PendingRetention.
//   - An unsubscribed address is never mailed again unless it subscribes, and confirms, again.
//   - An issue goes out once. It must be test-sent after its last change first.
//   - Tracking is `off` unless the owner raises a Tracking ceiling. A Subscriber is tracked
//     only with a Tracking consent, given by a confirmation click, and never beyond the lower
//     of the ceiling it was given under and the current one. Every real send states every
//     recipient's policy; test sends and confirmation emails are never tracked.
//   - Every issue links each Subscriber to their Preferences page, required while a ceiling is
//     raised. Withdraw ends the Tracking consent and keeps the subscription.
package newsletter

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/davideimola/salvo/internal/kannon"
	"github.com/davideimola/salvo/internal/store"
	"github.com/davideimola/salvo/internal/tracking"
)

const (
	ConfirmWindow    = 7 * 24 * time.Hour
	PendingRetention = 30 * 24 * time.Hour
	// ResendCooldown stops the form from being used to flood a mailbox with confirmations.
	ResendCooldown = 10 * time.Minute
)

var (
	ErrInvalidEmail      = errors.New("not a valid email address")
	ErrInvalidLink       = errors.New("the link is not valid")
	ErrExpiredLink       = errors.New("the link has expired")
	ErrInvalidIssue      = errors.New("the issue is not valid")
	ErrNoUnsubscribeLink = errors.New("the issue HTML has no {{ unsubscribe_url }}")
	// ErrNoPreferencesLink is an issue saved without its Preferences page link while a
	// Tracking ceiling is raised: no tracked issue may reach a Subscriber without a way to
	// Withdraw.
	ErrNoPreferencesLink = errors.New("the issue HTML has no {{ preferences_url }}, required while tracking is on")
	// ErrTrackingOff is tracking asked for while every Tracking ceiling is `off`.
	ErrTrackingOff = errors.New("tracking was asked for, but this newsletter tracks nothing")
	// ErrDomainBelowCeiling is a send refused because Kannon's Domain allows less tracking
	// than the Tracking ceiling, or could not be asked.
	ErrDomainBelowCeiling = errors.New("Kannon's Domain allows less tracking than the Tracking ceiling")
)

type Mailer interface {
	Send(ctx context.Context, m kannon.Message) (*kannon.Result, error)
}

// DomainPolicy reads the Tracking Policy Kannon's Domain allows: its ceiling, above which
// Kannon rejects each recipient on its own.
type DomainPolicy interface {
	DomainTracking(ctx context.Context) (kannon.Tracking, error)
}

type Service struct {
	Store  *store.Store
	Mailer Mailer
	// PublicURL is this service's origin, the base of the confirmation and unsubscribe links.
	PublicURL string
	// SiteURL is the website's origin: where a confirmed address lands, and where the
	// subscribe form lives.
	SiteURL string
	// TestRecipient receives the test sends.
	TestRecipient string
	// Ident is who the newsletter is; read it through Identity, which fills in the defaults.
	Ident Identity
	// Ceiling is the Tracking ceiling; the zero value tracks nothing.
	Ceiling tracking.Policy
	// NoticeVersion is recorded with every Tracking choice. Required while Ceiling is raised.
	NoticeVersion string
	// Domain is checked before every real send while Ceiling is raised. Required then: without
	// it such a send is refused.
	Domain DomainPolicy
	Now    func() time.Time
}

// NormalizeEmail returns the address as it is stored, or ErrInvalidEmail.
func NormalizeEmail(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	if len(email) > 254 {
		return "", ErrInvalidEmail
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email || addr.Name != "" {
		return "", ErrInvalidEmail
	}
	_, domain, _ := strings.Cut(email, "@")
	if !strings.Contains(domain, ".") || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return "", ErrInvalidEmail
	}
	return email, nil
}

// Subscribe starts the double opt-in for an address, asking for a Tracking consent too when
// wantsTracking. It returns nil whether the address is new, waiting, already confirmed or
// unsubscribed, so the caller cannot tell them apart. Asking for tracking while every
// ceiling is `off` is ErrTrackingOff.
func (s *Service) Subscribe(ctx context.Context, raw string, wantsTracking bool) error {
	if wantsTracking && !s.Ceiling.Raised() {
		return ErrTrackingOff
	}
	email, err := NormalizeEmail(raw)
	if err != nil {
		return err
	}
	now := s.Now()
	token, hash := newConfirmToken()

	sub, err := s.Store.SubscriberByEmail(ctx, email)
	switch {
	case errors.Is(err, store.ErrNotFound):
		sub, err = s.Store.CreatePending(ctx, email, hash, secret(), wantsTracking, now)
		if store.IsUniqueViolation(err) {
			return nil // the same address, submitted twice at once: the other request mails it
		}
		if err != nil {
			return err
		}
	case err != nil:
		return err
	case sub.Status == store.Confirmed:
		return nil
	case sub.Status == store.Pending && sub.ConfirmSentAt != nil && now.Sub(*sub.ConfirmSentAt) < ResendCooldown:
		// No second email inside the cooldown, but the link already sent confirms this
		// request: the latest choice is the one the click makes.
		if err := s.Store.SetConfirmTracking(ctx, sub.ID, wantsTracking); err != nil {
			return err
		}
		return s.requestTracking(ctx, sub.ID, wantsTracking)
	default:
		if err := s.Store.Rearm(ctx, sub.ID, hash, wantsTracking, now); err != nil {
			return err
		}
	}
	if err := s.requestTracking(ctx, sub.ID, wantsTracking); err != nil {
		return err
	}

	if err := s.sendConfirmation(ctx, email, token, wantsTracking); err != nil {
		s.Store.ClearConfirmSent(ctx, sub.ID)
		return err
	}
	return nil
}

// requestTracking records that tracking was asked for and awaits the confirmation click.
func (s *Service) requestTracking(ctx context.Context, subscriberID int64, wantsTracking bool) error {
	if !wantsTracking {
		return nil
	}
	_, err := s.Store.AddTrackingChoice(ctx, subscriberID, s.choice(store.Requested, store.FromSubscribe))
	return err
}

// choice is a Tracking choice made now, under the ceiling and notice in force.
func (s *Service) choice(kind store.ChoiceKind, source store.ChoiceSource) store.TrackingChoice {
	return store.TrackingChoice{Kind: kind, At: s.Now(), Source: source, NoticeVersion: s.NoticeVersion, Ceiling: s.Ceiling}
}

// Confirm turns the token of a confirmation link into a subscription. A link clicked twice
// confirms once and succeeds both times. The same click gives the Tracking consent the
// request asked for, or, while a ceiling is raised, records that it was refused.
func (s *Service) Confirm(ctx context.Context, token string) error {
	sub, err := s.Store.SubscriberByConfirmHash(ctx, hashToken(token))
	if errors.Is(err, store.ErrNotFound) {
		return ErrInvalidLink
	}
	if err != nil {
		return err
	}
	switch sub.Status {
	case store.Confirmed:
		return nil
	case store.Pending:
		if sub.ConfirmSentAt != nil && s.Now().Sub(*sub.ConfirmSentAt) > ConfirmWindow {
			return ErrExpiredLink
		}
		choice, err := s.confirmationChoice(ctx, sub)
		if err != nil {
			return err
		}
		return s.Store.Confirm(ctx, sub.ID, s.Now(), choice)
	}
	return ErrInvalidLink
}

// confirmationChoice is the Tracking choice a confirmation click makes, or nil for none. A
// consent given here is held to the ceiling and notice of the request it confirms, the ones
// its confirmation email described, and never above the current ceiling.
func (s *Service) confirmationChoice(ctx context.Context, sub *store.Subscriber) (*store.TrackingChoice, error) {
	if sub.ConfirmTracking {
		req, err := s.Store.LatestTrackingRequest(ctx, sub.ID)
		switch {
		case err == nil:
			given := s.choice(store.Given, store.FromSubscribe)
			given.NoticeVersion = req.NoticeVersion
			given.Ceiling = req.Ceiling.Min(s.Ceiling)
			return &given, nil
		case !errors.Is(err, store.ErrNotFound):
			return nil, err
		}
	}
	if !s.Ceiling.Raised() {
		return nil, nil
	}
	refused := s.choice(store.Refused, store.FromSubscribe)
	return &refused, nil
}

// Unsubscriber returns the subscriber an unsubscribe link belongs to.
func (s *Service) Unsubscriber(ctx context.Context, token string) (*store.Subscriber, error) {
	if token == "" {
		return nil, ErrInvalidLink
	}
	sub, err := s.Store.SubscriberByUnsubscribeToken(ctx, token)
	if errors.Is(err, store.ErrNotFound) {
		return nil, ErrInvalidLink
	}
	return sub, err
}

func (s *Service) Unsubscribe(ctx context.Context, token string) error {
	sub, err := s.Unsubscriber(ctx, token)
	if err != nil {
		return err
	}
	return s.Store.Unsubscribe(ctx, sub.ID, s.Now())
}

// Cleanup deletes the addresses that asked to subscribe and never confirmed.
func (s *Service) Cleanup(ctx context.Context) (int64, error) {
	return s.Store.DeleteStalePending(ctx, s.Now().Add(-PendingRetention))
}

var (
	slugPattern        = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
	unsubscribePattern = regexp.MustCompile(`\{\{ *unsubscribe_url *\}\}`)
	preferencesPattern = regexp.MustCompile(`\{\{ *preferences_url *\}\}`)
)

// SaveIssue stores an issue as the site rendered it, ready for its test send.
func (s *Service) SaveIssue(ctx context.Context, slug, subject, html string) (*store.Issue, error) {
	subject = strings.TrimSpace(subject)
	if !slugPattern.MatchString(slug) || subject == "" || strings.TrimSpace(html) == "" {
		return nil, ErrInvalidIssue
	}
	if !unsubscribePattern.MatchString(html) {
		return nil, ErrNoUnsubscribeLink
	}
	if s.Ceiling.Raised() && !preferencesPattern.MatchString(html) {
		return nil, ErrNoPreferencesLink
	}
	return s.Store.SaveDraft(ctx, slug, subject, html, s.Now())
}

// TestSend mails the issue to TestRecipient only. Its unsubscribe and preferences links are
// dummies that act on nobody.
func (s *Service) TestSend(ctx context.Context, slug string) error {
	issue, err := s.Store.Issue(ctx, slug)
	if err != nil {
		return err
	}
	if issue.Status != store.Draft {
		return store.ErrAlreadySent
	}
	_, err = s.Mailer.Send(ctx, kannon.Message{
		Subject: "[Test] " + issue.Subject,
		HTML:    issue.HTML,
		Recipients: []kannon.Recipient{{Email: s.TestRecipient, Fields: map[string]string{
			"token": "test", "unsubscribe_url": s.unsubscribeURL("test"),
			"preferences_url": s.preferencesURL("test"),
		}}},
		UnsubscribeURL: s.PublicURL + "/unsubscribe?token={{ token }}",
	})
	if err != nil {
		return err
	}
	return s.Store.MarkTestSent(ctx, slug, s.Now())
}

// Send mails the issue to every confirmed subscriber, once.
func (s *Service) Send(ctx context.Context, slug string) (*store.Issue, error) {
	if err := s.Store.BeginSend(ctx, slug); err != nil {
		return nil, err
	}
	if err := s.checkDomain(ctx); err != nil {
		s.Store.FailSend(ctx, slug, err.Error())
		return nil, err
	}
	issue, err := s.Store.Issue(ctx, slug)
	if err != nil {
		return nil, err
	}
	recipients, err := s.Store.ConfirmedRecipients(ctx)
	if err != nil {
		s.Store.FailSend(ctx, slug, err.Error())
		return nil, err
	}
	if len(recipients) == 0 {
		return s.finish(ctx, slug, store.SendOutcome{}, &kannon.Result{})
	}
	// With every ceiling `off` the message is exactly what it was before tracking existed:
	// batch `off`, no per-recipient field. Otherwise every recipient states its own policy,
	// `off` included, so a field left out can never widen tracking up to the batch's.
	raised := s.Ceiling.Raised()
	outcome := store.SendOutcome{Recipients: len(recipients)}
	to := make([]kannon.Recipient, len(recipients))
	for i, r := range recipients {
		to[i] = kannon.Recipient{Email: r.Email, Fields: map[string]string{
			"token": r.UnsubscribeToken, "unsubscribe_url": s.unsubscribeURL(r.UnsubscribeToken),
			"preferences_url": s.preferencesURL(r.UnsubscribeToken),
		}}
		if raised {
			p := tracking.Effective(r.Consent, s.Ceiling)
			if p.Raised() {
				outcome.Tracked++
			}
			t := kannon.PolicyTracking(p)
			to[i].Tracking = &t
		}
	}
	res, err := s.Mailer.Send(ctx, kannon.Message{
		Subject:        issue.Subject,
		HTML:           issue.HTML,
		Recipients:     to,
		UnsubscribeURL: s.PublicURL + "/unsubscribe?token={{ token }}",
		Tracking:       kannon.PolicyTracking(s.Ceiling),
	})
	var refused *kannon.RefusedError
	switch {
	case errors.As(err, &refused):
		s.Store.FailSend(ctx, slug, err.Error())
		return nil, err
	case err != nil:
		s.Store.NoteSendError(ctx, slug, "outcome unknown, check Kannon before doing anything: "+err.Error())
		return nil, err
	}
	return s.finish(ctx, slug, outcome, res)
}

// checkDomain refuses a send, before anything goes out, when Kannon's Domain would reject the
// Subscribers who consented one by one. With every ceiling `off` there is nothing to check.
func (s *Service) checkDomain(ctx context.Context) error {
	if !s.Ceiling.Raised() {
		return nil
	}
	if s.Domain == nil {
		return fmt.Errorf("%w: the Domain cannot be checked without Kannon's admin token", ErrDomainBelowCeiling)
	}
	got, err := s.Domain.DomainTracking(ctx)
	if err != nil {
		return fmt.Errorf("%w: Kannon did not answer the check of the Domain: %v", ErrDomainBelowCeiling, err)
	}
	if !got.Allows(s.Ceiling) {
		want := kannon.PolicyTracking(s.Ceiling)
		return fmt.Errorf("%w: the Domain allows opens %s and links %s, the ceiling asks for opens %s and links %s",
			ErrDomainBelowCeiling, got.Opens, got.Links, want.Opens, want.Links)
	}
	return nil
}

func (s *Service) finish(ctx context.Context, slug string, o store.SendOutcome, res *kannon.Result) (*store.Issue, error) {
	o.Accepted, o.Rejected, o.MessageID = res.Accepted, len(res.Rejected), res.MessageID
	if err := s.Store.FinishSend(ctx, slug, s.Now(), o); err != nil {
		return nil, err
	}
	return s.Store.Issue(ctx, slug)
}

func (s *Service) unsubscribeURL(token string) string {
	return s.PublicURL + "/unsubscribe?token=" + token
}

// sendConfirmation mails the confirmation link. It is never tracked: the email asking for
// consent is the last place tracking could hide.
func (s *Service) sendConfirmation(ctx context.Context, email, token string, wantsTracking bool) error {
	id := s.Identity()
	html, err := renderConfirmation(id, s.PublicURL+"/confirm?token="+token, s.Ceiling, wantsTracking)
	if err != nil {
		return err
	}
	_, err = s.Mailer.Send(ctx, kannon.Message{
		Subject:    id.ConfirmSubject,
		HTML:       html,
		Recipients: []kannon.Recipient{{Email: email}},
	})
	if err != nil {
		return fmt.Errorf("send the confirmation: %w", err)
	}
	return nil
}

// secret is 32 random bytes, URL-safe. It is the whole of every token: nothing in it is
// derived from the address.
func secret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func newConfirmToken() (string, []byte) {
	t := secret()
	return t, hashToken(t)
}

func hashToken(t string) []byte {
	h := sha256.Sum256([]byte(t))
	return h[:]
}
