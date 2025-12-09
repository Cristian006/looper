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

const openaiAPIURL = "https://api.openai.com/v1/chat/completions"

// OpenAIProvider implements the Provider interface for OpenAI's API
type OpenAIProvider struct {
	config *ProviderConfig
	client *http.Client
}

// NewOpenAIProvider creates a new OpenAI provider
func NewOpenAIProvider(config *ProviderConfig) *OpenAIProvider {
	if config.BaseURL == "" {
		config.BaseURL = openaiAPIURL
	}
	if config.Model == "" {
		config.Model = "gpt-4o"
	}
	return &OpenAIProvider{
		config: config,
		client: &http.Client{},
	}
}

func (p *OpenAIProvider) Name() string {
	return "openai"
}

// openaiRequest represents a request to the OpenAI API
type openaiRequest struct {
	Model       string       `json:"model"`
	Messages    []openaiMsg  `json:"messages"`
	MaxTokens   int          `json:"max_tokens,omitempty"`
	Temperature float64      `json:"temperature,omitempty"`
	Tools       []openaiTool `json:"tools,omitempty"`
}

type openaiMsg struct {
	Role       string           `json:"role"`
	Content    string           `json:"content,omitempty"`
	ToolCalls  []openaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type openaiTool struct {
	Type     string         `json:"type"`
	Function openaiFunction `json:"function"`
}

type openaiFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

type openaiToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// openaiResponse represents a response from the OpenAI API
type openaiResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int       `json:"index"`
		Message      openaiMsg `json:"message"`
		FinishReason string    `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

// Streaming types for OpenAI SSE
type openaiStreamResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int               `json:"index"`
		Delta        openaiStreamDelta `json:"delta"`
		FinishReason string            `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage,omitempty"`
}

type openaiStreamDelta struct {
	Role      string                 `json:"role,omitempty"`
	Content   string                 `json:"content,omitempty"`
	ToolCalls []openaiStreamToolCall `json:"tool_calls,omitempty"`
}

type openaiStreamToolCall struct {
	Index    int    `json:"index"`
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function"`
}

func (p *OpenAIProvider) Complete(ctx context.Context, req *CompletionRequest) (*Response, error) {
	if p.config.APIKey == "" {
		return nil, ErrNoAPIKey
	}

	// Convert messages to OpenAI format
	msgs := make([]openaiMsg, 0, len(req.Messages)+1)

	// Add system message if provided
	if req.System != "" {
		msgs = append(msgs, openaiMsg{
			Role:    "system",
			Content: req.System,
		})
	}

	for _, msg := range req.Messages {
		switch msg.Role {
		case RoleSystem:
			msgs = append(msgs, openaiMsg{
				Role:    "system",
				Content: msg.Content,
			})
		case RoleUser:
			msgs = append(msgs, openaiMsg{
				Role:    "user",
				Content: msg.Content,
			})
		case RoleAssistant:
			oaiMsg := openaiMsg{
				Role:    "assistant",
				Content: msg.Content,
			}
			if len(msg.ToolCalls) > 0 {
				oaiMsg.ToolCalls = make([]openaiToolCall, len(msg.ToolCalls))
				for i, tc := range msg.ToolCalls {
					oaiMsg.ToolCalls[i] = openaiToolCall{
						ID:   tc.ID,
						Type: "function",
					}
					oaiMsg.ToolCalls[i].Function.Name = tc.Name
					oaiMsg.ToolCalls[i].Function.Arguments = string(tc.Arguments)
				}
			}
			msgs = append(msgs, oaiMsg)
		case RoleTool:
			msgs = append(msgs, openaiMsg{
				Role:       "tool",
				Content:    msg.Content,
				ToolCallID: msg.ToolCallID,
			})
		}
	}

	// Convert tools to OpenAI format
	var tools []openaiTool
	if len(req.Tools) > 0 {
		tools = make([]openaiTool, len(req.Tools))
		for i, t := range req.Tools {
			tools[i] = openaiTool{
				Type: "function",
				Function: openaiFunction{
					Name:        t.Name,
					Description: t.Description,
					Parameters:  t.Parameters,
				},
			}
		}
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = p.config.MaxTokens
	}

	temp := req.Temperature
	if temp == 0 {
		temp = p.config.Temperature
	}

	openaiReq := openaiRequest{
		Model:       req.Model,
		Messages:    msgs,
		MaxTokens:   maxTokens,
		Temperature: temp,
		Tools:       tools,
	}

	if openaiReq.Model == "" {
		openaiReq.Model = p.config.Model
	}

	body, err := json.Marshal(openaiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.config.BaseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.config.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var openaiResp openaiResponse
	if err := json.Unmarshal(respBody, &openaiResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if openaiResp.Error != nil {
		return nil, fmt.Errorf("%w: %s - %s", ErrAPIError, openaiResp.Error.Type, openaiResp.Error.Message)
	}

	if len(openaiResp.Choices) == 0 {
		return nil, fmt.Errorf("%w: no choices in response", ErrAPIError)
	}

	choice := openaiResp.Choices[0]
	response := &Response{
		Content:    choice.Message.Content,
		StopReason: choice.FinishReason,
		Usage: Usage{
			InputTokens:  openaiResp.Usage.PromptTokens,
			OutputTokens: openaiResp.Usage.CompletionTokens,
		},
	}

	// Convert tool calls
	for _, tc := range choice.Message.ToolCalls {
		response.ToolCalls = append(response.ToolCalls, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: json.RawMessage(tc.Function.Arguments),
		})
	}

	return response, nil
}

// CompleteStream sends messages to the LLM and streams the response
func (p *OpenAIProvider) CompleteStream(ctx context.Context, req *CompletionRequest) (<-chan StreamEvent, error) {
	if p.config.APIKey == "" {
		return nil, ErrNoAPIKey
	}

	// Convert messages to OpenAI format (same as Complete)
	msgs := make([]openaiMsg, 0, len(req.Messages)+1)

	if req.System != "" {
		msgs = append(msgs, openaiMsg{
			Role:    "system",
			Content: req.System,
		})
	}

	for _, msg := range req.Messages {
		switch msg.Role {
		case RoleSystem:
			msgs = append(msgs, openaiMsg{
				Role:    "system",
				Content: msg.Content,
			})
		case RoleUser:
			msgs = append(msgs, openaiMsg{
				Role:    "user",
				Content: msg.Content,
			})
		case RoleAssistant:
			oaiMsg := openaiMsg{
				Role:    "assistant",
				Content: msg.Content,
			}
			if len(msg.ToolCalls) > 0 {
				oaiMsg.ToolCalls = make([]openaiToolCall, len(msg.ToolCalls))
				for i, tc := range msg.ToolCalls {
					oaiMsg.ToolCalls[i] = openaiToolCall{
						ID:   tc.ID,
						Type: "function",
					}
					oaiMsg.ToolCalls[i].Function.Name = tc.Name
					oaiMsg.ToolCalls[i].Function.Arguments = string(tc.Arguments)
				}
			}
			msgs = append(msgs, oaiMsg)
		case RoleTool:
			msgs = append(msgs, openaiMsg{
				Role:       "tool",
				Content:    msg.Content,
				ToolCallID: msg.ToolCallID,
			})
		}
	}

	var tools []openaiTool
	if len(req.Tools) > 0 {
		tools = make([]openaiTool, len(req.Tools))
		for i, t := range req.Tools {
			tools[i] = openaiTool{
				Type: "function",
				Function: openaiFunction{
					Name:        t.Name,
					Description: t.Description,
					Parameters:  t.Parameters,
				},
			}
		}
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = p.config.MaxTokens
	}

	temp := req.Temperature
	if temp == 0 {
		temp = p.config.Temperature
	}

	// Use anonymous struct to include stream fields
	openaiReq := struct {
		Model         string       `json:"model"`
		Messages      []openaiMsg  `json:"messages"`
		MaxTokens     int          `json:"max_tokens,omitempty"`
		Temperature   float64      `json:"temperature,omitempty"`
		Tools         []openaiTool `json:"tools,omitempty"`
		Stream        bool         `json:"stream"`
		StreamOptions *struct {
			IncludeUsage bool `json:"include_usage"`
		} `json:"stream_options,omitempty"`
	}{
		Model:       req.Model,
		Messages:    msgs,
		MaxTokens:   maxTokens,
		Temperature: temp,
		Tools:       tools,
		Stream:      true,
		StreamOptions: &struct {
			IncludeUsage bool `json:"include_usage"`
		}{IncludeUsage: true},
	}

	if openaiReq.Model == "" {
		openaiReq.Model = p.config.Model
	}

	body, err := json.Marshal(openaiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.config.BaseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.config.APIKey)

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
		var inputTokens, outputTokens int
		var stopReason string

		// Track tool calls being built
		toolCalls := make(map[int]*ToolCall)
		toolCallArgs := make(map[int]string)
		toolCallStarted := make(map[int]bool)

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

			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				break
			}

			var streamResp openaiStreamResponse
			if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
				continue
			}

			// Handle usage info
			if streamResp.Usage != nil {
				inputTokens = streamResp.Usage.PromptTokens
				outputTokens = streamResp.Usage.CompletionTokens
			}

			if len(streamResp.Choices) == 0 {
				continue
			}

			choice := streamResp.Choices[0]

			// Handle finish reason
			if choice.FinishReason != "" {
				stopReason = choice.FinishReason
			}

			// Handle text content
			if choice.Delta.Content != "" {
				eventChan <- StreamEvent{
					Type: StreamEventText,
					Text: choice.Delta.Content,
				}
			}

			// Handle tool calls
			for _, tcDelta := range choice.Delta.ToolCalls {
				idx := tcDelta.Index

				// Start of a new tool call
				if tcDelta.ID != "" || tcDelta.Function.Name != "" {
					if !toolCallStarted[idx] {
						tc := &ToolCall{
							ID:   tcDelta.ID,
							Name: tcDelta.Function.Name,
						}
						toolCalls[idx] = tc
						toolCallArgs[idx] = ""
						toolCallStarted[idx] = true
						eventChan <- StreamEvent{
							Type:          StreamEventToolCallStart,
							ToolCall:      tc,
							ToolCallIndex: idx,
						}
					} else {
						// Update existing tool call metadata
						if tcDelta.ID != "" {
							toolCalls[idx].ID = tcDelta.ID
						}
						if tcDelta.Function.Name != "" {
							toolCalls[idx].Name = tcDelta.Function.Name
						}
					}
				}

				// Accumulate arguments
				if tcDelta.Function.Arguments != "" {
					toolCallArgs[idx] += tcDelta.Function.Arguments
					eventChan <- StreamEvent{
						Type:          StreamEventToolCallDelta,
						ToolCallIndex: idx,
						ArgumentDelta: tcDelta.Function.Arguments,
					}
				}
			}
		}

		// Finalize any pending tool calls
		for idx, tc := range toolCalls {
			tc.Arguments = json.RawMessage(toolCallArgs[idx])
			eventChan <- StreamEvent{
				Type:          StreamEventToolCallEnd,
				ToolCall:      tc,
				ToolCallIndex: idx,
			}
		}

		// Send done event
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
