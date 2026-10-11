package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

const maxArticleUpload = 18 << 20
const articleMarker = "<!-- RECENT_ARTICLES -->"

type article struct {
	ID                                   int64
	Title, Body, Image, Created, Updated string
	HTML                                 template.HTML
}

func (a article) Date() string {
	t, err := time.Parse(timeFormat, a.Created)
	if err != nil {
		return a.Created
	}
	return t.Format("January 2, 2006")
}

func (a *application) imageDir() string {
	return filepath.Join(a.articleDir, "images")
}

func imageURL(name string) string { return "/article-images/" + name }
func articleURL(id int64) string  { return fmt.Sprintf("/articles/%d", id) }

var markdown = goldmark.New(goldmark.WithExtensions(extension.GFM))

func renderMarkdown(source string) (template.HTML, error) {
	var out bytes.Buffer
	if err := markdown.Convert([]byte(source), &out); err != nil {
		return "", err
	}
	return template.HTML(out.String()), nil
}

func (a *application) home(w http.ResponseWriter, r *http.Request) {
	items, err := a.recentArticles(4)
	if err != nil {
		serverError(w, err)
		return
	}
	raw, err := staticFiles.ReadFile("static/index.html")
	if err != nil {
		serverError(w, err)
		return
	}
	page := string(raw)
	if len(items) > 0 {
		var cards bytes.Buffer
		if err := recentTemplate.Execute(&cards, items); err != nil {
			serverError(w, err)
			return
		}
		page = strings.Replace(page, articleMarker, cards.String(), 1)
	}
	for _, block := range []string{"ARTICLES_NAV", "ARTICLES_SECTION"} {
		page = conditionalHTML(page, block, len(items) > 0)
	}
	page = strings.Replace(page, `<span class="progress-label">01 / 07</span>`, fmt.Sprintf(`<span class="progress-label">01 / %02d</span>`, strings.Count(page, `class="chapter `)), 1)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(page))
}

func conditionalHTML(page, name string, show bool) string {
	start := "<!-- " + name + " -->"
	end := "<!-- /" + name + " -->"
	for {
		from := strings.Index(page, start)
		if from < 0 {
			return page
		}
		to := strings.Index(page[from+len(start):], end)
		if to < 0 {
			return page
		}
		to += from + len(start)
		content := ""
		if show {
			content = page[from+len(start) : to]
		}
		page = page[:from] + content + page[to+len(end):]
	}
}

func (a *application) publicArticle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		methodNotAllowed(w, "GET, HEAD")
		return
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/articles/"), 10, 64)
	if err != nil || id < 1 {
		http.NotFound(w, r)
		return
	}
	item, err := a.getArticle(id)
	if errors.Is(err, os.ErrNotExist) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	item.HTML, err = renderMarkdown(item.Body)
	if err != nil {
		serverError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := publicArticleTemplate.Execute(w, item); err != nil {
		log.Printf("render article: %v", err)
	}
}

func (a *application) articleImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		methodNotAllowed(w, "GET, HEAD")
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/article-images/")
	if !validArticleImage(name) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeFile(w, r, filepath.Join(a.imageDir(), name))
}

func validArticleImage(name string) bool {
	return len(name) == 48 && strings.HasSuffix(name, ".webp") && validImageName(name[:43])
}

func validImageName(name string) bool {
	for i := 0; i < len(name); i++ {
		c := name[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func (a *application) adminArticles(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	token, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		items, err := a.recentArticles(-1)
		if err != nil {
			serverError(w, err)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := articleListTemplate.Execute(w, struct {
			Items []article
			CSRF  string
		}{items, csrf(token)}); err != nil {
			log.Printf("render articles: %v", err)
		}
	case http.MethodPost:
		a.saveArticle(w, r, token, 0)
	default:
		methodNotAllowed(w, "GET, POST")
	}
}

func (a *application) adminArticleAction(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	token, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/admin/articles/")
	if path == "new" && r.Method == http.MethodGet {
		a.articleForm(w, article{}, csrf(token), "", http.StatusOK)
		return
	}
	parts := strings.Split(path, "/")
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id < 1 {
		http.NotFound(w, r)
		return
	}
	switch parts[1] {
	case "edit":
		if r.Method != http.MethodGet {
			methodNotAllowed(w, "GET")
			return
		}
		item, err := a.getArticle(id)
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			serverError(w, err)
			return
		}
		a.articleForm(w, item, csrf(token), "", http.StatusOK)
	case "save":
		if r.Method != http.MethodPost {
			methodNotAllowed(w, "POST")
			return
		}
		a.saveArticle(w, r, token, id)
	case "delete":
		if r.Method != http.MethodPost {
			methodNotAllowed(w, "POST")
			return
		}
		if !parseForm(w, r, 2<<10) {
			return
		}
		if !checkCSRF(r, token) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		item, err := a.getArticle(id)
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			serverError(w, err)
			return
		}
		a.articleMu.Lock()
		err = os.Remove(a.articlePath(id))
		a.articleMu.Unlock()
		if err != nil {
			serverError(w, err)
			return
		}
		_ = os.Remove(filepath.Join(a.imageDir(), item.Image))
		http.Redirect(w, r, "/admin/articles", http.StatusSeeOther)
	default:
		http.NotFound(w, r)
	}
}

func (a *application) articleForm(w http.ResponseWriter, item article, token, problem string, status int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := articleFormTemplate.Execute(w, struct {
		Item        article
		CSRF, Error string
		New         bool
	}{item, token, problem, item.ID == 0}); err != nil {
		log.Printf("render article form: %v", err)
	}
}

func (a *application) saveArticle(w http.ResponseWriter, r *http.Request, token string, id int64) {
	r.Body = http.MaxBytesReader(w, r.Body, maxArticleUpload)
	if err := r.ParseMultipartForm(maxArticleUpload); err != nil {
		http.Error(w, "Upload must be under 18 MB.", http.StatusRequestEntityTooLarge)
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	if !checkCSRF(r, token) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	title := strings.TrimSpace(r.PostForm.Get("title"))
	body := strings.TrimSpace(r.PostForm.Get("body"))
	item := article{ID: id, Title: title, Body: body}
	var old article
	var err error
	if id != 0 {
		old, err = a.getArticle(id)
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			serverError(w, err)
			return
		}
		item.Image = old.Image
	}
	if title == "" || len(title) > 200 || body == "" || len(body) > 100000 {
		a.articleForm(w, item, csrf(token), "Enter a title (up to 200 characters) and Markdown content (up to 100,000 characters).", http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("image")
	if err != nil && !errors.Is(err, http.ErrMissingFile) {
		http.Error(w, "Invalid image upload.", http.StatusBadRequest)
		return
	}
	if errors.Is(err, http.ErrMissingFile) {
		err = nil
	}
	if id == 0 && header == nil {
		a.articleForm(w, item, csrf(token), "Choose an image for the article.", http.StatusBadRequest)
		return
	}
	if header != nil {
		defer file.Close()
		name, err := a.optimizeArticleImage(r.Context(), file)
		if err != nil {
			log.Printf("article image processing: %v", err)
			a.articleForm(w, item, csrf(token), "The image could not be processed. Try a JPG, PNG, WebP, HEIC, or GIF under 18 MB.", http.StatusBadRequest)
			return
		}
		item.Image = name
	}
	now := stamp(time.Now().UTC())
	a.articleMu.Lock()
	if id == 0 {
		item.ID, err = a.nextArticleID()
		item.Created = now
	} else {
		item.Created = old.Created
	}
	item.Updated = now
	if err == nil {
		err = a.writeArticle(item)
	}
	a.articleMu.Unlock()
	if err != nil {
		if item.Image != old.Image {
			_ = os.Remove(filepath.Join(a.imageDir(), item.Image))
		}
		serverError(w, err)
		return
	}
	if old.Image != "" && old.Image != item.Image {
		_ = os.Remove(filepath.Join(a.imageDir(), old.Image))
	}
	http.Redirect(w, r, "/admin/articles", http.StatusSeeOther)
}

func (a *application) optimizeArticleImage(parent context.Context, input io.Reader) (string, error) {
	if err := os.MkdirAll(a.imageDir(), 0700); err != nil {
		return "", err
	}
	source, err := os.CreateTemp(a.imageDir(), "source-*")
	if err != nil {
		return "", err
	}
	sourceName := source.Name()
	defer os.Remove(sourceName)
	if _, err = io.Copy(source, input); err != nil {
		source.Close()
		return "", err
	}
	if err = source.Close(); err != nil {
		return "", err
	}
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	// A fresh random filename makes replacements cache-safe and prevents user names from reaching the filesystem.
	name := fmt.Sprintf("%s.webp", token[:43])
	output := filepath.Join(a.imageDir(), name)
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-nostdin", "-threads", "2", "-i", sourceName, "-map", "0:v:0", "-frames:v", "1", "-vf", "scale=w='min(1600,iw)':h='min(1200,ih)':force_original_aspect_ratio=decrease,format=yuv420p", "-c:v", "libwebp", "-quality", "72", "-compression_level", "6", "-an", "-y", output)
	result, err := cmd.CombinedOutput()
	if err != nil {
		_ = os.Remove(output)
		return "", fmt.Errorf("ffmpeg: %w: %s", err, string(result))
	}
	info, err := os.Stat(output)
	if err != nil {
		return "", err
	}
	if info.Size() == 0 || info.Size() > 4<<20 {
		_ = os.Remove(output)
		return "", fmt.Errorf("processed image size %d", info.Size())
	}
	return name, nil
}

var recentTemplate = template.Must(template.New("recent").Funcs(template.FuncMap{"imageURL": imageURL, "articleURL": articleURL}).Parse(`{{if .}}<div class="article-grid">{{range .}}<a class="article-card" href="{{articleURL .ID}}"><img src="{{imageURL .Image}}" alt="" loading="lazy" decoding="async" width="640" height="420"><span class="article-card-copy"><small>{{.Date}}</small><strong>{{.Title}}</strong><span>Read article ↗</span></span></a>{{end}}</div>{{else}}<p class="article-empty">New articles will appear here soon.</p>{{end}}`))
var articleListTemplate = template.Must(template.New("article-list").Funcs(template.FuncMap{"imageURL": imageURL, "articleURL": articleURL}).Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="robots" content="noindex,nofollow"><title>Articles | England Software</title><link rel="stylesheet" href="/static/admin.css"></head><body><main class="inbox"><header><div><p class="overline">England Software / Admin</p><h1>Articles</h1></div><div class="admin-header-actions"><a class="admin-link" href="/admin">Messages</a><a class="admin-link primary" href="/admin/articles/new">New article</a></div></header>{{if .Items}}{{range .Items}}<article class="message article-admin-row"><img src="{{imageURL .Image}}" alt="" width="120" height="80"><div><h2>{{.Title}}</h2><p class="count">{{.Created}}</p><div class="actions"><a class="admin-link" href="{{articleURL .ID}}" target="_blank" rel="noopener">View</a><a class="admin-link" href="/admin/articles/{{.ID}}/edit">Edit</a><form method="post" action="/admin/articles/{{.ID}}/delete" onsubmit="return confirm('Delete this article?')"><input type="hidden" name="csrf" value="{{$.CSRF}}"><button class="danger" type="submit">Delete</button></form></div></div></article>{{end}}{{else}}<p class="empty">No articles yet. Write the first one.</p>{{end}}</main></body></html>`))
var articleFormTemplate = template.Must(template.New("article-form").Funcs(template.FuncMap{"imageURL": imageURL}).Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="robots" content="noindex,nofollow"><title>{{if .New}}New{{else}}Edit{{end}} article | England Software</title><link rel="stylesheet" href="/static/admin.css"></head><body><main class="inbox"><header><div><p class="overline">England Software / Articles</p><h1>{{if .New}}New article{{else}}Edit article{{end}}</h1></div><a class="admin-link" href="/admin/articles">All articles</a></header>{{if .Error}}<p class="error" role="alert">{{.Error}}</p>{{end}}<form class="article-editor" action="{{if .New}}/admin/articles{{else}}/admin/articles/{{.Item.ID}}/save{{end}}" method="post" enctype="multipart/form-data"><input type="hidden" name="csrf" value="{{.CSRF}}"><label>Title<input name="title" value="{{.Item.Title}}" maxlength="200" required></label><label>Image{{if not .New}} <span class="optional">Leave blank to keep the current image</span>{{end}}<input type="file" name="image" accept="image/*" {{if .New}}required{{end}}></label>{{if .Item.Image}}<img class="current-image" src="{{imageURL .Item.Image}}" alt="Current article image">{{end}}<label>Content <span class="optional">Markdown</span><textarea name="body" rows="20" maxlength="100000" required spellcheck="true">{{.Item.Body}}</textarea></label><p class="editor-help">Use Markdown for headings, links, lists, and emphasis. Save to publish immediately.</p><div class="actions"><button type="submit">Save article</button><a class="admin-link" href="/admin/articles">Cancel</a></div></form></main></body></html>`))
var publicArticleTemplate = template.Must(template.New("public-article").Funcs(template.FuncMap{"imageURL": imageURL}).Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="theme-color" content="#142849"><meta name="description" content="{{.Title}} — an article from England Software."><title>{{.Title}} | England Software</title><link rel="stylesheet" href="/static/styles.css"><link rel="stylesheet" href="/static/article.css"></head><body class="article-page"><header class="site-header"><a class="brand" href="/" aria-label="England Software home"><img src="/static/logo.svg" alt="" width="38" height="44"><span>ENGLAND <strong>SOFTWARE</strong></span></a><nav class="site-nav" aria-label="Main navigation"><a href="/#articles">Articles</a><a href="/#contact">Contact</a></nav></header><main class="article-main"><a class="article-back" href="/#articles">← Back to articles</a><article><p class="eyebrow">ENGLAND SOFTWARE / ARTICLE</p><h1>{{.Title}}</h1><p class="article-date"><time datetime="{{.Created}}">{{.Date}}</time></p><img class="article-cover" src="{{imageURL .Image}}" alt="" width="1600" height="1200"><div class="article-prose">{{.HTML}}</div></article><a class="article-back" href="/#articles">← More articles</a></main><footer class="site-footer"><span>ENGLAND SOFTWARE · TULSA, OKLAHOMA</span><a href="/#contact">Get in touch ↗</a></footer></body></html>`))
