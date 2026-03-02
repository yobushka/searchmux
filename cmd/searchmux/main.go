package main

import (
	"flag"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/johnuopini/searchmux/internal/config"
	"github.com/johnuopini/searchmux/internal/formatter"
	"github.com/johnuopini/searchmux/internal/provider"
	"github.com/johnuopini/searchmux/internal/ratelimit"
	"github.com/johnuopini/searchmux/internal/router"
	"github.com/johnuopini/searchmux/internal/server"
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

		if pc.RateLimit.Requests > 0 && pc.RateLimit.Window > 0 {
			tracker.Configure(pc.Name, pc.RateLimit.Requests, pc.RateLimit.Window)
		}
		r.AddProvider(p, pc.Tier)
		slog.Info("registered provider", "name", pc.Name, "tier", pc.Tier)
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
