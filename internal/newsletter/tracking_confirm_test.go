package newsletter_test

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/davideimola/salvo/internal/kannon"
	"github.com/davideimola/salvo/internal/newsletter"
	"github.com/davideimola/salvo/internal/store"
)

var trackingConfirmLink = regexp.MustCompile(`https://news\.example\.com/confirm-tracking\?token=([A-Za-z0-9_-]+)`)

func trackingToken(t *testing.T, m kannon.Message) string {
	t.Helper()
	match := trackingConfirmLink.FindStringSubmatch(m.HTML)
	if match == nil {
		t.Fatalf("no tracking confirmation link in:\n%s", m.HTML)
	}
	return match[1]
}

// untracked fails unless the message tracks nobody: off at batch level, no per-recipient
// field.
func untracked(t *testing.T, msg kannon.Message) {
	t.Helper()
	if msg.Tracking.Opens != "" && msg.Tracking.Opens != kannon.ModeOff || msg.Tracking.Links != "" && msg.Tracking.Links != kannon.ModeOff {
		t.Errorf("tracking = %+v, want off", msg.Tracking)
	}
	for _, r := range msg.Recipients {
		if r.Tracking != nil {
			t.Errorf("recipient tracking = %+v, want none", r.Tracking)
		}
	}
}

func TestTrackingAskedForFromThePreferencesPageCountsOnlyAfterTheClick(t *testing.T) {
	ctx := context.Background()
	svc, m, c := setup(t)
	raise(svc, both)
	join(t, svc, m, "a@example.org", false)
	token := unsubscribeToken(t, svc, "a@example.org")
	before := m.count()

	c.advance(time.Hour)
	if err := svc.AllowTracking(ctx, token); err != nil {
		t.Fatal(err)
	}
	if m.count() != before+1 {
		t.Fatalf("mailed %d, want the tracking confirmation email", m.count()-before)
	}
	email := m.last()
	if len(email.Recipients) != 1 || email.Recipients[0].Email != "a@example.org" {
		t.Errorf("tracking confirmation recipients = %+v", email.Recipients)
	}
	untracked(t, email)
	cs := choices(t, svc, "a@example.org")
	if kinds(cs) != "refused,requested" {
		t.Fatalf("choices after asking = %s", kinds(cs))
	}
	if r := cs[1]; r.Source != store.FromPreferences || r.NoticeVersion != "2026-11" || r.Ceiling != both || !r.At.Equal(c.now()) {
		t.Errorf("requested = %+v", r)
	}
	if p, _ := svc.Preferences(ctx, token); p.Consented() {
		t.Fatal("asking alone must not give a Tracking consent")
	}

	c.advance(time.Hour)
	if err := svc.ConfirmTracking(ctx, trackingToken(t, email)); err != nil {
		t.Fatal(err)
	}
	cs = choices(t, svc, "a@example.org")
	if kinds(cs) != "refused,requested,given" {
		t.Fatalf("choices after the click = %s", kinds(cs))
	}
	if g := cs[2]; g.Source != store.FromPreferences || g.NoticeVersion != "2026-11" || g.Ceiling != both || !g.At.Equal(c.now()) {
		t.Errorf("given = %+v", g)
	}

	issue, msg := sendIssue(t, svc, m, c)
	pseudo := kannon.Tracking{Opens: kannon.ModePseudonymous, Links: kannon.ModePseudonymous}
	if got := recipientTracking(t, msg)["a@example.org"]; got != pseudo {
		t.Errorf("tracking after the click = %+v, want %+v", got, pseudo)
	}
	if *issue.TrackedRecipients != 1 {
		t.Errorf("tracked recipients = %d, want 1", *issue.TrackedRecipients)
	}
}

func TestAConfirmedSubscriberTickingTheBoxGetsTheTrackingConfirmationEmail(t *testing.T) {
	ctx := context.Background()
	svc, m, c := setup(t)
	raise(svc, both)
	join(t, svc, m, "a@example.org", false)
	before := m.count()

	c.advance(time.Hour)
	if err := svc.Subscribe(ctx, "A@example.org", true); err != nil {
		t.Fatal(err)
	}
	if m.count() != before+1 {
		t.Fatalf("mailed %d, want the tracking confirmation email", m.count()-before)
	}
	email := m.last()
	untracked(t, email)
	if confirmLink.MatchString(email.HTML) {
		t.Error("an address already on the list must not be asked to subscribe again")
	}
	if err := svc.ConfirmTracking(ctx, trackingToken(t, email)); err != nil {
		t.Fatal(err)
	}
	cs := choices(t, svc, "a@example.org")
	if kinds(cs) != "refused,requested,given" {
		t.Fatalf("choices = %s", kinds(cs))
	}
	if cs[1].Source != store.FromSubscribe || cs[2].Source != store.FromSubscribe {
		t.Errorf("sources = %s, %s; want subscribe", cs[1].Source, cs[2].Source)
	}
	if status(t, svc, "a@example.org") != store.Confirmed {
		t.Error("the subscription must be untouched")
	}
}

func TestAConfirmedSubscriberLeavingTheBoxBlankChangesNothing(t *testing.T) {
	ctx := context.Background()
	svc, m, c := setup(t)
	raise(svc, both)
	join(t, svc, m, "yes@example.org", true)
	join(t, svc, m, "no@example.org", false)
	before := m.count()

	c.advance(time.Hour)
	for _, email := range []string{"yes@example.org", "no@example.org"} {
		if err := svc.Subscribe(ctx, email, false); err != nil {
			t.Fatal(err)
		}
	}
	if m.count() != before {
		t.Errorf("mailed %d, want nothing", m.count()-before)
	}
	if got := kinds(choices(t, svc, "yes@example.org")); got != "requested,given" {
		t.Errorf("consented: choices = %s", got)
	}
	if got := kinds(choices(t, svc, "no@example.org")); got != "refused" {
		t.Errorf("refused: choices = %s", got)
	}
}

// asked joins an address without Tracking consent and asks for it from the Preferences page,
// returning the tracking confirmation email.
func asked(t *testing.T, svc *newsletter.Service, m *mailer, email string) kannon.Message {
	t.Helper()
	join(t, svc, m, email, false)
	if err := svc.AllowTracking(context.Background(), unsubscribeToken(t, svc, email)); err != nil {
		t.Fatal(err)
	}
	return m.last()
}

func TestTheTrackingConfirmationLinkWorksOnce(t *testing.T) {
	ctx := context.Background()
	svc, m, _ := setup(t)
	raise(svc, both)
	token := trackingToken(t, asked(t, svc, m, "a@example.org"))
	if err := svc.ConfirmTracking(ctx, token); err != nil {
		t.Fatal(err)
	}
	if err := svc.ConfirmTracking(ctx, token); !errors.Is(err, newsletter.ErrInvalidLink) {
		t.Errorf("second click: err = %v, want ErrInvalidLink", err)
	}
	if got := kinds(choices(t, svc, "a@example.org")); got != "refused,requested,given" {
		t.Errorf("choices = %s", got)
	}
	sub, _ := svc.Store.SubscriberByEmail(ctx, "a@example.org")
	if _, err := svc.Store.SubscriberByTrackingConfirmHash(ctx, []byte(token)); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("the token must never be stored as it is: err = %v", err)
	}
	if sub.Status != store.Confirmed {
		t.Errorf("status = %s", sub.Status)
	}
}

func TestTheTrackingConfirmationLinkExpires(t *testing.T) {
	ctx := context.Background()
	svc, m, c := setup(t)
	raise(svc, both)
	token := trackingToken(t, asked(t, svc, m, "a@example.org"))
	c.advance(newsletter.ConfirmWindow + time.Minute)
	if err := svc.ConfirmTracking(ctx, token); !errors.Is(err, newsletter.ErrExpiredLink) {
		t.Fatalf("err = %v, want ErrExpiredLink", err)
	}
	if got := kinds(choices(t, svc, "a@example.org")); got != "refused,requested" {
		t.Errorf("choices = %s", got)
	}
}

func TestTheTrackingConfirmationRespectsTheResendCooldown(t *testing.T) {
	ctx := context.Background()
	svc, m, c := setup(t)
	raise(svc, both)
	first := trackingToken(t, asked(t, svc, m, "a@example.org"))
	pref := unsubscribeToken(t, svc, "a@example.org")
	before := m.count()

	c.advance(time.Minute)
	svc.AllowTracking(ctx, pref)
	svc.Subscribe(ctx, "a@example.org", true)
	if m.count() != before {
		t.Fatalf("mailed %d times inside the cooldown, want none", m.count()-before)
	}

	c.advance(newsletter.ResendCooldown)
	if err := svc.AllowTracking(ctx, pref); err != nil {
		t.Fatal(err)
	}
	if m.count() != before+1 {
		t.Fatalf("mailed %d times after the cooldown, want 1", m.count()-before)
	}
	if err := svc.ConfirmTracking(ctx, first); !errors.Is(err, newsletter.ErrInvalidLink) {
		t.Errorf("the replaced link: err = %v, want ErrInvalidLink", err)
	}
	if err := svc.ConfirmTracking(ctx, trackingToken(t, m.last())); err != nil {
		t.Errorf("the new link: %v", err)
	}
}

func TestAFailedTrackingConfirmationCanBeRetriedAtOnce(t *testing.T) {
	ctx := context.Background()
	svc, m, _ := setup(t)
	raise(svc, both)
	join(t, svc, m, "a@example.org", false)
	pref := unsubscribeToken(t, svc, "a@example.org")
	m.err = errors.New("kannon is down")
	if err := svc.AllowTracking(ctx, pref); err == nil {
		t.Fatal("want the send error")
	}
	m.err = nil
	if err := svc.AllowTracking(ctx, pref); err != nil {
		t.Fatal(err)
	}
	if err := svc.ConfirmTracking(ctx, trackingToken(t, m.last())); err != nil {
		t.Fatal(err)
	}
}

func TestATrackingConfirmationLinkDiesWithItsSubscription(t *testing.T) {
	ctx := context.Background()
	svc, m, _ := setup(t)
	raise(svc, both)
	token := trackingToken(t, asked(t, svc, m, "a@example.org"))
	svc.Unsubscribe(ctx, unsubscribeToken(t, svc, "a@example.org"))
	if err := svc.ConfirmTracking(ctx, token); !errors.Is(err, newsletter.ErrInvalidLink) {
		t.Fatalf("after Unsubscribe: err = %v, want ErrInvalidLink", err)
	}
	join(t, svc, m, "a@example.org", false)
	if err := svc.ConfirmTracking(ctx, token); !errors.Is(err, newsletter.ErrInvalidLink) {
		t.Fatalf("in the next subscription: err = %v, want ErrInvalidLink", err)
	}
	if p, _ := svc.Preferences(ctx, unsubscribeToken(t, svc, "a@example.org")); p.Consented() {
		t.Error("an old link must not give a consent to a new subscription")
	}
}

func TestAskingForTrackingWithAConsentAlreadyGivenDoesNothing(t *testing.T) {
	ctx := context.Background()
	svc, m, _ := setup(t)
	raise(svc, both)
	join(t, svc, m, "a@example.org", true)
	before := m.count()
	svc.AllowTracking(ctx, unsubscribeToken(t, svc, "a@example.org"))
	svc.Subscribe(ctx, "a@example.org", true)
	if m.count() != before {
		t.Errorf("mailed %d, want nothing", m.count()-before)
	}
	if got := kinds(choices(t, svc, "a@example.org")); got != "requested,given" {
		t.Errorf("choices = %s", got)
	}
}

func TestAskingForTrackingWhileOffIsRefused(t *testing.T) {
	svc, m, _ := setup(t)
	confirmed(t, svc, m, "a@example.org")
	if err := svc.AllowTracking(context.Background(), unsubscribeToken(t, svc, "a@example.org")); !errors.Is(err, newsletter.ErrTrackingOff) {
		t.Errorf("err = %v, want ErrTrackingOff", err)
	}
}

func TestTheTrackingConfirmationEmailSaysWhatItAgreesTo(t *testing.T) {
	svc, m, _ := setup(t)
	raise(svc, linksOnly)
	email := asked(t, svc, m, "a@example.org")
	for _, want := range []string{"measure your clicks", "seven days", "no tracking pixel"} {
		if !strings.Contains(email.HTML, want) {
			t.Errorf("email lacks %q:\n%s", want, email.HTML)
		}
	}
	if strings.Contains(email.HTML, "opens") || strings.Contains(email.HTML, "<img") {
		t.Errorf("email names opens or carries an image:\n%s", email.HTML)
	}
}

func TestResubscribingAndAskingForTrackingAtOnceSendsTheTrackingConfirmation(t *testing.T) {
	ctx := context.Background()
	svc, m, c := setup(t)
	raise(svc, both)
	asked(t, svc, m, "a@example.org")
	svc.Unsubscribe(ctx, unsubscribeToken(t, svc, "a@example.org"))

	c.advance(time.Minute)
	join(t, svc, m, "a@example.org", false)
	before := m.count()
	if err := svc.AllowTracking(ctx, unsubscribeToken(t, svc, "a@example.org")); err != nil {
		t.Fatal(err)
	}
	if m.count() != before+1 {
		t.Fatalf("mailed %d, want the tracking confirmation email of the new subscription", m.count()-before)
	}
	if err := svc.ConfirmTracking(ctx, trackingToken(t, m.last())); err != nil {
		t.Fatal(err)
	}
}
