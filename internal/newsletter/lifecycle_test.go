package newsletter_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/davideimola/salvo/internal/kannon"
	"github.com/davideimola/salvo/internal/tracking"
)

var (
	pseudoBoth = kannon.Tracking{Opens: kannon.ModePseudonymous, Links: kannon.ModePseudonymous}
	pseudoLink = kannon.Tracking{Opens: kannon.ModeOff, Links: kannon.ModePseudonymous}
	offBoth    = kannon.Tracking{Opens: kannon.ModeOff, Links: kannon.ModeOff}
)

func TestUnsubscribingRecordsNoWithdraw(t *testing.T) {
	ctx := context.Background()
	svc, m, _ := setup(t)
	raise(svc, both)
	join(t, svc, m, "a@example.org", true)
	sub, _ := svc.Store.SubscriberByEmail(ctx, "a@example.org")
	if err := svc.Unsubscribe(ctx, sub.UnsubscribeToken); err != nil {
		t.Fatal(err)
	}
	if got := kinds(choices(t, svc, "a@example.org")); got != "requested,given" {
		t.Errorf("choices = %s: unsubscribing is not a Withdraw", got)
	}
}

func TestAnOldConsentIsNotRevivedWhenTheNewSubscriptionRecordsNothing(t *testing.T) {
	ctx := context.Background()
	svc, m, c := setup(t)
	raise(svc, both)
	join(t, svc, m, "a@example.org", true)
	sub, _ := svc.Store.SubscriberByEmail(ctx, "a@example.org")
	svc.Unsubscribe(ctx, sub.UnsubscribeToken)

	// Back while every ceiling is off: the new subscription records no choice at all.
	svc.Ceiling = tracking.None
	c.advance(time.Hour)
	join(t, svc, m, "a@example.org", false)
	raise(svc, both)

	_, msg := sendIssue(t, svc, m, c)
	if got := recipientTracking(t, msg)["a@example.org"]; got != offBoth {
		t.Errorf("tracking = %+v, want off: the consent belonged to the earlier subscription", got)
	}
	if got := kinds(choices(t, svc, "a@example.org")); got != "requested,given" {
		t.Errorf("choices = %s: the earlier ones stay as proof", got)
	}
}

func TestAResubscriptionTicksTheBoxAfresh(t *testing.T) {
	ctx := context.Background()
	svc, m, c := setup(t)
	raise(svc, both)
	join(t, svc, m, "a@example.org", true)
	sub, _ := svc.Store.SubscriberByEmail(ctx, "a@example.org")
	svc.Unsubscribe(ctx, sub.UnsubscribeToken)

	// Asking for tracking again with the new subscription is not enough: only its click counts.
	c.advance(time.Hour)
	svc.Subscribe(ctx, "a@example.org", true)
	confirmed(t, svc, m, "other@example.org")
	_, msg := sendIssue(t, svc, m, c)
	for _, r := range msg.Recipients {
		if r.Email == "a@example.org" {
			t.Fatal("a resubscription awaiting its click was sent the issue")
		}
	}

	c.advance(time.Minute)
	svc.Confirm(ctx, confirmToken(t, lastTo(t, m, "a@example.org")))
	if got := kinds(choices(t, svc, "a@example.org")); got != "requested,given,requested,given" {
		t.Errorf("choices = %s", got)
	}
}

func TestALoweredCeilingAppliesToEveryone(t *testing.T) {
	svc, m, c := setup(t)
	raise(svc, linksOnly)
	join(t, svc, m, "links@example.org", true)
	raise(svc, both)
	join(t, svc, m, "both@example.org", true)
	join(t, svc, m, "no@example.org", false)

	raise(svc, tracking.Policy{Opens: tracking.Off, Links: tracking.Pseudonymous})
	issue, msg := sendIssue(t, svc, m, c)
	got := recipientTracking(t, msg)
	want := map[string]kannon.Tracking{"links@example.org": pseudoLink, "both@example.org": pseudoLink, "no@example.org": offBoth}
	for email, w := range want {
		if got[email] != w {
			t.Errorf("%s tracking = %+v, want %+v", email, got[email], w)
		}
	}
	if *issue.TrackedRecipients != 2 {
		t.Errorf("tracked = %d, want 2", *issue.TrackedRecipients)
	}
}

func TestCeilingsLoweredToOffTrackNobody(t *testing.T) {
	svc, m, c := setup(t)
	raise(svc, both)
	join(t, svc, m, "yes@example.org", true)
	svc.Ceiling = tracking.None

	issue, msg := sendIssue(t, svc, m, c)
	if msg.Tracking.Raised() {
		t.Errorf("batch tracking = %+v", msg.Tracking)
	}
	for _, r := range msg.Recipients {
		if r.Tracking != nil && r.Tracking.Raised() {
			t.Errorf("%s tracking = %+v, want off", r.Email, r.Tracking)
		}
	}
	if *issue.TrackedRecipients != 0 {
		t.Errorf("tracked = %d, want 0", *issue.TrackedRecipients)
	}
}

func TestSubscribersFromBeforeTrackingAndKitImportsAreNeverTracked(t *testing.T) {
	ctx := context.Background()
	svc, m, c := setup(t)
	confirmed(t, svc, m, "before@example.org") // joined while every ceiling was off
	if _, err := svc.ImportKit(ctx, strings.NewReader(kitExport)); err != nil {
		t.Fatal(err)
	}
	raise(svc, both)
	if _, err := svc.ImportKit(ctx, strings.NewReader(
		"first_name,email,created_at,status,tags,city,state,country\n,late@example.org,2026-07-22 14:31:55 UTC,active,,,,\n")); err != nil {
		t.Fatal(err)
	}

	issue, msg := sendIssue(t, svc, m, c)
	got := recipientTracking(t, msg)
	for _, email := range []string{"before@example.org", "reader@example.org", "already@example.org", "late@example.org"} {
		if got[email] != offBoth {
			t.Errorf("%s tracking = %+v, want off", email, got[email])
		}
		if cs := choices(t, svc, email); len(cs) != 0 {
			t.Errorf("%s choices = %s, want none", email, kinds(cs))
		}
	}
	if *issue.TrackedRecipients != 0 {
		t.Errorf("tracked = %d, want 0", *issue.TrackedRecipients)
	}
}

// lastTo is the latest message sent to an address alone.
func lastTo(t *testing.T, m *mailer, email string) kannon.Message {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := len(m.sent) - 1; i >= 0; i-- {
		if msg := m.sent[i]; len(msg.Recipients) == 1 && msg.Recipients[0].Email == email {
			return msg
		}
	}
	t.Fatalf("nothing sent to %s", email)
	return kannon.Message{}
}
