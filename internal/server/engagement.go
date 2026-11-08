package server

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/davideimola/salvo/internal/kannon"
	"github.com/davideimola/salvo/internal/store"
)

// engagementPanel is how an issue did among its Tracked recipients: the distinct tracking
// identities Kannon reports for the issue's batch, never the events themselves.
type engagementPanel struct {
	Tracked     int // the issue's Tracked recipients
	NotMeasured int // recipients without Tracking consent
	Metrics     []engagementMetric
	Error       string
}

type engagementMetric struct {
	Name   string // opens, clicks
	Who    string // openers, clickers
	Unique int    // distinct tracking identities
	Share  string // of the Tracked recipients, empty when there were none
	Over   bool   // more identities than Tracked recipients: someone was measured without consent
}

var engagementTypes = []struct{ Type, Name, Who string }{
	{"opened", "opens", "openers"},
	{"clicked", "clicks", "clickers"},
}

// engagementFor reads a sent issue's opens and clicks from Kannon. Like the delivery panel it
// never fails the page: a Kannon that does not answer becomes a line on it.
func (s *Server) engagementFor(ctx context.Context, issue *store.Issue) *engagementPanel {
	if s.delivery == nil || issue.SentAt == nil || issue.KannonMessageID == nil {
		return nil
	}
	p := &engagementPanel{Tracked: countOf(issue.TrackedRecipients)}
	p.NotMeasured = countOf(issue.Recipients) - p.Tracked
	for _, et := range engagementTypes {
		events, err := s.delivery.Events(ctx, s.kannonDomain, kannon.EventQuery{
			BatchID: *issue.KannonMessageID, Type: et.Type,
			From: issue.SentAt.Truncate(time.Hour), To: s.svc.Now().Add(time.Minute),
		})
		if err != nil {
			slog.Error("issue engagement", "slug", issue.Slug, "err", err)
			p.Metrics, p.Error = nil, "Kannon did not answer for opens and clicks: "+err.Error()
			return p
		}
		ids := map[string]bool{}
		for _, ev := range events {
			if ev.Identity != "" {
				ids[ev.Identity] = true
			}
		}
		m := engagementMetric{Name: et.Name, Who: et.Who, Unique: len(ids), Over: len(ids) > p.Tracked}
		if p.Tracked > 0 {
			m.Share = fmt.Sprintf("%.0f%%", 100*float64(len(ids))/float64(p.Tracked))
		}
		p.Metrics = append(p.Metrics, m)
	}
	return p
}

func countOf(n *int) int {
	if n == nil {
		return 0
	}
	return *n
}
