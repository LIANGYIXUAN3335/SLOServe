package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"sloserve/gateway/internal/admission"
	"sloserve/gateway/internal/engine"
	"sloserve/gateway/internal/metrics"
)

type Config struct {
	DefaultModel string
	MaxQueueWait time.Duration
	TTFTTarget   time.Duration
	CacheSecret  string
	MaxRetries   int
	Logger       *slog.Logger
}

type Server struct {
	config  Config
	engine  engine.Engine
	gate    *admission.Controller
	metrics *metrics.Metrics
	mux     *http.ServeMux
}

type chatRequest struct {
	Model               string        `json:"model"`
	Messages            []chatMessage `json:"messages"`
	Stream              bool          `json:"stream"`
	MaxTokens           int           `json:"max_tokens"`
	MaxCompletionTokens int           `json:"max_completion_tokens"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func New(config Config, inference engine.Engine, gate *admission.Controller, telemetry *metrics.Metrics) *Server {
	if config.DefaultModel == "" {
		config.DefaultModel = "fake-engine/v1"
	}
	if config.MaxQueueWait <= 0 {
		config.MaxQueueWait = 500 * time.Millisecond
	}
	if config.TTFTTarget <= 0 {
		config.TTFTTarget = 300 * time.Millisecond
	}
	if config.CacheSecret == "" {
		config.CacheSecret = "local-development-only"
	}
	if config.MaxRetries < 0 {
		config.MaxRetries = 0
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	server := &Server{config: config, engine: inference, gate: gate, metrics: telemetry, mux: http.NewServeMux()}
	server.routes()
	return server
}

func (s *Server) Handler() http.Handler {
	return requestIDMiddleware(s.mux)
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /livez", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "alive"})
	})
	s.mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := s.engine.Ready(r.Context()); err != nil {
			s.config.Logger.Warn("engine readiness check failed", "error", err)
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready", "engine": s.engine.Name()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready", "engine": s.engine.Name()})
	})
	s.mux.HandleFunc("GET /metrics", s.handleMetrics)
	s.mux.HandleFunc("GET /debug/state", s.handleState)
	s.mux.HandleFunc("POST /v1/chat/completions", s.handleChat)
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	var request chatRequest
	decoder := json.NewDecoder(io.LimitReader(r.Body, 2<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if len(request.Messages) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "messages must not be empty")
		return
	}
	if request.Model == "" {
		request.Model = s.config.DefaultModel
	}
	maxTokens := request.MaxCompletionTokens
	if maxTokens <= 0 {
		maxTokens = request.MaxTokens
	}
	if maxTokens <= 0 {
		maxTokens = 64
	}
	if maxTokens > 4096 {
		writeError(w, http.StatusBadRequest, "invalid_request", "max tokens must be <= 4096")
		return
	}
	prompt := flattenMessages(request.Messages)
	estimatedTokens := estimateTokens(prompt) + maxTokens
	priority := priorityFromRequest(r)

	queueContext, cancelQueue := context.WithTimeout(r.Context(), s.config.MaxQueueWait)
	lease, queueWait, err := s.gate.Acquire(queueContext, priority, estimatedTokens)
	cancelQueue()
	s.metrics.ObserveQueue(queueWait.Seconds())
	if err != nil {
		if errors.Is(err, context.Canceled) {
			s.metrics.Inc("request_canceled")
			return
		}
		s.metrics.Inc("request_shed")
		w.Header().Set("Retry-After", "1")
		writeError(w, http.StatusTooManyRequests, "overloaded", "token queue is full; retry with jitter")
		return
	}
	defer lease.Release()

	result := s.runInference(w, r, request, prompt, maxTokens, started)
	s.metrics.ObserveDuration(time.Since(started).Seconds())
	if errors.Is(result, context.Canceled) {
		s.metrics.Inc("request_canceled")
		return
	}
	if result != nil {
		s.metrics.Inc("request_upstream_error")
		return
	}
	s.metrics.Inc("request_completed")
}

func (s *Server) runInference(w http.ResponseWriter, r *http.Request, request chatRequest, prompt string, maxTokens int, started time.Time) error {
	var output strings.Builder
	firstTokenAt := time.Time{}
	metTTFTTarget := false
	streamStarted := false
	flusher, canFlush := w.(http.Flusher)

	send := func(token engine.Token) error {
		now := time.Now()
		if firstTokenAt.IsZero() {
			firstTokenAt = now
			ttft := firstTokenAt.Sub(started)
			s.metrics.ObserveTTFT(ttft.Seconds())
			metTTFTTarget = ttft <= s.config.TTFTTarget
		}
		s.metrics.Inc("output_token")
		output.WriteString(token.Text)

		if !request.Stream {
			return nil
		}
		if !streamStarted {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache, no-transform")
			w.Header().Set("X-Accel-Buffering", "no")
			w.WriteHeader(http.StatusOK)
			streamStarted = true
		}
		chunk := map[string]any{
			"id":      requestID(r.Context()),
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   request.Model,
			"choices": []map[string]any{{"index": 0, "delta": map[string]string{"content": token.Text}, "finish_reason": nil}},
		}
		payload, _ := json.Marshal(chunk)
		if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
			return err
		}
		if canFlush {
			flusher.Flush()
		}
		return nil
	}

	var inferenceErr error
	for attempt := 0; attempt <= s.config.MaxRetries; attempt++ {
		inferenceErr = s.engine.Stream(r.Context(), engine.Request{
			Model: request.Model, Messages: toEngineMessages(request.Messages), MaxTokens: maxTokens,
			CacheSalt: cacheSalt(s.config.CacheSecret, r.Header.Get("X-Tenant-ID")), Attempt: attempt,
		}, send)
		if inferenceErr == nil {
			break
		}
		var streamErr *engine.StreamError
		if !errors.As(inferenceErr, &streamErr) || !streamErr.BeforeFirstToken || !firstTokenAt.IsZero() || attempt == s.config.MaxRetries {
			break
		}
		s.metrics.Inc("retry")
	}

	if inferenceErr != nil {
		if errors.Is(inferenceErr, context.Canceled) || errors.Is(r.Context().Err(), context.Canceled) {
			return context.Canceled
		}
		if streamStarted {
			payload, _ := json.Marshal(map[string]any{"error": map[string]string{"type": "upstream_stream_error", "message": "stream terminated; response was not replayed"}})
			fmt.Fprintf(w, "event: error\ndata: %s\n\n", payload)
			if canFlush {
				flusher.Flush()
			}
			return inferenceErr
		}
		writeError(w, http.StatusBadGateway, "upstream_error", inferenceErr.Error())
		return inferenceErr
	}
	if metTTFTTarget {
		s.metrics.Inc("goodput")
	}

	if request.Stream {
		if !streamStarted {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		if canFlush {
			flusher.Flush()
		}
		return nil
	}

	completionTokens := strings.Count(strings.TrimSpace(output.String()), " ") + 1
	response := map[string]any{
		"id":      requestID(r.Context()),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   request.Model,
		"choices": []map[string]any{{"index": 0, "message": map[string]string{"role": "assistant", "content": output.String()}, "finish_reason": "stop"}},
		"usage":   map[string]int{"prompt_tokens": estimateTokens(prompt), "completion_tokens": completionTokens, "total_tokens": estimateTokens(prompt) + completionTokens},
	}
	writeJSON(w, http.StatusOK, response)
	return nil
}

func (s *Server) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	snapshot := s.gate.Snapshot()
	s.metrics.Render(w, snapshot.Running, snapshot.QueuedRequests, snapshot.QueuedTokens)
}

func (s *Server) handleState(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"engine":    s.engine.Info(),
		"admission": s.gate.Snapshot(),
		"limits":    s.gate.Config(),
		"gateway": map[string]any{
			"max_queue_wait_ms": s.config.MaxQueueWait.Milliseconds(),
			"ttft_target_ms":    s.config.TTFTTarget.Milliseconds(),
			"max_retries":       s.config.MaxRetries,
		},
	})
}

func toEngineMessages(messages []chatMessage) []engine.Message {
	converted := make([]engine.Message, 0, len(messages))
	for _, message := range messages {
		converted = append(converted, engine.Message{Role: message.Role, Content: message.Content})
	}
	return converted
}

func flattenMessages(messages []chatMessage) string {
	var builder strings.Builder
	for _, message := range messages {
		builder.WriteString(message.Role)
		builder.WriteString(": ")
		builder.WriteString(message.Content)
		builder.WriteByte('\n')
	}
	return builder.String()
}

func estimateTokens(text string) int {
	count := (len([]rune(text)) + 3) / 4
	if count < 1 {
		return 1
	}
	return count
}

func priorityFromRequest(r *http.Request) admission.Priority {
	if strings.EqualFold(r.Header.Get("X-SLO-Priority"), "batch") {
		return admission.Batch
	}
	return admission.Interactive
}

func cacheSalt(secret, tenant string) string {
	if tenant == "" {
		tenant = "anonymous"
	}
	digest := sha256.Sum256([]byte(secret + "\x00" + tenant))
	return hex.EncodeToString(digest[:16])
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, errorType, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"type": errorType, "message": message}})
}

type requestIDKey struct{}

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = fmt.Sprintf("req_%x", sha256.Sum256([]byte(fmt.Sprintf("%d-%s", time.Now().UnixNano(), r.RemoteAddr))))[:20]
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id)))
	})
}

func requestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}
