package main

import (
	"embed"
	"fmt"
	"log"
	"net/http"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"
)

const portEnv = "ENGLANDSOFTWARE_PORT"
const defaultPort = "8493"

//go:embed static/*
var staticFiles embed.FS

func main() {
	if err := loadEnvFile("config/.env"); err != nil {
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
		username:       os.Getenv("ENGLANDSOFTWARE_ADMIN_USERNAME"),
		password:       os.Getenv("ENGLANDSOFTWARE_ADMIN_PASSWORD"),
		secureCookies:  os.Getenv("ENGLANDSOFTWARE_SECURE_COOKIES") == "true",
		trustedProxies: trustedProxies,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer app.db.Close()
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
	log.Printf("England Software listening on %s", addr)
	if err := http.ListenAndServe(addr, app.handler()); err != nil {
		log.Fatal(err)
	}
}

func publicPages(mux *http.ServeMux) {
	mux.Handle("/static/", http.FileServer(http.FS(staticFiles)))
	pages := map[string]string{
		"/": "index.html", "/websites": "websites.html", "/web-applications": "web-applications.html",
		"/managed-hosting": "managed-hosting.html", "/contract-engineering": "contract-engineering.html",
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
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
