package server

import (
	"github.com/johnuopini/searchmux/internal/router"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// providerCollector reads a fresh snapshot on every scrape, including cooldown
// transitions even when no search request has arrived.
type providerCollector struct{ router *router.Router }

var providerMetricDescriptions = []*prometheus.Desc{
	prometheus.NewDesc("searchmux_provider_up", "Provider routing availability (not an active connectivity probe)", []string{"provider"}, nil),
	prometheus.NewDesc("searchmux_circuit_breaker_state", "Circuit state: closed=0, open=1, half-open=2", []string{"provider"}, nil),
	prometheus.NewDesc("searchmux_provider_requests_total", "All upstream attempts", []string{"provider"}, nil),
	prometheus.NewDesc("searchmux_provider_failures_total", "Failed upstream attempts", []string{"provider"}, nil),
	prometheus.NewDesc("searchmux_provider_failure_rate", "Process-lifetime fraction of failed attempts", []string{"provider"}, nil),
	prometheus.NewDesc("searchmux_provider_latency_seconds", "Mean upstream attempt latency including failures", []string{"provider"}, nil),
}

func (c providerCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, desc := range providerMetricDescriptions {
		ch <- desc
	}
}

func (c providerCollector) Collect(ch chan<- prometheus.Metric) {
	for name, stats := range c.router.Stats() {
		up, state := 0.0, 0.0
		if stats.Available {
			up = 1
		}
		switch stats.Circuit {
		case "open":
			state = 1
		case "half-open":
			state = 2
		}
		values := []float64{up, state, float64(stats.Requests), float64(stats.Failures), stats.FailureRate, stats.AverageLatencySeconds}
		for i, value := range values {
			kind := prometheus.GaugeValue
			if i == 2 || i == 3 {
				kind = prometheus.CounterValue
			}
			ch <- prometheus.MustNewConstMetric(providerMetricDescriptions[i], kind, value, name)
		}
	}
}

func (h *Handler) metricsRegistry() prometheus.Gatherer {
	registry := prometheus.NewRegistry()
	registry.MustRegister(providerCollector{router: h.router})
	return prometheus.Gatherers{prometheus.DefaultGatherer, registry}
}

var (
	// RequestsTotal counts total search requests by provider, tier, and status.
	RequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "searchmux_requests_total",
			Help: "Total search requests by provider, tier, and status",
		},
		[]string{"provider", "tier", "status"},
	)

	// FallbacksTotal counts total fallbacks from one tier to another.
	FallbacksTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "searchmux_fallbacks_total",
			Help: "Total fallbacks from one tier to another",
		},
		[]string{"from_tier", "to_tier"},
	)

	// LatencyHistogram observes search request latency by provider.
	LatencyHistogram = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "searchmux_latency_seconds",
			Help:    "Search request latency by provider",
			Buckets: []float64{0.1, 0.25, 0.5, 1.0, 2.5, 5.0},
		},
		[]string{"provider"},
	)
)
