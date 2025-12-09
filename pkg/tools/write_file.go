package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WriteFileTool writes content to files
type WriteFileTool struct {
	workspaceRoot string
}

// NewWriteFileTool creates a new write file tool
func NewWriteFileTool(workspaceRoot string) *WriteFileTool {
	return &WriteFileTool{
		workspaceRoot: workspaceRoot,
	}
}

func (t *WriteFileTool) Name() string {
	return "write_file"
}

func (t *WriteFileTool) Description() string {
	return "Write content to a file in the workspace. Creates the file if it doesn't exist, or overwrites it if it does. Creates parent directories as needed."
}

func (t *WriteFileTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "The file path relative to the workspace root",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "The content to write to the file",
			},
		},
		"required": []string{"path", "content"},
	}
}

func (t *WriteFileTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	path, ok := args["path"].(string)
	if !ok || path == "" {
		return "", fmt.Errorf("path is required")
	}

	content, ok := args["content"].(string)
	if !ok {
		return "", fmt.Errorf("content is required")
	}

	fullPath := filepath.Join(t.workspaceRoot, path)

	// Validate path is within workspace
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}
	absWorkspace, _ := filepath.Abs(t.workspaceRoot)
	if !strings.HasPrefix(absPath, absWorkspace) {
		return "", fmt.Errorf("path must be within workspace")
	}

	// Check context cancellation
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	// Create parent directories if needed
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directories: %w", err)
	}

	// Check if file exists (for response message)
	_, err = os.Stat(fullPath)
	fileExists := !os.IsNotExist(err)

	// Write file
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	if fileExists {
		return fmt.Sprintf("Successfully updated file: %s", path), nil
	}
	return fmt.Sprintf("Successfully created file: %s", path), nil
}
