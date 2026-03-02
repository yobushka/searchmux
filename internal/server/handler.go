package server

import (
	"encoding/json"
	"net/http"
	"strconv"

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
	if query == "" {
		writeJSONError(w, http.StatusBadRequest, "missing required parameter: q")
		return
	}

	opts := provider.SearchOpts{
		Language: r.URL.Query().Get("lang"),
	}
	if mr := r.URL.Query().Get("max_results"); mr != "" {
		if n, err := strconv.Atoi(mr); err == nil && n > 0 {
			opts.MaxResults = n
		}
	}

	result, err := h.router.Search(r.Context(), query, opts)
	if err != nil {
		RequestsTotal.WithLabelValues("none", "0", "error").Inc()
		writeJSONError(w, http.StatusServiceUnavailable, err.Error())
		return
	}

	// Record metrics for successful search.
	tierStr := strconv.Itoa(result.TierUsed)
	RequestsTotal.WithLabelValues(result.ProviderUsed, tierStr, "success").Inc()
	LatencyHistogram.WithLabelValues(result.ProviderUsed).Observe(result.Latency.Seconds())
	if result.Fallback {
		FallbacksTotal.WithLabelValues("1", tierStr).Inc()
	}

	formatName := r.URL.Query().Get("format")
	if formatName == "" {
		formatName = h.defaultFormat
	}

	fmt, err := h.formatters.Get(formatName)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	meta := formatter.Metadata{
		ProviderUsed: result.ProviderUsed,
		TierUsed:     result.TierUsed,
		Fallback:     result.Fallback,
		Latency:      result.Latency,
	}

	data, err := fmt.Format(query, result.Results, meta)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "format error: "+err.Error())
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
	mux.HandleFunc("/", h.HandleSearch)         // root alias
	mux.HandleFunc("/health", h.HandleHealth)
	mux.HandleFunc("/_/health", h.HandleHealth) // OpenFaaS watchdog
	mux.Handle("/metrics", promhttp.Handler())
	return mux
}

// writeJSONError writes an error response as JSON with the given status code.
func writeJSONError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
