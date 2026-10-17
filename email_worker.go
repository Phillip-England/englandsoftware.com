package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"strings"
	"time"
)

const emailSender = "noreply@messages.englandsoftware.com"

type emailSettings struct {
	host, key, recipient string
	client               *http.Client
}
type emailMessage struct {
	name, email, message string
	followup             bool
}

func emailSettingsFromEnv() (emailSettings, error) {
	s := emailSettings{host: strings.TrimRight(os.Getenv("EGATE_HOST"), "/"), key: os.Getenv("EGATE_API_KEY"), recipient: os.Getenv("CONTACT_EMAIL_TO"), client: &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	if s.host == "" {
		return s, errors.New("EGATE_HOST is required and must be the public gateway base URL")
	}
	u, err := url.Parse(s.host)
	if err != nil || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1"))) || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return s, errors.New("EGATE_HOST must be an HTTPS base URL (HTTP localhost is allowed for tests)")
	}
	address, err := mail.ParseAddress(s.recipient)
	if err != nil || address.Address != s.recipient {
		return s, errors.New("CONTACT_EMAIL_TO must be one email address")
	}
	if s.key == "" {
		return s, errors.New("EGATE_API_KEY is required")
	}
	return s, nil
}

func (s emailSettings) send(ctx context.Context, m emailMessage, ip string) error {
	followup := "No"
	if m.followup {
		followup = "Yes"
	}
	body := fmt.Sprintf("New England Software contact message\n\nName: %s\nEmail: %s\nConsulting/application follow-up opt-in: %s\n\nMessage:\n%s", m.name, m.email, followup, m.message)
	payload, err := json.Marshal(map[string]string{"from": emailSender, "to": s.recipient, "subject": "New website contact from " + m.name, "text_body": body, "reply_to": m.email})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.host+"/v1/email", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Egate-Client-IP", ip)
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("egate returned HTTP %d", resp.StatusCode)
	}
	return nil
}
