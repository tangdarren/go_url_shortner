package main

import (
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
)

//go:embed web/*
var assets embed.FS

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func apiError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func (a *app) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		serveAsset(w, r, "index.html", "text/html; charset=utf-8")
	})
	mux.HandleFunc("GET /static/style.css", func(w http.ResponseWriter, r *http.Request) { serveAsset(w, r, "style.css", "text/css; charset=utf-8") })
	mux.HandleFunc("GET /static/app.js", func(w http.ResponseWriter, r *http.Request) {
		serveAsset(w, r, "app.js", "text/javascript; charset=utf-8")
	})
	mux.HandleFunc("POST /api/shorten", a.shorten)
	mux.HandleFunc("GET /api/links", a.links)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok\n")) })
	mux.HandleFunc("GET /{code}", a.redirect)
	mux.HandleFunc("/", notFound)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		w.Header().Set("Cache-Control", "no-store")
		mux.ServeHTTP(w, r)
	})
}

func serveAsset(w http.ResponseWriter, r *http.Request, name, contentType string) {
	data, err := assets.ReadFile("web/" + name)
	if err != nil {
		http.Error(w, "Page unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Write(data)
}

func (a *app) shorten(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	defer r.Body.Close()
	var input struct {
		URL string `json:"url"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	err := decoder.Decode(&input)
	if err == nil {
		var extra any
		if next := decoder.Decode(&extra); next != io.EOF {
			if next == nil {
				err = errors.New("multiple JSON values")
			} else {
				err = next
			}
		}
	}
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			apiError(w, http.StatusRequestEntityTooLarge, "Request body must be 16 KiB or smaller.")
		} else {
			apiError(w, http.StatusBadRequest, "Send a JSON object with a single url string.")
		}
		return
	}
	original, err := validateURL(input.URL)
	if err != nil {
		apiError(w, http.StatusBadRequest, err.Error())
		return
	}
	l, err := a.create(r.Context(), original)
	if err != nil {
		log.Printf("create link: %v", err)
		apiError(w, http.StatusInternalServerError, "Could not save your link. Please try again.")
		return
	}
	writeJSON(w, http.StatusCreated, l)
}

func (a *app) links(w http.ResponseWriter, r *http.Request) {
	links, err := a.recent(r.Context())
	if err != nil {
		log.Printf("list links: %v", err)
		apiError(w, http.StatusInternalServerError, "Could not load recent links.")
		return
	}
	writeJSON(w, http.StatusOK, links)
}

func (a *app) redirect(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	if len(code) != 8 || strings.ContainsAny(code, "\r\n") {
		notFound(w, r)
		return
	}
	var original string
	var err error
	if r.Method == http.MethodHead {
		err = a.db.QueryRowContext(r.Context(), `SELECT original_url FROM links WHERE code = ?`, code).Scan(&original)
	} else {
		err = a.db.QueryRowContext(r.Context(), `UPDATE links SET clicks = clicks + 1 WHERE code = ? RETURNING original_url`, code).Scan(&original)
	}
	if errors.Is(err, sql.ErrNoRows) {
		notFound(w, r)
		return
	}
	if err != nil {
		log.Printf("resolve link: %v", err)
		http.Error(w, "Could not open this link. Please try again.", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, original, http.StatusFound)
}

func notFound(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	w.Write([]byte(`<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Link not found · Shorten</title><link rel="preconnect" href="https://fonts.googleapis.com"><link rel="preconnect" href="https://fonts.gstatic.com" crossorigin><link href="https://fonts.googleapis.com/css2?family=Outfit:wght@400;500;600;700&family=Work+Sans:wght@400;500;600&display=swap" rel="stylesheet"><link rel="stylesheet" href="/static/style.css"><body><div class="atmosphere" aria-hidden="true"></div><main class="not-found"><p class="eyebrow">404 · Link not found</p><h1>This link is missing.</h1><p>Check the short link for a typo, or create a new one.</p><a href="/">Back to Shorten</a></main></body></html>`))
}
