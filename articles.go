package main

import (
	"bytes"
	"context"
	"encoding/base64"
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
const siteOrigin = "https://englandsoftware.com"

type article struct {
	ID                                   int64
	Title, Body, Image, Created, Updated string
	HTML                                 template.HTML
	CoverURL                             template.URL
	ShareURL, ShareImageURL              string
	Draft                                bool
	Preview                              bool
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

func imageURL(name string) string       { return "/article-images/" + name }
func articleURL(id int64) string        { return fmt.Sprintf("/articles/%d", id) }
func shareImageName(name string) string { return strings.TrimSuffix(name, ".webp") + ".jpg" }

func (a *application) removeArticleImage(name string) {
	_ = os.Remove(filepath.Join(a.imageDir(), name))
	_ = os.Remove(filepath.Join(a.imageDir(), shareImageName(name)))
}

var markdown = goldmark.New(goldmark.WithExtensions(extension.GFM))

func renderMarkdown(source string) (template.HTML, error) {
	var out bytes.Buffer
	if err := markdown.Convert([]byte(source), &out); err != nil {
		return "", err
	}
	return template.HTML(out.String()), nil
}

func (a *application) home(w http.ResponseWriter, r *http.Request) {
	items, err := a.publishedArticles(4)
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
	if item.Draft {
		http.NotFound(w, r)
		return
	}
	item.CoverURL = template.URL(imageURL(item.Image))
	item.ShareURL = siteOrigin + articleURL(item.ID)
	item.ShareImageURL = siteOrigin + imageURL(shareImageName(item.Image))
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
	if !validArticleImage(name) && !(strings.HasSuffix(name, ".jpg") && validArticleImage(strings.TrimSuffix(name, ".jpg")+".webp")) {
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
	if path == "preview" {
		if r.Method != http.MethodPost {
			methodNotAllowed(w, "POST")
			return
		}
		a.previewArticle(w, r, token)
		return
	}
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
		a.removeArticleImage(item.Image)
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
	switch r.PostForm.Get("status") {
	case "draft":
		item.Draft = true
	case "", "live":
	default:
		http.Error(w, "Invalid article status.", http.StatusBadRequest)
		return
	}
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
			a.removeArticleImage(item.Image)
		}
		serverError(w, err)
		return
	}
	if old.Image != "" && old.Image != item.Image {
		a.removeArticleImage(old.Image)
	}
	http.Redirect(w, r, "/admin/articles", http.StatusSeeOther)
}

func (a *application) previewArticle(w http.ResponseWriter, r *http.Request, token string) {
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
	item := article{Title: strings.TrimSpace(r.PostForm.Get("title")), Body: strings.TrimSpace(r.PostForm.Get("body")), Created: stamp(time.Now().UTC()), Preview: true}
	if len(item.Title) > 200 || len(item.Body) > 100000 {
		http.Error(w, "Article is too long.", http.StatusBadRequest)
		return
	}
	if item.Title == "" {
		item.Title = "Untitled article"
	}
	if id, err := strconv.ParseInt(r.PostForm.Get("id"), 10, 64); err == nil && id > 0 {
		saved, err := a.getArticle(id)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		item.Created = saved.Created
		item.Image = saved.Image
		item.CoverURL = template.URL(imageURL(saved.Image))
	}
	file, header, err := r.FormFile("image")
	if err != nil && !errors.Is(err, http.ErrMissingFile) {
		http.Error(w, "Invalid image upload.", http.StatusBadRequest)
		return
	}
	if header != nil {
		defer file.Close()
		name, err := a.optimizeArticleImage(r.Context(), file)
		if err != nil {
			log.Printf("article preview image processing: %v", err)
			http.Error(w, "The preview image could not be processed.", http.StatusBadRequest)
			return
		}
		imagePath := filepath.Join(a.imageDir(), name)
		data, readErr := os.ReadFile(imagePath)
		a.removeArticleImage(name)
		if readErr != nil {
			serverError(w, readErr)
			return
		}
		item.CoverURL = template.URL("data:image/webp;base64," + base64.StdEncoding.EncodeToString(data))
	}
	item.HTML, err = renderMarkdown(item.Body)
	if err != nil {
		serverError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := publicArticleTemplate.Execute(w, item); err != nil {
		log.Printf("render article preview: %v", err)
	}
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
	shareOutput := filepath.Join(a.imageDir(), shareImageName(name))
	cmd = exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-nostdin", "-threads", "2", "-i", output, "-frames:v", "1", "-q:v", "3", "-y", shareOutput)
	result, err = cmd.CombinedOutput()
	if err != nil {
		a.removeArticleImage(name)
		return "", fmt.Errorf("ffmpeg share image: %w: %s", err, string(result))
	}
	shareInfo, err := os.Stat(shareOutput)
	if err != nil || shareInfo.Size() == 0 || shareInfo.Size() > 4<<20 {
		a.removeArticleImage(name)
		return "", fmt.Errorf("processed share image: %v", err)
	}
	return name, nil
}

var recentTemplate = template.Must(template.New("recent").Funcs(template.FuncMap{"imageURL": imageURL, "articleURL": articleURL}).Parse(`{{if .}}<div class="article-grid">{{range .}}<a class="article-card" href="{{articleURL .ID}}"><span class="article-card-media"><img src="{{imageURL .Image}}" alt="" loading="lazy" decoding="async"></span><span class="article-card-copy"><small>{{.Date}}</small><strong>{{.Title}}</strong><span>Read article ↗</span></span></a>{{end}}</div>{{else}}<p class="article-empty">New articles will appear here soon.</p>{{end}}`))
var articleListTemplate = template.Must(template.New("article-list").Funcs(template.FuncMap{"imageURL": imageURL, "articleURL": articleURL}).Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="robots" content="noindex,nofollow"><title>Articles | England Software</title><link rel="stylesheet" href="/static/admin.css"></head><body><main class="inbox"><header><div><p class="overline">England Software / Admin</p><h1>Articles</h1></div><div class="admin-header-actions"><a class="admin-link" href="/admin">Messages</a><a class="admin-link primary" href="/admin/articles/new">New article</a></div></header>{{if .Items}}{{range .Items}}<article class="message article-admin-row"><img src="{{imageURL .Image}}" alt="" width="120" height="80"><div><h2>{{.Title}}</h2><p class="count"><span class="article-status {{if .Draft}}draft{{else}}live{{end}}">{{if .Draft}}Draft{{else}}Live{{end}}</span> · {{.Created}}</p><div class="actions">{{if not .Draft}}<a class="admin-link" href="{{articleURL .ID}}" target="_blank" rel="noopener">View</a>{{end}}<a class="admin-link" href="/admin/articles/{{.ID}}/edit">Edit</a><form method="post" action="/admin/articles/{{.ID}}/delete" onsubmit="return confirm('Delete this article?')"><input type="hidden" name="csrf" value="{{$.CSRF}}"><button class="danger" type="submit">Delete</button></form></div></div></article>{{end}}{{else}}<p class="empty">No articles yet. Write the first one.</p>{{end}}</main></body></html>`))
var articleFormTemplate = template.Must(template.New("article-form").Funcs(template.FuncMap{"imageURL": imageURL}).Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="robots" content="noindex,nofollow"><title>{{if .New}}New{{else}}Edit{{end}} article | England Software</title><link rel="stylesheet" href="/static/admin.css"></head><body><main class="inbox article-editor-page"><header><div><p class="overline">England Software / Articles</p><h1>{{if .New}}New article{{else}}Edit article{{end}}</h1></div><a class="admin-link" href="/admin/articles">All articles</a></header>{{if .Error}}<p class="error" role="alert">{{.Error}}</p>{{end}}<div class="article-editor-layout"><form class="article-editor" action="{{if .New}}/admin/articles{{else}}/admin/articles/{{.Item.ID}}/save{{end}}" method="post" enctype="multipart/form-data"><input type="hidden" name="csrf" value="{{.CSRF}}"><input type="hidden" name="id" value="{{.Item.ID}}"><label>Title<input name="title" value="{{.Item.Title}}" maxlength="200" required></label><label>Image{{if not .New}} <span class="optional">Leave blank to keep the current image</span>{{end}}<input type="file" name="image" accept="image/*" {{if .New}}required{{end}}></label>{{if .Item.Image}}<img class="current-image" src="{{imageURL .Item.Image}}" alt="Current article image">{{end}}<label>Publication status<select name="status"><option value="draft" {{if or .New .Item.Draft}}selected{{end}}>Draft — only admins can preview</option><option value="live" {{if and (not .New) (not .Item.Draft)}}selected{{end}}>Live — visible on the site</option></select></label><label>Content <span class="optional">Markdown</span><textarea name="body" rows="20" maxlength="100000" required spellcheck="true">{{.Item.Body}}</textarea></label><p class="editor-help">Use Markdown for headings, links, lists, and emphasis. Preview updates after you pause typing. Only Save article writes changes.</p><div class="actions"><button type="submit">Save article</button><button class="quiet" type="submit" formaction="/admin/articles/preview" formtarget="article-preview" formnovalidate data-preview-button>Update preview</button><a class="admin-link" href="/admin/articles">Cancel</a></div></form><section class="article-preview-section" aria-labelledby="preview-heading"><div class="preview-heading"><div><p class="overline">Preview</p><h2 id="preview-heading">How your article will look</h2></div><p>Preview updates automatically and does not save or publish.</p></div><iframe name="article-preview" title="Article preview" srcdoc="<p style='font:16px sans-serif;padding:32px;color:#526178'>Loading article preview…</p>"></iframe><p class="preview-status" role="status" aria-live="polite" data-preview-status></p></section></div></main><script src="/static/article-editor.js" defer></script></body></html>`))
var publicArticleTemplate = template.Must(template.New("public-article").Funcs(template.FuncMap{"imageURL": imageURL}).Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="theme-color" content="#142849">{{if .Preview}}<meta name="robots" content="noindex,nofollow">{{end}}<meta name="description" content="{{.Title}} — an article from England Software.">{{if not .Preview}}<link rel="canonical" href="{{.ShareURL}}"><meta property="og:type" content="article"><meta property="og:site_name" content="England Software"><meta property="og:title" content="{{.Title}}"><meta property="og:description" content="{{.Title}} — an article from England Software."><meta property="og:url" content="{{.ShareURL}}"><meta property="og:image" content="{{.ShareImageURL}}"><meta property="og:image:secure_url" content="{{.ShareImageURL}}"><meta property="og:image:type" content="image/jpeg"><meta property="og:image:alt" content="Cover image for {{.Title}}"><meta name="twitter:card" content="summary_large_image"><meta name="twitter:title" content="{{.Title}}"><meta name="twitter:description" content="{{.Title}} — an article from England Software."><meta name="twitter:image" content="{{.ShareImageURL}}">{{end}}<title>{{.Title}} | England Software</title><link rel="stylesheet" href="/static/styles.css"><link rel="stylesheet" href="/static/article.css"></head><body class="article-page"><a class="skip-link" href="#main">Skip to content</a><header class="site-header"><a class="brand" href="/#home" aria-label="England Software, back to the beginning"><img src="/static/logo.svg" alt="" width="38" height="44" /><span>ENGLAND <strong>SOFTWARE</strong></span></a><nav class="site-nav" aria-label="Explore the page"><a href="/#websites">Websites</a><a href="/#applications">Applications</a><a href="/#consulting">Consultation</a><a href="/#about">About</a><a href="/#articles">Articles</a></nav><a class="header-contact" href="tel:+19185008396" aria-label="Call 918-500-8396"><span class="header-phone-number">918-500-8396</span><svg class="header-phone-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M21 16.4v3a2 2 0 0 1-2.2 2A19 19 0 0 1 2.6 5.2 2 2 0 0 1 4.6 3h3a2 2 0 0 1 2 1.7l.5 3a2 2 0 0 1-.6 1.8L7.7 11a15 15 0 0 0 5.3 5.3l1.5-1.8a2 2 0 0 1 1.8-.6l3 .5a2 2 0 0 1 1.7 2Z"/></svg></a><a class="header-message" href="/#contact" aria-label="Go to contact form"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M20.5 11.5a8.5 8.5 0 0 1-8.5 8.5 8.7 8.7 0 0 1-4-.9L3 21l1.9-5a8.5 8.5 0 1 1 15.6-4.5Z"/><path d="M8 10h8M8 13.5h5"/></svg></a><details class="mobile-navigation"><summary>Menu</summary><nav aria-label="Explore the page"><a href="/#websites">Websites</a><a href="/#applications">Applications</a><a href="/#consulting">Consultation</a><a href="/#about">About</a><a href="/#articles">Articles</a><a href="/#contact">Contact</a></nav></details></header><a class="offer-reel" href="/#contact" aria-label="Click here for a $50/month business website. Go to the contact form."><span class="offer-reel-window" aria-hidden="true"><span class="offer-reel-track"><span class="offer-reel-group">Click here for a <span class="offer-reel-price"><mark>$50</mark><span>/month</span></span> business website <i>↗</i></span><span class="offer-reel-group">Click here for a <span class="offer-reel-price"><mark>$50</mark><span>/month</span></span> business website <i>↗</i></span><span class="offer-reel-group">Click here for a <span class="offer-reel-price"><mark>$50</mark><span>/month</span></span> business website <i>↗</i></span><span class="offer-reel-group">Click here for a <span class="offer-reel-price"><mark>$50</mark><span>/month</span></span> business website <i>↗</i></span><span class="offer-reel-group">Click here for a <span class="offer-reel-price"><mark>$50</mark><span>/month</span></span> business website <i>↗</i></span><span class="offer-reel-group">Click here for a <span class="offer-reel-price"><mark>$50</mark><span>/month</span></span> business website <i>↗</i></span></span></span></a><main class="article-main" id="main"><a class="article-back" href="/#articles">← Back to articles</a><article><p class="eyebrow">ENGLAND SOFTWARE / ARTICLE</p><h1>{{.Title}}</h1><p class="article-date"><time datetime="{{.Created}}">{{.Date}}</time></p>{{if .CoverURL}}<img class="article-cover" src="{{.CoverURL}}" alt="" width="1600" height="1200">{{end}}<div class="article-prose">{{.HTML}}</div></article><a class="article-back" href="/#articles">← More articles</a></main><footer class="site-footer"><span>ENGLAND SOFTWARE · TULSA, OKLAHOMA</span><a href="/#contact">Get in touch ↗</a></footer></body></html>`))
