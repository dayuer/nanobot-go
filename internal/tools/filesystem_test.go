package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadFileTool_Contract(t *testing.T) {
	RunToolContractTests(t, &ReadFileTool{})
}

func TestWriteFileTool_Contract(t *testing.T) {
	RunToolContractTests(t, &WriteFileTool{})
}

func TestEditFileTool_Contract(t *testing.T) {
	RunToolContractTests(t, &EditFileTool{})
}

func TestListDirTool_Contract(t *testing.T) {
	RunToolContractTests(t, &ListDirTool{})
}

func TestReadFileTool_Execute(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello world"), 0644)

	tool := &ReadFileTool{}
	result, err := tool.Execute(context.Background(), map[string]any{"path": path})
	require.NoError(t, err)
	assert.Equal(t, "hello world", result)
}

func TestReadFileTool_NotFound(t *testing.T) {
	tool := &ReadFileTool{}
	result, _ := tool.Execute(context.Background(), map[string]any{"path": "/nonexistent/file.txt"})
	assert.Contains(t, result, "File not found")
}

func TestReadFileTool_AllowedDir(t *testing.T) {
	tool := &ReadFileTool{AllowedDir: "/tmp/safe"}
	result, _ := tool.Execute(context.Background(), map[string]any{"path": "/etc/passwd"})
	assert.Contains(t, result, "outside allowed directory")
}

func TestWriteFileTool_Execute(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "new.txt")

	tool := &WriteFileTool{}
	result, err := tool.Execute(context.Background(), map[string]any{
		"path": path, "content": "hi",
	})
	require.NoError(t, err)
	assert.Contains(t, result, "Successfully wrote")

	data, _ := os.ReadFile(path)
	assert.Equal(t, "hi", string(data))
}

func TestEditFileTool_Execute(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "edit.txt")
	os.WriteFile(path, []byte("hello world"), 0644)

	tool := &EditFileTool{}
	result, err := tool.Execute(context.Background(), map[string]any{
		"path": path, "old_text": "world", "new_text": "go",
	})
	require.NoError(t, err)
	assert.Contains(t, result, "Successfully edited")

	data, _ := os.ReadFile(path)
	assert.Equal(t, "hello go", string(data))
}

func TestEditFileTool_NotFound(t *testing.T) {
	tool := &EditFileTool{}
	result, _ := tool.Execute(context.Background(), map[string]any{
		"path": "/nonexistent", "old_text": "a", "new_text": "b",
	})
	assert.Contains(t, result, "not found")
}

func TestEditFileTool_OldTextMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "edit.txt")
	os.WriteFile(path, []byte("hello"), 0644)

	tool := &EditFileTool{}
	result, _ := tool.Execute(context.Background(), map[string]any{
		"path": path, "old_text": "xyz", "new_text": "a",
	})
	assert.Contains(t, result, "old_text not found")
}

func TestEditFileTool_Ambiguous(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "edit.txt")
	os.WriteFile(path, []byte("aaa bbb aaa"), 0644)

	tool := &EditFileTool{}
	result, _ := tool.Execute(context.Background(), map[string]any{
		"path": path, "old_text": "aaa", "new_text": "ccc",
	})
	assert.Contains(t, result, "appears 2 times")
}

func TestListDirTool_Execute(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "subdir"), 0755)
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("x"), 0644)

	tool := &ListDirTool{}
	result, err := tool.Execute(context.Background(), map[string]any{"path": dir})
	require.NoError(t, err)
	assert.Contains(t, result, "📁 subdir")
	assert.Contains(t, result, "📄 file.txt")
}

func TestListDirTool_Empty(t *testing.T) {
	tool := &ListDirTool{}
	result, _ := tool.Execute(context.Background(), map[string]any{"path": t.TempDir()})
	assert.Contains(t, result, "is empty")
}
