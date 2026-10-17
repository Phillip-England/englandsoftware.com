package main

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func testApp(t *testing.T) *application {
	t.Helper()
	a, err := newApp(filepath.Join(t.TempDir(), "main.sqlite"), appConfig{username: "admin", password: "correct-password"})
	if err != nil {
		t.Fatal(err)
	}
	a.articleDir = filepath.Join(t.TempDir(), "articles")
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
	a.email = emailSettings{host: gateway.URL, key: "test-key", recipient: "owner@example.com", client: gateway.Client()}
	t.Cleanup(gateway.Close)
	t.Cleanup(func() { a.db.Close() })
	return a
}
func request(t *testing.T, a *application, method, path string, form url.Values, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var body *strings.Reader
	if form == nil {
		body = strings.NewReader("")
	} else {
		body = strings.NewReader(form.Encode())
	}
	r := httptest.NewRequest(method, path, body)
	r.RemoteAddr = "192.0.2.5:5555"
	if form != nil {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	a.handler().ServeHTTP(w, r)
	return w
}

func TestHeroImageIsServed(t *testing.T) {
	mux := http.NewServeMux()
	publicPages(mux)
	req := httptest.NewRequest(http.MethodGet, "/static/images/tulsa-skyline-hero.webp", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("hero image status: %d", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "image/webp" {
		t.Errorf("hero image content type: %q", got)
	}
	if w.Body.Len() == 0 {
		t.Error("hero image is empty")
	}
}

func TestSearchEndpoints(t *testing.T) {
	mux := http.NewServeMux()
	publicPages(mux)
	for path, expected := range map[string][2]string{
		"/robots.txt":  {"text/plain", "Sitemap: https://englandsoftware.com/sitemap.xml"},
		"/sitemap.xml": {"application/xml", "https://englandsoftware.com/"},
	} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusOK || !strings.HasPrefix(w.Header().Get("Content-Type"), expected[0]) || !strings.Contains(w.Body.String(), expected[1]) {
			t.Errorf("%s: status %d, content type %q, body %q", path, w.Code, w.Header().Get("Content-Type"), w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	for _, want := range []string{"Tulsa Web Design", "application/ld+json", "og:image", "tulsa-skyline-canvas", "TULSA, OKLAHOMA"} {
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("home page missing %q", want)
		}
	}
}

func TestPagesAndContact(t *testing.T) {
	a := testApp(t)
	for _, path := range []string{"/", "/static/styles.css", "/static/admin.css", "/admin/login"} {
		if got := request(t, a, "GET", path, nil, nil).Code; got != 200 {
			t.Errorf("GET %s: %d", path, got)
		}
	}
	if got := request(t, a, "GET", "/contract-engineering", nil, nil); got.Code != http.StatusMovedPermanently || got.Header().Get("Location") != "/#consulting" {
		t.Errorf("contract engineering redirect: status %d, location %q", got.Code, got.Header().Get("Location"))
	}
	for path, destination := range map[string]string{"/websites": "/#websites", "/managed-hosting": "/#websites", "/web-applications": "/#applications"} {
		if got := request(t, a, "GET", path, nil, nil); got.Code != http.StatusMovedPermanently || got.Header().Get("Location") != destination {
			t.Errorf("%s redirect: status %d, location %q", path, got.Code, got.Header().Get("Location"))
		}
	}
	if got := request(t, a, "GET", "/unknown", nil, nil).Code; got != 404 {
		t.Errorf("unknown: %d", got)
	}
	if got := request(t, a, "POST", "/", nil, nil).Code; got != 405 {
		t.Errorf("POST /: %d", got)
	}
	if got := request(t, a, "GET", "/admin", nil, nil).Code; got != 303 {
		t.Errorf("unauthenticated admin: %d", got)
	}
	form := url.Values{"name": {"Alex"}, "email": {"alex@example.com"}, "message": {"Hello <script>alert(1)</script>"}}
	if got := request(t, a, "POST", "/contact", form, nil).Code; got != 303 {
		t.Fatalf("contact: %d", got)
	}
	var count int
	if err := a.db.QueryRow("SELECT COUNT(*) FROM contact_messages").Scan(&count); err != nil || count != 1 {
		t.Fatalf("messages: %d, %v", count, err)
	}
	var optedIn int
	if err := a.db.QueryRow("SELECT followup_opt_in FROM contact_messages WHERE id=1").Scan(&optedIn); err != nil || optedIn != 0 {
		t.Fatalf("default follow-up preference: %d, %v", optedIn, err)
	}
	form.Set("followup_opt_in", "yes")
	if got := request(t, a, "POST", "/contact", form, nil).Code; got != 303 {
		t.Fatalf("opted-in contact: %d", got)
	}
	if err := a.db.QueryRow("SELECT followup_opt_in FROM contact_messages WHERE id=2").Scan(&optedIn); err != nil || optedIn != 1 {
		t.Fatalf("opted-in follow-up preference: %d, %v", optedIn, err)
	}
	form.Set("website", "bot")
	request(t, a, "POST", "/contact", form, nil)
	if err := a.db.QueryRow("SELECT COUNT(*) FROM contact_messages").Scan(&count); err != nil || count != 2 {
		t.Fatalf("honeypot stored message: %d, %v", count, err)
	}
}

func TestPublicOffersMatchBusinessBrief(t *testing.T) {
	a := testApp(t)
	checks := []struct {
		path  string
		wants []string
	}{
		{"/", []string{"$50 upfront to get started", "$50/month after the first month", "Your upfront payment covers the first month", "contact form", "BUSINESS WEBSITES", "WEB APPLICATIONS", "SOFTWARE CONSULTING", "Invoicing", "id=\"contact\""}},
	}
	for _, check := range checks {
		body := request(t, a, http.MethodGet, check.path, nil, nil).Body.String()
		for _, want := range check.wants {
			if !strings.Contains(body, want) {
				t.Errorf("%s missing %q", check.path, want)
			}
		}
		if strings.Contains(strings.ToLower(body), "contract engineering") {
			t.Errorf("%s still advertises contract engineering", check.path)
		}
		for _, oldPrice := range []string{"$300", "$200", "$80 per hour"} {
			if strings.Contains(body, oldPrice) {
				t.Errorf("%s still advertises old price %q", check.path, oldPrice)
			}
		}
		if !strings.Contains(body, "https://www.facebook.com/profile.php?id=61594819109541") {
			t.Errorf("%s missing Facebook link", check.path)
		}
	}
}

func TestContactInboxCapacity(t *testing.T) {
	a := testApp(t)
	a.cfg.messageCapacity = 3
	form := url.Values{"name": {"Alex"}, "email": {"alex@example.com"}}
	form.Set("website", "bot")
	form.Set("message", "ignored")
	if got := request(t, a, http.MethodPost, "/contact", form, nil).Code; got != http.StatusSeeOther {
		t.Fatalf("honeypot: %d", got)
	}
	form.Del("website")
	for i := 1; i <= 4; i++ {
		form.Set("message", strconv.Itoa(i))
		if got := request(t, a, http.MethodPost, "/contact", form, nil).Code; got != http.StatusSeeOther {
			t.Fatalf("submission %d: %d", i, got)
		}
	}
	var count, minID, maxID int
	if err := a.db.QueryRow("SELECT COUNT(*),MIN(id),MAX(id) FROM contact_messages").Scan(&count, &minID, &maxID); err != nil || count != 3 || minID != 2 || maxID != 4 {
		t.Fatalf("capacity: count=%d min=%d max=%d err=%v", count, minID, maxID, err)
	}
}
func TestInboxCapacityAppliesAtStartup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "main.sqlite")
	a, err := newApp(path, appConfig{username: "admin", password: "correct-password", messageCapacity: 5})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if _, err := a.db.Exec("INSERT INTO contact_messages(name,email,message,created_at) VALUES('Alex','alex@example.com','Hi',?)", stamp(time.Now())); err != nil {
			t.Fatal(err)
		}
	}
	a.db.Close()
	a, err = newApp(path, appConfig{username: "admin", password: "correct-password", messageCapacity: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer a.db.Close()
	var count, first int
	if err := a.db.QueryRow("SELECT COUNT(*),MIN(id) FROM contact_messages").Scan(&count, &first); err != nil || count != 2 || first != 4 {
		t.Fatalf("startup capacity: count=%d first=%d err=%v", count, first, err)
	}
}
func TestLoginHoneypotLockoutAndExpiry(t *testing.T) {
	a := testApp(t)
	wrong := url.Values{"username": {"admin"}, "password": {"wrong"}}
	wrong.Set("website", "filled by bot")
	if got := request(t, a, "POST", "/admin/login", wrong, nil).Code; got != 401 {
		t.Fatalf("honeypot: %d", got)
	}
	wrong.Del("website")
	for i := 0; i < 4; i++ {
		if got := request(t, a, "POST", "/admin/login", wrong, nil).Code; got != 401 {
			t.Fatalf("failure %d: %d", i, got)
		}
	}
	var failures int
	if err := a.db.QueryRow("SELECT COUNT(*) FROM login_failures").Scan(&failures); err != nil || failures != 5 {
		t.Fatalf("failures: %d, %v", failures, err)
	}
	correct := url.Values{"username": {"admin"}, "password": {"correct-password"}}
	if got := request(t, a, "POST", "/admin/login", correct, nil).Code; got != 429 {
		t.Fatalf("jailed login: %d", got)
	}
	if _, err := a.db.Exec("UPDATE banned_ips SET banned_until=?", stamp(time.Now().UTC().Add(-time.Second))); err != nil {
		t.Fatal(err)
	}
	if _, err := a.db.Exec("UPDATE login_failures SET created_at=?", stamp(time.Now().UTC().Add(-25*time.Hour))); err != nil {
		t.Fatal(err)
	}
	response := request(t, a, "POST", "/admin/login", correct, nil)
	if response.Code != 303 || len(response.Result().Cookies()) == 0 {
		t.Fatalf("expired jail login: %d", response.Code)
	}
	if err := a.db.QueryRow("SELECT COUNT(*) FROM login_failures").Scan(&failures); err != nil || failures != 0 {
		t.Fatalf("expired failures: %d, %v", failures, err)
	}
}
func TestAdminInboxActions(t *testing.T) {
	a := testApp(t)
	form := url.Values{"name": {"Alex"}, "email": {"alex@example.com"}, "message": {"Hello <script>alert(1)</script>"}}
	request(t, a, "POST", "/contact", form, nil)
	response := request(t, a, "POST", "/admin/login", url.Values{"username": {"admin"}, "password": {"correct-password"}}, nil)
	cookie := response.Result().Cookies()[0]
	inbox := request(t, a, "GET", "/admin", nil, cookie)
	if inbox.Code != 200 || !strings.Contains(inbox.Body.String(), "Hello &lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Fatalf("inbox status or escaping: %d", inbox.Code)
	}
	if got := request(t, a, "POST", "/admin/messages/delete", url.Values{"id": {"1"}}, cookie).Code; got != 403 {
		t.Fatalf("missing csrf: %d", got)
	}
	action := url.Values{"id": {"1"}, "csrf": {csrf(cookie.Value)}}
	if got := request(t, a, "POST", "/admin/messages/read", action, cookie).Code; got != 303 {
		t.Fatalf("mark read: %d", got)
	}
	var read int
	if err := a.db.QueryRow("SELECT is_read FROM contact_messages WHERE id=1").Scan(&read); err != nil || read != 1 {
		t.Fatalf("read state: %d, %v", read, err)
	}
	if got := request(t, a, "POST", "/admin/messages/delete", action, cookie).Code; got != 303 {
		t.Fatalf("delete: %d", got)
	}
	var count int
	if err := a.db.QueryRow("SELECT COUNT(*) FROM contact_messages").Scan(&count); err != nil || count != 0 {
		t.Fatalf("delete state: %d, %v", count, err)
	}
	if got := request(t, a, "POST", "/admin/logout", url.Values{"csrf": {csrf(cookie.Value)}}, cookie).Code; got != 303 {
		t.Fatalf("logout: %d", got)
	}
	if got := request(t, a, "GET", "/admin", nil, cookie).Code; got != 303 {
		t.Fatalf("old session: %d", got)
	}
}
func TestExistingMessageTableMigrates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "main.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE contact_messages (id INTEGER PRIMARY KEY AUTOINCREMENT,name TEXT NOT NULL,email TEXT NOT NULL,phone TEXT NOT NULL DEFAULT '',message TEXT NOT NULL,created_at DATETIME NOT NULL)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec("INSERT INTO contact_messages(name,email,message,created_at) VALUES('Old','old@example.com','Preserved',?)", stamp(time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	a, err := newApp(path, appConfig{username: "admin", password: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer a.db.Close()
	var body string
	var read int
	var optedIn int
	if err := a.db.QueryRow("SELECT message,is_read,followup_opt_in FROM contact_messages WHERE id=1").Scan(&body, &read, &optedIn); err != nil || body != "Preserved" || read != 0 || optedIn != 0 {
		t.Fatalf("migration: %q %d %v", body, read, err)
	}
}

func TestForwardedIPRequiresTrustedProxy(t *testing.T) {
	a := testApp(t)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	r.Header.Set("X-Forwarded-For", "198.51.100.7")
	if got := a.clientIP(r); got != "127.0.0.1" {
		t.Fatalf("untrusted proxy: %s", got)
	}
	a.cfg.trustedProxies = []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}
	if got := a.clientIP(r); got != "198.51.100.7" {
		t.Fatalf("trusted proxy: %s", got)
	}
}
