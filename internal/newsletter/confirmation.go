package newsletter

import (
	"bytes"
	"html/template"

	"github.com/davideimola/salvo/internal/tracking"
)

// The confirmation email. Inline styles only, because that is what mail clients keep, in a
// plain paper and ink. What it says about the newsletter comes from its Identity. No image of
// any kind: the email this service sends to ask for consent is the last place a pixel could
// hide.
var confirmationTemplate = template.Must(template.New("confirmation").Parse(`<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Confirm your subscription</title></head>
<body style="margin:0;padding:0;background:#F7F5F0;">
<div style="max-width:520px;margin:0 auto;padding:40px 24px;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Helvetica,Arial,sans-serif;color:#1C2233;font-size:16px;line-height:1.6;">
<p style="margin:0 0 24px;font-weight:700;font-size:18px;">{{.Header}}</p>
<p style="margin:0 0 16px;">Thanks for signing up. One click and you're in.</p>
{{if .About}}<p style="margin:0 0 24px;">{{.About}}</p>
{{end}}<p style="margin:0 0 24px;"><a href="{{.ConfirmURL}}" style="display:inline-block;background:#1C2233;color:#F7F5F0;text-decoration:none;padding:12px 20px;border-radius:4px;font-weight:600;">Confirm my subscription</a></p>
<p style="margin:0 0 16px;font-size:14px;color:#3B4152;">Or paste this link into your browser:<br><a href="{{.ConfirmURL}}" style="color:#1C2233;word-break:break-all;">{{.ConfirmURL}}</a></p>
<p style="margin:0 0 16px;">Didn't sign up? Ignore this email and nothing happens: the link stops working in seven days, and the address is deleted within 30.</p>
{{if .SignOff}}<p style="margin:0 0 16px;">{{.SignOff}}</p>
{{end}}<p style="margin:32px 0 0;font-size:13px;color:#5B6070;">{{if .Measured}}The newsletter measures {{.Measured}} for each issue, only for readers who agree to it.{{if .Requested}} You asked to be measured: confirming also agrees to it. You can withdraw at any time and stay subscribed.{{end}}{{else}}The newsletter carries no tracking pixel and no tracked links.{{end}}{{if .PrivacyURL}} <a href="{{.PrivacyURL}}" style="color:#5B6070;">Privacy</a>{{end}}</p>
</div>
</body>
</html>
`))

// renderConfirmation writes the confirmation email. Its closing line says what the
// newsletter measures under the ceiling and, when the request asked for tracking, that
// confirming agrees to it.
func renderConfirmation(id Identity, confirmURL string, ceiling tracking.Policy, requested bool) (string, error) {
	header := id.Sender
	if header == "" {
		header = id.Name
	}
	var b bytes.Buffer
	err := confirmationTemplate.Execute(&b, struct {
		Header, About, SignOff, PrivacyURL, ConfirmURL, Measured string
		Requested                                                bool
	}{header, id.About, id.SignOff, id.PrivacyURL, confirmURL, measured(ceiling), requested})
	return b.String(), err
}

// measured names what a ceiling measures, in a reader's words, or "" for nothing.
func measured(p tracking.Policy) string {
	opens, links := p.Opens == tracking.Pseudonymous, p.Links == tracking.Pseudonymous
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
