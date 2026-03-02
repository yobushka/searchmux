package server

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

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
