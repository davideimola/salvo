package newsletter

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"

	"github.com/davideimola/salvo/internal/kannon"
	"github.com/davideimola/salvo/internal/store"
	"github.com/davideimola/salvo/internal/tracking"
)

// AllowTracking asks for a Tracking consent from the Preferences page: it records
// `requested` and mails the tracking confirmation email, whose click gives the consent. A
// button alone would let anyone holding a forwarded issue consent for its recipient (ADR
// 0001). A Subscriber who already holds a consent records nothing.
func (s *Service) AllowTracking(ctx context.Context, token string) error {
	if !s.Ceiling.Raised() {
		return ErrTrackingOff
	}
	sub, err := s.Unsubscriber(ctx, token)
	if err != nil {
		return err
	}
	return s.askTracking(ctx, sub, store.FromPreferences)
}

// askTracking records a confirmed Subscriber's request for tracking and mails its
// confirmation link. Nothing happens when they already hold a Tracking consent, or when a
// link went out within ResendCooldown: that link still answers the request.
func (s *Service) askTracking(ctx context.Context, sub *store.Subscriber, source store.ChoiceSource) error {
	if sub.Status != store.Confirmed {
		return nil
	}
	latest, err := s.Store.LatestTrackingChoice(ctx, sub.ID)
	switch {
	case err == nil && latest.Kind == store.Given:
		return nil
	case err != nil && !errors.Is(err, store.ErrNotFound):
		return err
	}
	now := s.Now()
	if sub.TrackingConfirmSentAt != nil && now.Sub(*sub.TrackingConfirmSentAt) < ResendCooldown {
		return nil
	}
	token, hash := newConfirmToken()
	err = s.Store.RequestTracking(ctx, sub.ID, hash, s.choice(store.Requested, source))
	if errors.Is(err, store.ErrNotFound) {
		return nil // unsubscribed in the meantime
	}
	if err != nil {
		return err
	}
	if err := s.sendTrackingConfirmation(ctx, sub.Email, token); err != nil {
		s.Store.ClearTrackingConfirmSent(ctx, sub.ID)
		return err
	}
	return nil
}

// ConfirmTracking turns the token of a tracking confirmation link into a Tracking consent.
// The link works once, for ConfirmWindow, and only for the subscription it was mailed in. The
// consent is held to the ceiling and notice of the request it confirms, the ones its email
// described, and never above the current ceiling.
func (s *Service) ConfirmTracking(ctx context.Context, token string) error {
	hash := hashToken(token)
	sub, err := s.Store.SubscriberByTrackingConfirmHash(ctx, hash)
	if errors.Is(err, store.ErrNotFound) {
		return ErrInvalidLink
	}
	if err != nil {
		return err
	}
	if sub.Status != store.Confirmed {
		return ErrInvalidLink
	}
	if sub.TrackingConfirmSentAt == nil || s.Now().Sub(*sub.TrackingConfirmSentAt) > ConfirmWindow {
		return ErrExpiredLink
	}
	req, err := s.Store.LatestTrackingRequest(ctx, sub.ID)
	if errors.Is(err, store.ErrNotFound) {
		return ErrInvalidLink
	}
	if err != nil {
		return err
	}
	given := s.choice(store.Given, req.Source)
	given.NoticeVersion = req.NoticeVersion
	given.Ceiling = req.Ceiling.Min(s.Ceiling)
	err = s.Store.GiveTracking(ctx, sub.ID, hash, given)
	if errors.Is(err, store.ErrNotFound) {
		return ErrInvalidLink // spent by a click a moment earlier
	}
	return err
}

// sendTrackingConfirmation mails the tracking confirmation link. Like the confirmation email,
// it is never tracked.
func (s *Service) sendTrackingConfirmation(ctx context.Context, email, token string) error {
	id := s.Identity()
	html, err := renderTrackingConfirmation(id, s.PublicURL+"/confirm-tracking?token="+token, s.Ceiling)
	if err != nil {
		return err
	}
	_, err = s.Mailer.Send(ctx, kannon.Message{
		Subject:    "Confirm that " + id.Name + " may measure your " + measured(s.Ceiling),
		HTML:       html,
		Recipients: []kannon.Recipient{{Email: email}},
	})
	if err != nil {
		return fmt.Errorf("send the tracking confirmation: %w", err)
	}
	return nil
}

// The tracking confirmation email, in the confirmation email's paper and ink, and like it
// without any image.
var trackingConfirmationTemplate = template.Must(template.New("tracking-confirmation").Parse(`<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Confirm tracking</title></head>
<body style="margin:0;padding:0;background:#F7F5F0;">
<div style="max-width:520px;margin:0 auto;padding:40px 24px;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Helvetica,Arial,sans-serif;color:#1C2233;font-size:16px;line-height:1.6;">
<p style="margin:0 0 24px;font-weight:700;font-size:18px;">{{.Header}}</p>
<p style="margin:0 0 16px;">You asked for {{.Name}} to measure your {{.Measured}} in each issue. It counts only once you confirm it here.</p>
<p style="margin:0 0 24px;"><a href="{{.ConfirmURL}}" style="display:inline-block;background:#1C2233;color:#F7F5F0;text-decoration:none;padding:12px 20px;border-radius:4px;font-weight:600;">Yes, measure my {{.Measured}}</a></p>
<p style="margin:0 0 16px;font-size:14px;color:#3B4152;">Or paste this link into your browser:<br><a href="{{.ConfirmURL}}" style="color:#1C2233;word-break:break-all;">{{.ConfirmURL}}</a></p>
<p style="margin:0 0 16px;">Didn't ask? Ignore this email and nothing changes: the link stops working in seven days. You can withdraw at any time from the preferences link in every issue and stay subscribed.</p>
{{if .SignOff}}<p style="margin:0 0 16px;">{{.SignOff}}</p>
{{end}}<p style="margin:32px 0 0;font-size:13px;color:#5B6070;">This email carries no tracking pixel and no tracked links.{{if .PrivacyURL}} <a href="{{.PrivacyURL}}" style="color:#5B6070;">Privacy</a>{{end}}</p>
</div>
</body>
</html>
`))

func renderTrackingConfirmation(id Identity, confirmURL string, ceiling tracking.Policy) (string, error) {
	header := id.Sender
	if header == "" {
		header = id.Name
	}
	var b bytes.Buffer
	err := trackingConfirmationTemplate.Execute(&b, struct {
		Header, Name, SignOff, PrivacyURL, ConfirmURL, Measured string
	}{header, id.Name, id.SignOff, id.PrivacyURL, confirmURL, measured(ceiling)})
	return b.String(), err
}
