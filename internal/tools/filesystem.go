package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// resolvePath resolves a path and optionally enforces directory restriction.
func resolvePath(path string, allowedDir string) (string, error) {
	if strings.HasPrefix(path, "~") {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, path[1:])
	}
	resolved, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if allowedDir != "" {
		absAllowed, _ := filepath.Abs(allowedDir)
		if !strings.HasPrefix(resolved, absAllowed) {
			return "", fmt.Errorf("path %s is outside allowed directory %s", path, allowedDir)
		}
	}
	return resolved, nil
}

// ReadFileTool reads file contents.
type ReadFileTool struct{ AllowedDir string }

func (t *ReadFileTool) Name() string        { return "read_file" }
func (t *ReadFileTool) Description() string  { return "Read the contents of a file at the given path." }
func (t *ReadFileTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string", "description": "The file path to read"},
		},
		"required": []string{"path"},
	}
}

func (t *ReadFileTool) Execute(_ context.Context, args map[string]any) (string, error) {
	path, _ := args["path"].(string)
	resolved, err := resolvePath(path, t.AllowedDir)
	if err != nil {
		return fmt.Sprintf("Error: %v", err), nil
	}
	info, err := os.Stat(resolved)
	if os.IsNotExist(err) {
		return fmt.Sprintf("Error: File not found: %s", path), nil
	}
	if err != nil {
		return fmt.Sprintf("Error reading file: %v", err), nil
	}
	if info.IsDir() {
		return fmt.Sprintf("Error: Not a file: %s", path), nil
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return fmt.Sprintf("Error reading file: %v", err), nil
	}
	return string(data), nil
}

// WriteFileTool writes content to a file.
type WriteFileTool struct{ AllowedDir string }

func (t *WriteFileTool) Name() string        { return "write_file" }
func (t *WriteFileTool) Description() string  { return "Write content to a file. Creates parent directories." }
func (t *WriteFileTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":    map[string]any{"type": "string", "description": "The file path to write to"},
			"content": map[string]any{"type": "string", "description": "The content to write"},
		},
		"required": []string{"path", "content"},
	}
}

func (t *WriteFileTool) Execute(_ context.Context, args map[string]any) (string, error) {
	path, _ := args["path"].(string)
	content, _ := args["content"].(string)

	resolved, err := resolvePath(path, t.AllowedDir)
	if err != nil {
		return fmt.Sprintf("Error: %v", err), nil
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0755); err != nil {
		return fmt.Sprintf("Error writing file: %v", err), nil
	}
	if err := os.WriteFile(resolved, []byte(content), 0644); err != nil {
		return fmt.Sprintf("Error writing file: %v", err), nil
	}
	return fmt.Sprintf("Successfully wrote %d bytes to %s", len(content), path), nil
}

// EditFileTool edits a file by replacing old text with new text.
type EditFileTool struct{ AllowedDir string }

func (t *EditFileTool) Name() string        { return "edit_file" }
func (t *EditFileTool) Description() string  { return "Edit a file by replacing old_text with new_text." }
func (t *EditFileTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":     map[string]any{"type": "string", "description": "The file path to edit"},
			"old_text": map[string]any{"type": "string", "description": "The exact text to find"},
			"new_text": map[string]any{"type": "string", "description": "The replacement text"},
		},
		"required": []string{"path", "old_text", "new_text"},
	}
}

func (t *EditFileTool) Execute(_ context.Context, args map[string]any) (string, error) {
	path, _ := args["path"].(string)
	oldText, _ := args["old_text"].(string)
	newText, _ := args["new_text"].(string)

	resolved, err := resolvePath(path, t.AllowedDir)
	if err != nil {
		return fmt.Sprintf("Error: %v", err), nil
	}
	data, err := os.ReadFile(resolved)
	if os.IsNotExist(err) {
		return fmt.Sprintf("Error: File not found: %s", path), nil
	}
	if err != nil {
		return fmt.Sprintf("Error reading file: %v", err), nil
	}

	content := string(data)
	count := strings.Count(content, oldText)
	if count == 0 {
		return "Error: old_text not found in file. Make sure it matches exactly.", nil
	}
	if count > 1 {
		return fmt.Sprintf("Warning: old_text appears %d times. Please provide more context.", count), nil
	}

	newContent := strings.Replace(content, oldText, newText, 1)
	if err := os.WriteFile(resolved, []byte(newContent), 0644); err != nil {
		return fmt.Sprintf("Error writing file: %v", err), nil
	}
	return fmt.Sprintf("Successfully edited %s", path), nil
}

// ListDirTool lists directory contents.
type ListDirTool struct{ AllowedDir string }

func (t *ListDirTool) Name() string        { return "list_dir" }
func (t *ListDirTool) Description() string  { return "List the contents of a directory." }
func (t *ListDirTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string", "description": "The directory path to list"},
		},
		"required": []string{"path"},
	}
}

func (t *ListDirTool) Execute(_ context.Context, args map[string]any) (string, error) {
	path, _ := args["path"].(string)
	resolved, err := resolvePath(path, t.AllowedDir)
	if err != nil {
		return fmt.Sprintf("Error: %v", err), nil
	}
	info, err := os.Stat(resolved)
	if os.IsNotExist(err) {
		return fmt.Sprintf("Error: Directory not found: %s", path), nil
	}
	if !info.IsDir() {
		return fmt.Sprintf("Error: Not a directory: %s", path), nil
	}

	entries, err := os.ReadDir(resolved)
	if err != nil {
		return fmt.Sprintf("Error listing directory: %v", err), nil
	}
	if len(entries) == 0 {
		return fmt.Sprintf("Directory %s is empty", path), nil
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	var lines []string
	for _, e := range entries {
		prefix := "📄 "
		if e.IsDir() {
			prefix = "📁 "
		}
		lines = append(lines, prefix+e.Name())
	}
	return strings.Join(lines, "\n"), nil
}
