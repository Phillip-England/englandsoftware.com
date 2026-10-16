package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestContactQueueAndEgateDelivery(t *testing.T) {
	a := testApp(t)
	var received []map[string]string
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/email" || r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected gateway request: %s %s, auth %q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode gateway request: %v", err)
		}
		received = append(received, body)
		w.WriteHeader(http.StatusCreated)
	}))
	defer gateway.Close()
	form := url.Values{"name": {"Alex"}, "email": {"alex@example.com"}, "message": {"A website inquiry"}, "followup_opt_in": {"yes"}}
	if got := request(t, a, http.MethodPost, "/contact", form, nil).Code; got != http.StatusSeeOther {
		t.Fatalf("contact response: %d", got)
	}
	if len(received) != 0 {
		t.Fatal("HTTP handler sent email before returning")
	}
	var queued int
	if err := a.db.QueryRow("SELECT COUNT(*) FROM contact_email_jobs").Scan(&queued); err != nil || queued != 1 {
		t.Fatalf("queued jobs: %d, %v", queued, err)
	}
	s := emailSettings{host: gateway.URL, key: "test-key", recipient: "owner@example.com", client: gateway.Client()}
	if err := a.processEmailQueue(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	if len(received) != 1 || received[0]["from"] != emailSender || received[0]["to"] != "owner@example.com" || received[0]["reply_to"] != "alex@example.com" || !strings.Contains(received[0]["text_body"], "A website inquiry") || !strings.Contains(received[0]["text_body"], "opt-in: Yes") {
		t.Fatalf("unexpected delivered email: %+v", received)
	}
	if err := a.db.QueryRow("SELECT COUNT(*) FROM contact_email_jobs").Scan(&queued); err != nil || queued != 0 {
		t.Fatalf("job not removed after delivery: %d, %v", queued, err)
	}
}

func TestEmailQueueRetriesDefiniteFailure(t *testing.T) {
	a := testApp(t)
	form := url.Values{"name": {"Alex"}, "email": {"alex@example.com"}, "message": {"Inquiry"}}
	request(t, a, http.MethodPost, "/contact", form, nil)
	calls := 0
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer gateway.Close()
	s := emailSettings{host: gateway.URL, key: "test-key", recipient: "owner@example.com", client: gateway.Client()}
	if err := a.processEmailQueue(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	var status string
	var attempts int
	if err := a.db.QueryRow("SELECT status,attempts FROM contact_email_jobs").Scan(&status, &attempts); err != nil || status != "pending" || attempts != 1 || calls != 1 {
		t.Fatalf("failed delivery state: %q, attempts %d, calls %d, error %v", status, attempts, calls, err)
	}
	if _, err := a.db.Exec("UPDATE contact_email_jobs SET next_attempt_at=?", stamp(time.Now().Add(-time.Minute))); err != nil {
		t.Fatal(err)
	}
	if err := a.processEmailQueue(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("delivery attempts: %d", calls)
	}
}

type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("connection lost")
}

func TestEmailQueueHoldsUncertainDeliveryForReview(t *testing.T) {
	a := testApp(t)
	request(t, a, http.MethodPost, "/contact", url.Values{"name": {"Alex"}, "email": {"alex@example.com"}, "message": {"Inquiry"}}, nil)
	s := emailSettings{host: "https://example.com", key: "test-key", recipient: "owner@example.com", client: &http.Client{Transport: failingTransport{}}}
	if err := a.processEmailQueue(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := a.db.QueryRow("SELECT status FROM contact_email_jobs").Scan(&status); err != nil || status != "review" {
		t.Fatalf("uncertain delivery status: %q, %v", status, err)
	}
}

func TestContactAdditionalHoneypotsAndMessageLimit(t *testing.T) {
	a := testApp(t)
	base := url.Values{"name": {"Alex"}, "email": {"alex@example.com"}, "message": {"Inquiry"}}
	for _, field := range contactHoneypots {
		form := url.Values{"name": {"Alex"}, "email": {"alex@example.com"}, "message": {"Inquiry"}}
		form.Set(field, "filled")
		if got := request(t, a, http.MethodPost, "/contact", form, nil).Code; got != http.StatusSeeOther {
			t.Fatalf("honeypot %s response: %d", field, got)
		}
	}
	var count int
	if err := a.db.QueryRow("SELECT COUNT(*) FROM contact_email_jobs").Scan(&count); err != nil || count != 0 {
		t.Fatalf("honeypot queued mail: %d, %v", count, err)
	}
	base.Set("message", strings.Repeat("a", 3001))
	if got := request(t, a, http.MethodPost, "/contact", base, nil).Code; got != http.StatusBadRequest {
		t.Fatalf("oversized message response: %d", got)
	}
}
