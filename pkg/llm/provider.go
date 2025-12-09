package llm

import (
	"context"
	"errors"
)

var (
	ErrNoAPIKey       = errors.New("API key not configured")
	ErrInvalidRequest = errors.New("invalid request")
	ErrAPIError       = errors.New("API error")
)

// Provider is the interface that LLM providers must implement
type Provider interface {
	// Name returns the provider name
	Name() string

	// Complete sends messages to the LLM and returns a response
	Complete(ctx context.Context, req *CompletionRequest) (*Response, error)
}

// StreamProvider extends Provider with streaming support
type StreamProvider interface {
	Provider

	// CompleteStream sends messages to the LLM and streams the response
	CompleteStream(ctx context.Context, req *CompletionRequest) (<-chan StreamEvent, error)
}

// CompletionRequest contains the parameters for a completion request
type CompletionRequest struct {
	Model       string           `json:"model"`
	Messages    []Message        `json:"messages"`
	Tools       []ToolDefinition `json:"tools,omitempty"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
	Temperature float64          `json:"temperature,omitempty"`
	System      string           `json:"system,omitempty"`
}

// ProviderConfig holds configuration for LLM providers
type ProviderConfig struct {
	APIKey      string
	BaseURL     string
	Model       string
	MaxTokens   int
	Temperature float64
}

// DefaultConfig returns a default provider configuration
func DefaultConfig() *ProviderConfig {
	return &ProviderConfig{
		MaxTokens:   4096,
		Temperature: 0.7,
	}
}
