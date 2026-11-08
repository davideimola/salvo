package newsletter_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/davideimola/salvo/internal/kannon"
	"github.com/davideimola/salvo/internal/newsletter"
	"github.com/davideimola/salvo/internal/store"
)

// domain stands in for Kannon's Domain: the Tracking Policy it allows, or no answer at all.
type domain struct {
	tracking kannon.Tracking
	err      error
	reads    int
}

func (d *domain) DomainTracking(context.Context) (kannon.Tracking, error) {
	d.reads++
	return d.tracking, d.err
}

var pseudoDomain = kannon.Tracking{Opens: kannon.ModePseudonymous, Links: kannon.ModePseudonymous}

// readyToSend saves and test-sends an issue for one Subscriber, leaving only the real send.
func readyToSend(t *testing.T, svc *newsletter.Service, m *mailer, c *clock) {
	t.Helper()
	ctx := context.Background()
	confirmed(t, svc, m, "a@example.org")
	if _, err := svc.SaveIssue(ctx, "2026-11", "November", issueHTML); err != nil {
		t.Fatal(err)
	}
	c.advance(time.Minute)
	if err := svc.TestSend(ctx, "2026-11"); err != nil {
		t.Fatal(err)
	}
}

// refused asserts a send was refused before anything went out, the issue back to draft with
// a reason containing want.
func refused(t *testing.T, svc *newsletter.Service, m *mailer, want string) {
	t.Helper()
	ctx := context.Background()
	before := m.count()
	if _, err := svc.Send(ctx, "2026-11"); !errors.Is(err, newsletter.ErrDomainBelowCeiling) {
		t.Fatalf("send: err = %v, want ErrDomainBelowCeiling", err)
	}
	if m.count() != before {
		t.Error("something went out")
	}
	issue, err := svc.Store.Issue(ctx, "2026-11")
	if err != nil {
		t.Fatal(err)
	}
	if issue.Status != store.Draft || issue.LastError == nil || !strings.Contains(*issue.LastError, want) {
		t.Errorf("issue after the refusal: status %s, reason %v, want draft with %q", issue.Status, issue.LastError, want)
	}
}

func TestASendIsRefusedWhenTheDomainAllowsLessThanTheCeiling(t *testing.T) {
	for _, tc := range []struct {
		name   string
		domain kannon.Tracking
	}{
		{"opens off", kannon.Tracking{Opens: kannon.ModeOff, Links: kannon.ModePseudonymous}},
		{"links off", kannon.Tracking{Opens: kannon.ModePseudonymous, Links: kannon.ModeOff}},
		{"links anonymous", kannon.Tracking{Opens: kannon.ModePseudonymous, Links: "TRACKING_MODE_ANONYMOUS"}},
		{"opens unknown", kannon.Tracking{Opens: "TRACKING_MODE_SOMETHING_NEW", Links: kannon.ModePseudonymous}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, m, c := setup(t)
			raise(svc, both)
			svc.Domain = &domain{tracking: tc.domain}
			readyToSend(t, svc, m, c)
			refused(t, svc, m, "Domain")
		})
	}
}

func TestASendGoesOutWhenTheDomainAllowsTheCeiling(t *testing.T) {
	for _, tc := range []struct {
		name    string
		ceiling func(*newsletter.Service)
		domain  kannon.Tracking
	}{
		{"pseudonymous both", func(s *newsletter.Service) { raise(s, both) }, pseudoDomain},
		{"a Domain above the ceiling", func(s *newsletter.Service) { raise(s, both) },
			kannon.Tracking{Opens: "TRACKING_MODE_IDENTIFIED", Links: "TRACKING_MODE_FULL"}},
		{"links only, opens off on the Domain", func(s *newsletter.Service) { raise(s, linksOnly) },
			kannon.Tracking{Opens: kannon.ModeOff, Links: kannon.ModePseudonymous}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, m, c := setup(t)
			svc.Domain = &domain{tracking: tc.domain}
			tc.ceiling(svc)
			readyToSend(t, svc, m, c)
			issue, err := svc.Send(context.Background(), "2026-11")
			if err != nil {
				t.Fatal(err)
			}
			if issue.Status != store.Sent {
				t.Errorf("status = %s, want sent", issue.Status)
			}
		})
	}
}

func TestWithCeilingsOffTheDomainIsNotChecked(t *testing.T) {
	svc, m, c := setup(t)
	d := &domain{err: errors.New("connection refused")}
	svc.Domain = d
	readyToSend(t, svc, m, c)
	if _, err := svc.Send(context.Background(), "2026-11"); err != nil {
		t.Fatalf("send: %v", err)
	}
	if d.reads != 0 {
		t.Errorf("the Domain was read %d times, want none", d.reads)
	}
}

func TestAKannonThatDoesNotAnswerTheCheckRefusesTheSend(t *testing.T) {
	svc, m, c := setup(t)
	svc.Domain = &domain{err: errors.New("connection refused")}
	raise(svc, both)
	readyToSend(t, svc, m, c)
	refused(t, svc, m, "connection refused")
}

func TestARaisedCeilingWithoutADomainToCheckRefusesTheSend(t *testing.T) {
	svc, m, c := setup(t)
	raise(svc, both)
	svc.Domain = nil
	readyToSend(t, svc, m, c)
	refused(t, svc, m, "admin token")
}

func TestARefusedSendCanBeRetriedOnceTheDomainIsFixed(t *testing.T) {
	svc, m, c := setup(t)
	d := &domain{tracking: kannon.Tracking{Opens: kannon.ModeOff, Links: kannon.ModeOff}}
	svc.Domain = d
	raise(svc, both)
	readyToSend(t, svc, m, c)
	refused(t, svc, m, "Domain")

	d.tracking = pseudoDomain
	if _, err := svc.Send(context.Background(), "2026-11"); err != nil {
		t.Fatalf("retry: %v", err)
	}
}
