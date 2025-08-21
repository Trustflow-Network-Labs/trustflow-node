package utils

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
	"time"
)

// ResourceTracker provides centralized resource management and leak detection
type ResourceTracker struct {
	mu        sync.RWMutex
	resources map[string]*TrackedResource
	ctx       context.Context
	cancel    context.CancelFunc
	logger    Logger // Interface for logging
}

// TrackedResource represents a tracked system resource
type TrackedResource struct {
	ID          string
	Type        ResourceType
	CreatedAt   time.Time
	LastUsed    time.Time
	Closer      io.Closer
	CleanupFunc func() error
	Stack       string // Stack trace for debugging leaks
}

type ResourceType int

const (
	ResourceFile ResourceType = iota
	ResourceStream
	ResourceConnection
	ResourceBuffer
	ResourceChannel
)

// Logger interface for resource tracking
type Logger interface {
	Log(level, message, component string)
}

// NewResourceTracker creates a new resource tracker
func NewResourceTracker(parentCtx context.Context, logger Logger) *ResourceTracker {
	ctx, cancel := context.WithCancel(parentCtx)
	rt := &ResourceTracker{
		resources: make(map[string]*TrackedResource),
		ctx:       ctx,
		cancel:    cancel,
		logger:    logger,
	}

	// Start cleanup goroutine
	go rt.periodicCleanup()
	
	return rt
}

// TrackResource registers a resource for tracking and automatic cleanup
func (rt *ResourceTracker) TrackResource(id string, resourceType ResourceType, closer io.Closer) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	// Get stack trace for debugging
	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)
	stack := string(buf[:n])

	rt.resources[id] = &TrackedResource{
		ID:        id,
		Type:      resourceType,
		CreatedAt: time.Now(),
		LastUsed:  time.Now(),
		Closer:    closer,
		Stack:     stack,
	}

	if rt.logger != nil {
		rt.logger.Log("debug", fmt.Sprintf("Tracking resource %s (%v)", id, resourceType), "resource-tracker")
	}
}

// TrackResourceWithCleanup registers a resource with custom cleanup function
func (rt *ResourceTracker) TrackResourceWithCleanup(id string, resourceType ResourceType, cleanupFunc func() error) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)
	stack := string(buf[:n])

	rt.resources[id] = &TrackedResource{
		ID:          id,
		Type:        resourceType,
		CreatedAt:   time.Now(),
		LastUsed:    time.Now(),
		CleanupFunc: cleanupFunc,
		Stack:       stack,
	}

	if rt.logger != nil {
		rt.logger.Log("debug", fmt.Sprintf("Tracking resource %s (%v) with custom cleanup", id, resourceType), "resource-tracker")
	}
}

// UpdateLastUsed updates the last used timestamp for a resource
func (rt *ResourceTracker) UpdateLastUsed(id string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	if resource, exists := rt.resources[id]; exists {
		resource.LastUsed = time.Now()
	}
}

// ReleaseResource manually releases a tracked resource
func (rt *ResourceTracker) ReleaseResource(id string) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	resource, exists := rt.resources[id]
	if !exists {
		return fmt.Errorf("resource %s not found", id)
	}

	var err error
	if resource.CleanupFunc != nil {
		err = resource.CleanupFunc()
	} else if resource.Closer != nil {
		err = resource.Closer.Close()
	}

	delete(rt.resources, id)

	if rt.logger != nil {
		if err != nil {
			rt.logger.Log("warn", fmt.Sprintf("Failed to cleanup resource %s: %v", id, err), "resource-tracker")
		} else {
			rt.logger.Log("debug", fmt.Sprintf("Released resource %s", id), "resource-tracker")
		}
	}

	return err
}

// GetResourceStats returns statistics about tracked resources
func (rt *ResourceTracker) GetResourceStats() map[ResourceType]int {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	stats := make(map[ResourceType]int)
	for _, resource := range rt.resources {
		stats[resource.Type]++
	}
	return stats
}

// GetOldResources returns resources that haven't been used for the specified duration
func (rt *ResourceTracker) GetOldResources(maxAge time.Duration) []*TrackedResource {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	cutoff := time.Now().Add(-maxAge)
	var old []*TrackedResource

	for _, resource := range rt.resources {
		if resource.LastUsed.Before(cutoff) {
			old = append(old, resource)
		}
	}

	return old
}

// CleanupOldResources cleans up resources older than the specified duration
func (rt *ResourceTracker) CleanupOldResources(maxAge time.Duration) int {
	oldResources := rt.GetOldResources(maxAge)
	cleaned := 0

	for _, resource := range oldResources {
		if err := rt.ReleaseResource(resource.ID); err != nil {
			if rt.logger != nil {
				rt.logger.Log("error", fmt.Sprintf("Failed to cleanup old resource %s: %v", resource.ID, err), "resource-tracker")
			}
		} else {
			cleaned++
		}
	}

	if rt.logger != nil && cleaned > 0 {
		rt.logger.Log("info", fmt.Sprintf("Cleaned up %d old resources", cleaned), "resource-tracker")
	}

	return cleaned
}

// periodicCleanup runs periodic cleanup of old resources
func (rt *ResourceTracker) periodicCleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-rt.ctx.Done():
			return
		case <-ticker.C:
			// Clean up resources older than 30 minutes
			rt.CleanupOldResources(30 * time.Minute)
			
			// Log resource statistics
			stats := rt.GetResourceStats()
			if rt.logger != nil && len(stats) > 0 {
				rt.logger.Log("debug", fmt.Sprintf("Resource stats: %+v", stats), "resource-tracker")
			}
		}
	}
}

// Shutdown releases all tracked resources and stops the tracker
func (rt *ResourceTracker) Shutdown() error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	var errors []error
	for id, resource := range rt.resources {
		var err error
		if resource.CleanupFunc != nil {
			err = resource.CleanupFunc()
		} else if resource.Closer != nil {
			err = resource.Closer.Close()
		}

		if err != nil {
			errors = append(errors, fmt.Errorf("failed to cleanup resource %s: %v", id, err))
		}
	}

	rt.resources = make(map[string]*TrackedResource)
	rt.cancel()

	if len(errors) > 0 {
		return fmt.Errorf("shutdown errors: %v", errors)
	}

	return nil
}

// TrackedFile wraps os.File with resource tracking
type TrackedFile struct {
	*os.File
	tracker *ResourceTracker
	id      string
}

// NewTrackedFile creates a file with automatic resource tracking
func (rt *ResourceTracker) NewTrackedFile(filename string) (*TrackedFile, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}

	id := fmt.Sprintf("file:%s:%d", filename, file.Fd())
	tf := &TrackedFile{
		File:    file,
		tracker: rt,
		id:      id,
	}

	rt.TrackResource(id, ResourceFile, tf)
	return tf, nil
}

// Read wraps os.File.Read to update last used time
func (tf *TrackedFile) Read(b []byte) (n int, err error) {
	tf.tracker.UpdateLastUsed(tf.id)
	return tf.File.Read(b)
}

// Write wraps os.File.Write to update last used time
func (tf *TrackedFile) Write(b []byte) (n int, err error) {
	tf.tracker.UpdateLastUsed(tf.id)
	return tf.File.Write(b)
}

// Close properly releases the tracked file
func (tf *TrackedFile) Close() error {
	err := tf.File.Close()
	tf.tracker.ReleaseResource(tf.id)
	return err
}