package engine

import (
	"context"
	"errors"
	"strings"
	"time"
)

type FakeConfig struct {
	FirstTokenDelay time.Duration
	TokenDelay      time.Duration
}

type Fake struct {
	config FakeConfig
}

func NewFake(config FakeConfig) *Fake {
	if config.FirstTokenDelay <= 0 {
		config.FirstTokenDelay = 80 * time.Millisecond
	}
	if config.TokenDelay <= 0 {
		config.TokenDelay = 20 * time.Millisecond
	}
	return &Fake{config: config}
}

func (f *Fake) Name() string { return "fake-engine/v1" }

func (f *Fake) Info() map[string]any {
	return map[string]any{
		"name":                 f.Name(),
		"first_token_delay_ms": f.config.FirstTokenDelay.Milliseconds(),
		"token_delay_ms":       f.config.TokenDelay.Milliseconds(),
	}
}

func (f *Fake) Ready(context.Context) error { return nil }

func (f *Fake) Stream(ctx context.Context, request Request, send StreamFunc) error {
	prompt := PromptText(request.Messages)
	if strings.Contains(prompt, "[fail-before]") && request.Attempt == 0 {
		return &StreamError{BeforeFirstToken: true, Err: errors.New("injected pre-token failure")}
	}
	if err := wait(ctx, f.config.FirstTokenDelay); err != nil {
		return err
	}

	words := strings.Fields("SLOServe keeps interactive tokens moving while overload protection preserves the latency budget")
	limit := request.MaxTokens
	if limit <= 0 || limit > len(words) {
		limit = len(words)
	}
	for index := 0; index < limit; index++ {
		if index > 0 {
			if err := wait(ctx, f.config.TokenDelay); err != nil {
				return err
			}
		}
		text := words[index]
		if index < limit-1 {
			text += " "
		}
		if err := send(Token{Text: text, Index: index}); err != nil {
			return err
		}
		if strings.Contains(prompt, "[fail-after]") && index == 1 {
			return &StreamError{BeforeFirstToken: false, Err: errors.New("injected post-token failure")}
		}
	}
	return nil
}

func wait(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
