package main

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
)

const emailSender = "noreply@messages.englandsoftware.com"

type emailSettings struct {
	host, key, recipient string
	client               *http.Client
}

func emailSettingsFromEnv() (emailSettings, error) {
	s := emailSettings{
		host:      strings.TrimRight(os.Getenv("EGATE_HOST"), "/"),
		key:       os.Getenv("EGATE_API_KEY"),
		recipient: os.Getenv("CONTACT_EMAIL_TO"),
		client: &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}},
	}
	if s.host == "" {
		s.host = "https://englandsoftware.com"
	}
	u, err := url.Parse(s.host)
	if err != nil || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1"))) || u.Host == "" || u.Path != "" || u.RawQuery != "" {
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

type emailJob struct {
	id, attempts         int64
	name, email, message string
	followup             bool
}

func (a *application) nextEmailJob(now time.Time) (emailJob, error) {
	var j emailJob
	err := a.db.QueryRow("SELECT id,name,email,message,followup_opt_in,attempts FROM contact_email_jobs WHERE status='pending' AND next_attempt_at <= ? ORDER BY next_attempt_at,id LIMIT 1", stamp(now)).Scan(&j.id, &j.name, &j.email, &j.message, &j.followup, &j.attempts)
	return j, err
}

// uncertain is true when the gateway might have accepted the request but the
// worker did not receive a response. Those jobs require manual review.
func (s emailSettings) send(ctx context.Context, j emailJob) (uncertain bool, err error) {
	followup := "No"
	if j.followup {
		followup = "Yes"
	}
	body := fmt.Sprintf("New England Software contact message\n\nName: %s\nEmail: %s\nConsulting/application follow-up opt-in: %s\n\nMessage:\n%s", j.name, j.email, followup, j.message)
	payload, err := json.Marshal(map[string]string{
		"from": emailSender, "to": s.recipient,
		"subject":   "New website contact from " + j.name,
		"text_body": body, "reply_to": j.email,
	})
	if err != nil {
		return false, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.host+"/v1/email", bytes.NewReader(payload))
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+s.key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return true, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, fmt.Errorf("egate returned HTTP %d", resp.StatusCode)
	}
	return false, nil
}

func (a *application) processEmailQueue(ctx context.Context, s emailSettings) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		j, err := a.nextEmailJob(time.Now().UTC())
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		uncertain, sendErr := s.send(ctx, j)
		if err := ctx.Err(); err != nil {
			return err
		}
		if sendErr == nil {
			if _, err = a.db.Exec("DELETE FROM contact_email_jobs WHERE id=?", j.id); err != nil {
				return err
			}
			continue
		}
		if uncertain {
			if _, err = a.db.Exec("UPDATE contact_email_jobs SET status='review',attempts=attempts+1,last_error=? WHERE id=?", sendErr.Error(), j.id); err != nil {
				return err
			}
			log.Printf("contact email job %d needs delivery review: %v", j.id, sendErr)
			continue
		}
		attempts := j.attempts + 1
		delay := time.Minute << min(attempts-1, 6)
		if delay > time.Hour {
			delay = time.Hour
		}
		if _, dbErr := a.db.Exec("UPDATE contact_email_jobs SET attempts=?,next_attempt_at=?,last_error=? WHERE id=?", attempts, stamp(time.Now().UTC().Add(delay)), sendErr.Error(), j.id); dbErr != nil {
			return dbErr
		}
		log.Printf("contact email job %d failed (attempt %d); retrying: %v", j.id, attempts, sendErr)
	}
}

// The HTTP process supervises a separate worker. SQLite is the durable handoff;
// stdin only wakes the worker so the request never waits for egate.
func (a *application) superviseEmailWorker(ctx context.Context) {
	executable, err := os.Executable()
	if err != nil {
		log.Printf("email worker executable: %v", err)
		return
	}
	for ctx.Err() == nil {
		cmd := exec.CommandContext(ctx, executable, "email-worker")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		stdin, err := cmd.StdinPipe()
		if err == nil {
			err = cmd.Start()
		}
		if err != nil {
			log.Printf("email worker start: %v", err)
		} else {
			log.Printf("email worker started (pid %d)", cmd.Process.Pid)
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			waiting := true
			for waiting {
				select {
				case <-a.emailWake:
					if _, err := io.WriteString(stdin, "\n"); err != nil {
						waiting = false
					}
				case err := <-done:
					if ctx.Err() == nil {
						log.Printf("email worker stopped: %v", err)
					}
					waiting = false
				case <-ctx.Done():
					waiting = false
				}
			}
			stdin.Close()
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
}

func (a *application) runEmailWorker(ctx context.Context, s emailSettings) error {
	wake := make(chan struct{}, 1)
	stdinClosed := make(chan struct{})
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			select {
			case wake <- struct{}{}:
			default:
			}
		}
		close(stdinClosed)
	}()
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		if err := a.processEmailQueue(ctx, s); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("email queue: %v", err)
		}
		select {
		case <-wake:
		case <-ticker.C:
		case <-stdinClosed:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
