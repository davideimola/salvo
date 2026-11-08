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
	"github.com/davideimola/salvo/internal/tracking"
)

var (
	both      = tracking.Policy{Opens: tracking.Pseudonymous, Links: tracking.Pseudonymous}
	linksOnly = tracking.Policy{Opens: tracking.Off, Links: tracking.Pseudonymous}
)

// raise sets the Tracking ceiling, as an owner does in configuration.
func raise(svc *newsletter.Service, ceiling tracking.Policy) {
	svc.Ceiling = ceiling
	svc.NoticeVersion = "2026-11"
}

// join subscribes and confirms an address, ticking the tracking box or not.
func join(t *testing.T, svc *newsletter.Service, m *mailer, email string, wantsTracking bool) {
	t.Helper()
	if err := svc.Subscribe(context.Background(), email, wantsTracking); err != nil {
		t.Fatal(err)
	}
	if err := svc.Confirm(context.Background(), confirmToken(t, m.last())); err != nil {
		t.Fatal(err)
	}
}

func choices(t *testing.T, svc *newsletter.Service, email string) []store.TrackingChoice {
	t.Helper()
	sub, err := svc.Store.SubscriberByEmail(context.Background(), email)
	if err != nil {
		t.Fatal(err)
	}
	cs, err := svc.Store.TrackingChoices(context.Background(), sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	return cs
}

func kinds(cs []store.TrackingChoice) string {
	var out []string
	for _, c := range cs {
		out = append(out, string(c.Kind))
	}
	return strings.Join(out, ",")
}

// sendIssue saves, test-sends and sends an issue, returning the message the real send made.
func sendIssue(t *testing.T, svc *newsletter.Service, m *mailer, c *clock) (*store.Issue, kannon.Message) {
	t.Helper()
	ctx := context.Background()
	if _, err := svc.SaveIssue(ctx, "2026-11", "November", issueHTML); err != nil {
		t.Fatal(err)
	}
	c.advance(time.Minute)
	if err := svc.TestSend(ctx, "2026-11"); err != nil {
		t.Fatal(err)
	}
	test := m.last()
	if test.Tracking.Opens != "" && test.Tracking.Opens != kannon.ModeOff || test.Tracking.Links != "" && test.Tracking.Links != kannon.ModeOff {
		t.Errorf("test send tracking = %+v, want off", test.Tracking)
	}
	for _, r := range test.Recipients {
		if r.Tracking != nil {
			t.Errorf("test send recipient tracking = %+v, want none", r.Tracking)
		}
	}
	c.advance(time.Minute)
	issue, err := svc.Send(ctx, "2026-11")
	if err != nil {
		t.Fatal(err)
	}
	return issue, m.last()
}

func recipientTracking(t *testing.T, msg kannon.Message) map[string]kannon.Tracking {
	t.Helper()
	out := map[string]kannon.Tracking{}
	for _, r := range msg.Recipients {
		if r.Tracking == nil {
			t.Fatalf("%s has no tracking of its own: every recipient must state it", r.Email)
		}
		out[r.Email] = *r.Tracking
	}
	return out
}

func TestAMixedBatchTracksOnlyWhoConsented(t *testing.T) {
	svc, m, c := setup(t)
	raise(svc, both)
	join(t, svc, m, "yes@example.org", true)
	join(t, svc, m, "no@example.org", false)
	confirmed(t, svc, m, "before@example.org") // joined while ceilings are raised, unticked

	issue, msg := sendIssue(t, svc, m, c)

	pseudo := kannon.Tracking{Opens: kannon.ModePseudonymous, Links: kannon.ModePseudonymous}
	off := kannon.Tracking{Opens: kannon.ModeOff, Links: kannon.ModeOff}
	if msg.Tracking != pseudo {
		t.Errorf("batch tracking = %+v, want the ceiling", msg.Tracking)
	}
	got := recipientTracking(t, msg)
	want := map[string]kannon.Tracking{"yes@example.org": pseudo, "no@example.org": off, "before@example.org": off}
	for email, w := range want {
		if got[email] != w {
			t.Errorf("%s tracking = %+v, want %+v", email, got[email], w)
		}
	}
	if issue.TrackedRecipients == nil || *issue.TrackedRecipients != 1 || *issue.Recipients != 3 {
		t.Errorf("tracked = %v of %v, want 1 of 3", issue.TrackedRecipients, issue.Recipients)
	}
}

func TestACeilingRaisedAfterConsentDoesNotReachBeyondIt(t *testing.T) {
	svc, m, c := setup(t)
	raise(svc, linksOnly)
	join(t, svc, m, "yes@example.org", true)
	raise(svc, both)

	_, msg := sendIssue(t, svc, m, c)
	if got := recipientTracking(t, msg)["yes@example.org"]; got != (kannon.Tracking{Opens: kannon.ModeOff, Links: kannon.ModePseudonymous}) {
		t.Errorf("tracking = %+v, want only links, as agreed", got)
	}
}

func TestACeilingLoweredAfterConsentLowersTheTracking(t *testing.T) {
	svc, m, c := setup(t)
	raise(svc, both)
	join(t, svc, m, "yes@example.org", true)
	raise(svc, linksOnly)

	_, msg := sendIssue(t, svc, m, c)
	if msg.Tracking != (kannon.Tracking{Opens: kannon.ModeOff, Links: kannon.ModePseudonymous}) {
		t.Errorf("batch tracking = %+v", msg.Tracking)
	}
	if got := recipientTracking(t, msg)["yes@example.org"]; got != (kannon.Tracking{Opens: kannon.ModeOff, Links: kannon.ModePseudonymous}) {
		t.Errorf("tracking = %+v, want only links, the current ceiling", got)
	}
}

func TestWithCeilingsOffASendIsAsBefore(t *testing.T) {
	svc, m, c := setup(t)
	confirmed(t, svc, m, "a@example.org")
	issue, msg := sendIssue(t, svc, m, c)
	if msg.Tracking.Raised() {
		t.Errorf("batch tracking = %+v", msg.Tracking)
	}
	for _, r := range msg.Recipients {
		if r.Tracking != nil {
			t.Errorf("%s tracking = %+v, want no field, as before", r.Email, r.Tracking)
		}
	}
	if issue.TrackedRecipients == nil || *issue.TrackedRecipients != 0 {
		t.Errorf("tracked = %v, want 0", issue.TrackedRecipients)
	}
	if cs := choices(t, svc, "a@example.org"); len(cs) != 0 {
		t.Errorf("choices = %s, want none while tracking is off", kinds(cs))
	}
}

func TestTrackingAskedForWhileOffIsRefused(t *testing.T) {
	svc, m, _ := setup(t)
	if err := svc.Subscribe(context.Background(), "a@example.org", true); !errors.Is(err, newsletter.ErrTrackingOff) {
		t.Fatalf("err = %v, want ErrTrackingOff", err)
	}
	if m.count() != 0 {
		t.Error("nothing should have been sent")
	}
}

func TestTrackingConsentIsGivenOnlyByTheClick(t *testing.T) {
	ctx := context.Background()
	svc, m, c := setup(t)
	raise(svc, both)
	svc.Subscribe(ctx, "yes@example.org", true)
	if got := kinds(choices(t, svc, "yes@example.org")); got != "requested" {
		t.Fatalf("before the click: %s, want requested", got)
	}
	// An address that never clicks is never tracked.
	confirmed(t, svc, m, "other@example.org")
	_, msg := sendIssue(t, svc, m, c)
	for _, r := range msg.Recipients {
		if r.Email == "yes@example.org" {
			t.Fatal("a pending address was sent the issue")
		}
	}

	c.advance(time.Minute)
	svc.Confirm(ctx, confirmToken(t, firstTo(t, m, "yes@example.org")))
	cs := choices(t, svc, "yes@example.org")
	if kinds(cs) != "requested,given" {
		t.Fatalf("after the click: %s", kinds(cs))
	}
	given := cs[1]
	if given.Source != store.FromSubscribe || given.NoticeVersion != "2026-11" || given.Ceiling != both || !given.At.Equal(c.now()) {
		t.Errorf("given = %+v", given)
	}
	// A second click gives nothing more.
	svc.Confirm(ctx, confirmToken(t, firstTo(t, m, "yes@example.org")))
	if got := kinds(choices(t, svc, "yes@example.org")); got != "requested,given" {
		t.Errorf("after a second click: %s", got)
	}
}

func TestAConfirmationWithoutTheBoxRecordsARefusal(t *testing.T) {
	svc, m, _ := setup(t)
	raise(svc, linksOnly)
	join(t, svc, m, "no@example.org", false)
	cs := choices(t, svc, "no@example.org")
	if kinds(cs) != "refused" || cs[0].Ceiling != linksOnly || cs[0].NoticeVersion != "2026-11" {
		t.Errorf("choices = %+v, want one refusal under the ceiling in force", cs)
	}
}

func TestThePendingSubscribersLatestRequestIsTheOneConfirmed(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name          string
		first, second bool
		want          string
	}{
		{"ticked then unticked", true, false, "requested,refused"},
		{"unticked then ticked", false, true, "requested,given"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, m, c := setup(t)
			raise(svc, both)
			svc.Subscribe(ctx, "a@example.org", tc.first)
			c.advance(time.Minute) // inside the resend cooldown: no second email
			svc.Subscribe(ctx, "a@example.org", tc.second)
			if m.count() != 1 {
				t.Fatalf("mailed %d times, want 1", m.count())
			}
			svc.Confirm(ctx, confirmToken(t, m.last()))
			if got := kinds(choices(t, svc, "a@example.org")); got != tc.want {
				t.Errorf("choices = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestTheConfirmationEmailSaysWhatIsMeasured(t *testing.T) {
	ctx := context.Background()
	svc, m, _ := setup(t)
	svc.Subscribe(ctx, "off@example.org", false)
	if html := m.last().HTML; !strings.Contains(html, "The newsletter carries no tracking pixel and no tracked links.") {
		t.Errorf("with ceilings off the line must be unchanged:\n%s", html)
	}

	raise(svc, linksOnly)
	svc.Subscribe(ctx, "no@example.org", false)
	html := m.last().HTML
	if strings.Contains(html, "no tracking pixel") || !strings.Contains(html, "clicks") || strings.Contains(html, "opens") {
		t.Errorf("unticked, links only:\n%s", html)
	}
	if strings.Contains(html, "confirming also agrees") {
		t.Errorf("unticked, the email must not say confirming agrees to tracking:\n%s", html)
	}

	raise(svc, both)
	svc.Subscribe(ctx, "yes@example.org", true)
	html = m.last().HTML
	if !strings.Contains(html, "opens and clicks") || !strings.Contains(html, "confirming also agrees") {
		t.Errorf("ticked, both:\n%s", html)
	}
	if strings.Contains(strings.ToLower(html), "<img") {
		t.Error("the confirmation email must carry no image")
	}
	if m.last().Tracking.Raised() {
		t.Errorf("the confirmation email is tracked: %+v", m.last().Tracking)
	}
}

func TestNoTrackingConsentCarriesOverToANewSubscription(t *testing.T) {
	ctx := context.Background()
	svc, m, c := setup(t)
	raise(svc, both)
	join(t, svc, m, "a@example.org", true)
	sub, _ := svc.Store.SubscriberByEmail(ctx, "a@example.org")
	svc.Unsubscribe(ctx, sub.UnsubscribeToken)

	c.advance(time.Hour)
	svc.Subscribe(ctx, "a@example.org", false)
	svc.Confirm(ctx, confirmToken(t, m.last()))
	_, msg := sendIssue(t, svc, m, c)
	if got := recipientTracking(t, msg)["a@example.org"]; got.Raised() {
		t.Errorf("tracking = %+v, want off: the consent belonged to the earlier subscription", got)
	}
	if got := kinds(choices(t, svc, "a@example.org")); got != "requested,given,refused" {
		t.Errorf("choices = %s: the earlier ones stay as proof", got)
	}
}

func TestErasureDeletesTrackingChoices(t *testing.T) {
	ctx := context.Background()
	svc, m, _ := setup(t)
	raise(svc, both)
	join(t, svc, m, "a@example.org", true)
	sub, _ := svc.Store.SubscriberByEmail(ctx, "a@example.org")
	if err := svc.Store.Erase(ctx, sub.ID); err != nil {
		t.Fatal(err)
	}
	cs, err := svc.Store.TrackingChoices(ctx, sub.ID)
	if err != nil || len(cs) != 0 {
		t.Errorf("choices after erasure = %+v, %v", cs, err)
	}
}

// firstTo is the first message sent to an address.
func firstTo(t *testing.T, m *mailer, email string) kannon.Message {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, msg := range m.sent {
		if len(msg.Recipients) == 1 && msg.Recipients[0].Email == email {
			return msg
		}
	}
	t.Fatalf("nothing sent to %s", email)
	return kannon.Message{}
}
