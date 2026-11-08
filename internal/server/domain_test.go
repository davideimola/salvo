package server_test

import (
	"strings"
	"testing"

	"github.com/davideimola/salvo/internal/kannon"
	"github.com/davideimola/salvo/internal/tracking"
)

var pseudonymous = tracking.Policy{Opens: tracking.Pseudonymous, Links: tracking.Pseudonymous}

// raised is the admin with a Tracking ceiling raised, on a Kannon whose Domain has tr.
func raised(t *testing.T, ceiling tracking.Policy, tr kannon.Tracking) *env {
	t.Helper()
	e := withDelivery(t, &delivery{tracking: tr})
	e.svc.Ceiling, e.svc.NoticeVersion = ceiling, "2026-11"
	return e
}

func TestTheAdminWarnsWhenTheDomainAllowsLessThanTheCeiling(t *testing.T) {
	e := raised(t, pseudonymous, kannon.Tracking{Opens: kannon.ModePseudonymous, Links: kannon.ModeOff})
	body := e.ownerGet(t, "/admin")
	if !strings.Contains(body, "allows less tracking than the Tracking ceiling") {
		t.Errorf("no warning about the Domain:\n%s", body)
	}
	if strings.Contains(body, "Tracking is not off") {
		t.Error("the all-off warning while a ceiling is raised")
	}
}

func TestTheAdminDoesNotWarnWhenTheDomainAllowsTheCeiling(t *testing.T) {
	e := raised(t, pseudonymous, kannon.Tracking{Opens: kannon.ModePseudonymous, Links: kannon.ModePseudonymous})
	body := e.ownerGet(t, "/admin")
	for _, warning := range []string{"allows less tracking", "Tracking is not off"} {
		if strings.Contains(body, warning) {
			t.Errorf("warning %q with a Domain that allows the ceiling", warning)
		}
	}
}
