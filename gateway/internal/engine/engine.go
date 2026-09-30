package engine

import (
	"context"
	"fmt"
	"strings"
)

type Request struct {
	Model     string
	Messages  []Message
	MaxTokens int
	CacheSalt string
	Attempt   int
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Token struct {
	Text  string
	Index int
}

type StreamFunc func(Token) error

type Engine interface {
	Stream(context.Context, Request, StreamFunc) error
	Ready(context.Context) error
	Name() string
	Info() map[string]any
}

func PromptText(messages []Message) string {
	var builder strings.Builder
	for _, message := range messages {
		builder.WriteString(message.Role)
		builder.WriteString(": ")
		builder.WriteString(message.Content)
		builder.WriteByte('\n')
	}
	return builder.String()
}

type StreamError struct {
	BeforeFirstToken bool
	Err              error
}

func (e *StreamError) Error() string {
	return fmt.Sprintf("engine stream failed: %v", e.Err)
}

func (e *StreamError) Unwrap() error {
	return e.Err
}
