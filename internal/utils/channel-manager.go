package utils

import (
	"context"
	"sync"
	"time"
)

// ChannelManager provides a centralized way to manage channel lifecycles
// and prevent channel-related memory leaks
type ChannelManager struct {
	mu       sync.RWMutex
	channels map[string]chan struct{}
	ctx      context.Context
	cancel   context.CancelFunc
}

// NewChannelManager creates a new channel manager
func NewChannelManager(parentCtx context.Context) *ChannelManager {
	ctx, cancel := context.WithCancel(parentCtx)
	return &ChannelManager{
		channels: make(map[string]chan struct{}),
		ctx:      ctx,
		cancel:   cancel,
	}
}

// CreateChannel creates a managed channel with cleanup
func (cm *ChannelManager) CreateChannel(id string) chan struct{} {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Close existing channel if it exists
	if existing, exists := cm.channels[id]; exists {
		close(existing)
	}

	// Create new channel
	ch := make(chan struct{})
	cm.channels[id] = ch
	return ch
}

// CloseChannel safely closes a specific channel
func (cm *ChannelManager) CloseChannel(id string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if ch, exists := cm.channels[id]; exists {
		close(ch)
		delete(cm.channels, id)
	}
}

// Shutdown closes all managed channels
func (cm *ChannelManager) Shutdown() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	for id, ch := range cm.channels {
		close(ch)
		delete(cm.channels, id)
	}

	cm.cancel()
}

// SafeSend safely sends to a channel with timeout
func SafeSend[T any](ch chan<- T, value T, timeout time.Duration) bool {
	select {
	case ch <- value:
		return true
	case <-time.After(timeout):
		return false
	}
}

// SafeReceive safely receives from a channel with timeout
func SafeReceive[T any](ch <-chan T, timeout time.Duration) (T, bool) {
	var zero T
	select {
	case value := <-ch:
		return value, true
	case <-time.After(timeout):
		return zero, false
	}
}

// SafeClose safely closes a channel, ignoring panics from double-close
func SafeClose[T any](ch chan T) {
	defer func() {
		recover() // Ignore panic from closing already-closed channel
	}()
	close(ch)
}