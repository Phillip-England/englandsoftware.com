package main

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const sessionCookie = "englandsoftware_admin"
const timeFormat = time.RFC3339Nano

type appConfig struct {
	username, password string
	secureCookies      bool
	trustedProxies     []netip.Prefix
}
type application struct {
	db  *sql.DB
	cfg appConfig
}
type message struct {
	ID                                int64
	Name, Email, Phone, Body, Created string
	IsRead                            bool
	FollowupOptIn                     bool
}

func loadEnvFile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("missing %s: configure admin credentials before starting", path)
	}
	if err != nil {
		return err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("invalid line in %s", path)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
			value = value[1 : len(value)-1]
		}
		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, value)
		}
	}
	return s.Err()
}

func newApp(path string, cfg appConfig) (*application, error) {
	if cfg.username == "" || cfg.password == "" {
		return nil, errors.New("admin username and password must be set in config/.env")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE, 0600)
	if err != nil {
		return nil, err
	}
	file.Close()
	if err := os.Chmod(path, 0600); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err = db.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		db.Close()
		return nil, err
	}
	if _, err = db.Exec("PRAGMA journal_mode = WAL"); err != nil {
		db.Close()
		return nil, err
	}
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS contact_messages (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, email TEXT NOT NULL, phone TEXT NOT NULL DEFAULT '', message TEXT NOT NULL, created_at DATETIME NOT NULL, is_read INTEGER NOT NULL DEFAULT 0, followup_opt_in INTEGER NOT NULL DEFAULT 0)`,
		`CREATE TABLE IF NOT EXISTS login_failures (id INTEGER PRIMARY KEY AUTOINCREMENT, ip TEXT NOT NULL, created_at DATETIME NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS idx_login_failures_ip_created_at ON login_failures (ip, created_at)`,
		`CREATE TABLE IF NOT EXISTS banned_ips (ip TEXT PRIMARY KEY, banned_until DATETIME NOT NULL, created_at DATETIME NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS contact_message_submissions (id INTEGER PRIMARY KEY AUTOINCREMENT, ip TEXT NOT NULL, created_at DATETIME NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS idx_contact_message_submissions_ip_created_at ON contact_message_submissions (ip, created_at)`,
		`CREATE TABLE IF NOT EXISTS admin_sessions (token_hash TEXT PRIMARY KEY, expires_at TEXT NOT NULL)`,
	} {
		if _, err = db.Exec(stmt); err != nil {
			db.Close()
			return nil, err
		}
	}
	// Existing installations may lack either column.
	found := make(map[string]bool)
	rows, err := db.Query("PRAGMA table_info(contact_messages)")
	if err != nil {
		db.Close()
		return nil, err
	}
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var def sql.NullString
		if err = rows.Scan(&cid, &name, &typ, &notnull, &def, &pk); err != nil {
			break
		}
		found[name] = true
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		db.Close()
		return nil, err
	}
	if !found["is_read"] {
		if _, err = db.Exec("ALTER TABLE contact_messages ADD COLUMN is_read INTEGER NOT NULL DEFAULT 0"); err != nil {
			db.Close()
			return nil, err
		}
	}
	if !found["followup_opt_in"] {
		if _, err = db.Exec("ALTER TABLE contact_messages ADD COLUMN followup_opt_in INTEGER NOT NULL DEFAULT 0"); err != nil {
			db.Close()
			return nil, err
		}
	}
	a := &application{db: db, cfg: cfg}
	if err = a.prune(time.Now().UTC()); err != nil {
		db.Close()
		return nil, err
	}
	return a, nil
}

func (a *application) handler() http.Handler {
	mux := http.NewServeMux()
	publicPages(mux)
	mux.HandleFunc("/contact", a.contact)
	mux.HandleFunc("/admin/login", a.login)
	mux.HandleFunc("/admin/logout", a.logout)
	mux.HandleFunc("/admin/messages/read", a.setRead)
	mux.HandleFunc("/admin/messages/delete", a.deleteMessage)
	mux.HandleFunc("/admin", a.inbox)
	return mux
}

func methodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}
func noStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "same-origin")
}
func (a *application) clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	remote, err := netip.ParseAddr(host)
	if err != nil || !a.trustedIP(remote) {
		return host
	}
	forwarded := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for i := len(forwarded) - 1; i >= 0; i-- {
		candidate, parseErr := netip.ParseAddr(strings.TrimSpace(forwarded[i]))
		if parseErr != nil {
			break
		}
		remote = candidate
		if !a.trustedIP(candidate) {
			break
		}
	}
	return remote.String()
}

func (a *application) trustedIP(ip netip.Addr) bool {
	for _, prefix := range a.cfg.trustedProxies {
		if prefix.Contains(ip) {
			return true
		}
	}
	return false
}
func stamp(t time.Time) string { return t.UTC().Format(timeFormat) }
func (a *application) prune(now time.Time) error {
	for _, q := range []struct {
		sql string
		arg string
	}{
		{"DELETE FROM login_failures WHERE created_at < ?", stamp(now.Add(-24 * time.Hour))},
		{"DELETE FROM banned_ips WHERE banned_until <= ?", stamp(now)},
		{"DELETE FROM contact_message_submissions WHERE created_at < ?", stamp(now.Add(-24 * time.Hour))},
		{"DELETE FROM admin_sessions WHERE expires_at <= ?", stamp(now)},
	} {
		if _, err := a.db.Exec(q.sql, q.arg); err != nil {
			return err
		}
	}
	return nil
}
func (a *application) jailed(ip string, now time.Time) (bool, error) {
	var until string
	err := a.db.QueryRow("SELECT banned_until FROM banned_ips WHERE ip=?", ip).Scan(&until)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	t, err := time.Parse(timeFormat, until)
	return err == nil && t.After(now), err
}
func (a *application) failed(ip string, now time.Time) error {
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("DELETE FROM login_failures WHERE created_at < ?", stamp(now.Add(-24*time.Hour))); err != nil {
		return err
	}
	if _, err = tx.Exec("DELETE FROM banned_ips WHERE banned_until <= ?", stamp(now)); err != nil {
		return err
	}
	if _, err = tx.Exec("INSERT INTO login_failures(ip,created_at) VALUES(?,?)", ip, stamp(now)); err != nil {
		return err
	}
	var count int
	if err = tx.QueryRow("SELECT COUNT(*) FROM login_failures WHERE ip=? AND created_at >= ?", ip, stamp(now.Add(-24*time.Hour))).Scan(&count); err != nil {
		return err
	}
	if count >= 5 {
		if _, err = tx.Exec("INSERT INTO banned_ips(ip,banned_until,created_at) VALUES(?,?,?) ON CONFLICT(ip) DO UPDATE SET banned_until=excluded.banned_until", ip, stamp(now.Add(24*time.Hour)), stamp(now)); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func originOK(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	return origin == "http://"+r.Host || origin == "https://"+r.Host
}
func parseForm(w http.ResponseWriter, r *http.Request, limit int64) bool {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", http.StatusBadRequest)
		return false
	}
	return true
}
func (a *application) contact(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, "POST")
		return
	}
	if !originOK(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if !parseForm(w, r, 16<<10) {
		return
	}
	if r.PostForm.Get("website") != "" {
		http.Redirect(w, r, "/?contact=sent#contact", http.StatusSeeOther)
		return
	}
	name := strings.TrimSpace(r.PostForm.Get("name"))
	email := strings.TrimSpace(r.PostForm.Get("email"))
	body := strings.TrimSpace(r.PostForm.Get("message"))
	followupOptIn := r.PostForm.Get("followup_opt_in") == "yes"
	if name == "" || email == "" || body == "" || len(name) > 200 || len(email) > 320 || len(body) > 10000 || !strings.Contains(email, "@") {
		http.Error(w, "Please complete the name, email, and message fields.", http.StatusBadRequest)
		return
	}
	now := time.Now().UTC()
	ip := a.clientIP(r)
	tx, err := a.db.Begin()
	if err != nil {
		serverError(w, err)
		return
	}
	defer tx.Rollback()
	if _, err = tx.Exec("DELETE FROM contact_message_submissions WHERE created_at < ?", stamp(now.Add(-24*time.Hour))); err != nil {
		serverError(w, err)
		return
	}
	var count int
	if err = tx.QueryRow("SELECT COUNT(*) FROM contact_message_submissions WHERE ip=? AND created_at >= ?", ip, stamp(now.Add(-24*time.Hour))).Scan(&count); err != nil {
		serverError(w, err)
		return
	}
	if count >= 3 {
		http.Error(w, "Please try again later.", http.StatusTooManyRequests)
		return
	}
	if _, err = tx.Exec("INSERT INTO contact_messages(name,email,message,created_at,followup_opt_in) VALUES(?,?,?,?,?)", name, email, body, stamp(now), followupOptIn); err != nil {
		serverError(w, err)
		return
	}
	if _, err = tx.Exec("INSERT INTO contact_message_submissions(ip,created_at) VALUES(?,?)", ip, stamp(now)); err != nil {
		serverError(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		serverError(w, err)
		return
	}
	http.Redirect(w, r, "/?contact=sent#contact", http.StatusSeeOther)
}
func serverError(w http.ResponseWriter, err error) {
	log.Printf("server error: %v", err)
	http.Error(w, "Server error", http.StatusInternalServerError)
}
func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func (a *application) setCookie(w http.ResponseWriter, token string, maxAge int, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/admin", HttpOnly: true, Secure: a.cfg.secureCookies || r.TLS != nil, SameSite: http.SameSiteStrictMode, MaxAge: maxAge})
}
func (a *application) session(r *http.Request) (string, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || len(c.Value) > 100 {
		return "", false
	}
	var expires string
	err = a.db.QueryRow("SELECT expires_at FROM admin_sessions WHERE token_hash=?", tokenHash(c.Value)).Scan(&expires)
	if err != nil {
		return "", false
	}
	t, err := time.Parse(timeFormat, expires)
	return c.Value, err == nil && t.After(time.Now().UTC())
}
func csrf(token string) string { return tokenHash("csrf:" + token) }
func checkCSRF(r *http.Request, token string) bool {
	return subtle.ConstantTimeCompare([]byte(r.PostForm.Get("csrf")), []byte(csrf(token))) == 1 && originOK(r)
}
func (a *application) login(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	if r.Method == http.MethodGet {
		if _, ok := a.session(r); ok {
			http.Redirect(w, r, "/admin", http.StatusSeeOther)
			return
		}
		renderLogin(w, "", http.StatusOK)
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w, "GET, POST")
		return
	}
	if !originOK(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if !parseForm(w, r, 8<<10) {
		return
	}
	now := time.Now().UTC()
	ip := a.clientIP(r)
	if err := a.prune(now); err != nil {
		serverError(w, err)
		return
	}
	jailed, err := a.jailed(ip, now)
	if err != nil {
		serverError(w, err)
		return
	}
	if jailed {
		renderLogin(w, "Too many attempts. Try again in 24 hours.", http.StatusTooManyRequests)
		return
	}
	username := r.PostForm.Get("username")
	password := r.PostForm.Get("password")
	// Always perform the password check, including for an unknown username.
	providedPassword := sha256.Sum256([]byte(password))
	configuredPassword := sha256.Sum256([]byte(a.cfg.password))
	correctPassword := subtle.ConstantTimeCompare(providedPassword[:], configuredPassword[:]) == 1
	correctUser := subtle.ConstantTimeCompare([]byte(username), []byte(a.cfg.username)) == 1
	if r.PostForm.Get("website") != "" || !correctUser || !correctPassword {
		if err := a.failed(ip, now); err != nil {
			serverError(w, err)
			return
		}
		renderLogin(w, "Invalid credentials.", http.StatusUnauthorized)
		return
	}
	token, err := randomToken()
	if err != nil {
		serverError(w, err)
		return
	}
	if _, err = a.db.Exec("INSERT INTO admin_sessions(token_hash,expires_at) VALUES(?,?)", tokenHash(token), stamp(now.Add(24*time.Hour))); err != nil {
		serverError(w, err)
		return
	}
	a.setCookie(w, token, 86400, r)
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}
func (a *application) requireAdmin(w http.ResponseWriter, r *http.Request) (string, bool) {
	token, ok := a.session(r)
	if !ok {
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
	}
	return token, ok
}
func (a *application) logout(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	if r.Method != http.MethodPost {
		methodNotAllowed(w, "POST")
		return
	}
	token, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	if !parseForm(w, r, 2<<10) {
		return
	}
	if !checkCSRF(r, token) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if _, err := a.db.Exec("DELETE FROM admin_sessions WHERE token_hash=?", tokenHash(token)); err != nil {
		serverError(w, err)
		return
	}
	a.setCookie(w, "", -1, r)
	http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
}
func (a *application) inbox(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	if r.Method != http.MethodGet {
		methodNotAllowed(w, "GET")
		return
	}
	token, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	rows, err := a.db.Query("SELECT id,name,email,message,created_at,is_read,followup_opt_in FROM contact_messages ORDER BY id DESC")
	if err != nil {
		serverError(w, err)
		return
	}
	defer rows.Close()
	var messages []message
	for rows.Next() {
		var m message
		var read, followup int
		if err = rows.Scan(&m.ID, &m.Name, &m.Email, &m.Body, &m.Created, &read, &followup); err != nil {
			serverError(w, err)
			return
		}
		m.IsRead = read != 0
		m.FollowupOptIn = followup != 0
		messages = append(messages, m)
	}
	if err = rows.Err(); err != nil {
		serverError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err = inboxTemplate.Execute(w, struct {
		Messages []message
		CSRF     string
	}{messages, csrf(token)}); err != nil {
		log.Printf("render inbox: %v", err)
	}
}
func (a *application) messageAction(w http.ResponseWriter, r *http.Request, query string) {
	noStore(w)
	if r.Method != http.MethodPost {
		methodNotAllowed(w, "POST")
		return
	}
	token, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	if !parseForm(w, r, 2<<10) {
		return
	}
	if !checkCSRF(r, token) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	id, err := strconv.ParseInt(r.PostForm.Get("id"), 10, 64)
	if err != nil || id < 1 {
		http.Error(w, "Invalid message", http.StatusBadRequest)
		return
	}
	if _, err = a.db.Exec(query, id); err != nil {
		serverError(w, err)
		return
	}
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}
func (a *application) setRead(w http.ResponseWriter, r *http.Request) {
	a.messageAction(w, r, "UPDATE contact_messages SET is_read = CASE is_read WHEN 0 THEN 1 ELSE 0 END WHERE id=?")
}
func (a *application) deleteMessage(w http.ResponseWriter, r *http.Request) {
	a.messageAction(w, r, "DELETE FROM contact_messages WHERE id=?")
}

var loginTemplate = template.Must(template.New("login").Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="robots" content="noindex,nofollow"><title>Admin sign in | England Software</title><link rel="stylesheet" href="/static/admin.css"></head><body><main class="login-panel"><img src="/static/logo.svg" alt="" width="64" height="74"><h1>Admin sign in</h1>{{if .}}<p class="error" role="alert">{{.}}</p>{{end}}<form method="post" action="/admin/login"><label>Username<input name="username" autocomplete="username" required></label><label>Password<input name="password" type="password" autocomplete="current-password" required></label><div class="trap" aria-hidden="true"><label>Website<input name="website" tabindex="-1" autocomplete="off"></label></div><button type="submit">Sign in</button></form></main></body></html>`))
var inboxTemplate = template.Must(template.New("inbox").Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="robots" content="noindex,nofollow"><title>Messages | England Software</title><link rel="stylesheet" href="/static/admin.css"></head><body><main class="inbox"><header><div><p class="overline">England Software / Admin</p><h1>Messages</h1></div><form method="post" action="/admin/logout"><input type="hidden" name="csrf" value="{{.CSRF}}"><button class="quiet" type="submit">Sign out</button></form></header>{{if .Messages}}{{range .Messages}}<article class="message {{if not .IsRead}}unread{{end}}"><div class="message-top"><div><span class="status">{{if .IsRead}}Read{{else}}New{{end}}</span><h2>{{.Name}}</h2><a href="mailto:{{.Email}}">{{.Email}}</a></div><time>{{.Created}}</time></div><p class="body">{{.Body}}</p><p class="followup-status">Consulting/application email follow-up: {{if .FollowupOptIn}}Yes, opted in{{else}}No opt-in{{end}}</p><div class="actions"><form method="post" action="/admin/messages/read"><input type="hidden" name="csrf" value="{{$.CSRF}}"><input type="hidden" name="id" value="{{.ID}}"><button class="quiet" type="submit">{{if .IsRead}}Mark unread{{else}}Mark read{{end}}</button></form><form method="post" action="/admin/messages/delete" onsubmit="return confirm('Delete this message?')"><input type="hidden" name="csrf" value="{{$.CSRF}}"><input type="hidden" name="id" value="{{.ID}}"><button class="danger" type="submit">Delete</button></form></div></article>{{end}}{{else}}<p class="empty">No messages yet.</p>{{end}}</main></body></html>`))

func renderLogin(w http.ResponseWriter, problem string, status int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := loginTemplate.Execute(w, problem); err != nil {
		log.Printf("render login: %v", err)
	}
}
