package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type app struct {
	db      *sql.DB
	baseURL string
	code    func() (string, error)
}

type link struct {
	Code        string `json:"code"`
	OriginalURL string `json:"original_url"`
	ShortURL    string `json:"short_url"`
	CreatedAt   string `json:"created_at"`
	Clicks      int64  `json:"clicks"`
}

func newApp(path, baseURL string) (*app, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	dsn := (&url.URL{Scheme: "file", Path: absolute}).String() + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS links (
		code TEXT PRIMARY KEY CHECK(length(code) = 8),
		original_url TEXT NOT NULL,
		created_at TEXT NOT NULL,
		clicks INTEGER NOT NULL DEFAULT 0
	);
	CREATE INDEX IF NOT EXISTS links_created_at ON links(created_at DESC);`)
	if err != nil {
		db.Close()
		return nil, err
	}
	return &app{db: db, baseURL: baseURL, code: randomCode}, nil
}

func validateURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	invalid := errors.New("Enter an absolute HTTP or HTTPS URL with a valid hostname.")
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.Opaque != "" {
		return "", invalid
	}
	host := u.Hostname()
	if host == "" || strings.ContainsAny(raw, "\r\n\t") {
		return "", invalid
	}
	if strings.HasSuffix(u.Host, ":") {
		return "", invalid
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return "", invalid
		}
	}
	if net.ParseIP(host) == nil {
		host = strings.TrimSuffix(host, ".")
		if len(host) > 253 || strings.ContainsAny(u.Host, "[]") {
			return "", invalid
		}
		for _, label := range strings.Split(host, ".") {
			if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
				return "", invalid
			}
			for _, c := range label {
				if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
					return "", invalid
				}
			}
		}
	}
	return u.String(), nil
}

func randomCode() (string, error) {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	var code [8]byte
	for i := range code {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return "", err
		}
		code[i] = alphabet[n.Int64()]
	}
	return string(code[:]), nil
}

func (a *app) create(ctx context.Context, original string) (link, error) {
	for range 10 {
		code, err := a.code()
		if err != nil {
			return link{}, err
		}
		l := link{Code: code, OriginalURL: original, ShortURL: a.baseURL + "/" + code, CreatedAt: time.Now().UTC().Format("2006-01-02T15:04:05.000000000Z")}
		result, err := a.db.ExecContext(ctx, `INSERT INTO links(code, original_url, created_at) VALUES (?, ?, ?) ON CONFLICT(code) DO NOTHING`, l.Code, l.OriginalURL, l.CreatedAt)
		if err != nil {
			return link{}, err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return link{}, err
		}
		if rows == 1 {
			return l, nil
		}
	}
	return link{}, fmt.Errorf("could not allocate a unique code")
}

func (a *app) recent(ctx context.Context) ([]link, error) {
	rows, err := a.db.QueryContext(ctx, `SELECT code, original_url, created_at, clicks FROM links ORDER BY created_at DESC, rowid DESC LIMIT 20`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	links := make([]link, 0)
	for rows.Next() {
		var l link
		if err := rows.Scan(&l.Code, &l.OriginalURL, &l.CreatedAt, &l.Clicks); err != nil {
			return nil, err
		}
		l.ShortURL = a.baseURL + "/" + l.Code
		links = append(links, l)
	}
	return links, rows.Err()
}
