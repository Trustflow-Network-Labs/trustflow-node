package utils

import (
	"context"
	"fmt"
	"io"
	"maps"
	"runtime"
	"sync"
	"time"
)

// GoroutineTracker interface for goroutine management
type GoroutineTracker interface {
	SafeStart(name string, fn func()) bool
	SafeStartCritical(name string, fn func()) bool
}

// CleanupManager provides automatic resource cleanup patterns
type CleanupManager struct {
	mu           sync.RWMutex
	cleanupFuncs map[string]func() error
	ctx          context.Context
	cancel       context.CancelFunc
	logger       *LogsManager
}

// NewCleanupManager creates a new cleanup manager
func NewCleanupManager(parentCtx context.Context, logger *LogsManager) *CleanupManager {
	ctx, cancel := context.WithCancel(parentCtx)
	return &CleanupManager{
		cleanupFuncs: make(map[string]func() error),
		ctx:          ctx,
		cancel:       cancel,
		logger:       logger,
	}
}

// RegisterCleanup registers a cleanup function with a unique ID
func (cm *CleanupManager) RegisterCleanup(id string, cleanup func() error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.cleanupFuncs[id] = cleanup
}

// UnregisterCleanup removes a cleanup function
func (cm *CleanupManager) UnregisterCleanup(id string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	delete(cm.cleanupFuncs, id)
}

// RunCleanup executes and removes a specific cleanup function
func (cm *CleanupManager) RunCleanup(id string) error {
	cm.mu.Lock()
	cleanup, exists := cm.cleanupFuncs[id]
	if exists {
		delete(cm.cleanupFuncs, id)
	}
	cm.mu.Unlock()

	if !exists {
		return fmt.Errorf("cleanup function %s not found", id)
	}

	return cleanup()
}

// RunAllCleanups executes all registered cleanup functions
func (cm *CleanupManager) RunAllCleanups() []error {
	cm.mu.Lock()
	funcs := make(map[string]func() error)
	maps.Copy(funcs, cm.cleanupFuncs)
	cm.cleanupFuncs = make(map[string]func() error)
	cm.mu.Unlock()

	var errors []error
	for id, cleanup := range funcs {
		if err := cleanup(); err != nil {
			errors = append(errors, fmt.Errorf("cleanup %s failed: %v", id, err))
		}
	}

	return errors
}

// Shutdown runs all cleanups and stops the manager
func (cm *CleanupManager) Shutdown() []error {
	errors := cm.RunAllCleanups()
	cm.cancel()
	return errors
}

// WithCleanup provides automatic cleanup using defer pattern
func WithCleanup(cleanup func() error, operation func() error) error {
	defer func() {
		if err := cleanup(); err != nil {
			// Log cleanup error but don't override operation error
			runtime.GC() // Force GC after cleanup failure
		}
	}()
	return operation()
}

// WithTimeout executes an operation with timeout and automatic cleanup
func WithTimeout(timeout time.Duration, cleanup func() error, operation func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return WithCleanup(cleanup, func() error {
		return operation(ctx)
	})
}

// StreamCleanupWrapper provides automatic stream cleanup
type StreamCleanupWrapper struct {
	stream  io.ReadWriteCloser
	cleanup func() error
	closed  bool
	mu      sync.Mutex
}

// NewStreamWrapper creates a stream wrapper with automatic cleanup
func NewStreamWrapper(stream io.ReadWriteCloser, cleanup func() error) *StreamCleanupWrapper {
	return &StreamCleanupWrapper{
		stream:  stream,
		cleanup: cleanup,
	}
}

func (s *StreamCleanupWrapper) Read(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return 0, io.ErrClosedPipe
	}
	return s.stream.Read(p)
}

func (s *StreamCleanupWrapper) Write(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return 0, io.ErrClosedPipe
	}
	return s.stream.Write(p)
}

func (s *StreamCleanupWrapper) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true

	// Close stream first
	err := s.stream.Close()

	// Run cleanup
	if s.cleanup != nil {
		if cleanupErr := s.cleanup(); cleanupErr != nil && err == nil {
			err = cleanupErr
		}
	}

	return err
}

// MemoryPressureMonitor monitors memory usage and triggers cleanup
type MemoryPressureMonitor struct {
	thresholdPercent float64
	cleanupManager   *CleanupManager
	ticker           *time.Ticker
	ctx              context.Context
	cancel           context.CancelFunc
	logger           *LogsManager
	gt               GoroutineTracker
}

// NewMemoryPressureMonitor creates a memory pressure monitor
func NewMemoryPressureMonitor(thresholdPercent float64, cm *CleanupManager, logger *LogsManager, gt GoroutineTracker) *MemoryPressureMonitor {
	ctx, cancel := context.WithCancel(context.Background())

	monitor := &MemoryPressureMonitor{
		thresholdPercent: thresholdPercent,
		cleanupManager:   cm,
		ctx:              ctx,
		cancel:           cancel,
		logger:           logger,
		gt:               gt,
	}

	return monitor
}

// Start begins monitoring memory pressure
func (m *MemoryPressureMonitor) Start(interval time.Duration) {
	m.ticker = time.NewTicker(interval)

	// Use tracked goroutine if available, fallback to untracked
	if m.gt != nil {
		if !m.gt.SafeStart("memory-pressure-monitor", func() {
			m.logger.Log("debug", "Starting memory pressure monitor goroutine", "memory-pressure-monitor")
			defer m.ticker.Stop()
			for {
				select {
				case <-m.ctx.Done():
					m.logger.Log("debug", "Memory pressure monitor goroutine stopped", "memory-pressure-monitor")
					return
				case <-m.ticker.C:
					m.checkMemoryPressure()
				}
			}
		}) {
			m.logger.Log("warn", "Failed to start memory pressure monitor goroutine", "memory-pressure-monitor")
			// Fallback to untracked goroutine
			m.startUntracked()
		}
	} else {
		// Fallback for backward compatibility
		m.startUntracked()
	}
}

// startUntracked is a fallback for when goroutine tracking is unavailable
func (m *MemoryPressureMonitor) startUntracked() {
	go func() {
		m.logger.Log("debug", "Starting memory pressure monitor goroutine (untracked)", "memory-pressure-monitor")
		defer m.ticker.Stop()
		for {
			select {
			case <-m.ctx.Done():
				m.logger.Log("debug", "Memory pressure monitor goroutine stopped", "memory-pressure-monitor")
				return
			case <-m.ticker.C:
				m.checkMemoryPressure()
			}
		}
	}()
}

// Stop stops the memory pressure monitor
func (m *MemoryPressureMonitor) Stop() {
	if m.ticker != nil {
		m.ticker.Stop()
	}
	m.cancel()
}

func (m *MemoryPressureMonitor) checkMemoryPressure() {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Calculate memory usage percentage (rough estimate)
	heapInUse := float64(memStats.HeapInuse)
	heapSys := float64(memStats.HeapSys)

	if heapSys > 0 {
		usagePercent := (heapInUse / heapSys) * 100

		if usagePercent > m.thresholdPercent {
			if m.logger != nil {
				m.logger.Log("warn",
					fmt.Sprintf("High memory pressure detected: %.2f%% (threshold: %.2f%%)",
						usagePercent, m.thresholdPercent),
					"memory-monitor")
			}

			// Trigger cleanup
			runtime.GC()
			runtime.GC() // Double GC for better cleanup

			// Could also trigger specific cleanups here
			// m.cleanupManager.RunAllCleanups()
		}
	}
}

// AutoCleanupContext provides automatic cleanup when context is cancelled
func AutoCleanupContext(parentCtx context.Context, cleanupFunc func()) context.Context {
	ctx, cancel := context.WithCancel(parentCtx)

	go func() {
		<-ctx.Done()
		cleanupFunc()
	}()

	// Return a context that when cancelled, will trigger cleanup
	return &cleanupContext{Context: ctx, cancel: cancel}
}

// AutoCleanupContextTracked provides automatic cleanup when context is cancelled with goroutine tracking
func AutoCleanupContextTracked(parentCtx context.Context, cleanupFunc func(), gt GoroutineTracker, contextID string) context.Context {
	ctx, cancel := context.WithCancel(parentCtx)

	// Use tracked goroutine if available, fallback to untracked
	if gt != nil {
		if !gt.SafeStart(fmt.Sprintf("auto-cleanup-%s", contextID), func() {
			<-ctx.Done()
			cleanupFunc()
		}) {
			// Fallback to untracked goroutine if SafeStart fails
			go func() {
				<-ctx.Done()
				cleanupFunc()
			}()
		}
	} else {
		// Fallback for backward compatibility
		go func() {
			<-ctx.Done()
			cleanupFunc()
		}()
	}

	// Return a context that when cancelled, will trigger cleanup
	return &cleanupContext{Context: ctx, cancel: cancel}
}

type cleanupContext struct {
	context.Context
	cancel context.CancelFunc
}

func (c *cleanupContext) Done() <-chan struct{} {
	return c.Context.Done()
}

// ForceGC forces garbage collection and provides memory stats
func ForceGC() runtime.MemStats {
	runtime.GC()
	runtime.GC() // Run twice for better cleanup

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	return memStats
}
