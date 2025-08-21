package utils

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"
)

// SimpleLogger implements Logger interface for testing
type SimpleLogger struct{}

func (sl *SimpleLogger) Log(level, message, component string) {
	fmt.Printf("[%s] %s: %s\n", level, component, message)
}

// MemoryLeakTest provides utilities for testing memory leaks
type MemoryLeakTest struct {
	initialMemStats runtime.MemStats
	logger          *SimpleLogger
}

// NewMemoryLeakTest creates a new memory leak test
func NewMemoryLeakTest() *MemoryLeakTest {
	test := &MemoryLeakTest{
		logger: &SimpleLogger{},
	}
	
	// Force GC and get initial memory stats
	runtime.GC()
	runtime.GC()
	runtime.ReadMemStats(&test.initialMemStats)
	
	return test
}

// CheckMemoryLeak checks for memory leaks compared to initial state
func (mlt *MemoryLeakTest) CheckMemoryLeak(testName string) bool {
	// Force GC before checking
	runtime.GC()
	runtime.GC()
	
	var currentMemStats runtime.MemStats
	runtime.ReadMemStats(&currentMemStats)
	
	heapGrowth := int64(currentMemStats.HeapInuse) - int64(mlt.initialMemStats.HeapInuse)
	allocGrowth := int64(currentMemStats.TotalAlloc) - int64(mlt.initialMemStats.TotalAlloc)
	
	fmt.Printf("=== Memory Leak Test: %s ===\n", testName)
	fmt.Printf("Initial Heap InUse: %d bytes\n", mlt.initialMemStats.HeapInuse)
	fmt.Printf("Current Heap InUse: %d bytes\n", currentMemStats.HeapInuse)
	fmt.Printf("Heap Growth: %d bytes\n", heapGrowth)
	fmt.Printf("Total Alloc Growth: %d bytes\n", allocGrowth)
	fmt.Printf("Goroutines: %d\n", runtime.NumGoroutine())
	fmt.Println("====================================")
	
	// Consider it a leak if heap grew by more than 1MB without being cleaned up
	return heapGrowth > 1024*1024
}

// TestResourceTracker tests the resource tracker for memory leaks
func TestResourceTracker(t *testing.T) {
	test := NewMemoryLeakTest()
	
	// Test resource tracker
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	tracker := NewResourceTracker(ctx, test.logger)
	defer tracker.Shutdown()
	
	// Create and track many resources
	for i := 0; i < 1000; i++ {
		id := fmt.Sprintf("test-resource-%d", i)
		tracker.TrackResourceWithCleanup(id, ResourceBuffer, func() error {
			// Simulate cleanup
			return nil
		})
		
		if i%100 == 0 {
			// Periodically release some resources
			tracker.ReleaseResource(id)
		}
	}
	
	// Wait a bit for cleanup to occur
	time.Sleep(2 * time.Second)
	
	// Check for leaks
	if test.CheckMemoryLeak("ResourceTracker") {
		t.Error("Potential memory leak detected in ResourceTracker")
	}
}

// TestBufferPool tests the buffer pool for memory leaks
func TestBufferPool(t *testing.T) {
	test := NewMemoryLeakTest()
	
	pool := NewBufferPool(4096)
	
	// Simulate heavy buffer usage
	for i := 0; i < 10000; i++ {
		buf := pool.Get()
		
		// Use buffer
		for j := range buf {
			buf[j] = byte(i % 256)
		}
		
		// Return to pool
		pool.Put(buf)
		
		if i%1000 == 0 {
			runtime.GC()
		}
	}
	
	// Check for leaks
	if test.CheckMemoryLeak("BufferPool") {
		t.Error("Potential memory leak detected in BufferPool")
	}
}

// TestChannelManager tests the channel manager for memory leaks
func TestChannelManager(t *testing.T) {
	test := NewMemoryLeakTest()
	
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	cm := NewChannelManager(ctx)
	defer cm.Shutdown()
	
	// Create many channels
	for i := 0; i < 1000; i++ {
		id := fmt.Sprintf("test-channel-%d", i)
		ch := cm.CreateChannel(id)
		
		// Use channel briefly
		go func(ch chan struct{}) {
			select {
			case <-ch:
			case <-time.After(10 * time.Millisecond):
			}
		}(ch)
		
		if i%100 == 0 {
			cm.CloseChannel(id)
		}
	}
	
	time.Sleep(100 * time.Millisecond)
	
	// Check for leaks
	if test.CheckMemoryLeak("ChannelManager") {
		t.Error("Potential memory leak detected in ChannelManager")
	}
}

// TestCleanupManager tests the cleanup manager
func TestCleanupManager(t *testing.T) {
	test := NewMemoryLeakTest()
	
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	cm := NewCleanupManager(ctx, test.logger)
	
	// Register many cleanup functions
	for i := 0; i < 1000; i++ {
		id := fmt.Sprintf("cleanup-%d", i)
		cm.RegisterCleanup(id, func() error {
			// Simulate cleanup work
			time.Sleep(time.Microsecond)
			return nil
		})
	}
	
	// Run all cleanups
	errors := cm.Shutdown()
	if len(errors) > 0 {
		t.Errorf("Cleanup errors: %v", errors)
	}
	
	// Check for leaks
	if test.CheckMemoryLeak("CleanupManager") {
		t.Error("Potential memory leak detected in CleanupManager")
	}
}

// BenchmarkBufferPool benchmarks buffer pool performance
func BenchmarkBufferPool(b *testing.B) {
	pool := NewBufferPool(4096)
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			buf := pool.Get()
			// Simulate work
			buf[0] = 1
			pool.Put(buf)
		}
	})
}

// Example usage patterns for the memory leak prevention utilities
func ExampleMemoryLeakPrevention() {
	ctx := context.Background()
	logger := &SimpleLogger{}
	
	// 1. Use ResourceTracker for file descriptors
	tracker := NewResourceTracker(ctx, logger)
	defer tracker.Shutdown()
	
	// Track a file
	if file, err := tracker.NewTrackedFile("example.txt"); err == nil {
		defer file.Close() // Automatic cleanup
		// Use file...
	}
	
	// 2. Use BufferPool for efficient memory management
	err := SafeBufferOperation(P2PBufferPool, func(buf []byte) error {
		// Use buffer, automatically returned to pool
		copy(buf, []byte("example data"))
		return nil
	})
	if err != nil {
		logger.Log("error", fmt.Sprintf("Buffer operation failed: %v", err), "example")
	}
	
	// 3. Use CleanupManager for automatic cleanup
	cleanupMgr := NewCleanupManager(ctx, logger)
	defer cleanupMgr.Shutdown()
	
	cleanupMgr.RegisterCleanup("example", func() error {
		logger.Log("info", "Cleaning up example resources", "example")
		return nil
	})
	
	// 4. Use ChannelManager for channel lifecycle
	channelMgr := NewChannelManager(ctx)
	defer channelMgr.Shutdown()
	
	done := channelMgr.CreateChannel("example-work")
	go func() {
		// Do work
		time.Sleep(100 * time.Millisecond)
		channelMgr.CloseChannel("example-work")
	}()
	
	select {
	case <-done:
		logger.Log("info", "Work completed", "example")
	case <-time.After(1 * time.Second):
		logger.Log("warn", "Work timeout", "example")
	}
	
	// 5. Force garbage collection when needed
	memStats := ForceGC()
	logger.Log("info", 
		fmt.Sprintf("GC completed - HeapInuse: %d bytes", memStats.HeapInuse), 
		"example")
}