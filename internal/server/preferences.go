package server

import (
	"net/http"

	"github.com/davideimola/salvo/internal/tracking"
)

// preferencesPage is the Preferences page, addressed by the Subscriber's unsubscribe token.
// It only reads: link scanners open every link, and opening it must neither withdraw nor
// give a Tracking consent.
func (s *Server) preferencesPage(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "test" {
		s.page(w, http.StatusOK, "message", map[string]any{
			"Title": "This is a test send",
			"Body":  "In the real issue this link opens the tracking preferences of the reader who received it.",
		})
		return
	}
	p, err := s.svc.Preferences(r.Context(), token)
	if err != nil {
		s.invalidUnsubscribe(w, err)
		return
	}
	if !p.Subscribed {
		s.notSubscribed(w, p.Email)
		return
	}
	s.page(w, http.StatusOK, "preferences", map[string]any{
		"Email":     p.Email,
		"Token":     token,
		"Tracks":    p.Ceiling.Raised(),
		"Consented": p.Consented(),
		"Measured":  p.Measured.Raised(),
		// What is named is what is measured for this reader or, when nothing is, what the
		// ceiling could measure: a reader of a newsletter measuring only clicks never reads
		// about opens.
		"What": measurable(p.Measured, p.Ceiling),
	})
}

// measurable names what a policy measures, for the page's copy: the first one that measures
// anything.
func measurable(ps ...tracking.Policy) string {
	c := tracking.None
	for _, p := range ps {
		if p.Raised() {
			c = p
			break
		}
	}
	switch {
	case c.Opens != tracking.Off && c.Links != tracking.Off:
		return "opens and clicks"
	case c.Opens != tracking.Off:
		return "opens"
	}
	return "clicks"
}

// preferences takes the page's one button. Each action is its own form value, so the
// tracking confirmation request can sit next to Withdraw without sharing its path.
func (s *Server) preferences(w http.ResponseWriter, r *http.Request) {
	token := r.PostFormValue("token")
	switch r.PostFormValue("action") {
	case "withdraw":
		p, err := s.svc.Preferences(r.Context(), token)
		if err != nil {
			s.invalidUnsubscribe(w, err)
			return
		}
		if !p.Subscribed {
			s.notSubscribed(w, p.Email)
			return
		}
		if err := s.svc.Withdraw(r.Context(), token); err != nil {
			s.invalidUnsubscribe(w, err)
			return
		}
		s.page(w, http.StatusOK, "message", map[string]any{
			"Title": "Tracking stopped",
			"Body":  "Your opens and clicks are no longer measured. You are still subscribed: the next issue will reach you as before.",
			"Link":  "/preferences?token=" + token, "LinkText": "Back to your preferences",
		})
	default:
		http.Error(w, "unknown action", http.StatusBadRequest)
	}
}
