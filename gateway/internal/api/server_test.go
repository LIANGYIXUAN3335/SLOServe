package api

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"sloserve/gateway/internal/admission"
	"sloserve/gateway/internal/engine"
	"sloserve/gateway/internal/metrics"
)

func newTestServer(t *testing.T) (http.Handler, *metrics.Metrics) {
	t.Helper()
	telemetry := metrics.New()
	server := New(Config{
		MaxQueueWait: 20 * time.Millisecond,
		TTFTTarget:   300 * time.Millisecond,
		MaxRetries:   1,
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}, engine.NewFake(engine.FakeConfig{FirstTokenDelay: time.Millisecond, TokenDelay: time.Millisecond}), admission.New(admission.Config{
		MaxRunning: 2, InteractiveReserve: 1, MaxQueuedTokens: 1000,
	}), telemetry)
	return server.Handler(), telemetry
}

func TestStreamingCompletionUsesOpenAICompatibleSSE(t *testing.T) {
	handler, telemetry := newTestServer(t)

	body := `{"model":"fake","messages":[{"role":"user","content":"hello"}],"stream":true,"max_tokens":4}`
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	payload := recorder.Body.Bytes()
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d: %s", recorder.Code, payload)
	}
	if !bytes.Contains(payload, []byte("chat.completion.chunk")) || !bytes.Contains(payload, []byte("data: [DONE]")) {
		t.Fatalf("unexpected SSE payload: %s", payload)
	}
	if telemetry.Counter("request_completed") != 1 || telemetry.Counter("output_token") != 4 {
		t.Fatalf("unexpected metrics: completed=%d tokens=%d", telemetry.Counter("request_completed"), telemetry.Counter("output_token"))
	}
}

func TestFailureBeforeFirstTokenRetriesOnce(t *testing.T) {
	handler, telemetry := newTestServer(t)

	body := `{"messages":[{"role":"user","content":"[fail-before]"}],"stream":false,"max_tokens":2}`
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body.Bytes())
	}
	if telemetry.Counter("retry") != 1 {
		t.Fatalf("expected one retry, got %d", telemetry.Counter("retry"))
	}
}

func TestFailureAfterFirstTokenIsNotReplayed(t *testing.T) {
	handler, telemetry := newTestServer(t)

	body := `{"messages":[{"role":"user","content":"[fail-after]"}],"stream":true,"max_tokens":5}`
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	payload := recorder.Body.Bytes()
	if !bytes.Contains(payload, []byte("event: error")) {
		t.Fatalf("expected terminal stream error: %s", payload)
	}
	if bytes.Contains(payload, []byte("data: [DONE]")) {
		t.Fatalf("failed stream must not claim completion: %s", payload)
	}
	if telemetry.Counter("retry") != 0 {
		t.Fatalf("post-token failure was retried")
	}
	if telemetry.Counter("goodput") != 0 {
		t.Fatalf("failed stream must not count as goodput")
	}
}
