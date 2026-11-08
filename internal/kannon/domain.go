package kannon

import (
	"context"

	"github.com/davideimola/salvo/internal/tracking"
)

// modeRank orders Kannon's Tracking Modes from least to most tracking. A mode Salvo does not
// know ranks as off, so it can only ever make a Domain allow less.
var modeRank = map[string]int{
	ModeOff:                    0,
	"TRACKING_MODE_ANONYMOUS":  1,
	ModePseudonymous:           2,
	"TRACKING_MODE_IDENTIFIED": 3,
	"TRACKING_MODE_FULL":       4,
}

// Allows reports whether a Domain with this Tracking Policy accepts recipients sent with p:
// Kannon rejects, one by one, every recipient above its Domain's ceiling.
func (t Tracking) Allows(p tracking.Policy) bool {
	want := PolicyTracking(p)
	return modeRank[t.Opens] >= modeRank[want.Opens] && modeRank[t.Links] >= modeRank[want.Links]
}

// DomainPolicy reads one Domain's Tracking Policy through the Admin API.
type DomainPolicy struct {
	Admin  *Admin
	Domain string
}

func (d DomainPolicy) DomainTracking(ctx context.Context) (Tracking, error) {
	return d.Admin.DomainTracking(ctx, d.Domain)
}
