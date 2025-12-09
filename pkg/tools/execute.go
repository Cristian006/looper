package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/looper-ai/looper/pkg/sandbox"
)

// ExecuteTool runs code in a sandboxed environment
type ExecuteTool struct {
	sandbox sandbox.Sandbox
}

// NewExecuteTool creates a new execute tool
func NewExecuteTool(sb sandbox.Sandbox) *ExecuteTool {
	return &ExecuteTool{
		sandbox: sb,
	}
}

func (t *ExecuteTool) Name() string {
	return "execute"
}

func (t *ExecuteTool) Description() string {
	return "Execute code or shell commands in a sandboxed environment. Supports bash, python, node, and go."
}

func (t *ExecuteTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"language": map[string]interface{}{
				"type":        "string",
				"description": "The language/interpreter to use: 'bash', 'python', 'node', or 'go'",
				"enum":        []string{"bash", "python", "node", "go"},
			},
			"code": map[string]interface{}{
				"type":        "string",
				"description": "The code to execute",
			},
		},
		"required": []string{"language", "code"},
	}
}

func (t *ExecuteTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	language, ok := args["language"].(string)
	if !ok || language == "" {
		return "", fmt.Errorf("language is required")
	}

	code, ok := args["code"].(string)
	if !ok || code == "" {
		return "", fmt.Errorf("code is required")
	}

	// Map language to interpreter
	var interpreter string
	switch language {
	case "bash":
		interpreter = "bash"
	case "python":
		interpreter = "python3"
	case "node":
		interpreter = "node"
	case "go":
		interpreter = "go"
	default:
		return "", fmt.Errorf("unsupported language: %s", language)
	}

	result, err := t.sandbox.ExecuteScript(ctx, interpreter, code)
	if err != nil {
		return "", fmt.Errorf("execution failed: %w", err)
	}

	// Format output
	var output strings.Builder

	if result.TimedOut {
		output.WriteString("⚠️ Execution timed out\n\n")
	}

	if result.Stdout != "" {
		output.WriteString("STDOUT:\n")
		output.WriteString(result.Stdout)
		if !strings.HasSuffix(result.Stdout, "\n") {
			output.WriteString("\n")
		}
	}

	if result.Stderr != "" {
		output.WriteString("\nSTDERR:\n")
		output.WriteString(result.Stderr)
		if !strings.HasSuffix(result.Stderr, "\n") {
			output.WriteString("\n")
		}
	}

	output.WriteString(fmt.Sprintf("\nExit code: %d", result.ExitCode))
	output.WriteString(fmt.Sprintf("\nDuration: %s", result.Duration))

	return output.String(), nil
}

// BashTool runs bash commands directly
type BashTool struct {
	sandbox sandbox.Sandbox
}

// NewBashTool creates a new bash tool
func NewBashTool(sb sandbox.Sandbox) *BashTool {
	return &BashTool{
		sandbox: sb,
	}
}

func (t *BashTool) Name() string {
	return "bash"
}

func (t *BashTool) Description() string {
	return "Execute a bash command in a sandboxed environment."
}

func (t *BashTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "The bash command to execute",
			},
		},
		"required": []string{"command"},
	}
}

func (t *BashTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	command, ok := args["command"].(string)
	if !ok || command == "" {
		return "", fmt.Errorf("command is required")
	}

	result, err := t.sandbox.Execute(ctx, "bash", []string{"-c", command})
	if err != nil {
		return "", fmt.Errorf("execution failed: %w", err)
	}

	// Format output
	var output strings.Builder

	if result.TimedOut {
		output.WriteString("⚠️ Execution timed out\n\n")
	}

	if result.Stdout != "" {
		output.WriteString(result.Stdout)
		if !strings.HasSuffix(result.Stdout, "\n") {
			output.WriteString("\n")
		}
	}

	if result.Stderr != "" {
		if output.Len() > 0 {
			output.WriteString("\n")
		}
		output.WriteString("STDERR:\n")
		output.WriteString(result.Stderr)
	}

	if result.ExitCode != 0 {
		output.WriteString(fmt.Sprintf("\nExit code: %d", result.ExitCode))
	}

	return output.String(), nil
}
