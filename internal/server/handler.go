package server

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/johnuopini/searchmux/internal/formatter"
	"github.com/johnuopini/searchmux/internal/provider"
	"github.com/johnuopini/searchmux/internal/router"
)

// Handler holds the dependencies for HTTP request handling.
type Handler struct {
	router        *router.Router
	formatters    *formatter.Registry
	defaultFormat string
}

// NewHandler creates a new Handler with a router, formatter registry, and default format name.
func NewHandler(r *router.Router, fmts *formatter.Registry, defaultFormat string) *Handler {
	return &Handler{
		router:        r,
		formatters:    fmts,
		defaultFormat: defaultFormat,
	}
}

// HandleSearch handles GET /search?q=...&max_results=N&lang=en&format=searxng
func (h *Handler) HandleSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	formatName := r.URL.Query().Get("format")

	opts := provider.SearchOpts{
		Language: r.URL.Query().Get("lang"),
	}
	if opts.Language == "" {
		opts.Language = r.URL.Query().Get("language")
	}
	if mr := r.URL.Query().Get("max_results"); mr != "" {
		if n, err := strconv.Atoi(mr); err == nil && n > 0 {
			opts.MaxResults = n
		}
	}
	switch r.Method {
	case http.MethodGet:
	case http.MethodPost:
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "application/json" {
			writeJSONError(w, http.StatusUnsupportedMediaType, "expected application/json")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		defer r.Body.Close()
		var body searchRequest
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid search request")
			return
		}
		if err := decoder.Decode(new(json.RawMessage)); err != io.EOF || body.Version != 1 || body.Options.MaxResults < 0 || body.Options.SafeSearch < 0 || body.Options.SafeSearch > 2 {
			writeJSONError(w, http.StatusBadRequest, "invalid search request")
			return
		}
		query, formatName = body.Query, body.Format
		opts = provider.SearchOpts{MaxResults: body.Options.MaxResults, Language: body.Options.Language, SafeSearch: body.Options.SafeSearch, Credentials: make(map[string]string, len(body.Credentials))}
		for name, credential := range body.Credentials {
			switch name {
			case "brave", "serper", "tavily", "exa", "jina":
			default:
				writeJSONError(w, http.StatusBadRequest, "unsupported credential provider")
				return
			}
			if strings.TrimSpace(credential.APIKey) == "" || strings.ContainsAny(credential.APIKey, "\r\n") {
				writeJSONError(w, http.StatusBadRequest, "invalid provider credentials")
				return
			}
			opts.Credentials[name] = credential.APIKey
		}
	default:
		w.Header().Set("Allow", "GET, POST")
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if strings.TrimSpace(query) == "" {
		writeJSONError(w, http.StatusBadRequest, "missing required parameter: q")
		return
	}
	if opts.MaxResults == 0 {
		opts.MaxResults = 10
	}
	if formatName == "" {
		formatName = h.defaultFormat
	}
	if formatName == "json" {
		formatName = "searxng"
	}
	fmt, err := h.formatters.Get(formatName)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "unsupported output format")
		return
	}

	result, err := h.router.Search(r.Context(), query, opts)
	if err != nil {
		RequestsTotal.WithLabelValues("none", "0", "error").Inc()
		writeJSONError(w, http.StatusServiceUnavailable, "no search provider available")
		return
	}

	// Record metrics for successful search.
	tierStr := strconv.Itoa(result.TierUsed)
	RequestsTotal.WithLabelValues(result.ProviderUsed, tierStr, "success").Inc()
	LatencyHistogram.WithLabelValues(result.ProviderUsed).Observe(result.Latency.Seconds())
	if result.Fallback {
		FallbacksTotal.WithLabelValues("1", tierStr).Inc()
	}

	meta := formatter.Metadata{
		ProviderUsed: result.ProviderUsed,
		TierUsed:     result.TierUsed,
		Fallback:     result.Fallback,
		Latency:      result.Latency,
	}

	data, err := fmt.Format(query, result.Results, meta)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "response formatting failed")
		return
	}

	w.Header().Set("Content-Type", fmt.ContentType())
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// HandleHealth returns provider health status.
func (h *Handler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	health := h.router.Health()

	resp := map[string]interface{}{
		"status":    "ok",
		"providers": health,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// Mux returns an http.ServeMux with all routes registered.
func (h *Handler) Mux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/search", h.HandleSearch)
	mux.HandleFunc("/", h.HandleSearch) // root alias
	mux.HandleFunc("/health", h.HandleHealth)
	mux.HandleFunc("/_/health", h.HandleHealth) // OpenFaaS watchdog
	mux.HandleFunc("GET /stats", h.HandleStats)
	mux.Handle("GET /metrics", promhttp.HandlerFor(h.metricsRegistry(), promhttp.HandlerOpts{}))
	return mux
}

type searchRequest struct {
	Version     int    `json:"version"`
	Query       string `json:"q"`
	Credentials map[string]struct {
		APIKey string `json:"api_key"`
	} `json:"credentials"`
	Options struct {
		MaxResults int    `json:"max_results"`
		Language   string `json:"language"`
		SafeSearch int    `json:"safe_search"`
	} `json:"options"`
	Format string `json:"format"`
}

func (h *Handler) HandleStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"providers": h.router.Stats()})
}

// writeJSONError writes an error response as JSON with the given status code.
func writeJSONError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
