package kannon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/davideimola/salvo/internal/tracking"
)

// Admin reads from Kannon's Admin and Stats APIs and its REST event API, for the delivery panel on the admin pages.
// It only ever reads, but Kannon has no read-only credential: both APIs answer to the one
// admin token.
type Admin struct {
	URL   string
	Token string
	HTTP  *http.Client
}

// Count is one aggregated counter: how many Deliveries of a type, in the hour starting at At.
// It names no recipient.
type Count struct {
	Type  string
	At    time.Time
	Count int64
}

// AggregatedStats returns the Domain's counters between from and to.
func (a *Admin) AggregatedStats(ctx context.Context, domain string, from, to time.Time) ([]Count, error) {
	var res struct {
		Stats []struct {
			Type      string    `json:"type"`
			Timestamp time.Time `json:"timestamp"`
			Count     string    `json:"count"` // int64, which protojson writes as a string
		} `json:"stats"`
	}
	err := a.call(ctx, "/kannon.stats.apiv2.StatsApiV2/GetAggregatedStats", map[string]any{
		"domain": domain, "fromDate": from.UTC().Format(time.RFC3339), "toDate": to.UTC().Format(time.RFC3339),
	}, &res)
	if err != nil {
		return nil, err
	}
	out := make([]Count, 0, len(res.Stats))
	for _, s := range res.Stats {
		n, err := strconv.ParseInt(s.Count, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("kannon: count %q: %w", s.Count, err)
		}
		out = append(out, Count{Type: s.Type, At: s.Timestamp, Count: n})
	}
	return out, nil
}

// The Tracking Modes, as Kannon names them on the wire. Salvo only ever asks for these two
// (ADR 0002); a Domain may report others.
const (
	ModeOff          = "TRACKING_MODE_OFF"
	ModePseudonymous = "TRACKING_MODE_PSEUDONYMOUS"
)

// Tracking is a Tracking Policy, a Domain's, a batch's or a recipient's, as Kannon names the
// modes (TRACKING_MODE_OFF, ...).
type Tracking struct {
	Opens string `json:"opens"`
	Links string `json:"links"`
}

// Off reports whether neither opens nor links are tracked.
func (t Tracking) Off() bool {
	return t.Opens == ModeOff && t.Links == ModeOff
}

// PolicyTracking is Salvo's policy in Kannon's names.
func PolicyTracking(p tracking.Policy) Tracking {
	mode := func(m tracking.Mode) string {
		if m == tracking.Pseudonymous {
			return ModePseudonymous
		}
		return ModeOff
	}
	return Tracking{Opens: mode(p.Opens), Links: mode(p.Links)}
}

// Raised reports whether anything is tracked: a mode other than off, an empty one counting
// as off.
func (t Tracking) Raised() bool {
	t = t.orOff()
	return !t.Off()
}

func (t Tracking) orOff() Tracking {
	if t.Opens == "" {
		t.Opens = ModeOff
	}
	if t.Links == "" {
		t.Links = ModeOff
	}
	return t
}

func (a *Admin) DomainTracking(ctx context.Context, domain string) (Tracking, error) {
	var res struct {
		Domain struct {
			Tracking Tracking `json:"tracking"`
		} `json:"domain"`
	}
	err := a.call(ctx, "/pkg.kannon.admin.apiv1.Api/GetDomain", map[string]string{"domain": domain}, &res)
	return res.Domain.Tracking, err
}

func (a *Admin) call(ctx context.Context, path string, body, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(a.URL, "/")+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Kannon-Admin-Token", a.Token)
	return a.do(req, out)
}

func (a *Admin) do(req *http.Request, out any) error {
	hc := a.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return &RefusedError{Status: resp.StatusCode, Body: strings.TrimSpace(string(data))}
	}
	return json.Unmarshal(data, out)
}

// EventQuery narrows Kannon's retained events to one batch and one type, between From
// (inclusive) and To (exclusive).
type EventQuery struct {
	BatchID  string
	Type     string // opened, clicked, ...
	From, To time.Time
}

// Event is one retained event as Salvo needs it. Identity is whom Kannon says it measured:
// the tracking identity under pseudonymous tracking, which names nobody, or the address under
// a mode that keeps it. Salvo never stores it (ADR 0003).
type Event struct {
	Type         string
	TrackingMode string
	Identity     string
}

// Events reads every event matching q from Kannon's REST event API (v1beta1), following the
// cursor to the last page.
func (a *Admin) Events(ctx context.Context, domain string, q EventQuery) ([]Event, error) {
	base := strings.TrimRight(a.URL, "/") + "/v1beta1/domains/" + url.PathEscape(domain) + "/stats/events"
	params := url.Values{
		"from": {q.From.UTC().Format(time.RFC3339)}, "to": {q.To.UTC().Format(time.RFC3339)},
		"batch_id": {q.BatchID}, "type": {q.Type}, "limit": {"1000"},
	}
	var out []Event
	for {
		var page struct {
			Items []struct {
				Type             string `json:"type"`
				TrackingMode     string `json:"tracking_mode"`
				TrackingIdentity string `json:"tracking_identity"`
				Email            string `json:"email"`
			} `json:"items"`
			NextCursor *string `json:"next_cursor"`
		}
		if err := a.get(ctx, base+"?"+params.Encode(), &page); err != nil {
			return nil, err
		}
		for _, it := range page.Items {
			id := it.TrackingIdentity
			if id == "" {
				id = it.Email
			}
			out = append(out, Event{Type: it.Type, TrackingMode: it.TrackingMode, Identity: id})
		}
		if page.NextCursor == nil || *page.NextCursor == "" || *page.NextCursor == params.Get("cursor") {
			return out, nil
		}
		params.Set("cursor", *page.NextCursor)
	}
}

// get calls Kannon's REST API, which takes the admin token as a Bearer credential.
func (a *Admin) get(ctx context.Context, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+a.Token)
	return a.do(req, out)
}
