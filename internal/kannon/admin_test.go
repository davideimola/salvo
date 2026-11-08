package kannon_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/davideimola/salvo/internal/kannon"
)

func TestAdminReadsStatsAndTracking(t *testing.T) {
	var paths []string
	var statsReq map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.Header.Get("X-Kannon-Admin-Token") != "admin" {
			http.Error(w, "unauthenticated", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/kannon.stats.apiv2.StatsApiV2/GetAggregatedStats":
			json.NewDecoder(r.Body).Decode(&statsReq)
			w.Write([]byte(`{"stats":[{"type":"delivered","timestamp":"2026-10-28T21:00:00Z","count":"2"}]}`))
		case "/pkg.kannon.admin.apiv1.Api/GetDomain":
			w.Write([]byte(`{"domain":{"domain":"news.example.com","dkimPubKey":"x","tracking":{"opens":"TRACKING_MODE_OFF","links":"TRACKING_MODE_OFF"}}}`))
		}
	}))
	defer srv.Close()
	a := &kannon.Admin{URL: srv.URL, Token: "admin"}
	ctx := context.Background()

	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	stats, err := a.AggregatedStats(ctx, "news.example.com", from, from.AddDate(0, 1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 1 || stats[0].Type != "delivered" || stats[0].Count != 2 || !stats[0].At.Equal(time.Date(2026, 10, 28, 21, 0, 0, 0, time.UTC)) {
		t.Fatalf("stats = %+v", stats)
	}
	if statsReq["domain"] != "news.example.com" || statsReq["fromDate"] != "2026-10-01T00:00:00Z" || statsReq["toDate"] != "2026-11-01T00:00:00Z" {
		t.Errorf("request = %v", statsReq)
	}

	tr, err := a.DomainTracking(ctx, "news.example.com")
	if err != nil || !tr.Off() {
		t.Fatalf("tracking = %+v, %v", tr, err)
	}

	bad := &kannon.Admin{URL: srv.URL, Token: "wrong"}
	if _, err := bad.DomainTracking(ctx, "news.example.com"); err == nil {
		t.Error("want an error for a refused token")
	}
}

func TestTrackingOffNeedsBoth(t *testing.T) {
	if (kannon.Tracking{Opens: "TRACKING_MODE_OFF", Links: "TRACKING_MODE_IDENTIFIED"}).Off() {
		t.Error("links tracked, yet Off() says off")
	}
}

func TestAdminReadsABatchsEventsFollowingTheCursor(t *testing.T) {
	var queries []url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1beta1/domains/news.example.com/stats/events" {
			http.Error(w, "no route", http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") != "Bearer admin" {
			http.Error(w, "unauthenticated", http.StatusUnauthorized)
			return
		}
		q := r.URL.Query()
		queries = append(queries, q)
		switch q.Get("cursor") {
		case "":
			w.Write([]byte(`{"items":[
				{"id":"9","domain":"news.example.com","batch_id":"msg_1","timestamp":"2026-11-02T10:00:00Z","type":"opened","tracking_mode":"pseudonymous","tracking_identity":"aa@track.news.example.com","data":{}},
				{"id":"8","domain":"news.example.com","batch_id":"msg_1","timestamp":"2026-11-02T09:00:00Z","type":"opened","tracking_mode":"pseudonymous","tracking_identity":"bb@track.news.example.com","data":{}}
			],"next_cursor":"page2"}`))
		case "page2":
			w.Write([]byte(`{"items":[
				{"id":"7","domain":"news.example.com","batch_id":"msg_1","timestamp":"2026-11-02T08:00:00Z","type":"opened","tracking_mode":"identified","email":"reader@example.org","data":{}}
			],"next_cursor":null}`))
		default:
			http.Error(w, "invalid_cursor", http.StatusBadRequest)
		}
	}))
	defer srv.Close()
	a := &kannon.Admin{URL: srv.URL, Token: "admin"}

	from := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	events, err := a.Events(context.Background(), "news.example.com", kannon.EventQuery{
		BatchID: "msg_1", Type: "opened", From: from, To: from.AddDate(0, 0, 7),
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []kannon.Event{
		{Type: "opened", TrackingMode: "pseudonymous", Identity: "aa@track.news.example.com"},
		{Type: "opened", TrackingMode: "pseudonymous", Identity: "bb@track.news.example.com"},
		{Type: "opened", TrackingMode: "identified", Identity: "reader@example.org"},
	}
	if len(events) != len(want) {
		t.Fatalf("events = %+v", events)
	}
	for i := range want {
		if events[i] != want[i] {
			t.Errorf("event %d = %+v, want %+v", i, events[i], want[i])
		}
	}
	if len(queries) != 2 {
		t.Fatalf("%d requests, want 2 (one per page)", len(queries))
	}
	for _, q := range queries {
		if q.Get("batch_id") != "msg_1" || q.Get("type") != "opened" ||
			q.Get("from") != "2026-11-01T09:00:00Z" || q.Get("to") != "2026-11-08T09:00:00Z" || q.Get("limit") != "1000" {
			t.Errorf("query = %v", q)
		}
	}

	bad := &kannon.Admin{URL: srv.URL, Token: "wrong"}
	if _, err := bad.Events(context.Background(), "news.example.com", kannon.EventQuery{BatchID: "msg_1", Type: "opened", From: from, To: from}); err == nil {
		t.Error("want an error for a refused token")
	}
}
