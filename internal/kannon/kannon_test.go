package kannon_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/davideimola/salvo/internal/kannon"
)

func TestSendSpeaksKannonsJSON(t *testing.T) {
	var got map[string]any
	var auth, path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, auth = r.URL.Path, r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &got)
		w.Write([]byte(`{"messageId":"msg_x@news.example.com","acceptedCount":1,"rejectedRecipients":[{"email":"b@example.org","reason":"invalid_email"}]}`))
	}))
	defer srv.Close()

	c := &kannon.Client{URL: srv.URL + "/", Domain: "news.example.com", APIKey: "k_secret",
		Sender: kannon.Sender{Email: "hello@example.com", Alias: "Hello"}}
	res, err := c.Send(context.Background(), kannon.Message{
		Subject:        "Hi",
		HTML:           "<p>x</p>",
		Recipients:     []kannon.Recipient{{Email: "a@example.org", Fields: map[string]string{"token": "t"}}},
		UnsubscribeURL: "https://news.example.com/unsubscribe?token={{ token }}",
	})
	if err != nil {
		t.Fatal(err)
	}
	if path != "/pkg.kannon.mailer.apiv1.Mailer/SendHTML" {
		t.Errorf("path = %s", path)
	}
	if auth != "Basic "+base64.StdEncoding.EncodeToString([]byte("news.example.com:k_secret")) {
		t.Errorf("auth = %s", auth)
	}
	tracking, _ := got["tracking"].(map[string]any)
	if tracking["opens"] != "TRACKING_MODE_OFF" || tracking["links"] != "TRACKING_MODE_OFF" {
		t.Errorf("tracking = %v, want both off", got["tracking"])
	}
	unsub, _ := got["oneClickUnsubscribe"].(map[string]any)
	if unsub["urlTemplate"] != "https://news.example.com/unsubscribe?token={{ token }}" {
		t.Errorf("oneClickUnsubscribe = %v", got["oneClickUnsubscribe"])
	}
	sender, _ := got["sender"].(map[string]any)
	if sender["email"] != "hello@example.com" || sender["alias"] != "Hello" {
		t.Errorf("sender = %v", got["sender"])
	}
	if res.MessageID != "msg_x@news.example.com" || res.Accepted != 1 || len(res.Rejected) != 1 {
		t.Errorf("result = %+v", res)
	}
}

func TestNoUnsubscribeHeaderWhenNotAsked(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&got)
		w.Write([]byte(`{"messageId":"m","acceptedCount":1}`))
	}))
	defer srv.Close()
	c := &kannon.Client{URL: srv.URL, Domain: "d", APIKey: "k"}
	if _, err := c.Send(context.Background(), kannon.Message{Recipients: []kannon.Recipient{{Email: "a@example.org"}}}); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["oneClickUnsubscribe"]; ok {
		t.Error("oneClickUnsubscribe sent for a message that is not the newsletter")
	}
}

func TestRefusal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"code":"unauthenticated"}`, http.StatusUnauthorized)
	}))
	defer srv.Close()
	c := &kannon.Client{URL: srv.URL, Domain: "d", APIKey: "k"}
	_, err := c.Send(context.Background(), kannon.Message{})
	var refused *kannon.RefusedError
	if !errors.As(err, &refused) || refused.Status != http.StatusUnauthorized {
		t.Fatalf("err = %v, want a RefusedError with 401", err)
	}
}

func TestSendStatesTrackingPerRecipient(t *testing.T) {
	var got struct {
		Tracking   map[string]string `json:"tracking"`
		Recipients []struct {
			Email    string            `json:"email"`
			Tracking map[string]string `json:"tracking"`
		} `json:"recipients"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&got)
		w.Write([]byte(`{"messageId":"m","acceptedCount":2}`))
	}))
	defer srv.Close()
	c := &kannon.Client{URL: srv.URL, Domain: "d", APIKey: "k"}
	pseudo := kannon.Tracking{Opens: kannon.ModePseudonymous, Links: kannon.ModePseudonymous}
	_, err := c.Send(context.Background(), kannon.Message{
		Tracking: pseudo,
		Recipients: []kannon.Recipient{
			{Email: "yes@example.org", Tracking: &kannon.Tracking{Opens: kannon.ModeOff, Links: kannon.ModePseudonymous}},
			{Email: "no@example.org", Tracking: &kannon.Tracking{Opens: kannon.ModeOff, Links: kannon.ModeOff}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Tracking["opens"] != "TRACKING_MODE_PSEUDONYMOUS" || got.Tracking["links"] != "TRACKING_MODE_PSEUDONYMOUS" {
		t.Errorf("batch tracking = %v", got.Tracking)
	}
	if len(got.Recipients) != 2 {
		t.Fatalf("recipients = %+v", got.Recipients)
	}
	if r := got.Recipients[0].Tracking; r["opens"] != "TRACKING_MODE_OFF" || r["links"] != "TRACKING_MODE_PSEUDONYMOUS" {
		t.Errorf("yes@ tracking = %v", r)
	}
	if r := got.Recipients[1].Tracking; r["opens"] != "TRACKING_MODE_OFF" || r["links"] != "TRACKING_MODE_OFF" {
		t.Errorf("no@ tracking = %v, want both off stated explicitly", r)
	}
}

func TestNoPerRecipientTrackingWhenNotGiven(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&got)
		w.Write([]byte(`{"messageId":"m","acceptedCount":1}`))
	}))
	defer srv.Close()
	c := &kannon.Client{URL: srv.URL, Domain: "d", APIKey: "k"}
	if _, err := c.Send(context.Background(), kannon.Message{Recipients: []kannon.Recipient{{Email: "a@example.org"}}}); err != nil {
		t.Fatal(err)
	}
	r := got["recipients"].([]any)[0].(map[string]any)
	if _, ok := r["tracking"]; ok {
		t.Errorf("recipient = %v, want no tracking field", r)
	}
	tracking, _ := got["tracking"].(map[string]any)
	if tracking["opens"] != "TRACKING_MODE_OFF" || tracking["links"] != "TRACKING_MODE_OFF" {
		t.Errorf("batch tracking = %v, want both off by default", tracking)
	}
}
