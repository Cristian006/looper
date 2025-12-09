package tools

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ReadFileTool reads file contents
type ReadFileTool struct {
	workspaceRoot string
}

// NewReadFileTool creates a new read file tool
func NewReadFileTool(workspaceRoot string) *ReadFileTool {
	return &ReadFileTool{
		workspaceRoot: workspaceRoot,
	}
}

func (t *ReadFileTool) Name() string {
	return "read_file"
}

func (t *ReadFileTool) Description() string {
	return "Read the contents of a file from the workspace. Can optionally read specific line ranges."
}

func (t *ReadFileTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "The file path relative to the workspace root",
			},
			"start_line": map[string]interface{}{
				"type":        "integer",
				"description": "The starting line number (1-indexed). If not provided, reads from the beginning.",
			},
			"end_line": map[string]interface{}{
				"type":        "integer",
				"description": "The ending line number (inclusive). If not provided, reads to the end.",
			},
		},
		"required": []string{"path"},
	}
}

func (t *ReadFileTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	path, ok := args["path"].(string)
	if !ok || path == "" {
		return "", fmt.Errorf("path is required")
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

	// Check if file exists
	info, err := os.Stat(fullPath)
	if os.IsNotExist(err) {
		return "", fmt.Errorf("file not found: %s", path)
	}
	if err != nil {
		return "", fmt.Errorf("cannot access file: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("path is a directory, not a file")
	}

	startLine := 0
	if sl, ok := args["start_line"].(float64); ok {
		startLine = int(sl)
	}

	endLine := 0
	if el, ok := args["end_line"].(float64); ok {
		endLine = int(el)
	}

	// Read file
	file, err := os.Open(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++

		// Check context cancellation
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		// Skip lines before start_line
		if startLine > 0 && lineNum < startLine {
			continue
		}

		// Stop after end_line
		if endLine > 0 && lineNum > endLine {
			break
		}

		lines = append(lines, fmt.Sprintf("%6d|%s", lineNum, scanner.Text()))
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error reading file: %w", err)
	}

	if len(lines) == 0 {
		if startLine > 0 || endLine > 0 {
			return "No lines in the specified range.", nil
		}
		return "File is empty.", nil
	}

	return strings.Join(lines, "\n"), nil
}
