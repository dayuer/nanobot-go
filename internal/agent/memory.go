// Package agent implements the core agent — memory, skills, context, loop.
// Mirrors upstream nanobot/agent/*.py.
package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MemoryStore provides two-layer memory: MEMORY.md (long-term) + HISTORY.md (grep-searchable log).
type MemoryStore struct {
	MemoryDir  string
	MemoryFile string
	HistoryFile string
}

// NewMemoryStore creates a MemoryStore rooted at workspace/memory.
func NewMemoryStore(workspace string) *MemoryStore {
	dir := filepath.Join(workspace, "memory")
	os.MkdirAll(dir, 0o755)
	return &MemoryStore{
		MemoryDir:   dir,
		MemoryFile:  filepath.Join(dir, "MEMORY.md"),
		HistoryFile: filepath.Join(dir, "HISTORY.md"),
	}
}

// ReadLongTerm reads MEMORY.md.
func (m *MemoryStore) ReadLongTerm() string {
	data, err := os.ReadFile(m.MemoryFile)
	if err != nil {
		return ""
	}
	return string(data)
}

// WriteLongTerm writes MEMORY.md.
func (m *MemoryStore) WriteLongTerm(content string) error {
	return os.WriteFile(m.MemoryFile, []byte(content), 0o644)
}

// AppendHistory appends an entry to HISTORY.md.
func (m *MemoryStore) AppendHistory(entry string) error {
	f, err := os.OpenFile(m.HistoryFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(strings.TrimRight(entry, "\n") + "\n\n")
	return err
}

// GetMemoryContext returns formatted memory for inclusion in prompts.
func (m *MemoryStore) GetMemoryContext() string {
	lt := m.ReadLongTerm()
	if lt != "" {
		return fmt.Sprintf("## Long-term Memory\n%s", lt)
	}
	return ""
}
