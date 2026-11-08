// Package tracking is the vocabulary of Tracking ceilings and Tracking consent (GLOSSARY.md):
// the modes Salvo allows, and the policy a Subscriber is sent with.
//
// Salvo offers only `off` and `pseudonymous` (ADR 0002). Opens and links are always decided
// separately.
package tracking

import "fmt"

// Mode is how much of one kind of engagement, opens or links, is measured.
type Mode string

const (
	Off          Mode = "off"
	Pseudonymous Mode = "pseudonymous"
)

// ParseMode accepts exactly the modes Salvo has been built to handle lawfully.
func ParseMode(s string) (Mode, error) {
	switch m := Mode(s); m {
	case Off, Pseudonymous:
		return m, nil
	}
	return "", fmt.Errorf("tracking mode %q: want off or pseudonymous", s)
}

// rank orders the modes from least to most tracking. An unknown mode ranks as off, so a bad
// value can only ever lower what is measured.
func (m Mode) rank() int {
	if m == Pseudonymous {
		return 1
	}
	return 0
}

// Min is the lower of two modes.
func Min(a, b Mode) Mode {
	if a.rank() <= b.rank() {
		return a.orOff()
	}
	return b.orOff()
}

func (m Mode) orOff() Mode {
	if m.rank() == 0 {
		return Off
	}
	return m
}

// Policy is a mode for opens and one for links: a Tracking ceiling, the ceiling recorded with
// a Tracking choice, or what one recipient is sent with.
type Policy struct {
	Opens Mode
	Links Mode
}

// None tracks nothing.
var None = Policy{Opens: Off, Links: Off}

// Raised reports whether anything is tracked at all.
func (p Policy) Raised() bool {
	return p.Opens.rank() > 0 || p.Links.rank() > 0
}

// Measures names what the policy measures, in a reader's words: "opens and clicks", "opens",
// "clicks", or "" for nothing.
func (p Policy) Measures() string {
	opens, links := p.Opens.rank() > 0, p.Links.rank() > 0
	switch {
	case opens && links:
		return "opens and clicks"
	case opens:
		return "opens"
	case links:
		return "clicks"
	}
	return ""
}

// Allows reports whether p measures at least as much as q on each axis.
func (p Policy) Allows(q Policy) bool {
	return p.Opens.rank() >= q.Opens.rank() && p.Links.rank() >= q.Links.rank()
}

// Min is the lower of two policies, axis by axis.
func (p Policy) Min(q Policy) Policy {
	return Policy{Opens: Min(p.Opens, q.Opens), Links: Min(p.Links, q.Links)}
}

// Effective is the policy one Subscriber is sent with. Without a given Tracking consent
// (consent nil) it is `off`; otherwise the lower of the ceiling recorded with the consent and
// the current one, so a ceiling raised later never reaches beyond what was agreed to.
func Effective(consent *Policy, ceiling Policy) Policy {
	if consent == nil {
		return None
	}
	return consent.Min(ceiling)
}
