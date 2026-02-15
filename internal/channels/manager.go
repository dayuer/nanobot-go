package channels

import (
	"context"
	"log"
	"sync"

	"github.com/dayuer/nanobot-go/internal/bus"
)

// Manager manages all channel instances and routes outbound messages.
type Manager struct {
	Bus      *bus.MessageBus
	channels map[string]Channel
	mu       sync.RWMutex
}

// NewManager creates a channel manager.
func NewManager(msgBus *bus.MessageBus) *Manager {
	return &Manager{
		Bus:      msgBus,
		channels: make(map[string]Channel),
	}
}

// Register adds a channel to the manager.
func (m *Manager) Register(ch Channel) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.channels[ch.Name()] = ch
}

// Get returns a channel by name.
func (m *Manager) Get(name string) Channel {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.channels[name]
}

// EnabledChannels returns the list of registered channel names.
func (m *Manager) EnabledChannels() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.channels))
	for name := range m.channels {
		names = append(names, name)
	}
	return names
}

// StartAll starts all channels concurrently and dispatches outbound messages.
func (m *Manager) StartAll(ctx context.Context) error {
	if len(m.channels) == 0 {
		log.Println("No channels enabled")
		return nil
	}

	// Subscribe to outbound messages for each channel
	for name, ch := range m.channels {
		chName := name
		channel := ch
		m.Bus.Subscribe(chName, func(msg bus.OutboundMessage) {
			if err := channel.Send(msg); err != nil {
				log.Printf("Error sending to %s: %v", chName, err)
			}
		})
	}

	// Start outbound dispatcher
	go m.Bus.DispatchOutbound(ctx)

	// Start all channels concurrently
	var wg sync.WaitGroup
	for name, ch := range m.channels {
		wg.Add(1)
		go func(n string, c Channel) {
			defer wg.Done()
			log.Printf("Starting %s channel...", n)
			if err := c.Start(ctx); err != nil {
				log.Printf("Channel %s error: %v", n, err)
			}
		}(name, ch)
	}

	wg.Wait()
	return nil
}

// StopAll stops all channels.
func (m *Manager) StopAll() {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for name, ch := range m.channels {
		if err := ch.Stop(); err != nil {
			log.Printf("Error stopping %s: %v", name, err)
		}
	}
}

// GetStatus returns the running status of all channels.
func (m *Manager) GetStatus() map[string]bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	status := make(map[string]bool, len(m.channels))
	for name, ch := range m.channels {
		status[name] = ch.IsRunning()
	}
	return status
}
