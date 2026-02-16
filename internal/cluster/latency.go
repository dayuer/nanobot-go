package cluster

import (
	"sync"
	"time"
)

// latencyWindow tracks request latencies over a sliding window of 60 seconds.
// It provides the average latency for only the most recent window.
type latencyWindow struct {
	mu      sync.Mutex
	window  time.Duration
	entries []latencyEntry
}

type latencyEntry struct {
	ts      time.Time
	latency time.Duration
}

// newLatencyWindow creates a latency tracker with a sliding window.
func newLatencyWindow(window time.Duration) *latencyWindow {
	return &latencyWindow{
		window:  window,
		entries: make([]latencyEntry, 0, 128),
	}
}

// Record adds a latency sample.
func (w *latencyWindow) Record(d time.Duration) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.entries = append(w.entries, latencyEntry{ts: time.Now(), latency: d})
}

// Avg returns the average latency in milliseconds and the count of requests
// within the sliding window.
func (w *latencyWindow) Avg() (avgMs int64, count int64) {
	w.mu.Lock()
	defer w.mu.Unlock()

	cutoff := time.Now().Add(-w.window)

	// Trim expired entries from the front
	start := 0
	for start < len(w.entries) && w.entries[start].ts.Before(cutoff) {
		start++
	}
	if start > 0 {
		w.entries = w.entries[start:]
	}

	if len(w.entries) == 0 {
		return 0, 0
	}

	var total int64
	for _, e := range w.entries {
		total += e.latency.Milliseconds()
	}
	n := int64(len(w.entries))
	return total / n, n
}
