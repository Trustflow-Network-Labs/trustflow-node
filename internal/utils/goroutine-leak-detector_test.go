package utils

import (
	"context"
	"testing"
	"time"
)

func TestLeakDetector(t *testing.T) {
	detector := NewLeakDetector()
	
	// Set baseline with just test goroutines
	baseline := detector.SetBaseline()
	t.Logf("Baseline: %d goroutines", baseline.TotalCount)
	
	// Create some "leaky" goroutines that don't exit
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	// Start several goroutines that will "leak"
	for i := 0; i < 10; i++ {
		go func(id int) {
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					// Simulate work
				case <-ctx.Done():
					return
				}
			}
		}(i)
	}
	
	// Give goroutines time to start
	time.Sleep(200 * time.Millisecond)
	
	// Take snapshot and detect leaks
	currentStats := detector.GetCurrentStats()
	t.Logf("Current stats:\n%s", currentStats)
	
	leaks := detector.DetectLeaks()
	if len(leaks) == 0 {
		t.Error("Expected to detect goroutine leaks, but found none")
	} else {
		t.Logf("Detected %d leak types:", len(leaks))
		for i, leak := range leaks {
			t.Logf("%d. %s", i+1, leak.String())
		}
	}
	
	// Test leak logging
	detector.LogLeaks(func(level, msg, category string) {
		t.Logf("[%s] %s (%s)", level, msg, category)
	})
	
	// Cancel context to stop "leaked" goroutines
	cancel()
	time.Sleep(200 * time.Millisecond)
	
	// Verify goroutines were cleaned up
	finalStats := detector.GetCurrentStats()
	t.Logf("Final stats:\n%s", finalStats)
}

func TestGoroutinePatternExtraction(t *testing.T) {
	detector := NewLeakDetector()
	
	// Test pattern extraction with sample goroutine info
	testCases := []struct {
		header   string
		stack    []string
		expected string
	}{
		{
			header:   "goroutine 123 [chan receive]:",
			stack:    []string{"main.worker", "path/to/file.go:45", "runtime.gopark"},
			expected: "worker[chan receive]",
		},
		{
			header:   "goroutine 456 [sleep]:",
			stack:    []string{"time.Sleep", "runtime.sleep", "path/to/file.go:100"},
			expected: "Sleep[sleep]",
		},
		{
			header:   "goroutine 789 [select]:",
			stack:    []string{"github.com/libp2p/go-libp2p.handleConn", "net.go:200"},
			expected: "handleConn[select]",
		},
	}
	
	for _, tc := range testCases {
		pattern := detector.extractPattern(tc.header, tc.stack)
		if pattern != tc.expected {
			t.Errorf("Expected pattern %q, got %q for header %q", tc.expected, pattern, tc.header)
		}
	}
}