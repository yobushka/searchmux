# SearchMux

A search multiplexer that routes queries across multiple search providers with tiered fallback, rate limit tracking, and circuit breaking. Built for OpenFaaS deployment.

## Architecture

```
                    +------------------+
  HTTP Request ---->|   SearchMux      |
  /search?q=...    |                  |
                    |  +------------+  |
                    |  |   Router   |  |
                    |  | (round-    |  |
                    |  |  robin +   |  |
                    |  |  fallback) |  |
                    |  +-----+------+  |
                    |        |         |
                    |  Tier 1 (API)    |
                    |  +---+---+---+   |
                    |  | T | B | S |   |
                    |  +---+---+---+   |
                    |        |         |
                    |  Tier 2 (Self)   |
                    |  +----------+    |
                    |  | SearXNG  |    |
                    |  +----------+    |
                    |        |         |
                    |  +------------+  |
                    |  | Formatter  |  |
                    |  +------------+  |
                    +--------+---------+
                             |
  HTTP Response <------------+
  (SearXNG JSON format)
```

## Providers

| Provider | Tier | Free Tier        | Auth       |
|----------|------|------------------|------------|
| Tavily   | 1    | 1,000 req/month  | API key    |
| Brave    | 1    | 2,000 req/month  | API key    |
| Serper   | 1    | 2,500 req/month  | API key    |
| SearXNG  | 2    | Unlimited (self) | None       |

Tier 1 providers are tried first with round-robin. If all are exhausted or unavailable, SearXNG (Tier 2) is used as fallback. Total free capacity: ~5,500 API searches/month plus unlimited SearXNG fallback.

## Quick Start

```bash
# Build
go build -o searchmux ./cmd/searchmux

# Run with config
./searchmux --config config.example.yaml

# Or with Docker
docker build -t searchmux .
docker run -p 8080:8080 \
  -v /path/to/config.yaml:/etc/searchmux/config.yaml \
  -e TAVILY_API_KEY=tvly-... \
  -e BRAVE_API_KEY=BSA... \
  -e SERPER_API_KEY=... \
  searchmux
```

## Configuration

See `config.example.yaml` for a complete example.

**Tiers**: Lower numbers are higher priority. Tier 1 providers are tried before Tier 2. Within a tier, providers are selected by round-robin.

**Rate limits**: Configured per-provider with `requests` (count) and `window` (duration). The router tracks usage and skips providers nearing their limit.

**Environment variable substitution**: API keys support `${VAR}` syntax. Values are resolved from the environment at startup.

**Circuit breaker**: Opens after `threshold` consecutive failures. After `timeout`, it transitions to half-open and allows one probe request.

## API Reference

### GET /search

Search across providers with automatic routing.

```
GET /search?q=golang+tutorials&max_results=10&lang=en&format=searxng
```

| Parameter    | Required | Default  | Description                     |
|------------- |----------|----------|---------------------------------|
| q            | yes      | --       | Search query                    |
| max_results  | no       | 10       | Maximum number of results       |
| lang         | no       | --       | Language code (e.g., "en")      |
| format       | no       | searxng  | Output format name              |

Response (SearXNG format):

```json
{
  "query": "golang tutorials",
  "number_of_results": 5,
  "results": [
    {
      "title": "Go by Example",
      "url": "https://gobyexample.com",
      "content": "Annotated example programs...",
      "engine": "tavily"
    }
  ],
  "infoboxes": [],
  "x-provider": "tavily",
  "x-tier": 1,
  "x-fallback": false,
  "x-latency-ms": 342
}
```

### GET /health

Returns provider availability, circuit breaker states, and rate limit remaining counts.

```json
{
  "status": "ok",
  "providers": {
    "tavily":  {"available": true, "circuit": "closed", "rate_limit_remaining": 847},
    "brave":   {"available": true, "circuit": "closed", "rate_limit_remaining": 1923},
    "serper":  {"available": true, "circuit": "closed", "rate_limit_remaining": 2401},
    "searxng": {"available": true, "circuit": "closed", "rate_limit_remaining": -1}
  }
}
```

### GET /metrics

Prometheus metrics endpoint. Exports:

- `searchmux_requests_total{provider, tier, status}` -- request counter
- `searchmux_request_duration_seconds{provider}` -- latency histogram
- `searchmux_fallbacks_total{from_tier, to_tier}` -- fallback counter

## Adding Providers

Implement the `Provider` interface in `internal/provider/provider.go`:

```go
type Provider interface {
    Name() string
    Search(ctx context.Context, query string, opts SearchOpts) (*Response, error)
    Available() bool
}
```

- `Name()` returns a unique identifier (e.g., "google").
- `Search()` executes the query and returns results.
- `Available()` returns whether the provider is configured and reachable.

Register the provider in `cmd/searchmux/main.go` within the provider setup loop.

## Adding Output Formats

Implement the `Formatter` interface in `internal/formatter/formatter.go`:

```go
type Formatter interface {
    Name() string
    Format(query string, results []provider.Result, meta Metadata) ([]byte, error)
    ContentType() string
}
```

- `Name()` returns the format identifier used in the `?format=` query parameter.
- `Format()` serializes results into the output format.
- `ContentType()` returns the HTTP Content-Type header value.

Register the formatter in the `formatter.Registry` during server startup.

## License

MIT
