package utils

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
	"time"
)

// GoroutineSnapshot represents a snapshot of all goroutines at a point in time
type GoroutineSnapshot struct {
	Timestamp   time.Time
	TotalCount  int
	Goroutines  map[string]GoroutineInfo
	StackTraces string
}

// GoroutineInfo contains details about a specific goroutine pattern
type GoroutineInfo struct {
	Count     int
	FirstSeen time.Time
	LastSeen  time.Time
	Sample    string // Sample stack trace
}

// LeakDetector monitors goroutine counts and identifies leaks
type LeakDetector struct {
	baseline    *GoroutineSnapshot
	snapshots   []*GoroutineSnapshot
	maxHistory  int
}

// NewLeakDetector creates a new goroutine leak detector
func NewLeakDetector() *LeakDetector {
	return &LeakDetector{
		maxHistory: 10, // Keep last 10 snapshots
		snapshots:  make([]*GoroutineSnapshot, 0),
	}
}

// TakeSnapshot captures current goroutine state
func (ld *LeakDetector) TakeSnapshot() *GoroutineSnapshot {
	// Get stack traces of all goroutines
	buf := make([]byte, 1<<20) // 1MB buffer
	stackSize := runtime.Stack(buf, true)
	stacks := string(buf[:stackSize])
	
	snapshot := &GoroutineSnapshot{
		Timestamp:   time.Now(),
		TotalCount:  runtime.NumGoroutine(),
		Goroutines:  make(map[string]GoroutineInfo),
		StackTraces: stacks,
	}
	
	// Parse goroutines by pattern
	ld.parseGoroutines(snapshot, stacks)
	
	// Add to history
	ld.snapshots = append(ld.snapshots, snapshot)
	if len(ld.snapshots) > ld.maxHistory {
		ld.snapshots = ld.snapshots[1:]
	}
	
	return snapshot
}

// SetBaseline sets the baseline snapshot (e.g., after startup)
func (ld *LeakDetector) SetBaseline() *GoroutineSnapshot {
	ld.baseline = ld.TakeSnapshot()
	return ld.baseline
}

// DetectLeaks compares current state with baseline and identifies potential leaks
func (ld *LeakDetector) DetectLeaks() []LeakReport {
	if ld.baseline == nil {
		return nil
	}
	
	current := ld.TakeSnapshot()
	var leaks []LeakReport
	
	// Compare with baseline
	for pattern, currentInfo := range current.Goroutines {
		baselineCount := 0
		if baseInfo, exists := ld.baseline.Goroutines[pattern]; exists {
			baselineCount = baseInfo.Count
		}
		
		// Consider it a leak if:
		// 1. Count increased by more than 5 goroutines
		// 2. Or new pattern appeared with more than 3 goroutines
		increase := currentInfo.Count - baselineCount
		if increase > 5 || (baselineCount == 0 && currentInfo.Count > 3) {
			leaks = append(leaks, LeakReport{
				Pattern:       pattern,
				BaselineCount: baselineCount,
				CurrentCount:  currentInfo.Count,
				Increase:      increase,
				Sample:        currentInfo.Sample,
				Duration:      current.Timestamp.Sub(ld.baseline.Timestamp),
			})
		}
	}
	
	// Sort by increase amount (worst leaks first)
	sort.Slice(leaks, func(i, j int) bool {
		return leaks[i].Increase > leaks[j].Increase
	})
	
	return leaks
}

// LeakReport describes a potential goroutine leak
type LeakReport struct {
	Pattern       string
	BaselineCount int
	CurrentCount  int
	Increase      int
	Sample        string
	Duration      time.Duration
}

// String formats the leak report
func (lr LeakReport) String() string {
	return fmt.Sprintf("LEAK: %s increased by %d (%d→%d) over %v\nSample: %s",
		lr.Pattern, lr.Increase, lr.BaselineCount, lr.CurrentCount, 
		lr.Duration.Round(time.Second), truncateStack(lr.Sample, 200))
}

// parseGoroutines extracts goroutine patterns from stack traces
func (ld *LeakDetector) parseGoroutines(snapshot *GoroutineSnapshot, stacks string) {
	lines := strings.Split(stacks, "\n")
	var currentGoroutine string
	var currentStack []string
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		
		// New goroutine starts with "goroutine N [state]:"
		if strings.HasPrefix(line, "goroutine ") && strings.Contains(line, "[") {
			// Process previous goroutine if exists
			if currentGoroutine != "" {
				ld.addGoroutinePattern(snapshot, currentGoroutine, currentStack)
			}
			
			// Start new goroutine
			currentGoroutine = line
			currentStack = []string{}
		} else {
			// Add to current stack trace
			currentStack = append(currentStack, line)
		}
	}
	
	// Process last goroutine
	if currentGoroutine != "" {
		ld.addGoroutinePattern(snapshot, currentGoroutine, currentStack)
	}
}

// addGoroutinePattern adds a goroutine to the snapshot, grouped by pattern
func (ld *LeakDetector) addGoroutinePattern(snapshot *GoroutineSnapshot, header string, stack []string) {
	// Extract meaningful pattern from the stack
	pattern := ld.extractPattern(header, stack)
	
	// Create sample stack trace (first few lines)
	sample := header
	if len(stack) > 0 {
		// Add first 2-3 lines of actual stack
		for i, line := range stack {
			if i >= 3 {
				break
			}
			if strings.Contains(line, ".go:") {
				sample += "\n  " + line
			}
		}
	}
	
	// Update or create pattern entry
	if info, exists := snapshot.Goroutines[pattern]; exists {
		info.Count++
		info.LastSeen = snapshot.Timestamp
		snapshot.Goroutines[pattern] = info
	} else {
		snapshot.Goroutines[pattern] = GoroutineInfo{
			Count:     1,
			FirstSeen: snapshot.Timestamp,
			LastSeen:  snapshot.Timestamp,
			Sample:    sample,
		}
	}
}

// extractPattern creates a meaningful pattern from goroutine info
func (ld *LeakDetector) extractPattern(header string, stack []string) string {
	// Extract state from header: "goroutine N [state]:"
	state := "unknown"
	if start := strings.Index(header, "["); start != -1 {
		if end := strings.Index(header[start:], "]"); end != -1 {
			state = header[start+1 : start+end]
		}
	}
	
	// Find the most relevant function in the stack
	var topFunc string
	for _, line := range stack {
		// Look for function calls (lines that don't contain .go:)
		if !strings.Contains(line, ".go:") && !strings.Contains(line, "runtime.") && strings.Contains(line, ".") {
			// This is likely a function name
			funcName := strings.TrimSpace(line)
			// Simplify long function names
			if lastDot := strings.LastIndex(funcName, "."); lastDot != -1 {
				funcName = funcName[lastDot+1:]
			}
			// Remove parameters if present
			if parenIndex := strings.Index(funcName, "("); parenIndex != -1 {
				funcName = funcName[:parenIndex]
			}
			topFunc = funcName
			break
		}
	}
	
	if topFunc == "" {
		// Fallback to runtime function
		for _, line := range stack {
			if strings.Contains(line, ".go:") {
				parts := strings.Fields(line)
				if len(parts) > 0 {
					funcName := parts[0]
					if lastDot := strings.LastIndex(funcName, "."); lastDot != -1 {
						funcName = funcName[lastDot+1:]
					}
					topFunc = funcName
					break
				}
			}
		}
	}
	
	if topFunc == "" {
		topFunc = "unknown"
	}
	
	return fmt.Sprintf("%s[%s]", topFunc, state)
}

// GetCurrentStats returns current goroutine statistics
func (ld *LeakDetector) GetCurrentStats() string {
	current := ld.TakeSnapshot()
	
	var result strings.Builder
	result.WriteString(fmt.Sprintf("Total Goroutines: %d\n", current.TotalCount))
	
	if ld.baseline != nil {
		increase := current.TotalCount - ld.baseline.TotalCount
		duration := current.Timestamp.Sub(ld.baseline.Timestamp)
		result.WriteString(fmt.Sprintf("Increase since baseline: +%d over %v\n", 
			increase, duration.Round(time.Second)))
	}
	
	// Show top patterns
	var patterns []string
	for pattern, info := range current.Goroutines {
		patterns = append(patterns, fmt.Sprintf("%s: %d", pattern, info.Count))
	}
	
	sort.Slice(patterns, func(i, j int) bool {
		// Sort by count (descending)
		countI := extractCount(patterns[i])
		countJ := extractCount(patterns[j])
		return countI > countJ
	})
	
	result.WriteString("\nTop Goroutine Patterns:\n")
	for i, pattern := range patterns {
		if i >= 10 { // Show top 10
			break
		}
		result.WriteString(fmt.Sprintf("  %s\n", pattern))
	}
	
	return result.String()
}

// Helper function to extract count from pattern string
func extractCount(pattern string) int {
	parts := strings.Split(pattern, ": ")
	if len(parts) == 2 {
		var count int
		fmt.Sscanf(parts[1], "%d", &count)
		return count
	}
	return 0
}

// Helper function to truncate long stack traces
func truncateStack(stack string, maxLen int) string {
	if len(stack) <= maxLen {
		return stack
	}
	return stack[:maxLen] + "..."
}

// LogLeaks logs detected leaks in a readable format
func (ld *LeakDetector) LogLeaks(logger func(level, msg, category string)) {
	leaks := ld.DetectLeaks()
	if len(leaks) == 0 {
		logger("info", "No goroutine leaks detected", "leak-detector")
		return
	}
	
	logger("warn", fmt.Sprintf("Detected %d potential goroutine leaks:", len(leaks)), "leak-detector")
	for i, leak := range leaks {
		if i >= 5 { // Show top 5 leaks only
			logger("info", fmt.Sprintf("... and %d more leaks", len(leaks)-i), "leak-detector")
			break
		}
		logger("warn", leak.String(), "leak-detector")
	}
}