package newsletter

import (
	"net/url"
	"strings"
)

// Identity is who the newsletter is and where its pages live: everything a reader sees that
// is not the same for every Salvo. Every field may be left empty; Service.Identity fills in a default
// that reads well for any newsletter.
type Identity struct {
	// Name is the newsletter's name, e.g. "Ada's notes". Defaults to Sender.
	Name string
	// Sender is who the confirmation email comes from, printed at its top.
	Sender string
	// About is a sentence or two on what the newsletter is, in the confirmation email.
	About string
	// SignOff closes the confirmation email, e.g. "Ada".
	SignOff string
	// ConfirmSubject is the confirmation email's subject. Defaults to one naming Name.
	ConfirmSubject string
	// SubscribeURL is where the subscribe form is. Defaults to the website.
	SubscribeURL string
	// ConfirmedURL is where a confirmed reader lands. Defaults to Salvo's own /confirmed.
	ConfirmedURL string
	// PrivacyURL, if set, is linked from the confirmation email.
	PrivacyURL string
}

// Identity returns the newsletter's Identity with every default filled in.
func (s *Service) Identity() Identity {
	id := s.Ident
	if id.Name == "" {
		id.Name = id.Sender
	}
	if id.Name == "" {
		id.Name = "the newsletter"
	}
	if id.ConfirmSubject == "" {
		id.ConfirmSubject = "Confirm your subscription to " + id.Name
	}
	if id.SubscribeURL == "" {
		id.SubscribeURL = s.SiteURL
	}
	if id.ConfirmedURL == "" {
		id.ConfirmedURL = s.PublicURL + "/confirmed"
	}
	return id
}

// SiteName is the website's host, e.g. example.org, for a link back to it.
func (s *Service) SiteName() string {
	u, err := url.Parse(s.SiteURL)
	if err != nil || u.Host == "" {
		return s.SiteURL
	}
	return strings.TrimPrefix(u.Host, "www.")
}
