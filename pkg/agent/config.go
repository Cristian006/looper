package agent

import (
	"os"

	"github.com/looper-ai/looper/pkg/llm"
)

// Config holds the agent configuration
type Config struct {
	// Provider is the LLM provider to use ("anthropic" or "openai")
	Provider string

	// Model is the model to use (e.g., "claude-sonnet-4-20250514", "gpt-4o")
	Model string

	// WorkspacePath is the root directory for file operations
	WorkspacePath string

	// SystemPrompt is the base system prompt for the agent
	SystemPrompt string

	// MaxIterations limits the number of tool call iterations (0 = unlimited)
	MaxIterations int

	// MaxTokens is the maximum number of tokens in a response
	MaxTokens int

	// Temperature controls response randomness
	Temperature float64

	// ProviderConfig holds provider-specific configuration
	ProviderConfig *llm.ProviderConfig

	// CommandBlacklist is a list of command patterns to block
	// Set to nil to use default blacklist, empty slice to disable
	CommandBlacklist []string

	// DisableBlacklist disables the command blacklist entirely
	DisableBlacklist bool
}

// DefaultConfig returns a default agent configuration
func DefaultConfig() *Config {
	return &Config{
		Provider:      "anthropic",
		Model:         "claude-sonnet-4-20250514",
		WorkspacePath: ".",
		SystemPrompt:  defaultSystemPrompt,
		MaxIterations: 50,
		MaxTokens:     4096,
		Temperature:   0.7,
	}
}

// LoadFromEnv populates configuration from environment variables
func (c *Config) LoadFromEnv() {
	if provider := os.Getenv("LOOPER_PROVIDER"); provider != "" {
		c.Provider = provider
	}
	if model := os.Getenv("LOOPER_MODEL"); model != "" {
		c.Model = model
	}
	if workspace := os.Getenv("LOOPER_WORKSPACE"); workspace != "" {
		c.WorkspacePath = workspace
	}
}

// GetProviderConfig returns the LLM provider configuration
func (c *Config) GetProviderConfig() *llm.ProviderConfig {
	if c.ProviderConfig != nil {
		return c.ProviderConfig
	}

	config := llm.DefaultConfig()
	config.Model = c.Model
	config.MaxTokens = c.MaxTokens
	config.Temperature = c.Temperature

	// Load API keys from environment
	switch c.Provider {
	case "anthropic":
		config.APIKey = os.Getenv("ANTHROPIC_API_KEY")
	case "openai":
		config.APIKey = os.Getenv("OPENAI_API_KEY")
	}

	return config
}

const defaultSystemPrompt = `You are an AI assistant with access to tools for reading, writing, and executing code in a workspace environment. You can help users with various coding tasks.

## Core Capabilities
- Read and write files in the workspace
- Search for patterns in files (grep)
- List directory contents  
- Execute code in bash, Python, Node.js, or Go

## Workflow
1. Understand what the user wants to accomplish
2. Explore the codebase using read_file, grep, and list_dir
3. Make changes carefully using write_file
4. Test changes using the execute tool when appropriate

Always explain what you're doing and why.`
