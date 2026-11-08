// Package config reads the service's settings from the environment. README.md lists them.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/davideimola/salvo/internal/tracking"
)

type Config struct {
	Listen      string
	DatabaseURL string
	// PublicURL is this service's own origin, e.g. https://news.davideimola.dev.
	PublicURL string
	// SiteURL is the website's origin, e.g. https://davideimola.dev.
	SiteURL string
	// APIKey is what the website presents, as a bearer token, to /api.
	APIKey string

	KannonURL    string
	KannonDomain string
	KannonAPIKey string
	// KannonAdminToken, optional, lets the admin pages show Kannon's delivery counters.
	KannonAdminToken string
	SenderEmail      string
	SenderName       string
	// TestRecipient receives every test send.
	TestRecipient string

	// What readers see of the newsletter, all optional (newsletter.Identity).
	NewsletterName    string
	NewsletterAbout   string
	NewsletterSignOff string
	ConfirmSubject    string
	SubscribeURL      string
	ConfirmedURL      string
	PrivacyURL        string

	// TrackingCeiling is the most tracking the owner allows, for opens and for links
	// (TRACKING_OPENS, TRACKING_LINKS). Both `off` unless raised.
	TrackingCeiling tracking.Policy
	// NoticeVersion names the privacy notice in force, recorded with every Tracking choice.
	// Required while a ceiling is raised.
	NoticeVersion string

	OwnerEmail         string
	GoogleClientID     string
	GoogleClientSecret string
	// SessionKey signs the owner's session cookie. At least 32 bytes.
	SessionKey string
}

func FromEnv() (*Config, error) {
	var missing []string
	get := func(name string) string {
		v := strings.TrimSpace(os.Getenv(name))
		if v == "" {
			missing = append(missing, name)
		}
		return v
	}
	opt := func(name, fallback string) string {
		if v := strings.TrimSpace(os.Getenv(name)); v != "" {
			return v
		}
		return fallback
	}
	c := &Config{
		Listen:             opt("LISTEN_ADDR", ":8080"),
		DatabaseURL:        get("DATABASE_URL"),
		PublicURL:          strings.TrimRight(get("PUBLIC_URL"), "/"),
		SiteURL:            strings.TrimRight(get("SITE_URL"), "/"),
		APIKey:             get("API_KEY"),
		KannonURL:          get("KANNON_URL"),
		KannonDomain:       get("KANNON_DOMAIN"),
		KannonAPIKey:       get("KANNON_API_KEY"),
		KannonAdminToken:   opt("KANNON_ADMIN_TOKEN", ""),
		SenderEmail:        get("SENDER_EMAIL"),
		SenderName:         get("SENDER_NAME"),
		TestRecipient:      get("TEST_RECIPIENT"),
		NewsletterName:     opt("NEWSLETTER_NAME", ""),
		NewsletterAbout:    opt("NEWSLETTER_ABOUT", ""),
		NewsletterSignOff:  opt("NEWSLETTER_SIGNOFF", ""),
		ConfirmSubject:     opt("CONFIRM_SUBJECT", ""),
		SubscribeURL:       opt("SUBSCRIBE_URL", ""),
		ConfirmedURL:       opt("CONFIRMED_URL", ""),
		PrivacyURL:         opt("PRIVACY_URL", ""),
		OwnerEmail:         get("OWNER_EMAIL"),
		GoogleClientID:     get("GOOGLE_CLIENT_ID"),
		GoogleClientSecret: get("GOOGLE_CLIENT_SECRET"),
		SessionKey:         get("SESSION_KEY"),
		NoticeVersion:      opt("NOTICE_VERSION", ""),
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing environment variables: %s", strings.Join(missing, ", "))
	}
	var errs []error
	for name, mode := range map[string]*tracking.Mode{"TRACKING_OPENS": &c.TrackingCeiling.Opens, "TRACKING_LINKS": &c.TrackingCeiling.Links} {
		m, err := tracking.ParseMode(opt(name, string(tracking.Off)))
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
		*mode = m
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return c, c.validate()
}

// DatabaseOnly reads just what the `migrate` and `erase` commands need.
func DatabaseOnly() (string, error) {
	v := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if v == "" {
		return "", errors.New("missing environment variable: DATABASE_URL")
	}
	return v, nil
}

func (c *Config) validate() error {
	var errs []error
	for name, v := range map[string]string{"PUBLIC_URL": c.PublicURL, "SITE_URL": c.SiteURL, "KANNON_URL": c.KannonURL} {
		u, err := url.Parse(v)
		if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.Path != "" {
			errs = append(errs, fmt.Errorf("%s must be an origin like https://example.com, got %q", name, v))
		}
	}
	for name, v := range map[string]string{"SUBSCRIBE_URL": c.SubscribeURL, "CONFIRMED_URL": c.ConfirmedURL, "PRIVACY_URL": c.PrivacyURL} {
		if v == "" {
			continue
		}
		u, err := url.Parse(v)
		if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
			errs = append(errs, fmt.Errorf("%s must be an absolute URL like https://example.com/page, got %q", name, v))
		}
	}
	if len(c.APIKey) < 32 {
		errs = append(errs, errors.New("API_KEY must be at least 32 characters"))
	}
	if c.TrackingCeiling.Raised() && c.NoticeVersion == "" {
		errs = append(errs, errors.New("NOTICE_VERSION is required while TRACKING_OPENS or TRACKING_LINKS is raised"))
	}
	if len(c.SessionKey) < 32 {
		errs = append(errs, errors.New("SESSION_KEY must be at least 32 characters"))
	}
	return errors.Join(errs...)
}
