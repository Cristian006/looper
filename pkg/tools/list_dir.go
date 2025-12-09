package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ListDirTool lists directory contents
type ListDirTool struct {
	workspaceRoot string
}

// NewListDirTool creates a new list directory tool
func NewListDirTool(workspaceRoot string) *ListDirTool {
	return &ListDirTool{
		workspaceRoot: workspaceRoot,
	}
}

func (t *ListDirTool) Name() string {
	return "list_dir"
}

func (t *ListDirTool) Description() string {
	return "List the contents of a directory in the workspace. Shows files and subdirectories."
}

func (t *ListDirTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "The directory path relative to the workspace root. Defaults to workspace root.",
			},
			"recursive": map[string]interface{}{
				"type":        "boolean",
				"description": "Whether to list recursively. Defaults to false.",
			},
			"max_depth": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum depth for recursive listing. Defaults to 3.",
			},
		},
		"required": []string{},
	}
}

func (t *ListDirTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	path := ""
	if p, ok := args["path"].(string); ok {
		path = p
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

	// Check if path exists and is a directory
	info, err := os.Stat(fullPath)
	if os.IsNotExist(err) {
		return "", fmt.Errorf("directory not found: %s", path)
	}
	if err != nil {
		return "", fmt.Errorf("cannot access directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("path is not a directory")
	}

	recursive := false
	if r, ok := args["recursive"].(bool); ok {
		recursive = r
	}

	maxDepth := 3
	if md, ok := args["max_depth"].(float64); ok {
		maxDepth = int(md)
	}

	var entries []string

	if recursive {
		err = t.listRecursive(ctx, fullPath, "", 0, maxDepth, &entries)
	} else {
		err = t.listFlat(ctx, fullPath, &entries)
	}

	if err != nil {
		return "", err
	}

	if len(entries) == 0 {
		return "Directory is empty.", nil
	}

	sort.Strings(entries)
	return strings.Join(entries, "\n"), nil
}

func (t *ListDirTool) listFlat(ctx context.Context, dir string, entries *[]string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	items, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read directory: %w", err)
	}

	for _, item := range items {
		// Skip hidden files
		if strings.HasPrefix(item.Name(), ".") {
			continue
		}

		name := item.Name()
		if item.IsDir() {
			name += "/"
		}
		*entries = append(*entries, name)
	}

	return nil
}

func (t *ListDirTool) listRecursive(ctx context.Context, basePath, relPath string, depth, maxDepth int, entries *[]string) error {
	if depth > maxDepth {
		return nil
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	fullPath := filepath.Join(basePath, relPath)
	items, err := os.ReadDir(fullPath)
	if err != nil {
		return nil // Skip directories we can't read
	}

	for _, item := range items {
		// Skip hidden files
		if strings.HasPrefix(item.Name(), ".") {
			continue
		}

		itemRelPath := filepath.Join(relPath, item.Name())
		if item.IsDir() {
			*entries = append(*entries, itemRelPath+"/")
			if err := t.listRecursive(ctx, basePath, itemRelPath, depth+1, maxDepth, entries); err != nil {
				return err
			}
		} else {
			*entries = append(*entries, itemRelPath)
		}
	}

	return nil
}
