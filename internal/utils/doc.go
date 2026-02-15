// Package utils provides shared helper functions.
package utils

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// EnsureDir ensures a directory exists, creating it if necessary.
func EnsureDir(path string) (string, error) {
	if err := os.MkdirAll(path, 0755); err != nil {
		return "", err
	}
	return path, nil
}

// GetDataPath returns the nanobot data directory (~/.nanobot).
func GetDataPath() string {
	home, _ := os.UserHomeDir()
	p := filepath.Join(home, ".nanobot")
	os.MkdirAll(p, 0755)
	return p
}

// GetSessionsPath returns the sessions storage directory.
func GetSessionsPath() string {
	p := filepath.Join(GetDataPath(), "sessions")
	os.MkdirAll(p, 0755)
	return p
}

// GetWorkspacePath returns the workspace path.
func GetWorkspacePath(workspace string) string {
	if workspace != "" {
		if strings.HasPrefix(workspace, "~") {
			home, _ := os.UserHomeDir()
			workspace = filepath.Join(home, workspace[1:])
		}
		os.MkdirAll(workspace, 0755)
		return workspace
	}
	p := filepath.Join(GetDataPath(), "workspace")
	os.MkdirAll(p, 0755)
	return p
}

// Timestamp returns the current time as an ISO 8601 string.
func Timestamp() string {
	return time.Now().Format(time.RFC3339)
}

// TruncateString truncates a string to maxLen, adding suffix if truncated.
func TruncateString(s string, maxLen int, suffix string) string {
	if len(s) <= maxLen {
		return s
	}
	if suffix == "" {
		suffix = "..."
	}
	cutoff := maxLen - len(suffix)
	if cutoff < 0 {
		cutoff = 0
	}
	return s[:cutoff] + suffix
}

// SafeFilename converts a string to a safe filename by replacing unsafe characters.
func SafeFilename(name string) string {
	unsafe := `<>:"/\|?*`
	for _, c := range unsafe {
		name = strings.ReplaceAll(name, string(c), "_")
	}
	return strings.TrimSpace(name)
}

// ParseSessionKey splits a session key "channel:chat_id" into its parts.
func ParseSessionKey(key string) (channel, chatID string, err error) {
	parts := strings.SplitN(key, ":", 2)
	if len(parts) != 2 {
		return "", "", &InvalidSessionKeyError{Key: key}
	}
	return parts[0], parts[1], nil
}

// InvalidSessionKeyError is returned when a session key cannot be parsed.
type InvalidSessionKeyError struct {
	Key string
}

func (e *InvalidSessionKeyError) Error() string {
	return "invalid session key: " + e.Key
}
