package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const portEnv = "ENGLANDSOFTWARE_PORT"
const defaultPort = "8493"

//go:embed static/*.css static/*.html static/*.js static/*.png static/*.svg static/*.webp static/*.txt static/*.xml static/images/*.webp
var staticFiles embed.FS

func main() {
	if err := loadEnvFile("config/.env"); err != nil {
		log.Fatal(err)
	}
	emailConfig, err := emailSettingsFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	port := os.Getenv(portEnv)
	if port == "" {
		port = defaultPort
	}
	value, err := strconv.Atoi(port)
	if err != nil || value < 1 || value > 65535 {
		log.Fatalf("%s must be a port from 1 through 65535", portEnv)
	}
	var trustedProxies []netip.Prefix
	for _, item := range strings.Split(os.Getenv("ENGLANDSOFTWARE_TRUSTED_PROXY_CIDRS"), ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(item)
		if err != nil {
			log.Fatalf("invalid trusted proxy CIDR %q: %v", item, err)
		}
		trustedProxies = append(trustedProxies, prefix)
	}
	app, err := newApp("data/main.sqlite", appConfig{
		username:        os.Getenv("ENGLANDSOFTWARE_ADMIN_USERNAME"),
		password:        os.Getenv("ENGLANDSOFTWARE_ADMIN_PASSWORD"),
		secureCookies:   os.Getenv("ENGLANDSOFTWARE_SECURE_COOKIES") == "true",
		messageCapacity: messageCapacity(),
		trustedProxies:  trustedProxies,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer app.db.Close()
	app.email = emailConfig
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for now := range ticker.C {
			if err := app.prune(now.UTC()); err != nil {
				log.Printf("database cleanup: %v", err)
			}
		}
	}()
	addr := ":" + port
	log.Printf("England Software listening at http://localhost:%s", port)
	server := &http.Server{Addr: addr, Handler: app.handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("HTTP shutdown: %v", err)
		}
	}()
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func publicPages(mux *http.ServeMux) {
	mux.Handle("/static/", http.FileServer(http.FS(staticFiles)))
	for path, filename := range map[string]string{"/robots.txt": "robots.txt", "/sitemap.xml": "sitemap.xml"} {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				methodNotAllowed(w, "GET, HEAD")
				return
			}
			http.ServeFileFS(w, r, staticFiles, "static/"+filename)
		})
	}
	pages := map[string]string{"/": "index.html"}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/websites" {
			http.Redirect(w, r, "/#websites", http.StatusMovedPermanently)
			return
		}
		if r.URL.Path == "/contract-engineering" {
			http.Redirect(w, r, "/#consulting", http.StatusMovedPermanently)
			return
		}
		if r.URL.Path == "/managed-hosting" {
			http.Redirect(w, r, "/#websites", http.StatusMovedPermanently)
			return
		}
		if r.URL.Path == "/web-applications" {
			http.Redirect(w, r, "/#applications", http.StatusMovedPermanently)
			return
		}
		page, ok := pages[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			methodNotAllowed(w, "GET, HEAD")
			return
		}
		http.ServeFileFS(w, r, staticFiles, fmt.Sprintf("static/%s", page))
	})
}

func messageCapacity() int {
	v := os.Getenv("ENGLANDSOFTWARE_MESSAGE_CAPACITY")
	if v == "" {
		return 200
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		log.Fatal("ENGLANDSOFTWARE_MESSAGE_CAPACITY must be a positive integer")
	}
	return n
}
