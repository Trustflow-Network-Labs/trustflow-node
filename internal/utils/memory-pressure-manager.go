package utils

import (
	"context"
	"fmt"
	"runtime"
	"runtime/debug"
	"time"
)

// GoroutineTracker interface for goroutine management
type GoroutineTracker interface {
	SafeStart(name string, fn func()) bool
	SafeStartCritical(name string, fn func()) bool
}

// MemoryPressureMonitor monitors memory usage and triggers cleanup
type MemoryPressureMonitor struct {
	thresholdPercent float64
	ticker           *time.Ticker
	ctx              context.Context
	cancel           context.CancelFunc
	logger           *LogsManager
	gt               GoroutineTracker
}

// NewMemoryPressureMonitor creates a memory pressure monitor
func NewMemoryPressureMonitor(thresholdPercent float64, logger *LogsManager, gt GoroutineTracker) *MemoryPressureMonitor {
	ctx, cancel := context.WithCancel(context.Background())

	monitor := &MemoryPressureMonitor{
		thresholdPercent: thresholdPercent,
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
			ForceGC()
		}
	}
}

// ForceGC forces garbage collection and provides memory stats
func ForceGC() runtime.MemStats {
	// Force garbage collection to clean up unused memory
	runtime.GC()
	runtime.GC() // Run twice for better cleanup

	// Free OS memory back to the system
	debug.FreeOSMemory()

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	return memStats
}
