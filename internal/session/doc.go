// Package session implements conversation session management with JSONL persistence.
package session

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dayuer/nanobot-go/internal/utils"
)

// Message is a single conversation message.
type Message struct {
	Role      string         `json:"role"`
	Content   string         `json:"content"`
	Timestamp string         `json:"timestamp,omitempty"`
	Extra     map[string]any `json:"extra,omitempty"` // channel-specific metadata

	// Internal marker for metadata lines in JSONL
	Type string `json:"_type,omitempty"`
}

// Session holds a conversation's message history.
type Session struct {
	Key               string    `json:"key"`
	Messages          []Message `json:"-"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
	LastConsolidated  int       `json:"last_consolidated"`
}

// AddMessage appends a message to the session.
func (s *Session) AddMessage(role, content string) {
	s.Messages = append(s.Messages, Message{
		Role:      role,
		Content:   content,
		Timestamp: time.Now().Format(time.RFC3339),
	})
	s.UpdatedAt = time.Now()
}

// GetHistory returns the last N messages in LLM format (role + content only).
func (s *Session) GetHistory(maxMessages int) []map[string]string {
	start := 0
	if len(s.Messages) > maxMessages {
		start = len(s.Messages) - maxMessages
	}
	result := make([]map[string]string, 0, len(s.Messages)-start)
	for _, m := range s.Messages[start:] {
		result = append(result, map[string]string{
			"role":    m.Role,
			"content": m.Content,
		})
	}
	return result
}

// Clear removes all messages.
func (s *Session) Clear() {
	s.Messages = nil
	s.LastConsolidated = 0
	s.UpdatedAt = time.Now()
}

// Manager manages conversation sessions with JSONL persistence.
type Manager struct {
	sessionsDir string
	mu          sync.RWMutex
	cache       map[string]*Session
}

// NewManager creates a session manager.
func NewManager(dataDir string) *Manager {
	dir := filepath.Join(dataDir, "sessions")
	os.MkdirAll(dir, 0755)
	return &Manager{
		sessionsDir: dir,
		cache:       make(map[string]*Session),
	}
}

// GetOrCreate returns an existing session or creates a new one.
func (m *Manager) GetOrCreate(key string) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()

	if s, ok := m.cache[key]; ok {
		return s
	}

	s := m.load(key)
	if s == nil {
		s = &Session{
			Key:       key,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
	}
	m.cache[key] = s
	return s
}

// Save persists a session to disk as JSONL.
func (m *Manager) Save(s *Session) error {
	path := m.sessionPath(s.Key)

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// First line: metadata
	meta := map[string]any{
		"_type":              "metadata",
		"created_at":         s.CreatedAt.Format(time.RFC3339),
		"updated_at":         s.UpdatedAt.Format(time.RFC3339),
		"last_consolidated":  s.LastConsolidated,
	}
	metaLine, _ := json.Marshal(meta)
	f.Write(metaLine)
	f.WriteString("\n")

	// Remaining lines: messages
	for _, msg := range s.Messages {
		line, _ := json.Marshal(msg)
		f.Write(line)
		f.WriteString("\n")
	}

	m.mu.Lock()
	m.cache[s.Key] = s
	m.mu.Unlock()
	return nil
}

// Invalidate removes a session from the in-memory cache.
func (m *Manager) Invalidate(key string) {
	m.mu.Lock()
	delete(m.cache, key)
	m.mu.Unlock()
}

// ListSessions returns info about all stored sessions.
func (m *Manager) ListSessions() []map[string]string {
	var result []map[string]string

	entries, err := os.ReadDir(m.sessionsDir)
	if err != nil {
		return result
	}

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		path := filepath.Join(m.sessionsDir, entry.Name())
		f, err := os.Open(path)
		if err != nil {
			continue
		}

		scanner := bufio.NewScanner(f)
		if scanner.Scan() {
			var meta map[string]any
			if json.Unmarshal([]byte(scanner.Text()), &meta) == nil {
				if meta["_type"] == "metadata" {
					key := strings.TrimSuffix(entry.Name(), ".jsonl")
					key = strings.ReplaceAll(key, "_", ":")
					info := map[string]string{
						"key":  key,
						"path": path,
					}
					if v, ok := meta["created_at"].(string); ok {
						info["created_at"] = v
					}
					if v, ok := meta["updated_at"].(string); ok {
						info["updated_at"] = v
					}
					result = append(result, info)
				}
			}
		}
		f.Close()
	}
	return result
}

// --- internal ---

func (m *Manager) sessionPath(key string) string {
	safe := utils.SafeFilename(strings.ReplaceAll(key, ":", "_"))
	return filepath.Join(m.sessionsDir, safe+".jsonl")
}

func (m *Manager) load(key string) *Session {
	path := m.sessionPath(key)

	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var msgs []Message
	var createdAt, updatedAt time.Time
	var lastConsolidated int

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var raw map[string]any
		if json.Unmarshal([]byte(line), &raw) != nil {
			continue
		}

		if raw["_type"] == "metadata" {
			if v, ok := raw["created_at"].(string); ok {
				createdAt, _ = time.Parse(time.RFC3339, v)
			}
			if v, ok := raw["updated_at"].(string); ok {
				updatedAt, _ = time.Parse(time.RFC3339, v)
			}
			if v, ok := raw["last_consolidated"].(float64); ok {
				lastConsolidated = int(v)
			}
			continue
		}

		var msg Message
		if json.Unmarshal([]byte(line), &msg) == nil {
			msgs = append(msgs, msg)
		}
	}

	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	if updatedAt.IsZero() {
		updatedAt = time.Now()
	}

	return &Session{
		Key:              key,
		Messages:         msgs,
		CreatedAt:        createdAt,
		UpdatedAt:        updatedAt,
		LastConsolidated: lastConsolidated,
	}
}
