package session

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSession_AddMessage(t *testing.T) {
	s := &Session{Key: "test:1"}
	s.AddMessage("user", "hello")
	s.AddMessage("assistant", "hi there")

	assert.Len(t, s.Messages, 2)
	assert.Equal(t, "user", s.Messages[0].Role)
	assert.Equal(t, "hello", s.Messages[0].Content)
	assert.NotEmpty(t, s.Messages[0].Timestamp)
}

func TestSession_GetHistory(t *testing.T) {
	s := &Session{Key: "test:1"}
	for i := 0; i < 10; i++ {
		s.AddMessage("user", "msg")
	}

	history := s.GetHistory(5)
	assert.Len(t, history, 5)

	full := s.GetHistory(100)
	assert.Len(t, full, 10)
}

func TestSession_Clear(t *testing.T) {
	s := &Session{Key: "test:1"}
	s.AddMessage("user", "hello")
	s.Clear()

	assert.Empty(t, s.Messages)
	assert.Equal(t, 0, s.LastConsolidated)
}

func TestManager_GetOrCreate_New(t *testing.T) {
	mgr := NewManager(t.TempDir())
	s := mgr.GetOrCreate("telegram:123")

	assert.Equal(t, "telegram:123", s.Key)
	assert.Empty(t, s.Messages)
}

func TestManager_GetOrCreate_Cached(t *testing.T) {
	mgr := NewManager(t.TempDir())
	s1 := mgr.GetOrCreate("telegram:123")
	s1.AddMessage("user", "hello")

	s2 := mgr.GetOrCreate("telegram:123")
	assert.Same(t, s1, s2)
	assert.Len(t, s2.Messages, 1)
}

func TestManager_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir)

	s := mgr.GetOrCreate("telegram:456")
	s.AddMessage("user", "hello")
	s.AddMessage("assistant", "hi!")
	s.LastConsolidated = 1

	err := mgr.Save(s)
	require.NoError(t, err)

	// Verify file exists
	path := filepath.Join(dir, "sessions", "telegram_456.jsonl")
	_, err = os.Stat(path)
	require.NoError(t, err)

	// Load into new manager (cold cache)
	mgr2 := NewManager(dir)
	s2 := mgr2.GetOrCreate("telegram:456")

	assert.Equal(t, "telegram:456", s2.Key)
	assert.Len(t, s2.Messages, 2)
	assert.Equal(t, "hello", s2.Messages[0].Content)
	assert.Equal(t, "hi!", s2.Messages[1].Content)
	assert.Equal(t, 1, s2.LastConsolidated)
}

func TestManager_Invalidate(t *testing.T) {
	mgr := NewManager(t.TempDir())
	_ = mgr.GetOrCreate("test:1")
	mgr.Invalidate("test:1")

	// After invalidation, a new session is created (not cached)
	s := mgr.GetOrCreate("test:1")
	assert.Empty(t, s.Messages)
}

func TestManager_ListSessions(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir)

	s1 := mgr.GetOrCreate("telegram:1")
	s1.AddMessage("user", "a")
	require.NoError(t, mgr.Save(s1))

	s2 := mgr.GetOrCreate("discord:2")
	s2.AddMessage("user", "b")
	require.NoError(t, mgr.Save(s2))

	sessions := mgr.ListSessions()
	assert.Len(t, sessions, 2)

	// Verify keys are restored
	keys := []string{sessions[0]["key"], sessions[1]["key"]}
	assert.Contains(t, keys, "telegram:1")
	assert.Contains(t, keys, "discord:2")
}

func TestManager_EmptyDir(t *testing.T) {
	mgr := NewManager(t.TempDir())
	sessions := mgr.ListSessions()
	assert.Empty(t, sessions)
}
