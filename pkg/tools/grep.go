package tools

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// GrepTool searches for patterns in files
type GrepTool struct {
	workspaceRoot string
}

// NewGrepTool creates a new grep tool
func NewGrepTool(workspaceRoot string) *GrepTool {
	return &GrepTool{
		workspaceRoot: workspaceRoot,
	}
}

func (t *GrepTool) Name() string {
	return "grep"
}

func (t *GrepTool) Description() string {
	return "Search for a regex pattern in files within the workspace. Returns matching lines with file paths and line numbers."
}

func (t *GrepTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"pattern": map[string]interface{}{
				"type":        "string",
				"description": "The regex pattern to search for",
			},
			"path": map[string]interface{}{
				"type":        "string",
				"description": "The file or directory path to search in (relative to workspace root). Defaults to workspace root.",
			},
			"include": map[string]interface{}{
				"type":        "string",
				"description": "File pattern to include (e.g., '*.go', '*.py'). Defaults to all files.",
			},
			"case_insensitive": map[string]interface{}{
				"type":        "boolean",
				"description": "Whether to perform case-insensitive matching",
			},
			"max_results": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum number of results to return. Defaults to 100.",
			},
		},
		"required": []string{"pattern"},
	}
}

func (t *GrepTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	pattern, ok := args["pattern"].(string)
	if !ok || pattern == "" {
		return "", fmt.Errorf("pattern is required")
	}

	searchPath := t.workspaceRoot
	if p, ok := args["path"].(string); ok && p != "" {
		searchPath = filepath.Join(t.workspaceRoot, p)
	}

	// Validate path is within workspace
	absPath, err := filepath.Abs(searchPath)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}
	absWorkspace, _ := filepath.Abs(t.workspaceRoot)
	if !strings.HasPrefix(absPath, absWorkspace) {
		return "", fmt.Errorf("path must be within workspace")
	}

	caseInsensitive := false
	if ci, ok := args["case_insensitive"].(bool); ok {
		caseInsensitive = ci
	}

	maxResults := 100
	if mr, ok := args["max_results"].(float64); ok {
		maxResults = int(mr)
	}

	include := ""
	if inc, ok := args["include"].(string); ok {
		include = inc
	}

	// Compile regex
	flags := ""
	if caseInsensitive {
		flags = "(?i)"
	}
	re, err := regexp.Compile(flags + pattern)
	if err != nil {
		return "", fmt.Errorf("invalid regex pattern: %w", err)
	}

	var results []string
	resultCount := 0

	err = filepath.Walk(searchPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files we can't access
		}

		// Skip directories and hidden files
		if info.IsDir() {
			if strings.HasPrefix(info.Name(), ".") && path != searchPath {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip hidden files
		if strings.HasPrefix(info.Name(), ".") {
			return nil
		}

		// Apply include filter
		if include != "" {
			matched, _ := filepath.Match(include, info.Name())
			if !matched {
				return nil
			}
		}

		// Skip binary files (simple heuristic)
		if info.Size() > 10*1024*1024 { // Skip files larger than 10MB
			return nil
		}

		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		file, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer file.Close()

		relPath, _ := filepath.Rel(t.workspaceRoot, path)
		scanner := bufio.NewScanner(file)
		lineNum := 0

		for scanner.Scan() {
			lineNum++
			line := scanner.Text()

			if re.MatchString(line) {
				results = append(results, fmt.Sprintf("%s:%d: %s", relPath, lineNum, line))
				resultCount++

				if resultCount >= maxResults {
					results = append(results, fmt.Sprintf("\n... truncated (showing %d of potentially more results)", maxResults))
					return filepath.SkipAll
				}
			}
		}

		return nil
	})

	if err != nil && err != filepath.SkipAll {
		return "", fmt.Errorf("search failed: %w", err)
	}

	if len(results) == 0 {
		return "No matches found.", nil
	}

	return strings.Join(results, "\n"), nil
}
