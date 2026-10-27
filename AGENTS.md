# PROJECT KNOWLEDGE BASE

**Generated:** 2026-10-27
**Commit:** ad8e5dd
**Branch:** feat/hermes-searxng-compat

## OVERVIEW
SearchMux is a Go search gateway with tiered provider fallback and SearXNG-shaped JSON.
This fork targets the stock HERMES SearXNG client; compatibility is a goal, not a completed integration claim.
One Go module (`github.com/johnuopini/searchmux`), Go 1.23.0 with toolchain go1.23.8; YAML and Prometheus are the direct dependencies.

## STRUCTURE
```text
searchmux/
|-- cmd/searchmux/       # Startup wiring and explicit provider factory
|-- internal/config/    # YAML, environment substitution, defaults, validation
|-- internal/provider/  # Outbound adapters and normalized result contract
|-- internal/router/    # Tier selection and per-provider circuit breakers
|-- internal/ratelimit/ # Process-local quota state
|-- internal/formatter/ # Output registry and SearXNG serialization
|-- internal/server/   # HTTP parsing, metrics, request logging
|-- config.example.yaml
`-- .github/workflows/ # Test and tag-release automation
```

## WHERE TO LOOK
| Task | Location | Notes |
|------|----------|-------|
| Add a provider | `internal/provider/`, `cmd/searchmux/main.go` | Implement `Provider`; wire the constructor in `createProvider` |
| HERMES/API compatibility | `internal/server/handler.go`, `handler_test.go` | Inbound query parsing differs from outbound SearXNG |
| Output JSON | `internal/formatter/searxng.go`, `internal/provider/provider.go` | Envelope and result JSON tags live separately |
| Routing/fallback | `internal/router/router.go`, `router_test.go` | Ascending tiers, round-robin within each tier |
| Circuit recovery | `internal/router/circuit.go`, `circuit_test.go` | Eligibility checks interact with breaker state |
| Quotas | `internal/ratelimit/tracker.go`, `tracker_test.go` | In-memory windows and hard-limit tracking |
| Config wiring | `internal/config/config.go`, `cmd/searchmux/main.go` | Parsing a setting does not mean startup consumes it |
| Deployment | `Dockerfile`, `docker-compose.yml`, `config.example.yaml` | Compose supplies only the gateway |
| Release destinations | `.github/workflows/release.yml` | Check upstream registry destination before publishing from the fork |

## CODE MAP
LSP and ast-grep were unavailable during generation; symbol density and reference centrality are unmeasured.
Source-inspected dependency flow: server -> router -> provider; router also uses ratelimit, and server uses formatter.
Startup in `cmd/searchmux/main.go` assembles all six internal packages.

## CONVENTIONS
- Tests live beside implementation in each internal package; the executable has no test file.
- Provider registration is an explicit switch, not automatic discovery.
- New output formats require registration in `main`; only `searxng` is currently registered.
- Lower tier numbers mean higher priority; failures retry within a tier before falling back.
- `Result.Snippet` serializes as `content`, and `Result.Source` as `engine`.
- SearXNG output normalizes nil results to `[]` and includes empty `infoboxes` plus `x-*` routing metadata.
- Startup and request logs use structured `slog`; startup installs a JSON handler on stdout.

## ANTI-PATTERNS (THIS PROJECT)
- Do not describe HERMES integration as complete; the README explicitly calls it a development goal.
- Do not commit `config.yaml` or `.env` secrets; tracked examples are intentionally exempted from ignores.
- Do not broaden the root `/searchmux` ignore into a pattern that hides `cmd/searchmux` sources.

## UNIQUE STYLES
- Inbound search reads URL parameters `q`, `max_results`, `lang`, and `format`.
- Inbound `format=searxng` differs from the outbound SearXNG adapter's `format=json` and `language` parameter.
- `/` is a search alias and catches unmatched paths; routes currently have no HTTP-method restriction.
- Health is available at `/health` and `/_/health`; it reports state, not upstream connectivity readiness.

## COMMANDS
```bash
make build                         # Static, stripped binary for host GOOS/GOARCH
./searchmux --config config.example.yaml
make test                          # go test -race -count=1 ./...
make vet                           # go vet ./...
go test ./internal/server -count=1  # Focused API tests
make docker                        # docker build -t searchmux .
```
CI uses `go test ./... -count=1 -v` and `go vet ./...`; its test invocation does not enable the race detector.

## NOTES
- Default config path is `/etc/searchmux/config.yaml`; listener address comes from YAML, not `PORT`.
- Missing `${VAR}` environment substitutions remain literal text; validation does not reject missing credentials.
- Unknown provider names pass config validation but are skipped during startup.
- Search defaults other than timeout, routing strategy, and metrics enable/path settings are parsed but not wired into execution.
- Metrics are always exposed at `/metrics`; unknown output formats are rejected only after routing the search.
- Quota state is process-local and resets on restart; successful searches are recorded after completion.
- Compose's example fallback requires a separately reachable SearXNG at `http://searxng:8888`.
- Dockerfile defaults `TARGETARCH` to `arm64`; verify the target architecture when building on another host.
