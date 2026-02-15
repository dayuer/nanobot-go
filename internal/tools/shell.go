package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// DefaultDenyPatterns match dangerous shell commands.
var DefaultDenyPatterns = []string{
	`\brm\s+-[rf]{1,2}\b`,
	`\bdel\s+/[fq]\b`,
	`\brmdir\s+/s\b`,
	`\b(format|mkfs|diskpart)\b`,
	`\bdd\s+if=`,
	`>\s*/dev/sd`,
	`\b(shutdown|reboot|poweroff)\b`,
	`:\(\)\s*\{.*\};\s*:`,
}

// ExecTool executes shell commands with safety guards.
type ExecTool struct {
	Timeout             time.Duration
	WorkingDir          string
	DenyPatterns        []string
	AllowPatterns       []string
	RestrictToWorkspace bool
}

// NewExecTool creates an ExecTool with default safety patterns.
func NewExecTool() *ExecTool {
	return &ExecTool{
		Timeout:      60 * time.Second,
		DenyPatterns: DefaultDenyPatterns,
	}
}

func (t *ExecTool) Name() string        { return "exec" }
func (t *ExecTool) Description() string  { return "Execute a shell command and return its output." }
func (t *ExecTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command":     map[string]any{"type": "string", "description": "The shell command to execute"},
			"working_dir": map[string]any{"type": "string", "description": "Optional working directory"},
		},
		"required": []string{"command"},
	}
}

func (t *ExecTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	command, _ := args["command"].(string)
	if command == "" {
		return "Error: command is required", nil
	}

	cwd, _ := args["working_dir"].(string)
	if cwd == "" {
		cwd = t.WorkingDir
	}

	if err := t.guardCommand(command); err != "" {
		return err, nil
	}

	timeout := t.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	if cwd != "" {
		cmd.Dir = cwd
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	var parts []string
	if stdout.Len() > 0 {
		parts = append(parts, stdout.String())
	}
	if stderr.Len() > 0 {
		s := strings.TrimSpace(stderr.String())
		if s != "" {
			parts = append(parts, "STDERR:\n"+s)
		}
	}
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Sprintf("Error: Command timed out after %v", timeout), nil
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			parts = append(parts, fmt.Sprintf("\nExit code: %d", exitErr.ExitCode()))
		}
	}

	result := "(no output)"
	if len(parts) > 0 {
		result = strings.Join(parts, "\n")
	}

	// Truncate long output
	const maxLen = 10000
	if len(result) > maxLen {
		result = result[:maxLen] + fmt.Sprintf("\n... (truncated, %d more chars)", len(result)-maxLen)
	}
	return result, nil
}

func (t *ExecTool) guardCommand(command string) string {
	lower := strings.ToLower(strings.TrimSpace(command))

	for _, pattern := range t.DenyPatterns {
		if matched, _ := regexp.MatchString(pattern, lower); matched {
			return "Error: Command blocked by safety guard (dangerous pattern detected)"
		}
	}

	if len(t.AllowPatterns) > 0 {
		allowed := false
		for _, p := range t.AllowPatterns {
			if matched, _ := regexp.MatchString(p, lower); matched {
				allowed = true
				break
			}
		}
		if !allowed {
			return "Error: Command blocked by safety guard (not in allowlist)"
		}
	}

	if t.RestrictToWorkspace {
		if strings.Contains(command, "../") || strings.Contains(command, "..\\") {
			return "Error: Command blocked by safety guard (path traversal detected)"
		}
	}

	return ""
}
