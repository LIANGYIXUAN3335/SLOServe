package engine

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type OpenAIConfig struct {
	BaseURL string
	APIKey  string
	Client  *http.Client
}

type OpenAI struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

func NewOpenAI(config OpenAIConfig) *OpenAI {
	client := config.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Minute}
	}
	return &OpenAI{
		baseURL: strings.TrimRight(config.BaseURL, "/"),
		apiKey:  config.APIKey,
		client:  client,
	}
}

func (o *OpenAI) Name() string { return "openai-compatible-upstream" }

func (o *OpenAI) Info() map[string]any {
	return map[string]any{"name": o.Name()}
}

func (o *OpenAI) Ready(ctx context.Context) error {
	probe, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(probe, http.MethodGet, o.baseURL+"/health", nil)
	if err != nil {
		return err
	}
	if o.apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+o.apiKey)
	}
	response, err := o.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("upstream health status %d", response.StatusCode)
	}
	return nil
}

func (o *OpenAI) Stream(ctx context.Context, request Request, send StreamFunc) error {
	body := map[string]any{
		"model":      request.Model,
		"messages":   request.Messages,
		"max_tokens": request.MaxTokens,
		"stream":     true,
	}
	if request.CacheSalt != "" {
		body["cache_salt"] = request.CacheSalt
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return &StreamError{BeforeFirstToken: true, Err: err}
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return &StreamError{BeforeFirstToken: true, Err: err}
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	if o.apiKey != "" {
		httpRequest.Header.Set("Authorization", "Bearer "+o.apiKey)
	}

	response, err := o.client.Do(httpRequest)
	if err != nil {
		return &StreamError{BeforeFirstToken: true, Err: err}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return &StreamError{BeforeFirstToken: true, Err: fmt.Errorf("upstream status %d: %s", response.StatusCode, strings.TrimSpace(string(message)))}
	}

	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	seen := false
	index := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			return nil
		}
		var chunk struct {
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return &StreamError{BeforeFirstToken: !seen, Err: fmt.Errorf("decode upstream SSE chunk: %w", err)}
		}
		if chunk.Error != nil {
			return &StreamError{BeforeFirstToken: !seen, Err: fmt.Errorf("upstream stream error: %s", chunk.Error.Message)}
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content == "" {
				continue
			}
			if err := send(Token{Text: choice.Delta.Content, Index: index}); err != nil {
				return err
			}
			seen = true
			index++
		}
	}
	if err := scanner.Err(); err != nil {
		return &StreamError{BeforeFirstToken: !seen, Err: err}
	}
	return &StreamError{BeforeFirstToken: !seen, Err: errors.New("upstream stream ended without [DONE]")}
}
