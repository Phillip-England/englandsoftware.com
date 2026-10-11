package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func articleAdminCookie(t *testing.T, a *application) *http.Cookie {
	t.Helper()
	token := "test-article-session"
	if _, err := a.db.Exec("INSERT INTO admin_sessions(token_hash,expires_at) VALUES(?,?)", tokenHash(token), stamp(time.Now().Add(time.Hour))); err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: sessionCookie, Value: token}
}
func articlePost(t *testing.T, a *application, cookie *http.Cookie, path, title, body string, withImage bool, validCSRF bool) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	token := csrf(cookie.Value)
	if !validCSRF {
		token = "bad"
	}
	_ = writer.WriteField("csrf", token)
	_ = writer.WriteField("title", title)
	_ = writer.WriteField("body", body)
	if withImage {
		part, err := writer.CreateFormFile("image", "rough.png")
		if err != nil {
			t.Fatal(err)
		}
		pic := image.NewRGBA(image.Rect(0, 0, 30, 20))
		for y := 0; y < 20; y++ {
			for x := 0; x < 30; x++ {
				pic.Set(x, y, color.RGBA{R: 230, G: uint8(x * 7), B: uint8(y * 10), A: 255})
			}
		}
		if err := png.Encode(part, pic); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	a.handler().ServeHTTP(w, req)
	return w
}

func TestArticlesPublishEditDeleteAndRecentFour(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg unavailable")
	}
	a := testApp(t)
	cookie := articleAdminCookie(t, a)
	if got := request(t, a, "GET", "/admin/articles", nil, nil).Code; got != http.StatusSeeOther {
		t.Fatalf("unauthenticated articles: %d", got)
	}
	if got := articlePost(t, a, cookie, "/admin/articles", "No CSRF", "content", true, false).Code; got != http.StatusForbidden {
		t.Fatalf("CSRF: %d", got)
	}
	if got := articlePost(t, a, cookie, "/admin/articles", "No image", "content", false, true).Code; got != http.StatusBadRequest {
		t.Fatalf("missing image: %d", got)
	}
	for i := 1; i <= 5; i++ {
		body := "## Heading\n\nHello **world**."
		if i == 1 {
			body += "\n\n<script>alert(1)</script>\n\n[bad](javascript:alert(1))"
		}
		w := articlePost(t, a, cookie, "/admin/articles", "Article "+strconv.Itoa(i), body, true, true)
		if w.Code != http.StatusSeeOther {
			t.Fatalf("create %d: %d %s", i, w.Code, w.Body.String())
		}
	}
	items, err := a.recentArticles(5)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 5 || items[0].Title != "Article 5" {
		t.Fatalf("articles: %+v", items)
	}
	imageName := items[0].Image
	markdownFile, err := os.ReadFile(a.articlePath(items[0].ID))
	if err != nil || !strings.Contains(string(markdownFile), "title: Article 5") || !strings.Contains(string(markdownFile), "created_at:") || !strings.Contains(string(markdownFile), "## Heading") {
		t.Fatalf("Markdown article with metadata: %s %v", markdownFile, err)
	}
	imageResponse := request(t, a, "GET", imageURL(imageName), nil, nil)
	if imageResponse.Code != http.StatusOK || imageResponse.Header().Get("Content-Type") != "image/webp" || imageResponse.Body.Len() == 0 {
		t.Fatalf("image response: %d %q %d", imageResponse.Code, imageResponse.Header().Get("Content-Type"), imageResponse.Body.Len())
	}
	if info, err := os.Stat(filepath.Join(a.imageDir(), imageName)); err != nil || info.Size() > 4<<20 {
		t.Fatalf("image file: %v %v", info, err)
	}
	home := request(t, a, "GET", "/", nil, nil).Body.String()
	if strings.Contains(home, "Article 1") || !strings.Contains(home, "Article 5") || !strings.Contains(home, "Article 2") || !strings.Contains(home, "chapter-articles") {
		t.Fatal("home does not show latest four")
	}
	detail := request(t, a, "GET", "/articles/1", nil, nil).Body.String()
	if !strings.Contains(detail, "<h2>Heading</h2>") || !strings.Contains(detail, "<strong>world</strong>") || strings.Contains(detail, "<script>") || strings.Contains(detail, "href=\"javascript:") {
		t.Fatalf("unsafe or missing markdown: %s", detail)
	}
	edit := articlePost(t, a, cookie, "/admin/articles/5/save", "Revised title", "Updated *content*", false, true)
	if edit.Code != http.StatusSeeOther {
		t.Fatalf("edit: %d %s", edit.Code, edit.Body.String())
	}
	revised, err := a.getArticle(5)
	if err != nil || revised.Image != imageName || revised.Title != "Revised title" {
		t.Fatalf("revised: %+v %v", revised, err)
	}
	if got := articlePost(t, a, cookie, "/admin/articles/5/save", "Revised title", "Updated *content*", true, true).Code; got != http.StatusSeeOther {
		t.Fatalf("replace image: %d", got)
	}
	revised, err = a.getArticle(5)
	if err != nil || revised.Image == imageName {
		t.Fatalf("replacement image: %+v %v", revised, err)
	}
	if _, err := os.Stat(filepath.Join(a.imageDir(), imageName)); !os.IsNotExist(err) {
		t.Fatalf("old image remains after replacement: %v", err)
	}
	imageName = revised.Image
	deleteForm := strings.NewReader("csrf=" + csrf(cookie.Value))
	req := httptest.NewRequest(http.MethodPost, "/admin/articles/5/delete", deleteForm)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	a.handler().ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("delete: %d", w.Code)
	}
	if _, err := os.Stat(filepath.Join(a.imageDir(), imageName)); !os.IsNotExist(err) {
		t.Fatalf("deleted image exists: %v", err)
	}
	if got := request(t, a, "GET", "/articles/5", nil, nil).Code; got != http.StatusNotFound {
		t.Fatalf("deleted article: %d", got)
	}
}

func TestHomeShowsArticlesOnlyWhenTheyExist(t *testing.T) {
	a := testApp(t)
	check := func(wantArticles bool, wantProgress string) {
		t.Helper()
		home := request(t, a, "GET", "/", nil, nil).Body.String()
		if got := strings.Contains(home, `id="articles"`); got != wantArticles {
			t.Fatalf("article section present = %t, want %t", got, wantArticles)
		}
		if got := strings.Count(home, `href="#articles"`); got != map[bool]int{false: 0, true: 2}[wantArticles] {
			t.Fatalf("article navigation links = %d", got)
		}
		if !strings.Contains(home, `<span class="progress-label">`+wantProgress+`</span>`) {
			t.Fatalf("wrong initial chapter count: %s", wantProgress)
		}
	}
	check(false, "01 / 06")
	if err := a.writeArticle(article{ID: 1, Title: "First article", Body: "Body", Image: strings.Repeat("a", 43) + ".webp", Created: stamp(time.Now()), Updated: stamp(time.Now())}); err != nil {
		t.Fatal(err)
	}
	check(true, "01 / 07")
	if err := os.Remove(a.articlePath(1)); err != nil {
		t.Fatal(err)
	}
	check(false, "01 / 06")
}

func TestLegacyArticlesExportToMarkdown(t *testing.T) {
	a := testApp(t)
	imageName := strings.Repeat("b", 43) + ".webp"
	oldImageDir := filepath.Join(filepath.Dir(a.dataPath), "article-images")
	if err := os.MkdirAll(oldImageDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldImageDir, imageName), []byte("old image"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := a.db.Exec("CREATE TABLE articles (id INTEGER PRIMARY KEY, title TEXT, body_markdown TEXT, image_name TEXT, created_at TEXT, updated_at TEXT)"); err != nil {
		t.Fatal(err)
	}
	date := stamp(time.Now().UTC())
	if _, err := a.db.Exec("INSERT INTO articles VALUES (?,?,?,?,?,?)", 7, "Legacy title", "## Old content", imageName, date, date); err != nil {
		t.Fatal(err)
	}
	if err := a.importLegacyArticles(); err != nil {
		t.Fatal(err)
	}
	item, err := a.getArticle(7)
	if err != nil || item.Title != "Legacy title" || item.Body != "## Old content" {
		t.Fatalf("exported article: %+v %v", item, err)
	}
	if image, err := os.ReadFile(filepath.Join(a.imageDir(), imageName)); err != nil || string(image) != "old image" {
		t.Fatalf("exported cover: %s %v", image, err)
	}
	var count int
	if err := a.db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='articles'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("legacy table still exists: %d %v", count, err)
	}
}
