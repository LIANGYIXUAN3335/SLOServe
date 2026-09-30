package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"sloserve/gateway/internal/admission"
	"sloserve/gateway/internal/api"
	"sloserve/gateway/internal/engine"
	"sloserve/gateway/internal/metrics"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	port := env("PORT", "8080")
	mode := env("ENGINE_MODE", "fake")

	var inference engine.Engine
	switch mode {
	case "fake":
		inference = engine.NewFake(engine.FakeConfig{
			FirstTokenDelay: durationEnv("FAKE_TTFT", 80*time.Millisecond),
			TokenDelay:      durationEnv("FAKE_TPOT", 20*time.Millisecond),
		})
	case "openai":
		baseURL := os.Getenv("UPSTREAM_URL")
		if baseURL == "" {
			logger.Error("UPSTREAM_URL is required when ENGINE_MODE=openai")
			os.Exit(2)
		}
		inference = engine.NewOpenAI(engine.OpenAIConfig{BaseURL: baseURL, APIKey: os.Getenv("UPSTREAM_API_KEY")})
	default:
		logger.Error("unknown ENGINE_MODE", "mode", mode)
		os.Exit(2)
	}

	gate := admission.New(admission.Config{
		MaxRunning:         intEnv("MAX_RUNNING", 16),
		InteractiveReserve: intEnv("INTERACTIVE_RESERVE", 2),
		MaxQueuedTokens:    intEnv("MAX_QUEUED_TOKENS", 40000),
	})
	server := api.New(api.Config{
		DefaultModel: env("DEFAULT_MODEL", "fake-engine/v1"),
		MaxQueueWait: durationEnv("MAX_QUEUE_WAIT", 500*time.Millisecond),
		TTFTTarget:   durationEnv("TTFT_TARGET", 300*time.Millisecond),
		CacheSecret:  env("CACHE_SALT_SECRET", "local-development-only"),
		MaxRetries:   intEnv("MAX_PRE_TTFT_RETRIES", 1),
		Logger:       logger,
	}, inference, gate, metrics.New())

	httpServer := &http.Server{
		Addr:              ":" + port,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}

	go func() {
		logger.Info("SLOServe gateway listening", "port", port, "engine", inference.Name())
		if err := httpServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server stopped", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
	}
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func intEnv(name string, fallback int) int {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		panic(fmt.Sprintf("%s must be an integer: %v", name, err))
	}
	return parsed
}

func durationEnv(name string, fallback time.Duration) time.Duration {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		panic(fmt.Sprintf("%s must be a duration: %v", name, err))
	}
	return parsed
}
