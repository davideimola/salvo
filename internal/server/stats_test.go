package server_test

import (
	"context"
	"strings"
	"testing"

	"github.com/davideimola/salvo/internal/kannon"
	"github.com/davideimola/salvo/internal/tracking"
)

// sendTracked sends issue 2026-11 to the consenting addresses, with Tracking consent, and to
// the others, without.
func sendTracked(t *testing.T, e *env, consenting, others []string) {
	t.Helper()
	ctx := context.Background()
	e.svc.Ceiling = tracking.Policy{Opens: tracking.Pseudonymous, Links: tracking.Pseudonymous}
	e.svc.NoticeVersion = "1"
	join := func(email string, consents bool) {
		if err := e.svc.Subscribe(ctx, email, consents); err != nil {
			t.Fatal(err)
		}
		if err := e.svc.Confirm(ctx, tokenIn.FindStringSubmatch(e.mail.last().HTML)[1]); err != nil {
			t.Fatal(err)
		}
	}
	for _, a := range consenting {
		join(a, true)
	}
	for _, a := range others {
		join(a, false)
	}
	html := `<a href="{{ unsubscribe_url }}">u</a> <a href="{{ preferences_url }}">p</a>`
	if _, err := e.svc.SaveIssue(ctx, "2026-11", "November", html); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.TestSend(ctx, "2026-11"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Send(ctx, "2026-11"); err != nil {
		t.Fatal(err)
	}
}

func opened(ids ...string) []kannon.Event {
	var out []kannon.Event
	for _, id := range ids {
		out = append(out, kannon.Event{Type: "opened", TrackingMode: "pseudonymous", Identity: id})
	}
	return out
}

func clicked(ids ...string) []kannon.Event {
	var out []kannon.Event
	for _, id := range ids {
		out = append(out, kannon.Event{Type: "clicked", TrackingMode: "pseudonymous", Identity: id})
	}
	return out
}

func TestIssuePageShowsOpensAndClicksAmongTrackedRecipients(t *testing.T) {
	d := &delivery{tracking: off, events: map[string][]kannon.Event{
		"opened":  opened("aa", "aa", "bb"),
		"clicked": clicked("aa", "aa"),
	}}
	e := withDelivery(t, d)
	sendTracked(t, e, []string{"a@example.org", "b@example.org"}, []string{"c@example.org"})

	body := e.ownerGet(t, "/admin/issues/2026-11")
	for _, want := range []string{
		"Unique opens: <b>2</b> of 2 Tracked recipients (100%)",
		"Unique clicks: <b>1</b> of 2 Tracked recipients (50%)",
		"Not measured: <b>1</b> recipient without Tracking consent",
		"Apple Mail Privacy Protection",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the issue page lacks %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "more than the") {
		t.Error("a warning with no more identities than Tracked recipients")
	}
	types := map[string]bool{}
	for _, q := range d.queries {
		if q.BatchID != "msg_1" {
			t.Errorf("events read for batch %q, want the issue's msg_1", q.BatchID)
		}
		types[q.Type] = true
	}
	if !types["opened"] || !types["clicked"] {
		t.Errorf("event types read: %v", types)
	}
}

func TestIssuePageWarnsWhenKannonMeasuredMoreThanTheTrackedRecipients(t *testing.T) {
	e := withDelivery(t, &delivery{tracking: off, events: map[string][]kannon.Event{
		"opened": opened("aa", "bb"),
	}})
	sendTracked(t, e, []string{"a@example.org"}, []string{"c@example.org"})

	body := e.ownerGet(t, "/admin/issues/2026-11")
	if !strings.Contains(body, "Kannon measured 2 distinct openers, more than the 1 Tracked recipient") {
		t.Errorf("no warning:\n%s", body)
	}
}

func TestIssuePageSurvivesKannonNotAnsweringForEvents(t *testing.T) {
	e := withDelivery(t, &delivery{tracking: off, eventsErr: &kannon.RefusedError{Status: 503, Body: "down"}})
	sendTracked(t, e, []string{"a@example.org"}, nil)

	body := e.ownerGet(t, "/admin/issues/2026-11")
	if !strings.Contains(body, "Kannon did not answer for opens and clicks") {
		t.Errorf("no line about Kannon:\n%s", body)
	}
	if strings.Contains(body, "Unique opens") {
		t.Error("shares shown without events to compute them")
	}
}
