package newsletter_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/davideimola/salvo/internal/kannon"
	"github.com/davideimola/salvo/internal/newsletter"
	"github.com/davideimola/salvo/internal/store"
	"github.com/davideimola/salvo/internal/tracking"
)

const trackedIssueHTML = `<p>Hello</p><a href="{{ preferences_url }}">Preferences</a><a href="{{ unsubscribe_url }}">Unsubscribe</a>`

func unsubscribeToken(t *testing.T, svc *newsletter.Service, email string) string {
	t.Helper()
	sub, err := svc.Store.SubscriberByEmail(context.Background(), email)
	if err != nil {
		t.Fatal(err)
	}
	return sub.UnsubscribeToken
}

func TestEveryRecipientGetsTheirOwnPreferencesLink(t *testing.T) {
	for _, tc := range []struct {
		name    string
		ceiling tracking.Policy
	}{{"ceilings off", tracking.None}, {"ceilings raised", both}} {
		t.Run(tc.name, func(t *testing.T) {
			svc, m, c := setup(t)
			raise(svc, tc.ceiling)
			confirmed(t, svc, m, "a@example.org")
			_, msg := sendIssue(t, svc, m, c)
			want := "https://news.example.com/preferences?token=" + unsubscribeToken(t, svc, "a@example.org")
			if got := msg.Recipients[0].Fields["preferences_url"]; got != want {
				t.Errorf("preferences_url = %q, want %q", got, want)
			}
		})
	}
}

func TestTheTestSendCarriesADummyPreferencesLink(t *testing.T) {
	ctx := context.Background()
	svc, m, _ := setup(t)
	svc.SaveIssue(ctx, "2026-11", "November", issueHTML)
	if err := svc.TestSend(ctx, "2026-11"); err != nil {
		t.Fatal(err)
	}
	if got := m.last().Recipients[0].Fields["preferences_url"]; got != "https://news.example.com/preferences?token=test" {
		t.Errorf("preferences_url = %q", got)
	}
}

func TestThePreferencesLinkIsRequiredOnlyWhileACeilingIsRaised(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := setup(t)
	if _, err := svc.SaveIssue(ctx, "2026-11", "November", issueHTML); err != nil {
		t.Fatalf("ceilings off, no preferences link: err = %v", err)
	}
	raise(svc, linksOnly)
	if _, err := svc.SaveIssue(ctx, "2026-11", "November", issueHTML); !errors.Is(err, newsletter.ErrNoPreferencesLink) {
		t.Fatalf("ceiling raised, no preferences link: err = %v, want ErrNoPreferencesLink", err)
	}
	if _, err := svc.SaveIssue(ctx, "2026-11", "November", trackedIssueHTML); err != nil {
		t.Fatalf("ceiling raised, with the link: err = %v", err)
	}
}

func TestThePreferencesPageShowsWhatIsMeasured(t *testing.T) {
	ctx := context.Background()
	svc, m, _ := setup(t)
	raise(svc, linksOnly)
	join(t, svc, m, "yes@example.org", true)
	join(t, svc, m, "no@example.org", false)

	yes, err := svc.Preferences(ctx, unsubscribeToken(t, svc, "yes@example.org"))
	if err != nil {
		t.Fatal(err)
	}
	if yes.Email != "yes@example.org" || !yes.Subscribed || yes.Measured != linksOnly || yes.Ceiling != linksOnly {
		t.Errorf("consented: %+v", yes)
	}
	no, _ := svc.Preferences(ctx, unsubscribeToken(t, svc, "no@example.org"))
	if no.Measured.Raised() || no.Ceiling != linksOnly {
		t.Errorf("refused: %+v", no)
	}
	if got := kinds(choices(t, svc, "yes@example.org")); got != "requested,given" {
		t.Errorf("looking at the page changed the choices: %s", got)
	}
}

func TestWithCeilingsOffThePreferencesPageTracksNothing(t *testing.T) {
	svc, m, _ := setup(t)
	confirmed(t, svc, m, "a@example.org")
	p, err := svc.Preferences(context.Background(), unsubscribeToken(t, svc, "a@example.org"))
	if err != nil {
		t.Fatal(err)
	}
	if p.Ceiling.Raised() || p.Measured.Raised() {
		t.Errorf("preferences = %+v, want nothing tracked", p)
	}
}

func TestAnInvalidPreferencesTokenIsAnInvalidLink(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := setup(t)
	for _, token := range []string{"", "nope"} {
		if _, err := svc.Preferences(ctx, token); !errors.Is(err, newsletter.ErrInvalidLink) {
			t.Errorf("Preferences(%q): err = %v", token, err)
		}
		if err := svc.Withdraw(ctx, token); !errors.Is(err, newsletter.ErrInvalidLink) {
			t.Errorf("Withdraw(%q): err = %v", token, err)
		}
	}
}

func TestWithdrawKeepsTheSubscriberAndStopsTheTracking(t *testing.T) {
	ctx := context.Background()
	svc, m, c := setup(t)
	raise(svc, both)
	join(t, svc, m, "yes@example.org", true)
	token := unsubscribeToken(t, svc, "yes@example.org")

	c.advance(time.Hour)
	if err := svc.Withdraw(ctx, token); err != nil {
		t.Fatal(err)
	}
	cs := choices(t, svc, "yes@example.org")
	if kinds(cs) != "requested,given,withdrawn" {
		t.Fatalf("choices = %s", kinds(cs))
	}
	w := cs[2]
	if w.Source != store.FromPreferences || w.NoticeVersion != "2026-11" || w.Ceiling != both || !w.At.Equal(c.now()) {
		t.Errorf("withdrawn = %+v", w)
	}
	p, _ := svc.Preferences(ctx, token)
	if !p.Subscribed || p.Measured.Raised() {
		t.Errorf("after Withdraw: %+v, want subscribed and not measured", p)
	}

	issue, msg := sendIssue(t, svc, m, c)
	off := kannon.Tracking{Opens: kannon.ModeOff, Links: kannon.ModeOff}
	if got := recipientTracking(t, msg)["yes@example.org"]; got != off {
		t.Errorf("tracking after Withdraw = %+v, want off", got)
	}
	if *issue.Recipients != 1 || *issue.TrackedRecipients != 0 {
		t.Errorf("recipients %d, tracked %d; want 1, 0", *issue.Recipients, *issue.TrackedRecipients)
	}
}

func TestWithdrawWithoutATrackingConsentRecordsNothing(t *testing.T) {
	ctx := context.Background()
	svc, m, _ := setup(t)
	raise(svc, both)
	join(t, svc, m, "no@example.org", false)
	token := unsubscribeToken(t, svc, "no@example.org")
	if err := svc.Withdraw(ctx, token); err != nil {
		t.Fatal(err)
	}
	join(t, svc, m, "yes@example.org", true)
	yes := unsubscribeToken(t, svc, "yes@example.org")
	svc.Withdraw(ctx, yes)
	svc.Withdraw(ctx, yes) // twice: one Withdraw is enough
	if got := kinds(choices(t, svc, "no@example.org")); got != "refused" {
		t.Errorf("no consent: choices = %s", got)
	}
	if got := kinds(choices(t, svc, "yes@example.org")); got != "requested,given,withdrawn" {
		t.Errorf("withdrawn twice: choices = %s", got)
	}
}

func TestAnUnsubscribedAddressIsNotSubscribedOnItsPreferencesPage(t *testing.T) {
	ctx := context.Background()
	svc, m, _ := setup(t)
	raise(svc, both)
	join(t, svc, m, "a@example.org", true)
	token := unsubscribeToken(t, svc, "a@example.org")
	svc.Unsubscribe(ctx, token)
	p, err := svc.Preferences(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	if p.Subscribed || p.Measured.Raised() {
		t.Errorf("after Unsubscribe: %+v", p)
	}
}
