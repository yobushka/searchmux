package main

import (
	"flag"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/frenzy/searchmux/internal/config"
	"github.com/frenzy/searchmux/internal/formatter"
	"github.com/frenzy/searchmux/internal/provider"
	"github.com/frenzy/searchmux/internal/ratelimit"
	"github.com/frenzy/searchmux/internal/router"
	"github.com/frenzy/searchmux/internal/server"
)

func main() {
	configPath := flag.String("config", "/etc/searchmux/config.yaml", "path to config file")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}
	if err := cfg.Validate(); err != nil {
		slog.Error("invalid config", "error", err)
		os.Exit(1)
	}

	tracker := ratelimit.New()
	r := router.New(router.Config{
		CBThreshold: cfg.CircuitBreaker.Threshold,
		CBTimeout:   cfg.CircuitBreaker.Timeout,
		RateLimiter: tracker,
	})

	for _, pc := range cfg.Providers {
		p := createProvider(pc, cfg.Defaults)
		if p == nil {
			slog.Warn("unknown provider, skipping", "name", pc.Name)
			continue
		}

		tier := parseTier(pc.Tier)
		if pc.RateLimit > 0 {
			tracker.Configure(pc.Name, pc.RateLimit, time.Minute)
		}
		r.AddProvider(p, tier)
		slog.Info("registered provider", "name", pc.Name, "tier", tier)
	}

	fmtRegistry := formatter.NewRegistry()
	fmtRegistry.Register(formatter.NewSearXNG())

	h := server.NewHandler(r, fmtRegistry, "searxng")
	mux := h.Mux()

	slog.Info("starting searchmux", "listen", cfg.Listen)
	if err := http.ListenAndServe(cfg.Listen, server.LoggingMiddleware(mux)); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}

// parseTier converts a tier name string to its numeric priority.
// Lower numbers have higher priority.
func parseTier(tier string) int {
	switch tier {
	case "primary":
		return 1
	case "secondary":
		return 2
	case "fallback":
		return 3
	default:
		return 10
	}
}

func createProvider(pc config.ProviderConfig, defaults config.DefaultsConfig) provider.Provider {
	timeout := pc.Timeout
	if timeout == 0 {
		timeout = defaults.Timeout
	}
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	switch pc.Name {
	case "tavily":
		return provider.NewTavily(pc.APIKey, timeout)
	case "brave":
		return provider.NewBrave(pc.APIKey, timeout)
	case "serper":
		return provider.NewSerper(pc.APIKey, timeout)
	case "searxng":
		return provider.NewSearXNG(pc.APIBase, timeout)
	default:
		return nil
	}
}
