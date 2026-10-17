package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestContactSendsDirectlyToEgate(t *testing.T) {
	a := testApp(t)
	var received map[string]string
	var ip string
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/email" || r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("gateway request: %s %s", r.Method, r.URL.Path)
		}
		ip = r.Header.Get("X-Egate-Client-IP")
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Error(err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer gateway.Close()
	a.email = emailSettings{host: gateway.URL, key: "test-key", recipient: "owner@example.com", client: gateway.Client()}
	form := url.Values{"name": {"Alex"}, "email": {"alex@example.com"}, "message": {"A website inquiry"}, "followup_opt_in": {"yes"}}
	if got := request(t, a, http.MethodPost, "/contact", form, nil).Code; got != http.StatusSeeOther {
		t.Fatalf("contact response: %d", got)
	}
	if ip != "192.0.2.5" || received["from"] != emailSender || received["to"] != "owner@example.com" || received["reply_to"] != "alex@example.com" || !strings.Contains(received["text_body"], "A website inquiry") || !strings.Contains(received["text_body"], "opt-in: Yes") {
		t.Fatalf("request ip=%q body=%+v", ip, received)
	}
}

func TestEmailSettingsRequireGatewayHost(t *testing.T) {
	t.Setenv("EGATE_HOST", "")
	t.Setenv("EGATE_API_KEY", "eg_test")
	t.Setenv("CONTACT_EMAIL_TO", "owner@example.com")
	if _, err := emailSettingsFromEnv(); err == nil || !strings.Contains(err.Error(), "EGATE_HOST") {
		t.Fatalf("missing gateway host accepted: %v", err)
	}
}

func TestGatewayFailureStillSavesMessage(t *testing.T) {
	a := testApp(t)
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer gateway.Close()
	a.email = emailSettings{host: gateway.URL, key: "test-key", recipient: "owner@example.com", client: gateway.Client()}
	form := url.Values{"name": {"Alex"}, "email": {"alex@example.com"}, "message": {"Inquiry"}}
	if got := request(t, a, http.MethodPost, "/contact", form, nil).Code; got != http.StatusBadGateway {
		t.Fatalf("failure response: %d", got)
	}
	var count int
	if err := a.db.QueryRow("SELECT COUNT(*) FROM contact_messages").Scan(&count); err != nil || count != 1 {
		t.Fatalf("saved messages: %d %v", count, err)
	}
}

func TestContactAdditionalHoneypotsAndMessageLimit(t *testing.T) {
	a := testApp(t)
	base := url.Values{"name": {"Alex"}, "email": {"alex@example.com"}, "message": {"Inquiry"}}
	for _, field := range contactHoneypots {
		form := url.Values{"name": {"Alex"}, "email": {"alex@example.com"}, "message": {"Inquiry"}}
		form.Set(field, "filled")
		if got := request(t, a, http.MethodPost, "/contact", form, nil).Code; got != http.StatusSeeOther {
			t.Fatalf("honeypot %s: %d", field, got)
		}
	}
	var count int
	if err := a.db.QueryRow("SELECT COUNT(*) FROM contact_messages").Scan(&count); err != nil || count != 0 {
		t.Fatalf("honeypot stored: %d %v", count, err)
	}
	base.Set("message", strings.Repeat("a", 3001))
	if got := request(t, a, http.MethodPost, "/contact", base, nil).Code; got != http.StatusBadRequest {
		t.Fatalf("oversized: %d", got)
	}
}
