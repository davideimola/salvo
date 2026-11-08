package newsletter

import (
	"context"
	"errors"

	"github.com/davideimola/salvo/internal/store"
	"github.com/davideimola/salvo/internal/tracking"
)

// Preferences is what the Preferences page shows a Subscriber. It is addressed by the
// Subscriber's unsubscribe token: that link can already act only on its own address, and a
// second secret would add nothing.
type Preferences struct {
	Email string
	// Subscribed is false once the address has unsubscribed: nothing reaches it, so nothing
	// is measured either.
	Subscribed bool
	// Ceiling is the Tracking ceiling in force. Not raised, the newsletter tracks nothing.
	Ceiling tracking.Policy
	// Measured is what the next issue would measure for this Subscriber.
	Measured tracking.Policy
	// Latest is the latest Tracking choice of the current subscription, nil for none.
	Latest *store.TrackingChoice

	subscriberID int64
}

// Consented reports whether the Subscriber holds a Tracking consent now.
func (p *Preferences) Consented() bool {
	return p.Subscribed && p.Latest != nil && p.Latest.Kind == store.Given
}

// Preferences reads the Subscriber's tracking state for their Preferences page. It changes
// nothing: a mail scanner following the link must not withdraw or give consent.
func (s *Service) Preferences(ctx context.Context, token string) (*Preferences, error) {
	sub, err := s.Unsubscriber(ctx, token)
	if err != nil {
		return nil, err
	}
	p := &Preferences{subscriberID: sub.ID, Email: sub.Email, Subscribed: sub.Status == store.Confirmed, Ceiling: s.Ceiling, Measured: tracking.None}
	latest, err := s.Store.LatestTrackingChoice(ctx, sub.ID)
	switch {
	case err == nil:
		p.Latest = latest
	case !errors.Is(err, store.ErrNotFound):
		return nil, err
	}
	if p.Consented() {
		p.Measured = tracking.Effective(&p.Latest.Ceiling, s.Ceiling)
	}
	return p, nil
}

// Withdraw ends the Tracking consent of the Subscriber whose Preferences p were just read;
// they stay subscribed. Without a consent to end, it records nothing, so a second press or a
// stale page, read again, adds no noise to the proof.
func (s *Service) Withdraw(ctx context.Context, p *Preferences) error {
	if !p.Consented() {
		return nil
	}
	_, err := s.Store.AddTrackingChoice(ctx, p.subscriberID, s.choice(store.Withdrawn, store.FromPreferences))
	return err
}

func (s *Service) preferencesURL(token string) string {
	return s.PublicURL + "/preferences?token=" + token
}
