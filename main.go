package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
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

// listen opens PORT when set; otherwise the first free port from 3000 upward.
func listen() (net.Listener, string, error) {
	if port := os.Getenv("PORT"); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return nil, "", errors.New("PORT must be a number from 1 to 65535")
		}
		ln, err := net.Listen("tcp", ":"+port)
		if err != nil {
			return nil, "", err
		}
		return ln, port, nil
	}
	for port := 3000; port <= 65535; port++ {
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err == nil {
			return ln, strconv.Itoa(port), nil
		}
	}
	return nil, "", errors.New("no free TCP port available from 3000 upward")
}

func run() error {
	ln, port, err := listen()
	if err != nil {
		return err
	}
	defer ln.Close()

	baseURL, err := validateURL(env("BASE_URL", "http://localhost:"+port))
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
		Handler:           app.handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
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
	if err := server.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
