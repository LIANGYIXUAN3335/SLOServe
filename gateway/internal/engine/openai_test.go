package engine

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestOpenAIForwardsMessageRoles(t *testing.T) {
	want := []Message{
		{Role: "system", Content: "Be concise."},
		{Role: "user", Content: "Explain overload."},
	}
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		var body struct {
			Messages []Message `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(body.Messages, want) {
			t.Fatalf("messages changed: got %#v want %#v", body.Messages, want)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n")),
		}, nil
	})}

	upstream := NewOpenAI(OpenAIConfig{BaseURL: "http://upstream.test", Client: client})
	var output strings.Builder
	err := upstream.Stream(context.Background(), Request{Model: "test", Messages: want, MaxTokens: 4}, func(token Token) error {
		output.WriteString(token.Text)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if output.String() != "ok" {
		t.Fatalf("unexpected output %q", output.String())
	}
}

func TestOpenAIMalformedSSEIsAnError(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("data: not-json\n\n")),
		}, nil
	})}
	upstream := NewOpenAI(OpenAIConfig{BaseURL: "http://upstream.test", Client: client})
	err := upstream.Stream(context.Background(), Request{Model: "test", Messages: []Message{{Role: "user", Content: "hello"}}}, func(Token) error {
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "decode upstream SSE chunk") {
		t.Fatalf("expected decode error, got %v", err)
	}
}
