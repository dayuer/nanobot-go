package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryStore_ReadWriteLongTerm(t *testing.T) {
	dir := t.TempDir()
	m := NewMemoryStore(dir)

	// Initially empty
	assert.Equal(t, "", m.ReadLongTerm())

	// Write and read
	require.NoError(t, m.WriteLongTerm("User lives in Beijing"))
	assert.Equal(t, "User lives in Beijing", m.ReadLongTerm())

	// Overwrite
	require.NoError(t, m.WriteLongTerm("Updated memory"))
	assert.Equal(t, "Updated memory", m.ReadLongTerm())
}

func TestMemoryStore_AppendHistory(t *testing.T) {
	dir := t.TempDir()
	m := NewMemoryStore(dir)

	require.NoError(t, m.AppendHistory("First entry"))
	require.NoError(t, m.AppendHistory("Second entry"))

	data, err := os.ReadFile(m.HistoryFile)
	require.NoError(t, err)
	assert.Equal(t, "First entry\n\nSecond entry\n\n", string(data))
}

func TestMemoryStore_AppendHistory_TrimsTrailingNewlines(t *testing.T) {
	dir := t.TempDir()
	m := NewMemoryStore(dir)

	require.NoError(t, m.AppendHistory("entry with newline\n"))
	data, err := os.ReadFile(m.HistoryFile)
	require.NoError(t, err)
	assert.Equal(t, "entry with newline\n\n", string(data))
}

func TestMemoryStore_GetMemoryContext(t *testing.T) {
	dir := t.TempDir()
	m := NewMemoryStore(dir)

	// Empty returns ""
	assert.Equal(t, "", m.GetMemoryContext())

	// With content
	m.WriteLongTerm("User prefers dark mode")
	assert.Equal(t, "## Long-term Memory\nUser prefers dark mode", m.GetMemoryContext())
}

func TestMemoryStore_CreatesDir(t *testing.T) {
	dir := t.TempDir()
	m := NewMemoryStore(dir)
	_, err := os.Stat(m.MemoryDir)
	assert.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "memory"), m.MemoryDir)
}
