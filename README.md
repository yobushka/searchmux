# SearchMux — адаптация для HERMES

Этот репозиторий — [форк SearchMux](https://github.com/johnuopini/searchmux), предназначенный для адаптации сервиса к работе с **HERMES Agent**.

SearchMux служит единым поисковым шлюзом: направляет запросы к нескольким поисковым провайдерам, переключается на резервные по уровням приоритета, отслеживает лимиты и временно исключает провайдеров при повторяющихся ошибках.

## Назначение форка

Форк поддерживает два режима: совместимый с SearXNG GET API для поиска без upstream-ключей и Multi-Tenant BYOK POST API для ключей, передаваемых клиентом в каждом запросе. Оба режима сохраняют маршрутизацию по уровням приоритета, резервные провайдеры и выдачу результатов в формате SearXNG JSON.

Совместимость со штатным клиентом HERMES — цель доработок форка, а не заявление о завершённой интеграции. Текущие контракты обоих режимов описаны ниже.

Оригинальный проект: [johnuopini/searchmux](https://github.com/johnuopini/searchmux). Этот форк не является официальным репозиторием HERMES Agent.

## Integration Modes

### Mode 1: Native SearXNG GET compatibility

Use `GET /search?q=...&format=searxng` for keyless/self-hosted SearXNG search and fallbacks. The root `/` is a search alias; `format=json` is an alias for `searxng`, and `language` is accepted as an alias for `lang`. GET cannot supply upstream API keys, so the five BYOK providers are skipped.

This mode targets the native HERMES SearXNG client. **Keyless refers to the upstream, not the gateway:** every gateway request still needs a Bearer token. A client or its deployment must supply that header; GET parameter compatibility alone does not establish end-to-end HERMES integration.

### Mode 2: Multi-Tenant BYOK

Use `POST /search` with a versioned JSON payload and request-scoped credentials for Brave, Serper, Tavily, Exa, and Jina. The gateway Bearer token authenticates the client; the separate upstream API keys authorize searches at each provider. Only configured providers with keys in that request can be selected, with keyless SearXNG available as a fallback.

The HERMES `web-searchmux` plugin leverages this mode. It is external to this repository, at `~/.hermes/plugins/web-searchmux`; it is not bundled or installed by SearchMux. Configure its gateway URL, gateway Bearer token, and upstream credentials according to that plugin's own instructions. The wire contract is documented below.

## Stateless Zero-Retention Security Model

- The gateway stores **only SHA-256 hashes of client Bearer tokens** in `auth.token_hashes`, never the plaintext tokens as configuration. Incoming tokens are hashed and compared against the configured digests. All routes, including health, stats, and metrics, require authentication; invalid or missing tokens receive HTTP 401.
- Upstream API keys exist only in the current request's memory. They are never stored on disk by the gateway, never logged, and never persisted or cached across requests. Each adapter receives only its own key, not the other providers' credentials. Legacy `api_key` configuration is rejected.
- Request logs contain method, route, status, and duration, not bodies, queries, or headers. Upstream response bodies are not exposed as client error messages.
- Upstream **401, 403, and 429** responses from BYOK providers trigger failover for that request without opening the shared global circuit or consuming another tenant's gateway quota. These failed attempts still appear in aggregate stats. Other provider failures can affect the shared circuit.

Zero retention describes application-level credential handling, not guaranteed immediate memory erasure: Go does not guarantee wiping freed memory. Circuit state, round-robin counters, aggregate statistics, and keyless-provider quotas remain in process memory and reset on restart; they do not retain tenant keys. The guarantee does not extend to the external plugin, reverse proxies, or upstream providers. Use HTTPS when sending credentials across a network and avoid request-body/header logging in surrounding infrastructure.

## Architecture

```
                    +------------------+
  HTTP Request ---->|   SearchMux      |
  GET or POST       |                  |
                    |  +------------+  |
                    |  |   Router   |  |
                    |  | (round-    |  |
                    |  |  robin +   |  |
                    |  |  fallback) |  |
                    |  +-----+------+  |
                    |        |         |
                    |  Tier 1 (API)    |
                    |  Tavily, Brave   |
                    |  Serper, Exa     |
                    |  Jina (BYOK)     |
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

| Provider | Config / credential name | Example tier | Credential source |
|----------|--------------------------|--------------|-------------------|
| Brave | `brave` | 1 | POST `credentials.brave.api_key` |
| Tavily | `tavily` | 1 | POST `credentials.tavily.api_key` |
| Serper | `serper` | 1 | POST `credentials.serper.api_key` |
| Exa | `exa` | 1 | POST `credentials.exa.api_key` |
| Jina | `jina` | 1 | POST `credentials.jina.api_key` |
| SearXNG | `searxng` | 2 | None; configure a reachable `api_base` |

Tier 1 providers with credentials in the current request are tried first with round-robin. If unavailable, SearXNG (Tier 2) is used as fallback. Upstream keys are never configured or retained by the gateway.

Tiers are configurable, not fixed provider properties. Upstream pricing and quotas are determined by each client's provider account, not by this table. Exa uses its search API and maps returned text (or highlights) to result content. Jina uses Jina AI Search and maps description (or content) to result content. Both adapters require request credentials; result limits are enforced locally as well. Support for language and safe-search options varies by adapter; Exa and Jina do not forward those two options.

## Quick Start

### Docker

Build this BYOK version from the repository directory:

```bash
cp .env.example .env
# Edit .env: set GATEWAY_TOKEN_SHA256 to the digest generated below.
# Set TARGETARCH=arm64 for ARM hosts (the example defaults to amd64).
docker compose up -d --build
```

Before starting Compose, choose a high-entropy gateway token and compute its SHA-256 hex digest locally. For example, in Bash, read an existing token without echoing it or putting its value in shell history:

```bash
read -rsp 'Gateway token: ' GATEWAY_TOKEN; printf '\n'
printf %s "$GATEWAY_TOKEN" | sha256sum
```

Copy only the 64-character hex digest (not the trailing filename marker) into `.env` as `GATEWAY_TOKEN_SHA256`. Keep the plaintext token on the client for the Authorization header. Do not put upstream keys in `.env`, YAML, or committed request files. All credential strings shown below are placeholders, not usable secrets.

Docker Compose passes only the gateway token hash to the container. The service mounts `config.example.yaml` read-only and exposes port `8080`. Add multiple hashes under `auth.token_hashes` for multiple tenants. Generate a digest with `printf %s "$GATEWAY_TOKEN" | sha256sum`; use a high-entropy token.

Alternatively, run the image directly:

```bash
docker run -d --name searchmux -p 8080:8080 \
  -v "$(pwd)/config.example.yaml:/etc/searchmux/config.yaml:ro" \
  -e GATEWAY_TOKEN_SHA256="$GATEWAY_TOKEN_SHA256" \
  searchmux
```

For the direct Docker command, first build this checkout with `docker build --build-arg TARGETARCH=amd64 -t searchmux .` (use `arm64` on ARM). Published older images do not implement this BYOK contract.

The listener is configured by `listen` in `config.example.yaml`; `PORT` does not override it. The example's SearXNG fallback requires a separately reachable instance at `http://searxng:8888`; this Compose file starts only SearchMux.

### Build from source

```bash
# Build
go build -o searchmux ./cmd/searchmux

# Run with config
export GATEWAY_TOKEN_SHA256  # Set this variable to the computed digest first.
./searchmux --config config.example.yaml
```

The binary does not automatically load `.env`; export the digest in its environment before starting it. Docker Compose reads `.env` and passes the digest to the container.

## Configuration

See [`config.example.yaml`](config.example.yaml) for the complete configuration and [`.env.example`](.env.example) for Compose variables. The example enables all six providers, with the five BYOK providers in tier 1 and SearXNG in tier 2. Change SearXNG's `api_base` to your reachable instance; Compose does not provision one.

**Tiers**: Lower numbers are higher priority. Tier 1 providers are tried before Tier 2. Within a tier, providers are selected by round-robin.

**Rate limits**: Configured per-provider for keyless providers. BYOK adapters do not share tenant quotas; upstream 401/403/429 responses fail over without opening a global circuit or exhausting another tenant's quota.

**Authentication**: All endpoints require `Authorization: Bearer <gateway-token>`, including stats, metrics, and health. Only SHA-256 gateway token hashes are stored. Empty hash lists fail startup validation. Legacy `api_key` configuration is rejected; upstream keys belong only in POST request bodies. `${VAR}` substitution remains available for non-upstream-secret configuration.

Provider `timeout` overrides `defaults.timeout`. The current handler defaults to 10 results; `defaults.max_results`, `defaults.language`, and `defaults.safe_search` are parsed but not applied to requests. Routing currently uses round-robin regardless of `routing.strategy`. Metrics are always exposed at `/metrics`; the parsed `metrics.enabled` and `metrics.path` settings are not wired into route registration.

### Sliding-Window Circuit Breaker and Availability

```yaml
circuit_breaker:
  threshold: 3
  window: 60s
  cooldown: 30s
```

| Setting | Meaning | Default when omitted |
|---------|---------|----------------------|
| `window` | Rolling interval in which counted failures accumulate | `60s` |
| `threshold` | Failure count needed to open the circuit | `5` (the example uses `3`) |
| `cooldown` | Time an open circuit blocks attempts before permitting a probe | `30s` |

The failure threshold is configured as **`threshold`**, not `failure_threshold`. `failure_threshold` is not a supported YAML setting; use the exact spelling above. Legacy `timeout` supplies the cooldown when `cooldown` is omitted. Durations use Go duration strings, such as `60s` or `1m`.

In **CLOSED**, failures within the sliding `window` accumulate even when successes occur between them; this is a failure-count threshold, not a failure-rate threshold. At the threshold, the circuit becomes **OPEN**, and routing skips that provider until `cooldown` elapses. In **HALF-OPEN**, exactly one real search attempt is admitted as the probe; concurrent attempts skip that provider. A successful probe clears the failure history and closes the circuit. A counted probe failure reopens it for another cooldown. An ignored BYOK 401/403/429 or canceled request releases the probe slot without counting as provider failure.

Breakers are shared per provider, not per credential. Health, stats, and metrics reads can observe the cooldown transition but do not send upstream probes or reserve the probe slot. Reported availability means routing eligibility, not verified connectivity, valid tenant credentials, or a guarantee that a half-open probe slot is free.

## API Reference

### POST /search

Send `Content-Type: application/json` and the gateway Bearer token:

```json
{
  "version": 1,
  "q": "golang tutorials",
  "credentials": {
    "brave": {"api_key": "REPLACE_WITH_BRAVE_KEY"},
    "serper": {"api_key": "REPLACE_WITH_SERPER_KEY"},
    "tavily": {"api_key": "REPLACE_WITH_TAVILY_KEY"},
    "exa": {"api_key": "REPLACE_WITH_EXA_KEY"},
    "jina": {"api_key": "REPLACE_WITH_JINA_KEY"}
  },
  "options": {
    "max_results": 5,
    "language": "en",
    "safe_search": 1
  },
  "format": "searxng"
}
```

Include only the credential entries you want to use; do not send placeholder or empty keys. The exact request fields are:

| JSON field | Type | Requirement / default |
|------------|------|-----------------------|
| `version` | integer | Required; must equal `1` |
| `q` | string | Required; must contain non-whitespace text |
| `credentials` | object | Optional; omitted, `null`, or `{}` uses keyless providers only |
| `credentials.<provider>.api_key` | string | Required for each supplied entry; nonblank, with no CR/LF characters |
| `options` | object | Optional; omitted or `null` uses field defaults |
| `options.max_results` | integer | Nonnegative; omitted or `0` becomes `10` |
| `options.language` | string | Optional; default is empty |
| `options.safe_search` | integer | `0` (off, default), `1` (moderate), or `2` (strict); adapter support varies |
| `format` | string | Optional; empty or omitted means `searxng`; `json` is an alias |

Supported credential names are exactly `brave`, `serper`, `tavily`, `exa`, and `jina`; there is no You adapter or SearXNG credential entry. The decoder also accepts JSON `null` for scalar fields as their zero value; required values still must pass validation. A POST body replaces the URL search parameters rather than merging with them.

Bodies are limited to 1 MiB and must contain exactly one JSON value. Malformed or oversized bodies, unknown fields, unsupported credential names or versions, invalid option values, and unsupported formats return HTTP 400. A missing or non-JSON Content-Type returns 415. Unsupported search methods return 405 with `Allow: GET, POST`. If all eligible providers fail or none can run, the response is HTTP 503 with `{"error":"no search provider available"}`. Search errors use a JSON `error` string; upstream error bodies and keys are not returned.

For a minimal keyless POST with an existing gateway token in your client shell:

```bash
curl -sS http://localhost:8080/search \
  -H "Authorization: Bearer $GATEWAY_TOKEN" \
  -H 'Content-Type: application/json' \
  --data '{"version":1,"q":"golang tutorials"}'
```

For BYOK, construct the JSON object above in the client using its credential store and send it directly; keep upstream keys out of command history and on-disk payload files.

### GET /search

Search across keyless providers with automatic routing. GET parameters and the root alias remain compatible; upstream credentials are accepted only in POST bodies.

```
GET /search?q=golang+tutorials&max_results=10&lang=en&format=searxng
```

```bash
curl -sS 'http://localhost:8080/search?q=golang+tutorials&format=searxng' \
  -H "Authorization: Bearer $GATEWAY_TOKEN"
```

| Parameter    | Required | Default  | Description                                 |
|------------- |----------|----------|---------------------------------------------|
| q            | yes      | --       | Search query                                |
| max_results  | no       | 10       | Maximum number of results                   |
| lang         | no       | --       | Language code (e.g., "en")                  |
| language     | no       | --       | Language code alias (SearXNG compatibility) |
| format       | no       | searxng  | Output format name (searxng, json)          |

Response (SearXNG format, shared by GET and POST; this example shows a BYOK result):

```json
{
  "query": "golang tutorials",
  "number_of_results": 1,
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

## Observability

All observability endpoints require the same gateway Bearer authentication as search. Stats and metrics aggregate across tenants; they contain no credentials or per-tenant request records.

### GET /stats

Returns a JSON `providers` object keyed by configured provider name. Illustrative entry:

```json
{
  "providers": {
    "exa": {
      "available": true,
      "circuit": "closed",
      "requests": 10,
      "successes": 8,
      "failures": 2,
      "success_rate": 0.8,
      "failure_rate": 0.2,
      "average_latency_seconds": 0.35,
      "last_latency_seconds": 0.25
    }
  }
}
```

Counts cover completed upstream attempts since process startup, including failed attempts and BYOK credential errors. One gateway search can produce multiple attempts during failover. Skipped providers do not increment counts. Rates are lifetime fractions from 0 to 1, not percentages or sliding-window breaker rates; they are zero before any attempts. Mean and last latency are in seconds and include failed attempts. All aggregates reset on restart.

`available` describes routing eligibility, not verified connectivity or validity of a tenant's key. `circuit` is `closed`, `open`, or `half-open`. Reading stats does not reserve a probe.

### GET /health

Returns JSON service health and routing state, also available at `/_/health`. It can be used as an authenticated service check, but **is not an upstream-readiness gate**: the current handler returns HTTP 200 and `"status":"ok"` even if every provider is unavailable. Inspect provider fields for eligibility; the endpoint does not test upstream connectivity or tenant keys.

```json
{
  "status": "ok",
  "providers": {
    "tavily":  {"available": true, "circuit": "closed", "rate_limit_remaining": -1},
    "brave":   {"available": true, "circuit": "closed", "rate_limit_remaining": -1},
    "serper":  {"available": true, "circuit": "closed", "rate_limit_remaining": -1},
    "exa":     {"available": true, "circuit": "closed", "rate_limit_remaining": -1},
    "jina":    {"available": true, "circuit": "closed", "rate_limit_remaining": -1},
    "searxng": {"available": true, "circuit": "closed", "rate_limit_remaining": -1}
  }
}
```

`rate_limit_remaining` is `-1` for BYOK providers and keyless providers without a configured gateway quota; it does not report the client's upstream account balance.

### GET /metrics

Prometheus metrics endpoint. Exports:

- `searchmux_requests_total{provider, tier, status}` -- request counter
- `searchmux_latency_seconds{provider}` -- latency histogram
- `searchmux_fallbacks_total{from_tier, to_tier}` -- fallback counter
- `searchmux_provider_up{provider}` -- routing availability
- `searchmux_circuit_breaker_state{provider}` -- closed=0, open=1, half-open=2
- `searchmux_provider_requests_total{provider}` and `searchmux_provider_failures_total{provider}` -- all upstream attempts and failures
- `searchmux_provider_failure_rate{provider}` and `searchmux_provider_latency_seconds{provider}` -- lifetime failure fraction and mean attempt latency

Provider metrics are refreshed on each scrape, including cooldown transitions. `searchmux_provider_up` is 1 for routing eligibility and 0 otherwise, not an active connectivity probe. The provider counters include failed failover attempts; `searchmux_requests_total` records routed search outcomes, not authentication or validation rejections. `searchmux_latency_seconds` measures the successful upstream attempt rather than the entire failover chain. The fallback counter records successful searches marked as fallback; its `from_tier` label is currently fixed to `1`, even with custom tier configurations. Configure the scraper to send the gateway Bearer token.

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
- `Available()` reports static provider availability; it does not actively test reachability.

Register the provider in the `createProvider` switch in `cmd/searchmux/main.go`. For BYOK adapters, implement `RequiresAPIKey() bool`, use only the request's `SearchOpts.APIKey`, and add the credential name to the POST handler's allowlist. Never retain the key in the adapter.

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
