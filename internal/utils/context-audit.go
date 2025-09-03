package utils

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"
)

// GoroutineTracker interface for tracking goroutines
type GoroutineTracker interface {
	SafeStart(name string, fn func()) bool
	SafeStartCritical(name string, fn func()) bool
}

// ContextTracker helps identify potential context leaks and long-lived contexts
type ContextTracker struct {
	contexts     map[string]*ContextInfo
	mu           sync.RWMutex
	logger       Logger
	maxAge       time.Duration
	checkInterval time.Duration
	ticker       *time.Ticker
	ctx          context.Context
	cancel       context.CancelFunc
	gt           GoroutineTracker // For tracking goroutines
}

// ContextInfo stores information about tracked contexts
type ContextInfo struct {
	ID          string
	CreatedAt   time.Time
	Description string
	StackTrace  string
	Timeout     time.Duration
	Active      bool
}

// NewContextTracker creates a new context tracker
func NewContextTracker(logger Logger, gt GoroutineTracker) *ContextTracker {
	ctx, cancel := context.WithCancel(context.Background())
	
	tracker := &ContextTracker{
		contexts:      make(map[string]*ContextInfo),
		logger:        logger,
		maxAge:        5 * time.Minute,        // Warn about contexts older than 5 minutes
		checkInterval: 30 * time.Second,       // Check every 30 seconds
		ctx:           ctx,
		cancel:        cancel,
		gt:            gt,                     // Store goroutine tracker
	}
	
	// Start monitoring goroutine with tracking
	if gt != nil {
		tracker.startMonitoring()
	} else {
		// Fallback for when gt is nil (backward compatibility)
		tracker.startMonitoringUntracked()
	}
	
	return tracker
}

// TrackContext registers a new context for tracking
func (ct *ContextTracker) TrackContext(ctx context.Context, description string, timeout time.Duration) string {
	id := fmt.Sprintf("ctx_%d_%s", time.Now().UnixNano(), description)
	
	// Capture stack trace
	stackBuf := make([]byte, 4096)
	stackLen := runtime.Stack(stackBuf, false)
	stackTrace := string(stackBuf[:stackLen])
	
	ct.mu.Lock()
	defer ct.mu.Unlock()
	
	ct.contexts[id] = &ContextInfo{
		ID:          id,
		CreatedAt:   time.Now(),
		Description: description,
		StackTrace:  stackTrace,
		Timeout:     timeout,
		Active:      true,
	}
	
	// Monitor context completion with proper tracking
	if ct.gt != nil {
		if !ct.gt.SafeStart(fmt.Sprintf("context-monitor-%s", id), func() {
			ct.monitorContext(ctx, id)
		}) {
			ct.logger.Log("warn", fmt.Sprintf("Failed to start context monitor goroutine for %s", id), "context-tracker")
		}
	} else {
		// Fallback for backward compatibility
		go func() {
			ct.monitorContext(ctx, id)
		}()
	}
	
	return id
}

// ReleaseContext marks a context as no longer active
func (ct *ContextTracker) ReleaseContext(id string) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	
	if info, exists := ct.contexts[id]; exists {
		info.Active = false
	}
}

// monitorContext watches for context completion
func (ct *ContextTracker) monitorContext(ctx context.Context, id string) {
	<-ctx.Done()
	ct.ReleaseContext(id)
}

// startMonitoring begins the background monitoring process with proper tracking
func (ct *ContextTracker) startMonitoring() {
	ct.ticker = time.NewTicker(ct.checkInterval)
	
	if !ct.gt.SafeStart("context-leak-checker", func() {
		defer ct.ticker.Stop()
		for {
			select {
			case <-ct.ctx.Done():
				return
			case <-ct.ticker.C:
				ct.checkForLeaks()
			}
		}
	}) {
		ct.logger.Log("warn", "Failed to start context leak checker goroutine", "context-tracker")
		// Fallback to untracked goroutine if SafeStart fails
		ct.startMonitoringUntracked()
	}
}

// startMonitoringUntracked is a fallback for when goroutine tracking is unavailable
func (ct *ContextTracker) startMonitoringUntracked() {
	ct.ticker = time.NewTicker(ct.checkInterval)
	
	go func() {
		defer ct.ticker.Stop()
		for {
			select {
			case <-ct.ctx.Done():
				return
			case <-ct.ticker.C:
				ct.checkForLeaks()
			}
		}
	}()
}

// checkForLeaks identifies potentially leaked or long-running contexts
func (ct *ContextTracker) checkForLeaks() {
	ct.mu.RLock()
	now := time.Now()
	
	var longRunning []ContextInfo
	var suspicious []ContextInfo
	
	for _, info := range ct.contexts {
		if !info.Active {
			continue
		}
		
		age := now.Sub(info.CreatedAt)
		
		// Long-running contexts (over maxAge)
		if age > ct.maxAge {
			longRunning = append(longRunning, *info)
		}
		
		// Suspicious contexts (much longer than their timeout)
		if info.Timeout > 0 && age > info.Timeout*2 {
			suspicious = append(suspicious, *info)
		}
	}
	ct.mu.RUnlock()
	
	// Log findings
	if len(longRunning) > 0 {
		ct.logger.Log("warn", 
			fmt.Sprintf("Found %d long-running contexts (>%v)", len(longRunning), ct.maxAge),
			"context-audit")
			
		for _, info := range longRunning {
			ct.logger.Log("warn",
				fmt.Sprintf("Long-running context: %s (age: %v, desc: %s)",
					info.ID, now.Sub(info.CreatedAt), info.Description),
				"context-audit")
		}
	}
	
	if len(suspicious) > 0 {
		ct.logger.Log("error",
			fmt.Sprintf("Found %d suspicious contexts (outlived timeout)", len(suspicious)),
			"context-audit")
			
		for _, info := range suspicious {
			ct.logger.Log("error",
				fmt.Sprintf("Suspicious context: %s (age: %v, timeout: %v, desc: %s)",
					info.ID, now.Sub(info.CreatedAt), info.Timeout, info.Description),
				"context-audit")
		}
	}
}

// GetStats returns current context tracking statistics
func (ct *ContextTracker) GetStats() (active, total int, oldestAge time.Duration) {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	
	now := time.Now()
	total = len(ct.contexts)
	
	var oldest time.Time
	for _, info := range ct.contexts {
		if info.Active {
			active++
			if oldest.IsZero() || info.CreatedAt.Before(oldest) {
				oldest = info.CreatedAt
			}
		}
	}
	
	if !oldest.IsZero() {
		oldestAge = now.Sub(oldest)
	}
	
	return
}

// DrainOldContexts removes old inactive context records
func (ct *ContextTracker) DrainOldContexts() {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	
	cutoff := time.Now().Add(-10 * time.Minute)
	var removed int
	
	for id, info := range ct.contexts {
		if !info.Active && info.CreatedAt.Before(cutoff) {
			delete(ct.contexts, id)
			removed++
		}
	}
	
	if removed > 0 {
		ct.logger.Log("info", 
			fmt.Sprintf("Drained %d old context records", removed),
			"context-audit")
	}
}

// Shutdown stops the context tracker
func (ct *ContextTracker) Shutdown() {
	if ct.ticker != nil {
		ct.ticker.Stop()
	}
	ct.cancel()
	
	ct.mu.Lock()
	defer ct.mu.Unlock()
	
	active, total, oldestAge := ct.GetStats()
	ct.logger.Log("info",
		fmt.Sprintf("Context tracker shutdown - Active: %d, Total: %d, Oldest: %v",
			active, total, oldestAge),
		"context-audit")
}

// Global context tracker instance
var GlobalContextTracker *ContextTracker

// InitializeContextTracking initializes global context tracking
func InitializeContextTracking(logger Logger, gt GoroutineTracker) {
	GlobalContextTracker = NewContextTracker(logger, gt)
}

// TrackContextWithTimeout is a helper for creating and tracking timeout contexts
func TrackContextWithTimeout(parent context.Context, timeout time.Duration, description string) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	
	if GlobalContextTracker != nil {
		GlobalContextTracker.TrackContext(ctx, description, timeout)
	}
	
	return ctx, cancel
}

// TrackContextWithCancel is a helper for creating and tracking cancelable contexts  
func TrackContextWithCancel(parent context.Context, description string) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	
	if GlobalContextTracker != nil {
		GlobalContextTracker.TrackContext(ctx, description, 0)
	}
	
	return ctx, cancel
}