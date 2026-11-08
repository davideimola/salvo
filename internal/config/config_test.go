package config_test

import (
	"strings"
	"testing"

	"github.com/davideimola/salvo/internal/config"
	"github.com/davideimola/salvo/internal/tracking"
)

// required sets every variable FromEnv cannot do without.
func required(t *testing.T) {
	t.Helper()
	for k, v := range map[string]string{
		"DATABASE_URL": "postgres://x", "PUBLIC_URL": "https://news.example.com", "SITE_URL": "https://example.com",
		"API_KEY": strings.Repeat("k", 32), "KANNON_URL": "http://kannon:50051", "KANNON_DOMAIN": "news.example.com",
		"KANNON_API_KEY": "k", "SENDER_EMAIL": "a@example.com", "SENDER_NAME": "A", "TEST_RECIPIENT": "a@example.com",
		"OWNER_EMAIL": "a@example.com", "GOOGLE_CLIENT_ID": "id", "GOOGLE_CLIENT_SECRET": "s",
		"SESSION_KEY": strings.Repeat("s", 32),
	} {
		t.Setenv(k, v)
	}
}

func TestTrackingIsOffByDefault(t *testing.T) {
	required(t)
	c, err := config.FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if c.TrackingCeiling != (tracking.Policy{Opens: tracking.Off, Links: tracking.Off}) || c.TrackingCeiling.Raised() {
		t.Errorf("ceiling = %+v, want both off", c.TrackingCeiling)
	}
}

func TestTrackingCeilings(t *testing.T) {
	required(t)
	t.Setenv("TRACKING_LINKS", "pseudonymous")
	t.Setenv("NOTICE_VERSION", "2026-11")
	c, err := config.FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if c.TrackingCeiling != (tracking.Policy{Opens: tracking.Off, Links: tracking.Pseudonymous}) || c.NoticeVersion != "2026-11" {
		t.Errorf("ceiling = %+v, notice %q", c.TrackingCeiling, c.NoticeVersion)
	}
}

func TestOnlyOffAndPseudonymousAreAccepted(t *testing.T) {
	for _, mode := range []string{"anonymous", "identified", "full", "on", "PSEUDONYMOUS "} {
		for _, name := range []string{"TRACKING_OPENS", "TRACKING_LINKS"} {
			t.Run(name+"="+mode, func(t *testing.T) {
				required(t)
				t.Setenv("NOTICE_VERSION", "1")
				t.Setenv(name, mode)
				if _, err := config.FromEnv(); err == nil || !strings.Contains(err.Error(), name) {
					t.Errorf("err = %v, want one naming %s", err, name)
				}
			})
		}
	}
}

func TestARaisedCeilingNeedsANoticeVersion(t *testing.T) {
	required(t)
	t.Setenv("TRACKING_OPENS", "pseudonymous")
	if _, err := config.FromEnv(); err == nil || !strings.Contains(err.Error(), "NOTICE_VERSION") {
		t.Fatalf("err = %v, want one naming NOTICE_VERSION", err)
	}
}
