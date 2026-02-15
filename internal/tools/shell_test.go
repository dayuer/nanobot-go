package tools

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecTool_Contract(t *testing.T) {
	RunToolContractTests(t, NewExecTool())
}

func TestExecTool_SimpleCommand(t *testing.T) {
	tool := NewExecTool()
	result, err := tool.Execute(context.Background(), map[string]any{
		"command": "echo hello",
	})
	require.NoError(t, err)
	assert.Contains(t, result, "hello")
}

func TestExecTool_WorkingDir(t *testing.T) {
	tool := NewExecTool()
	dir := t.TempDir()
	result, err := tool.Execute(context.Background(), map[string]any{
		"command":     "pwd",
		"working_dir": dir,
	})
	require.NoError(t, err)
	assert.Contains(t, result, dir)
}

func TestExecTool_Stderr(t *testing.T) {
	tool := NewExecTool()
	result, err := tool.Execute(context.Background(), map[string]any{
		"command": "echo err >&2",
	})
	require.NoError(t, err)
	assert.Contains(t, result, "STDERR:")
	assert.Contains(t, result, "err")
}

func TestExecTool_ExitCode(t *testing.T) {
	tool := NewExecTool()
	result, err := tool.Execute(context.Background(), map[string]any{
		"command": "exit 42",
	})
	require.NoError(t, err)
	assert.Contains(t, result, "Exit code: 42")
}

func TestExecTool_Timeout(t *testing.T) {
	tool := NewExecTool()
	tool.Timeout = 500 * time.Millisecond
	result, err := tool.Execute(context.Background(), map[string]any{
		"command": "sleep 10",
	})
	require.NoError(t, err)
	assert.Contains(t, result, "timed out")
}

func TestExecTool_DenyPattern_RmRf(t *testing.T) {
	tool := NewExecTool()
	result, err := tool.Execute(context.Background(), map[string]any{
		"command": "rm -rf /",
	})
	require.NoError(t, err)
	assert.Contains(t, result, "blocked by safety guard")
}

func TestExecTool_DenyPattern_Shutdown(t *testing.T) {
	tool := NewExecTool()
	result, err := tool.Execute(context.Background(), map[string]any{
		"command": "shutdown now",
	})
	require.NoError(t, err)
	assert.Contains(t, result, "blocked by safety guard")
}

func TestExecTool_AllowPatterns(t *testing.T) {
	tool := NewExecTool()
	tool.AllowPatterns = []string{`^ls\b`}

	result, err := tool.Execute(context.Background(), map[string]any{
		"command": "echo not-allowed",
	})
	require.NoError(t, err)
	assert.Contains(t, result, "not in allowlist")

	result, err = tool.Execute(context.Background(), map[string]any{
		"command": "ls /tmp",
	})
	require.NoError(t, err)
	assert.NotContains(t, result, "blocked")
}

func TestExecTool_PathTraversal(t *testing.T) {
	tool := NewExecTool()
	tool.RestrictToWorkspace = true
	result, err := tool.Execute(context.Background(), map[string]any{
		"command": "cat ../../../etc/passwd",
	})
	require.NoError(t, err)
	assert.Contains(t, result, "path traversal")
}

func TestExecTool_EmptyCommand(t *testing.T) {
	tool := NewExecTool()
	result, err := tool.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	assert.Contains(t, result, "command is required")
}
