package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	anthropicAPIURL     = "https://api.anthropic.com/v1/messages"
	anthropicAPIVersion = "2023-06-01"
)

// AnthropicProvider implements the Provider interface for Anthropic's Claude API
type AnthropicProvider struct {
	config *ProviderConfig
	client *http.Client
}

// NewAnthropicProvider creates a new Anthropic provider
func NewAnthropicProvider(config *ProviderConfig) *AnthropicProvider {
	if config.BaseURL == "" {
		config.BaseURL = anthropicAPIURL
	}
	if config.Model == "" {
		config.Model = "claude-sonnet-4-20250514"
	}
	return &AnthropicProvider{
		config: config,
		client: &http.Client{},
	}
}

func (p *AnthropicProvider) Name() string {
	return "anthropic"
}

// anthropicRequest represents a request to the Anthropic API
type anthropicRequest struct {
	Model     string          `json:"model"`
	Messages  []anthropicMsg  `json:"messages"`
	System    string          `json:"system,omitempty"`
	MaxTokens int             `json:"max_tokens"`
	Tools     []anthropicTool `json:"tools,omitempty"`
}

type anthropicMsg struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

type anthropicTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

type anthropicToolUse struct {
	Type  string          `json:"type"`
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

type anthropicToolResult struct {
	Type      string `json:"type"`
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
}

// anthropicResponse represents a response from the Anthropic API
type anthropicResponse struct {
	ID         string           `json:"id"`
	Type       string           `json:"type"`
	Role       string           `json:"role"`
	Content    []anthropicBlock `json:"content"`
	StopReason string           `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type anthropicBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

// Streaming event types for Anthropic SSE
type anthropicStreamEvent struct {
	Type         string               `json:"type"`
	Message      *anthropicResponse   `json:"message,omitempty"`
	Index        int                  `json:"index,omitempty"`
	ContentBlock *anthropicBlock      `json:"content_block,omitempty"`
	Delta        *anthropicEventDelta `json:"delta,omitempty"`
	Usage        *struct {
		OutputTokens int `json:"output_tokens"`
	} `json:"usage,omitempty"`
}

type anthropicEventDelta struct {
	Type        string `json:"type,omitempty"`
	Text        string `json:"text,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
	StopReason  string `json:"stop_reason,omitempty"`
}

func (p *AnthropicProvider) Complete(ctx context.Context, req *CompletionRequest) (*Response, error) {
	if p.config.APIKey == "" {
		return nil, ErrNoAPIKey
	}

	// Convert messages to Anthropic format
	msgs := make([]anthropicMsg, 0, len(req.Messages))
	systemPrompt := req.System

	for _, msg := range req.Messages {
		switch msg.Role {
		case RoleSystem:
			if systemPrompt == "" {
				systemPrompt = msg.Content
			}
		case RoleUser:
			msgs = append(msgs, anthropicMsg{
				Role:    "user",
				Content: msg.Content,
			})
		case RoleAssistant:
			if len(msg.ToolCalls) > 0 {
				// Assistant message with tool calls
				content := make([]interface{}, 0)
				if msg.Content != "" {
					content = append(content, map[string]string{
						"type": "text",
						"text": msg.Content,
					})
				}
				for _, tc := range msg.ToolCalls {
					content = append(content, map[string]interface{}{
						"type":  "tool_use",
						"id":    tc.ID,
						"name":  tc.Name,
						"input": json.RawMessage(tc.Arguments),
					})
				}
				msgs = append(msgs, anthropicMsg{
					Role:    "assistant",
					Content: content,
				})
			} else {
				msgs = append(msgs, anthropicMsg{
					Role:    "assistant",
					Content: msg.Content,
				})
			}
		case RoleTool:
			// Tool results in Anthropic are user messages with tool_result content
			msgs = append(msgs, anthropicMsg{
				Role: "user",
				Content: []anthropicToolResult{{
					Type:      "tool_result",
					ToolUseID: msg.ToolCallID,
					Content:   msg.Content,
				}},
			})
		}
	}

	// Convert tools to Anthropic format
	var tools []anthropicTool
	if len(req.Tools) > 0 {
		tools = make([]anthropicTool, len(req.Tools))
		for i, t := range req.Tools {
			tools[i] = anthropicTool{
				Name:        t.Name,
				Description: t.Description,
				InputSchema: t.Parameters,
			}
		}
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = p.config.MaxTokens
	}

	anthropicReq := anthropicRequest{
		Model:     req.Model,
		Messages:  msgs,
		System:    systemPrompt,
		MaxTokens: maxTokens,
		Tools:     tools,
	}

	if anthropicReq.Model == "" {
		anthropicReq.Model = p.config.Model
	}

	body, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.config.BaseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.config.APIKey)
	httpReq.Header.Set("anthropic-version", anthropicAPIVersion)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var anthropicResp anthropicResponse
	if err := json.Unmarshal(respBody, &anthropicResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if anthropicResp.Error != nil {
		return nil, fmt.Errorf("%w: %s - %s", ErrAPIError, anthropicResp.Error.Type, anthropicResp.Error.Message)
	}

	// Convert response to common format
	response := &Response{
		StopReason: anthropicResp.StopReason,
		Usage: Usage{
			InputTokens:  anthropicResp.Usage.InputTokens,
			OutputTokens: anthropicResp.Usage.OutputTokens,
		},
	}

	for _, block := range anthropicResp.Content {
		switch block.Type {
		case "text":
			response.Content += block.Text
		case "tool_use":
			response.ToolCalls = append(response.ToolCalls, ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: block.Input,
			})
		}
	}

	return response, nil
}

// CompleteStream sends messages to the LLM and streams the response
func (p *AnthropicProvider) CompleteStream(ctx context.Context, req *CompletionRequest) (<-chan StreamEvent, error) {
	if p.config.APIKey == "" {
		return nil, ErrNoAPIKey
	}

	// Convert messages to Anthropic format (same as Complete)
	msgs := make([]anthropicMsg, 0, len(req.Messages))
	systemPrompt := req.System

	for _, msg := range req.Messages {
		switch msg.Role {
		case RoleSystem:
			if systemPrompt == "" {
				systemPrompt = msg.Content
			}
		case RoleUser:
			msgs = append(msgs, anthropicMsg{
				Role:    "user",
				Content: msg.Content,
			})
		case RoleAssistant:
			if len(msg.ToolCalls) > 0 {
				content := make([]interface{}, 0)
				if msg.Content != "" {
					content = append(content, map[string]string{
						"type": "text",
						"text": msg.Content,
					})
				}
				for _, tc := range msg.ToolCalls {
					content = append(content, map[string]interface{}{
						"type":  "tool_use",
						"id":    tc.ID,
						"name":  tc.Name,
						"input": json.RawMessage(tc.Arguments),
					})
				}
				msgs = append(msgs, anthropicMsg{
					Role:    "assistant",
					Content: content,
				})
			} else {
				msgs = append(msgs, anthropicMsg{
					Role:    "assistant",
					Content: msg.Content,
				})
			}
		case RoleTool:
			msgs = append(msgs, anthropicMsg{
				Role: "user",
				Content: []anthropicToolResult{{
					Type:      "tool_result",
					ToolUseID: msg.ToolCallID,
					Content:   msg.Content,
				}},
			})
		}
	}

	var tools []anthropicTool
	if len(req.Tools) > 0 {
		tools = make([]anthropicTool, len(req.Tools))
		for i, t := range req.Tools {
			tools[i] = anthropicTool{
				Name:        t.Name,
				Description: t.Description,
				InputSchema: t.Parameters,
			}
		}
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = p.config.MaxTokens
	}

	// Use anonymous struct to include stream field
	anthropicReq := struct {
		Model     string          `json:"model"`
		Messages  []anthropicMsg  `json:"messages"`
		System    string          `json:"system,omitempty"`
		MaxTokens int             `json:"max_tokens"`
		Tools     []anthropicTool `json:"tools,omitempty"`
		Stream    bool            `json:"stream"`
	}{
		Model:     req.Model,
		Messages:  msgs,
		System:    systemPrompt,
		MaxTokens: maxTokens,
		Tools:     tools,
		Stream:    true,
	}

	if anthropicReq.Model == "" {
		anthropicReq.Model = p.config.Model
	}

	body, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.config.BaseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.config.APIKey)
	httpReq.Header.Set("anthropic-version", anthropicAPIVersion)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%w: status %d: %s", ErrAPIError, resp.StatusCode, string(respBody))
	}

	eventChan := make(chan StreamEvent, 100)

	go func() {
		defer close(eventChan)
		defer resp.Body.Close()

		reader := bufio.NewReader(resp.Body)
		var inputTokens int
		var outputTokens int
		var stopReason string

		// Track tool calls being built
		toolCalls := make(map[int]*ToolCall)
		toolCallArgs := make(map[int]string)

		for {
			select {
			case <-ctx.Done():
				eventChan <- StreamEvent{Type: StreamEventError, Error: ctx.Err()}
				return
			default:
			}

			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					break
				}
				eventChan <- StreamEvent{Type: StreamEventError, Error: err}
				return
			}

			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			// Parse SSE format
			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				break
			}

			var event anthropicStreamEvent
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}

			switch event.Type {
			case "message_start":
				if event.Message != nil {
					inputTokens = event.Message.Usage.InputTokens
				}

			case "content_block_start":
				if event.ContentBlock != nil {
					switch event.ContentBlock.Type {
					case "tool_use":
						tc := &ToolCall{
							ID:   event.ContentBlock.ID,
							Name: event.ContentBlock.Name,
						}
						toolCalls[event.Index] = tc
						toolCallArgs[event.Index] = ""
						eventChan <- StreamEvent{
							Type:          StreamEventToolCallStart,
							ToolCall:      tc,
							ToolCallIndex: event.Index,
						}
					}
				}

			case "content_block_delta":
				if event.Delta != nil {
					switch event.Delta.Type {
					case "text_delta":
						if event.Delta.Text != "" {
							eventChan <- StreamEvent{
								Type: StreamEventText,
								Text: event.Delta.Text,
							}
						}
					case "input_json_delta":
						if event.Delta.PartialJSON != "" {
							toolCallArgs[event.Index] += event.Delta.PartialJSON
							eventChan <- StreamEvent{
								Type:          StreamEventToolCallDelta,
								ToolCallIndex: event.Index,
								ArgumentDelta: event.Delta.PartialJSON,
							}
						}
					}
				}

			case "content_block_stop":
				if tc, ok := toolCalls[event.Index]; ok {
					tc.Arguments = json.RawMessage(toolCallArgs[event.Index])
					eventChan <- StreamEvent{
						Type:          StreamEventToolCallEnd,
						ToolCall:      tc,
						ToolCallIndex: event.Index,
					}
				}

			case "message_delta":
				if event.Delta != nil && event.Delta.StopReason != "" {
					stopReason = event.Delta.StopReason
				}
				if event.Usage != nil {
					outputTokens = event.Usage.OutputTokens
				}

			case "message_stop":
				eventChan <- StreamEvent{
					Type:       StreamEventDone,
					StopReason: stopReason,
					Usage: Usage{
						InputTokens:  inputTokens,
						OutputTokens: outputTokens,
					},
				}
				return
			}
		}

		// Send done event if we haven't already
		eventChan <- StreamEvent{
			Type:       StreamEventDone,
			StopReason: stopReason,
			Usage: Usage{
				InputTokens:  inputTokens,
				OutputTokens: outputTokens,
			},
		}
	}()

	return eventChan, nil
}
