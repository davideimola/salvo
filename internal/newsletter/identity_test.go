package newsletter_test

import (
	"context"
	"strings"
	"testing"

	"github.com/davideimola/salvo/internal/newsletter"
)

// With nothing set, the confirmation names the sender and promises nothing it cannot keep.
func TestConfirmationDefaults(t *testing.T) {
	svc, m, _ := setup(t)
	svc.Ident = newsletter.Identity{Sender: "Ada Lovelace"}
	if err := svc.Subscribe(context.Background(), "reader@example.org"); err != nil {
		t.Fatal(err)
	}
	msg := m.last()
	if msg.Subject != "Confirm your subscription to Ada Lovelace" {
		t.Errorf("subject = %q", msg.Subject)
	}
	if !strings.Contains(msg.HTML, ">Ada Lovelace</p>") {
		t.Error("the sender is not at the top")
	}
	if strings.Contains(msg.HTML, "Privacy") {
		t.Error("a Privacy link with no PRIVACY_URL")
	}

	id := svc.Identity()
	if id.SubscribeURL != "https://example.com" || id.ConfirmedURL != "https://news.example.com/confirmed" {
		t.Errorf("default URLs: %+v", id)
	}
}

// Everything the newsletter says about itself comes from its Identity.
func TestConfirmationSaysWhatTheIdentitySays(t *testing.T) {
	svc, m, _ := setup(t)
	svc.Ident = newsletter.Identity{
		Name:           "Ada's notes",
		Sender:         "Ada Lovelace",
		About:          "Once a month, the engine's progress.",
		SignOff:        "Ada",
		ConfirmSubject: "Confirm your subscription to example.com",
		PrivacyURL:     "https://example.com/privacy",
	}
	if err := svc.Subscribe(context.Background(), "reader@example.org"); err != nil {
		t.Fatal(err)
	}
	msg := m.last()
	if msg.Subject != "Confirm your subscription to example.com" {
		t.Errorf("subject = %q", msg.Subject)
	}
	for _, want := range []string{">Ada Lovelace</p>", ">Once a month, the engine&#39;s progress.</p>", ">Ada</p>", `href="https://example.com/privacy"`} {
		if !strings.Contains(msg.HTML, want) {
			t.Errorf("no %s in the confirmation", want)
		}
	}
}
