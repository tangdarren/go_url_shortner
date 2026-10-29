package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func run() error {
	port := env("PORT", "8080")
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return errors.New("PORT must be a number from 1 to 65535")
	}
	baseURL, err := validateURL(env("BASE_URL", "http://localhost:8080"))
	if err != nil {
		return errors.New("BASE_URL must be an absolute HTTP or HTTPS URL with a valid hostname")
	}
	base, err := url.Parse(baseURL)
	if err != nil || (base.Path != "" && base.Path != "/") || base.RawQuery != "" || base.ForceQuery || base.Fragment != "" {
		return errors.New("BASE_URL must be an origin without a path, query, or fragment")
	}
	app, err := newApp(env("DB_PATH", "./data/shortener.db"), strings.TrimRight(baseURL, "/"))
	if err != nil {
		return err
	}
	defer app.db.Close()
	server := &http.Server{
		Addr: ":" + port, Handler: app.handler(),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown: %v", err)
		}
	}()
	log.Printf("Listening on http://localhost:%s (short links: %s)", port, app.baseURL)
	if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
