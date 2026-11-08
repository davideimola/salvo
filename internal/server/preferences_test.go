package server_test

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/davideimola/salvo/internal/tracking"
)

// joined subscribes and confirms an address through the service, returning its unsubscribe
// token, the one its Preferences page is addressed by.
func (e *env) joined(t *testing.T, email string, wantsTracking bool) string {
	t.Helper()
	ctx := context.Background()
	if err := e.svc.Subscribe(ctx, email, wantsTracking); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Confirm(ctx, tokenIn.FindStringSubmatch(e.mail.last().HTML)[1]); err != nil {
		t.Fatal(err)
	}
	sub, err := e.svc.Store.SubscriberByEmail(ctx, email)
	if err != nil {
		t.Fatal(err)
	}
	return sub.UnsubscribeToken
}

func (e *env) raise(p tracking.Policy) {
	e.svc.Ceiling = p
	e.svc.NoticeVersion = "2026-11"
}

func get(t *testing.T, u string) (int, string) {
	t.Helper()
	r, err := http.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	return r.StatusCode, read(t, r)
}

func TestPreferencesPageWithdraws(t *testing.T) {
	e := setup(t)
	e.raise(tracking.Policy{Opens: tracking.Pseudonymous, Links: tracking.Pseudonymous})
	token := e.joined(t, "yes@example.org", true)

	status, body := get(t, e.srv.URL+"/preferences?token="+token)
	if status != http.StatusOK || !strings.Contains(body, "yes@example.org") || !strings.Contains(body, "Your opens and clicks are measured") {
		t.Fatalf("GET: %d\n%s", status, body)
	}
	if !strings.Contains(body, `href="/unsubscribe?token=`+token+`"`) {
		t.Errorf("the page must link to Unsubscribe as a separate step:\n%s", body)
	}
	if strings.Contains(body, `action="/unsubscribe"`) {
		t.Errorf("the page must not unsubscribe by itself:\n%s", body)
	}
	if p, _ := e.svc.Preferences(context.Background(), token); !p.Consented() {
		t.Fatal("opening the page must change nothing")
	}

	r, err := http.PostForm(e.srv.URL+"/preferences", url.Values{"token": {token}, "action": {"withdraw"}})
	if err != nil {
		t.Fatal(err)
	}
	if body := read(t, r); r.StatusCode != http.StatusOK || !strings.Contains(body, "still subscribed") {
		t.Fatalf("POST withdraw: %d\n%s", r.StatusCode, body)
	}
	p, _ := e.svc.Preferences(context.Background(), token)
	if p.Consented() || !p.Subscribed {
		t.Errorf("after Withdraw: %+v, want subscribed without consent", p)
	}
	if _, body := get(t, e.srv.URL+"/preferences?token="+token); !strings.Contains(body, "Your opens and clicks are not measured") {
		t.Errorf("GET after Withdraw:\n%s", body)
	}
}

func TestPreferencesPageSaysWhatIsMeasured(t *testing.T) {
	e := setup(t)
	e.raise(tracking.Policy{Opens: tracking.Off, Links: tracking.Pseudonymous})
	yes := e.joined(t, "yes@example.org", true)
	no := e.joined(t, "no@example.org", false)

	if _, body := get(t, e.srv.URL+"/preferences?token="+yes); !strings.Contains(body, "Your clicks are measured") || strings.Contains(body, "opens") {
		t.Errorf("consented, links only:\n%s", body)
	}
	_, body := get(t, e.srv.URL+"/preferences?token="+no)
	if !strings.Contains(body, "Your clicks are not measured") || strings.Contains(body, `value="withdraw"`) {
		t.Errorf("no consent, links only:\n%s", body)
	}
}

func TestPreferencesPageWithCeilingsOff(t *testing.T) {
	e := setup(t)
	token := e.joined(t, "a@example.org", false)
	status, body := get(t, e.srv.URL+"/preferences?token="+token)
	if status != http.StatusOK || !strings.Contains(body, "This newsletter tracks nothing") || strings.Contains(body, "<button") {
		t.Errorf("ceilings off: %d\n%s", status, body)
	}
	if !strings.Contains(body, `href="/unsubscribe?token=`+token+`"`) {
		t.Errorf("no link to Unsubscribe:\n%s", body)
	}
}

func TestPreferencesPageInvalidTokenIsAnInvalidUnsubscribeLink(t *testing.T) {
	e := setup(t)
	wantStatus, want := get(t, e.srv.URL+"/unsubscribe?token=bogus")
	for _, path := range []string{"/preferences?token=bogus", "/preferences"} {
		if status, body := get(t, e.srv.URL+path); status != wantStatus || body != want {
			t.Errorf("GET %s: %d, want the invalid unsubscribe answer (%d)\n%s", path, status, wantStatus, body)
		}
	}
	r, _ := http.PostForm(e.srv.URL+"/preferences", url.Values{"token": {"bogus"}, "action": {"withdraw"}})
	if body := read(t, r); r.StatusCode != wantStatus || body != want {
		t.Errorf("POST: %d\n%s", r.StatusCode, body)
	}
}

func TestPreferencesPageOfAnUnsubscribedAddress(t *testing.T) {
	e := setup(t)
	e.raise(tracking.Policy{Opens: tracking.Pseudonymous, Links: tracking.Pseudonymous})
	token := e.joined(t, "a@example.org", true)
	e.svc.Unsubscribe(context.Background(), token)
	if status, body := get(t, e.srv.URL+"/preferences?token="+token); status != http.StatusOK || !strings.Contains(body, "You are not subscribed") {
		t.Errorf("unsubscribed: %d\n%s", status, body)
	}
	r, _ := http.PostForm(e.srv.URL+"/preferences", url.Values{"token": {token}, "action": {"withdraw"}})
	if body := read(t, r); !strings.Contains(body, "You are not subscribed") {
		t.Errorf("POST withdraw, unsubscribed: %d\n%s", r.StatusCode, body)
	}
}

func TestPreferencesPageOfATestSend(t *testing.T) {
	e := setup(t)
	if status, body := get(t, e.srv.URL+"/preferences?token=test"); status != http.StatusOK || !strings.Contains(body, "This is a test send") {
		t.Errorf("test send: %d\n%s", status, body)
	}
}

func TestPreferencesPostNeedsAKnownAction(t *testing.T) {
	e := setup(t)
	e.raise(tracking.Policy{Opens: tracking.Pseudonymous, Links: tracking.Pseudonymous})
	token := e.joined(t, "a@example.org", true)
	r, _ := http.PostForm(e.srv.URL+"/preferences", url.Values{"token": {token}, "action": {"bogus"}})
	if r.StatusCode != http.StatusBadRequest {
		t.Errorf("unknown action: %d, want 400", r.StatusCode)
	}
	if p, _ := e.svc.Preferences(context.Background(), token); !p.Consented() {
		t.Error("an unknown action must change nothing")
	}
}

func TestAPIRefusesAnIssueWithoutThePreferencesLinkWhileTracking(t *testing.T) {
	e := setup(t)
	e.raise(tracking.Policy{Opens: tracking.Off, Links: tracking.Pseudonymous})
	r := e.api(t, "PUT", "/api/issues/2026-11", apiKey, `{"subject":"November","html":"<a href=\"{{ unsubscribe_url }}\">u</a>"}`)
	if body := read(t, r); r.StatusCode != http.StatusBadRequest || !strings.Contains(body, "no_preferences_url") {
		t.Errorf("no preferences link: %d %s", r.StatusCode, body)
	}
}
