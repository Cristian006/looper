package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/looper-ai/looper/pkg/llm"
	"github.com/looper-ai/looper/pkg/sandbox"
	"github.com/looper-ai/looper/pkg/skills"
	"github.com/looper-ai/looper/pkg/tools"
)

// Agent represents an AI agent with tools and skills
type Agent struct {
	config    *Config
	provider  llm.Provider
	registry  *tools.Registry
	discovery *skills.Discovery
	ctx       *Context
}

// New creates a new agent with the given configuration
func New(config *Config) (*Agent, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// Create LLM provider
	providerConfig := config.GetProviderConfig()
	var provider llm.Provider

	switch config.Provider {
	case "anthropic":
		provider = llm.NewAnthropicProvider(providerConfig)
	case "openai":
		provider = llm.NewOpenAIProvider(providerConfig)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", config.Provider)
	}

	// Create tool registry
	registry := tools.NewRegistry()

	// Create sandbox
	sandboxConfig := sandbox.DefaultConfig(config.WorkspacePath)

	// Configure command blacklist
	if config.DisableBlacklist {
		sandboxConfig.CommandBlacklist = nil
	} else if config.CommandBlacklist != nil {
		sandboxConfig.CommandBlacklist = config.CommandBlacklist
	}
	// else use the default blacklist from sandbox.DefaultConfig

	sb := sandbox.NewProcessSandbox(sandboxConfig)

	// Register built-in tools
	registry.Register(tools.NewReadFileTool(config.WorkspacePath))
	registry.Register(tools.NewWriteFileTool(config.WorkspacePath))
	registry.Register(tools.NewGrepTool(config.WorkspacePath))
	registry.Register(tools.NewListDirTool(config.WorkspacePath))
	registry.Register(tools.NewExecuteTool(sb))
	registry.Register(tools.NewBashTool(sb))

	// Create skill discovery
	discovery := skills.NewDiscovery(config.WorkspacePath)
	discovery.Discover()

	// Create context
	agentCtx := NewContext(config.WorkspacePath)

	agent := &Agent{
		config:    config,
		provider:  provider,
		registry:  registry,
		discovery: discovery,
		ctx:       agentCtx,
	}

	// Auto-load all discovered skills
	allSkills, _ := discovery.GetAll()
	for _, skill := range allSkills {
		agentCtx.LoadSkill(skill)
	}

	return agent, nil
}

// Context returns the agent's conversation context
func (a *Agent) Context() *Context {
	return a.ctx
}

// Registry returns the agent's tool registry
func (a *Agent) Registry() *tools.Registry {
	return a.registry
}

// Discovery returns the skill discovery instance
func (a *Agent) Discovery() *skills.Discovery {
	return a.discovery
}

// LoadSkill loads a skill by name
func (a *Agent) LoadSkill(name string) error {
	skill, err := a.discovery.Get(name)
	if err != nil {
		return fmt.Errorf("failed to load skill %q: %w", name, err)
	}
	if skill == nil {
		return fmt.Errorf("skill %q not found", name)
	}
	a.ctx.LoadSkill(skill)
	return nil
}

// Run executes the agent loop for a user message
func (a *Agent) Run(ctx context.Context, userMessage string) (string, error) {
	// Add user message to context
	a.ctx.AddUserMessage(userMessage)

	// Run the agent loop
	for {
		// Check iteration limit
		if a.config.MaxIterations > 0 && a.ctx.IterationCount >= a.config.MaxIterations {
			return "", fmt.Errorf("max iterations (%d) reached", a.config.MaxIterations)
		}
		a.ctx.IterationCount++

		// Check context cancellation
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		// Build system prompt with active skills
		systemPrompt := a.config.SystemPrompt + a.ctx.GetSkillPrompt()

		// Build tool definitions
		toolDefs := tools.ToDefinitions(a.registry.List())

		// Create completion request
		req := &llm.CompletionRequest{
			Model:     a.config.Model,
			Messages:  a.ctx.Messages,
			Tools:     toolDefs,
			MaxTokens: a.config.MaxTokens,
			System:    systemPrompt,
		}

		// Call LLM
		resp, err := a.provider.Complete(ctx, req)
		if err != nil {
			return "", fmt.Errorf("LLM error: %w", err)
		}

		// Update usage stats
		a.ctx.UpdateUsage(resp.Usage)

		// Handle response
		if len(resp.ToolCalls) > 0 {
			// Add assistant message with tool calls
			a.ctx.AddMessage(llm.NewAssistantToolCallMessage(resp.ToolCalls))

			// Execute each tool call
			for _, tc := range resp.ToolCalls {
				result, err := a.executeTool(ctx, tc)
				if err != nil {
					result = fmt.Sprintf("Error: %s", err.Error())
				}
				a.ctx.AddToolResult(tc.ID, result)
			}

			// Continue the loop to get next response
			continue
		}

		// No tool calls - add final response and return
		if resp.Content != "" {
			a.ctx.AddAssistantMessage(resp.Content)
		}

		return resp.Content, nil
	}
}

// executeTool runs a tool and returns the result
func (a *Agent) executeTool(ctx context.Context, tc llm.ToolCall) (string, error) {
	tool, ok := a.registry.Get(tc.Name)
	if !ok {
		return "", fmt.Errorf("unknown tool: %s", tc.Name)
	}

	// Parse arguments
	var args map[string]interface{}
	if err := json.Unmarshal(tc.Arguments, &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	// Execute tool
	result, err := tool.Execute(ctx, args)
	if err != nil {
		return "", err
	}

	return result, nil
}

// Reset clears the conversation context
func (a *Agent) Reset() {
	a.ctx.Clear()
}

// SetSystemPrompt updates the system prompt
func (a *Agent) SetSystemPrompt(prompt string) {
	a.config.SystemPrompt = prompt
}

// StreamHandler handles different types of streaming events
type StreamHandler struct {
	OnText      func(text string)
	OnToolStart func(toolCall llm.ToolCall)
	OnToolEnd   func(toolCall llm.ToolCall, result string, err error)
	OnUsage     func(inputTokens, outputTokens int)
	OnDone      func()
}

// RunStream executes the agent loop with streaming output
func (a *Agent) RunStream(ctx context.Context, userMessage string, handler *StreamHandler) (string, error) {
	// Check if provider supports streaming
	streamProvider, ok := a.provider.(llm.StreamProvider)
	if !ok {
		// Fall back to non-streaming
		result, err := a.Run(ctx, userMessage)
		if err != nil {
			return "", err
		}
		if handler != nil && handler.OnText != nil {
			handler.OnText(result)
		}
		if handler != nil && handler.OnDone != nil {
			handler.OnDone()
		}
		return result, nil
	}

	// Add user message to context
	a.ctx.AddUserMessage(userMessage)

	var finalContent string

	// Run the agent loop
	for {
		// Check iteration limit
		if a.config.MaxIterations > 0 && a.ctx.IterationCount >= a.config.MaxIterations {
			return "", fmt.Errorf("max iterations (%d) reached", a.config.MaxIterations)
		}
		a.ctx.IterationCount++

		// Check context cancellation
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		// Build system prompt with active skills
		systemPrompt := a.config.SystemPrompt + a.ctx.GetSkillPrompt()

		// Build tool definitions
		toolDefs := tools.ToDefinitions(a.registry.List())

		// Create completion request
		req := &llm.CompletionRequest{
			Model:     a.config.Model,
			Messages:  a.ctx.Messages,
			Tools:     toolDefs,
			MaxTokens: a.config.MaxTokens,
			System:    systemPrompt,
		}

		// Start streaming
		eventChan, err := streamProvider.CompleteStream(ctx, req)
		if err != nil {
			return "", fmt.Errorf("LLM error: %w", err)
		}

		// Process stream events
		var content string
		var toolCalls []llm.ToolCall
		currentToolCalls := make(map[int]*llm.ToolCall)
		var usage llm.Usage

		for event := range eventChan {
			switch event.Type {
			case llm.StreamEventText:
				content += event.Text
				if handler != nil && handler.OnText != nil {
					handler.OnText(event.Text)
				}

			case llm.StreamEventToolCallStart:
				tc := &llm.ToolCall{
					ID:   event.ToolCall.ID,
					Name: event.ToolCall.Name,
				}
				currentToolCalls[event.ToolCallIndex] = tc

			case llm.StreamEventToolCallEnd:
				if tc, ok := currentToolCalls[event.ToolCallIndex]; ok {
					tc.Arguments = event.ToolCall.Arguments
					toolCalls = append(toolCalls, *tc)
				}

			case llm.StreamEventDone:
				usage = event.Usage

			case llm.StreamEventError:
				return "", event.Error
			}
		}

		// Update usage stats
		a.ctx.UpdateUsage(usage)
		if handler != nil && handler.OnUsage != nil {
			handler.OnUsage(usage.InputTokens, usage.OutputTokens)
		}

		// Handle tool calls
		if len(toolCalls) > 0 {
			// Add assistant message with tool calls
			a.ctx.AddMessage(llm.NewAssistantToolCallMessage(toolCalls))

			// Execute each tool call
			for _, tc := range toolCalls {
				if handler != nil && handler.OnToolStart != nil {
					handler.OnToolStart(tc)
				}

				result, err := a.executeTool(ctx, tc)
				toolErr := err
				if err != nil {
					result = fmt.Sprintf("Error: %s", err.Error())
				}

				if handler != nil && handler.OnToolEnd != nil {
					handler.OnToolEnd(tc, result, toolErr)
				}

				a.ctx.AddToolResult(tc.ID, result)
			}

			// Continue the loop to get next response
			continue
		}

		// No tool calls - add final response and return
		if content != "" {
			a.ctx.AddAssistantMessage(content)
		}

		finalContent = content

		if handler != nil && handler.OnDone != nil {
			handler.OnDone()
		}

		return finalContent, nil
	}
}
