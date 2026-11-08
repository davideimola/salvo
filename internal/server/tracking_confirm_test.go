package server_test

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/davideimola/salvo/internal/tracking"
)

var trackingLink = regexp.MustCompile(`/confirm-tracking\?token=([A-Za-z0-9_-]+)`)

func TestPreferencesPageAllowsTrackingByAClickInAnEmail(t *testing.T) {
	e := setup(t)
	e.raise(tracking.Policy{Opens: tracking.Pseudonymous, Links: tracking.Pseudonymous})
	token := e.joined(t, "a@example.org", false)

	_, body := get(t, e.srv.URL+"/preferences?token="+token)
	if !strings.Contains(body, `value="allow"`) || strings.Contains(body, `value="withdraw"`) {
		t.Fatalf("GET without consent must offer to allow tracking:\n%s", body)
	}
	before := e.mail.count()
	r, err := http.PostForm(e.srv.URL+"/preferences", url.Values{"token": {token}, "action": {"allow"}})
	if err != nil {
		t.Fatal(err)
	}
	if body := read(t, r); r.StatusCode != http.StatusOK || !strings.Contains(body, "Check your inbox") {
		t.Fatalf("POST allow: %d\n%s", r.StatusCode, body)
	}
	if e.mail.count() != before+1 {
		t.Fatalf("mailed %d, want the tracking confirmation email", e.mail.count()-before)
	}
	if p, _ := e.svc.Preferences(context.Background(), token); p.Consented() {
		t.Fatal("the button alone must not give a Tracking consent")
	}

	link := trackingLink.FindStringSubmatch(e.mail.last().HTML)
	if link == nil {
		t.Fatalf("no tracking confirmation link in:\n%s", e.mail.last().HTML)
	}
	status, body := get(t, e.srv.URL+"/confirm-tracking?token="+link[1])
	if status != http.StatusOK || !strings.Contains(body, "Tracking confirmed") {
		t.Fatalf("GET confirm-tracking: %d\n%s", status, body)
	}
	if p, _ := e.svc.Preferences(context.Background(), token); !p.Consented() {
		t.Fatal("the click must give the Tracking consent")
	}
	if status, _ := get(t, e.srv.URL+"/confirm-tracking?token="+link[1]); status != http.StatusNotFound {
		t.Errorf("second click: %d, want 404", status)
	}
}

func TestPreferencesPageOffersNothingToAllowWhileOff(t *testing.T) {
	e := setup(t)
	token := e.joined(t, "a@example.org", false)
	r, _ := http.PostForm(e.srv.URL+"/preferences", url.Values{"token": {token}, "action": {"allow"}})
	if r.StatusCode != http.StatusBadRequest {
		t.Errorf("allow while off: %d, want 400", r.StatusCode)
	}
}

func TestAPIAnswersAConfirmedTickedBoxWithTheTrackingConfirmationEmail(t *testing.T) {
	e := setup(t)
	e.raise(tracking.Policy{Opens: tracking.Off, Links: tracking.Pseudonymous})
	e.joined(t, "a@example.org", false)
	before := e.mail.count()
	r := e.api(t, "POST", "/api/subscribers", apiKey, `{"email":"a@example.org","tracking":true}`)
	if r.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", r.StatusCode)
	}
	if e.mail.count() != before+1 || trackingLink.FindString(e.mail.last().HTML) == "" {
		t.Errorf("want the tracking confirmation email")
	}
}
