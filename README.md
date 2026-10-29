# Shorten

A small Go URL shortener using `net/http`, SQLite, and plain HTML, CSS, and JavaScript. SQLite runs through the maintained pure-Go [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) driver; no C compiler, external API, API key, frontend framework, or Docker is required.

## Run

Install Go 1.26 or newer, then run from the repository root:

```sh
go run .
```

Open <http://localhost:8080>. Go downloads module dependencies on the first run. The server creates `./data/shortener.db` and its parent directory automatically. Links and click counts persist across restarts. Stop with Ctrl+C.

## Environment variables

- `PORT`: listening port, default `8080`.
- `BASE_URL`: public HTTP(S) origin used to build short links, default `http://localhost:8080`. Use an origin without a path, query, or fragment. Change this separately when changing the port.
- `DB_PATH`: SQLite file location, default `./data/shortener.db`.

For example:

```sh
PORT=9090 BASE_URL=http://localhost:9090 DB_PATH=./data/local.db go run .
```

Use your public HTTPS origin for `BASE_URL` when hosting behind a reverse proxy. The application serves HTTP itself.

## API examples

Create a short link:

```sh
curl -i -X POST http://localhost:8080/api/shorten \
  -H 'Content-Type: application/json' \
  -d '{"url":"https://example.com/a/long/path"}'
```

Returns HTTP 201 and a JSON object with `code`, `short_url`, `original_url`, `created_at` (UTC), and `clicks`. Codes contain eight random alphanumeric characters. Duplicate destinations receive separate short links. Validation errors return HTTP 400 with an `error` message; request bodies above 16 KiB return HTTP 413. URLs are trimmed and must use HTTP or HTTPS with a valid ASCII hostname or IP address and an optional port from 1–65535. International hostnames must use punycode. URLs containing credentials are rejected. Submitted URLs are never fetched by the server.

List the 20 most recent links, newest first:

```sh
curl http://localhost:8080/api/links
```

Returns a JSON array of link objects (or `[]`). All links are public to visitors of this server; avoid submitting private or sensitive URLs.

Resolve a link (replace `Ab12Cd34` with a returned code):

```sh
curl -i http://localhost:8080/Ab12Cd34
```

GET returns HTTP 302 with the original URL in `Location`, atomically increments the click count, and disables caching so future requests reach the server. HEAD resolves without counting a click. Unknown codes return a readable HTTP 404 page.

Health check:

```sh
curl -i http://localhost:8080/health
```

Returns HTTP 200 with `ok`.

## Checks and layout

```sh
gofmt -w *.go
go test ./...
go vet ./...
```

- `main.go`: configuration, HTTP timeouts, and graceful shutdown.
- `store.go`: validation, random codes, and parameterized SQLite queries.
- `handlers.go`: routes, JSON responses, and embedded static assets.
- `web/`: responsive interface with safe DOM text rendering and clipboard support.
- `app_test.go`: validation, API behavior, atomic counts, collision retries, recent-link limits, and persistence.

Database files and SQLite sidecar files are excluded by `.gitignore`. Preserve the database file to retain links; stop the server before making a simple file-copy backup.
