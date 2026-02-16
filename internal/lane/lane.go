// Package lane provides session-based concurrency control for chat requests.
//
// It solves the problem of concurrent messages from the same user in IM channels
// (e.g., WeChat rapid-fire messages). Each session gets its own "lane" that
// serializes processing and supports three modes:
//
//   - Followup: Process each message sequentially (FIFO)
//   - Collect:  Wait a time window, merge rapid-fire messages into one
//   - Interrupt: Discard queued messages, process only the latest
//
// This mirrors survival/nanobot/session_lane.py.
package lane

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// Mode defines the lane processing strategy.
type Mode string

const (
	ModeFollowup  Mode = "followup"  // Process each message sequentially.
	ModeCollect   Mode = "collect"   // Wait & merge rapid-fire messages.
	ModeInterrupt Mode = "interrupt" // Discard old, process latest only.
)

// ChatRequest is a pending chat request.
type ChatRequest struct {
	Content    string
	SessionKey string
	Channel    string
	ChatID     string
	PersonID   string
	RoleID     string
	Metadata   map[string]any
	Timestamp  time.Time
}

// ChatResult is the processing result.
type ChatResult struct {
	Content        string
	AgentID        string
	Error          string
	RequestsMerged int // how many requests were merged (Collect mode)
	RouteInfo      any // routing decision metadata (cluster.RouteInfo)
}

// ChatHandler processes a single (possibly merged) chat request.
type ChatHandler func(ctx context.Context, req ChatRequest) ChatResult

// laneItem wraps a request with its result channel.
type laneItem struct {
	request ChatRequest
	done    chan ChatResult
}

// lane manages a single session's message queue.
type lane struct {
	sessionKey    string
	mode          Mode
	collectWindow time.Duration
	queue         chan laneItem
	idle          bool
	lastActive    time.Time
	mu            sync.Mutex
}

// Manager manages lanes for all sessions.
type Manager struct {
	mu              sync.RWMutex
	lanes           map[string]*lane
	handler         ChatHandler
	defaultMode     Mode
	collectWindow   time.Duration
	maxLanes        int
	cleanupInterval time.Duration
	stopCh          chan struct{}
}

// ManagerConfig configures a lane Manager.
type ManagerConfig struct {
	Handler         ChatHandler
	DefaultMode     Mode
	CollectWindow   time.Duration // Collect window (default 2s)
	MaxLanes        int           // Max concurrent lanes (default 1000)
	CleanupInterval time.Duration // Idle lane cleanup interval (default 10m)
}

// NewManager creates a lane manager.
func NewManager(cfg ManagerConfig) *Manager {
	if cfg.DefaultMode == "" {
		cfg.DefaultMode = ModeCollect
	}
	if cfg.CollectWindow == 0 {
		cfg.CollectWindow = 2 * time.Second
	}
	if cfg.MaxLanes == 0 {
		cfg.MaxLanes = 1000
	}
	if cfg.CleanupInterval == 0 {
		cfg.CleanupInterval = 10 * time.Minute
	}

	m := &Manager{
		lanes:           make(map[string]*lane),
		handler:         cfg.Handler,
		defaultMode:     cfg.DefaultMode,
		collectWindow:   cfg.CollectWindow,
		maxLanes:        cfg.MaxLanes,
		cleanupInterval: cfg.CleanupInterval,
		stopCh:          make(chan struct{}),
	}

	go m.periodicCleanup()
	return m
}

// Submit sends a chat request to its session's lane and waits for the result.
func (m *Manager) Submit(ctx context.Context, req ChatRequest, mode Mode) (ChatResult, error) {
	if mode == "" {
		mode = m.defaultMode
	}
	if req.Timestamp.IsZero() {
		req.Timestamp = time.Now()
	}

	l := m.getOrCreateLane(req.SessionKey, mode)
	item := laneItem{
		request: req,
		done:    make(chan ChatResult, 1),
	}

	select {
	case l.queue <- item:
	case <-ctx.Done():
		return ChatResult{}, ctx.Err()
	}

	select {
	case result := <-item.done:
		return result, nil
	case <-ctx.Done():
		return ChatResult{}, ctx.Err()
	}
}

// getOrCreateLane gets or creates a lane for the given session.
func (m *Manager) getOrCreateLane(sessionKey string, mode Mode) *lane {
	m.mu.Lock()
	defer m.mu.Unlock()

	if l, ok := m.lanes[sessionKey]; ok {
		return l
	}

	// Evict oldest if at capacity
	if len(m.lanes) >= m.maxLanes {
		m.cleanupIdleLanes()
	}

	l := &lane{
		sessionKey:    sessionKey,
		mode:          mode,
		collectWindow: m.collectWindow,
		queue:         make(chan laneItem, 100),
		lastActive:    time.Now(),
	}
	m.lanes[sessionKey] = l

	// Start worker goroutine
	go m.runWorker(l)
	return l
}

// runWorker is the per-lane worker loop.
func (m *Manager) runWorker(l *lane) {
	for {
		select {
		case item := <-l.queue:
			l.mu.Lock()
			l.idle = false
			l.lastActive = time.Now()
			l.mu.Unlock()

			var result ChatResult
			switch l.mode {
			case ModeFollowup:
				result = m.processFollowup(l, item)
			case ModeCollect:
				result = m.processCollect(l, item)
			case ModeInterrupt:
				result = m.processInterrupt(l, item)
			default:
				result = m.processFollowup(l, item)
			}

			item.done <- result

			l.mu.Lock()
			l.idle = true
			l.lastActive = time.Now()
			l.mu.Unlock()

		case <-time.After(5 * time.Minute):
			// No activity for 5 minutes, exit worker
			m.mu.Lock()
			delete(m.lanes, l.sessionKey)
			m.mu.Unlock()
			return

		case <-m.stopCh:
			return
		}
	}
}

// processFollowup: direct processing, one at a time.
func (m *Manager) processFollowup(_ *lane, item laneItem) ChatResult {
	return m.handler(context.Background(), item.request)
}

// processCollect: wait a window, merge queued messages, process once.
func (m *Manager) processCollect(l *lane, item laneItem) ChatResult {
	// Wait for collect window
	timer := time.NewTimer(l.collectWindow)
	defer timer.Stop()

	merged := []string{item.request.Content}
	extras := []laneItem{}

	// Drain queue during window
	for collecting := true; collecting; {
		select {
		case extra := <-l.queue:
			merged = append(merged, extra.request.Content)
			extras = append(extras, extra)
		case <-timer.C:
			collecting = false
		}
	}

	// Build merged request
	mergedReq := item.request
	mergedReq.Content = strings.Join(merged, "\n")

	result := m.handler(context.Background(), mergedReq)
	result.RequestsMerged = len(merged)

	if len(merged) > 1 {
		log.Printf("[Lane] Collect merged %d messages for session %s",
			len(merged), l.sessionKey)
	}

	// Send same result to all merged requests
	for _, e := range extras {
		e.done <- result
	}

	return result
}

// processInterrupt: discard queued, process only latest.
func (m *Manager) processInterrupt(l *lane, item laneItem) ChatResult {
	// Drain queue, keep only the latest
	latest := item
	for {
		select {
		case newer := <-l.queue:
			// Return empty result to discarded request
			latest.done <- ChatResult{Content: "", Error: "interrupted by newer message"}
			latest = newer
		default:
			goto process
		}
	}
process:
	return m.handler(context.Background(), latest.request)
}

// cleanupIdleLanes removes long-idle lanes (called under lock).
func (m *Manager) cleanupIdleLanes() {
	threshold := time.Now().Add(-10 * time.Minute)
	for key, l := range m.lanes {
		l.mu.Lock()
		if l.idle && l.lastActive.Before(threshold) {
			delete(m.lanes, key)
		}
		l.mu.Unlock()
	}
}

// periodicCleanup runs idle lane cleanup periodically.
func (m *Manager) periodicCleanup() {
	ticker := time.NewTicker(m.cleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.mu.Lock()
			m.cleanupIdleLanes()
			m.mu.Unlock()
		case <-m.stopCh:
			return
		}
	}
}

// Stop shuts down the manager.
func (m *Manager) Stop() {
	close(m.stopCh)
}

// Stats returns lane manager statistics.
func (m *Manager) Stats() map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()

	active := 0
	for _, l := range m.lanes {
		l.mu.Lock()
		if !l.idle {
			active++
		}
		l.mu.Unlock()
	}

	return map[string]any{
		"totalLanes":  len(m.lanes),
		"activeLanes": active,
		"defaultMode": string(m.defaultMode),
	}
}

// ActiveCount returns the number of active (non-idle) lanes.
func (m *Manager) ActiveCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	count := 0
	for _, l := range m.lanes {
		l.mu.Lock()
		if !l.idle {
			count++
		}
		l.mu.Unlock()
	}
	return count
}

// Describe returns a string describing the lane mode.
func (mode Mode) Describe() string {
	switch mode {
	case ModeFollowup:
		return "Process each message sequentially"
	case ModeCollect:
		return "Wait and merge rapid-fire messages"
	case ModeInterrupt:
		return "Discard old, process only latest"
	default:
		return fmt.Sprintf("Unknown mode: %s", string(mode))
	}
}
