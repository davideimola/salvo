package kannon

import (
	"context"

	"github.com/davideimola/salvo/internal/tracking"
)

// allowed is the most of Salvo's modes that a Domain at Kannon's mode accepts. Kannon's
// `identified` and `full` track more than `pseudonymous`, so they accept it; `anonymous` tracks
// less, so it accepts only `off`. A mode Salvo does not know accepts only `off`, so it can only
// ever make a Domain allow less.
func allowed(mode string) tracking.Mode {
	switch mode {
	case ModePseudonymous, "TRACKING_MODE_IDENTIFIED", "TRACKING_MODE_FULL":
		return tracking.Pseudonymous
	}
	return tracking.Off
}

// Allows reports whether a Domain with this Tracking Policy accepts recipients sent with p:
// Kannon rejects, one by one, every recipient above its Domain's ceiling.
func (t Tracking) Allows(p tracking.Policy) bool {
	return tracking.Policy{Opens: allowed(t.Opens), Links: allowed(t.Links)}.Allows(p)
}

// DomainPolicy reads one Domain's Tracking Policy through the Admin API.
type DomainPolicy struct {
	Admin  *Admin
	Domain string
}

func (d DomainPolicy) DomainTracking(ctx context.Context) (Tracking, error) {
	return d.Admin.DomainTracking(ctx, d.Domain)
}
