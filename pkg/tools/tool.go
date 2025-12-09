package tools

import (
	"context"

	"github.com/looper-ai/looper/pkg/llm"
)

// Tool is the interface that all agent tools must implement
type Tool interface {
	// Name returns the unique name of the tool
	Name() string

	// Description returns a description of what the tool does
	Description() string

	// Schema returns the JSON schema for the tool's parameters
	Schema() map[string]interface{}

	// Execute runs the tool with the given arguments and returns the result
	Execute(ctx context.Context, args map[string]interface{}) (string, error)
}

// ToDefinition converts a Tool to an LLM ToolDefinition
func ToDefinition(t Tool) llm.ToolDefinition {
	return llm.ToolDefinition{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters:  t.Schema(),
	}
}

// ToDefinitions converts multiple Tools to LLM ToolDefinitions
func ToDefinitions(tools []Tool) []llm.ToolDefinition {
	defs := make([]llm.ToolDefinition, len(tools))
	for i, t := range tools {
		defs[i] = ToDefinition(t)
	}
	return defs
}
