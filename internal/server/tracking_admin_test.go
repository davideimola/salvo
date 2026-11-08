package server_test

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/davideimola/salvo/internal/store"
	"github.com/davideimola/salvo/internal/tracking"
)

var linksOnly = tracking.Policy{Opens: tracking.Off, Links: tracking.Pseudonymous}

// joinWith subscribes and confirms an address through the API, ticking the tracking box or not.
func (e *env) joinWith(t *testing.T, email string, wantsTracking bool) int64 {
	t.Helper()
	body := `{"email":"` + email + `","tracking":` + strconv.FormatBool(wantsTracking) + `}`
	if r := e.api(t, "POST", "/api/subscribers", apiKey, body); r.StatusCode != 202 {
		t.Fatalf("subscribe %s: %d", email, r.StatusCode)
	}
	if err := e.svc.Confirm(context.Background(), tokenIn.FindStringSubmatch(e.mail.last().HTML)[1]); err != nil {
		t.Fatal(err)
	}
	sub, err := e.svc.Store.SubscriberByEmail(context.Background(), email)
	if err != nil {
		t.Fatal(err)
	}
	return sub.ID
}

func TestSubscriberPageListsTrackingChoices(t *testing.T) {
	e := setup(t)
	e.now = time.Date(2026, 11, 3, 9, 30, 0, 0, time.UTC)
	e.svc.Ceiling = linksOnly
	e.svc.NoticeVersion = "2026-11"
	id := e.joinWith(t, "yes@example.org", true)

	body := e.ownerGet(t, "/admin/subscribers/"+strconv.FormatInt(id, 10))
	for _, want := range []string{"Tracking choices", "requested", "given", "2026-11-03 09:30 UTC", "subscribe", "2026-11", "opens off, links pseudonymous"} {
		if !strings.Contains(body, want) {
			t.Errorf("the subscriber page lacks %q", want)
		}
	}
}

func TestNothingAboutTrackingWhenOffAndNoChoice(t *testing.T) {
	e := setup(t)
	id := e.joinWith(t, "a@example.org", false)
	for _, path := range []string{"/admin/subscribers/" + strconv.FormatInt(id, 10), "/admin"} {
		if body := e.ownerGet(t, path); strings.Contains(body, "Tracking choices") || strings.Contains(body, "Tracking consents") {
			t.Errorf("%s shows tracking with both ceilings off and no choice", path)
		}
	}
}

func TestOverviewCountsCurrentConsentsPerNoticeVersion(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	e.svc.Ceiling = linksOnly
	e.svc.NoticeVersion = "2026-11"
	e.joinWith(t, "early@example.org", true)
	e.joinWith(t, "no@example.org", false)
	left := e.joinWith(t, "left@example.org", true)
	withdrew := e.joinWith(t, "withdrew@example.org", true)

	e.now = e.now.Add(time.Hour)
	e.svc.NoticeVersion = "2027-01"
	e.joinWith(t, "late1@example.org", true)
	e.joinWith(t, "late2@example.org", true)
	e.svc.Store.AddTrackingChoice(ctx, withdrew, store.TrackingChoice{
		Kind: store.Withdrawn, At: e.now, Source: store.FromPreferences, NoticeVersion: "2027-01", Ceiling: linksOnly,
	})
	sub, _ := e.svc.Store.SubscriberByID(ctx, left)
	if err := e.svc.Unsubscribe(ctx, sub.UnsubscribeToken); err != nil {
		t.Fatal(err)
	}

	body := e.ownerGet(t, "/admin")
	if !strings.Contains(body, "Tracking consents") {
		t.Fatalf("no consents panel:\n%s", body)
	}
	for version, n := range map[string]string{"2026-11": "1", "2027-01": "2"} {
		row := regexp.MustCompile(`<td class="mono">` + version + `</td>\s*<td>` + n + `</td>`)
		if !row.MatchString(body) {
			t.Errorf("notice %s: want %s current consents", version, n)
		}
	}
}

func TestOverviewSaysSoWhenRaisedWithoutConsents(t *testing.T) {
	e := setup(t)
	e.svc.Ceiling = linksOnly
	e.svc.NoticeVersion = "2026-11"
	body := e.ownerGet(t, "/admin")
	if !strings.Contains(body, "Tracking consents") || !strings.Contains(body, "No Subscriber has a Tracking consent") {
		t.Errorf("no consents line with a ceiling raised:\n%s", body)
	}
}
