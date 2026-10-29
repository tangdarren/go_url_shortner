package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
)

func testApp(t *testing.T) *app {
	t.Helper()
	a, err := newApp(filepath.Join(t.TempDir(), "nested", "links.db"), "https://short.example")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.db.Close() })
	return a
}

func request(a *app, method, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	a.handler().ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
	return w
}

func TestValidateURL(t *testing.T) {
	for _, raw := range []string{"https://example.com", " http://localhost:8080/a?q=x#part ", "https://sub.example.com./path", "http://127.0.0.1", "http://[::1]:8080/path", "https://example.com/?q=%3Cscript%3E"} {
		t.Run(raw, func(t *testing.T) {
			got, err := validateURL(raw)
			if err != nil || got != strings.TrimSpace(raw) {
				t.Fatalf("got %q, %v", got, err)
			}
		})
	}
	for _, raw := range []string{"", "example.com", "//example.com", "/relative", "ftp://example.com", "javascript:alert(1)", "https://", "https://bad host.com", "https://-bad.com", "https://bad_.com", "https://example..com", "http://user:pass@example.com", "http://example.com:", "http://example.com:0", "http://example.com:65536", "http://example.com:abc", "http://[not-ip]", "http://example.com/\npath", "https://" + strings.Repeat("a", 64) + ".com", "https://例え.jp"} {
		t.Run(raw, func(t *testing.T) {
			if _, err := validateURL(raw); err == nil {
				t.Fatalf("accepted %q", raw)
			}
		})
	}
}

func TestCreateResolveAndClicks(t *testing.T) {
	a := testApp(t)
	w := request(a, "POST", "/api/shorten", `{"url":"  https://example.com/path?q=1  "}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var l link
	if err := json.Unmarshal(w.Body.Bytes(), &l); err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^[A-Za-z0-9]{8}$`).MatchString(l.Code) || l.ShortURL != "https://short.example/"+l.Code || l.Clicks != 0 || l.CreatedAt == "" {
		t.Fatalf("invalid link: %+v", l)
	}
	const count = 12
	var wg sync.WaitGroup
	for range count {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := request(a, "GET", "/"+l.Code, "")
			if w.Code != http.StatusFound || w.Header().Get("Location") != "https://example.com/path?q=1" {
				t.Errorf("redirect: %d %s", w.Code, w.Header().Get("Location"))
			}
		}()
	}
	wg.Wait()
	if w := request(a, "HEAD", "/"+l.Code, ""); w.Code != http.StatusFound {
		t.Fatalf("HEAD: %d", w.Code)
	}
	w = request(a, "GET", "/api/links", "")
	var links []link
	if err := json.Unmarshal(w.Body.Bytes(), &links); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusOK || len(links) != 1 || links[0].Clicks != count {
		t.Fatalf("links: %s", w.Body.String())
	}
}

func TestUnknownAndPages(t *testing.T) {
	a := testApp(t)
	for _, path := range []string{"/abcdefgh", "/missing", "/api/missing", "/a/b"} {
		w := request(a, "GET", path, "")
		if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "This link is missing.") {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
	}
	for _, path := range []string{"/", "/static/style.css", "/static/app.js", "/health"} {
		if w := request(a, "GET", path, ""); w.Code != http.StatusOK {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
}

func TestInvalidRequests(t *testing.T) {
	a := testApp(t)
	for _, body := range []string{`{`, `null`, `{}`, `{"url":42}`, `{"url":"javascript:alert(1)"}`, `{"url":"https://example.com","extra":1}`, `{"url":"https://example.com"} {}`} {
		w := request(a, "POST", "/api/shorten", body)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%q: %d", body, w.Code)
		}
		if !strings.Contains(w.Header().Get("Content-Type"), "application/json") {
			t.Fatal("expected JSON error")
		}
	}
	w := request(a, "POST", "/api/shorten", `{"url":"https://example.com/`+strings.Repeat("x", 16<<10)+`"}`)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized: %d", w.Code)
	}
}

func TestPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "shortener.db")
	a, err := newApp(path, "http://localhost:8080")
	if err != nil {
		t.Fatal(err)
	}
	l, err := a.create(context.Background(), "https://example.com")
	if err != nil {
		a.db.Close()
		t.Fatal(err)
	}
	request(a, "GET", "/"+l.Code, "")
	if err := a.db.Close(); err != nil {
		t.Fatal(err)
	}
	a, err = newApp(path, "http://localhost:8080")
	if err != nil {
		t.Fatal(err)
	}
	defer a.db.Close()
	links, err := a.recent(context.Background())
	if err != nil || len(links) != 1 || links[0].Code != l.Code || links[0].Clicks != 1 || links[0].CreatedAt != l.CreatedAt {
		t.Fatalf("persisted links: %+v, %v", links, err)
	}
	if w := request(a, "GET", "/"+l.Code, ""); w.Code != http.StatusFound {
		t.Fatalf("reopened redirect: %d", w.Code)
	}
}

func TestCollisionRetryAndLimit(t *testing.T) {
	a := testApp(t)
	ctx := context.Background()
	a.code = func() (string, error) { return "aaaaaaaa", nil }
	if _, err := a.create(ctx, "https://example.com/first"); err != nil {
		t.Fatal(err)
	}
	calls := 0
	a.code = func() (string, error) {
		calls++
		if calls == 1 {
			return "aaaaaaaa", nil
		}
		return "bbbbbbbb", nil
	}
	if l, err := a.create(ctx, "https://example.com/second"); err != nil || l.Code != "bbbbbbbb" || calls != 2 {
		t.Fatalf("collision retry: %+v, %v, calls=%d", l, err, calls)
	}
	a.code = func() (string, error) { return "aaaaaaaa", nil }
	if _, err := a.create(ctx, "https://example.com/third"); err == nil {
		t.Fatal("expected retry exhaustion")
	}
	a.code = randomCode
	var last link
	for i := range 23 {
		var err error
		last, err = a.create(ctx, fmt.Sprintf("https://example.com/%d", i))
		if err != nil {
			t.Fatal(err)
		}
	}
	links, err := a.recent(ctx)
	if err != nil || len(links) != 20 || links[0].Code != last.Code {
		t.Fatalf("recent order/limit: %+v %v", links, err)
	}
}
