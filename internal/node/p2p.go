package node

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"slices"

	blacklist_node "github.com/adgsm/trustflow-node/internal/blacklist-node"
	"github.com/adgsm/trustflow-node/internal/database"
	"github.com/adgsm/trustflow-node/internal/keystore"
	"github.com/adgsm/trustflow-node/internal/node_types"
	"github.com/adgsm/trustflow-node/internal/settings"
	"github.com/adgsm/trustflow-node/internal/ui"
	"github.com/adgsm/trustflow-node/internal/utils"
	"github.com/adgsm/trustflow-node/internal/workflow"
	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	drouting "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	dutil "github.com/libp2p/go-libp2p/p2p/discovery/util"
	rcmgr "github.com/libp2p/go-libp2p/p2p/host/resource-manager"
	"github.com/libp2p/go-libp2p/p2p/muxer/yamux"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	relayv2 "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
	identify "github.com/libp2p/go-libp2p/p2p/protocol/identify"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	libp2ptls "github.com/libp2p/go-libp2p/p2p/security/tls"
	ma "github.com/multiformats/go-multiaddr"
)

const LOOKUP_SERVICE string = "lookup.service"

// streamJob represents a stream processing job
type streamJob struct {
	stream     network.Stream
	jobType    string                // "proposal" or "message"
	streamData node_types.StreamData // Optional stream data for message processing
}

// streamInfo tracks information about active streams including data type for configurable timeouts
type streamInfo struct {
	startTime time.Time
	dataType  uint16          // Data type from StreamData (0-13)
	stream    network.Stream  // Reference to the actual stream for cleanup
}

type P2PManager struct {
	daemon                bool
	public                bool
	relay                 bool
	bootstrapAddrs        []string
	relayAddrs            []string
	topicNames            []string
	completeTopicNames    []string
	topicsSubscribed      map[string]*pubsub.Topic
	subscriptions         []*pubsub.Subscription
	protocolID            protocol.ID
	ps                    *pubsub.PubSub
	idht                  *dht.IpfsDHT
	h                     host.Host
	PeerId                peer.ID
	ctx                   context.Context
	cancel                context.CancelFunc // Context cancel function for clean shutdown
	tcm                   *TopicAwareConnectionManager
	DB                    *sql.DB
	cm                    *utils.ConfigManager
	Lm                    *utils.LogsManager
	leakDetector          *utils.LeakDetector
	cleanupCycleCount     int // Counter for detailed leak reporting frequency
	wm                    *workflow.WorkflowManager
	sc                    *node_types.ServiceOffersCache
	UI                    ui.UI
	peerDiscoveryTicker   *time.Ticker
	connectionMaintTicker *time.Ticker
	cronStopChan          chan struct{}
	cronWg                sync.WaitGroup               // For periodic maintenance tasks
	streamWorkersWg       sync.WaitGroup               // For stream workers specifically
	streamWorkerPool      chan chan streamJob          // Pool of worker channels
	streamWorkers         []chan streamJob             // Individual worker channels
	activeStreams         map[string]*streamInfo       // Track active streams with data type for cleanup
	streamsMutex          sync.RWMutex                 // Protect activeStreams map
	closingStreams        map[string]bool              // Track streams being closed
	closingMutex          sync.RWMutex                 // Protect closingStreams map
	activeGoroutines      int64                        // Track active goroutines (atomic)
	maxGoroutines         int64                        // Maximum allowed goroutines
	resourceTracker       *utils.ResourceTracker       // Memory leak prevention
	cleanupManager        *utils.CleanupManager        // Automatic cleanup
	memoryMonitor         *utils.MemoryPressureMonitor // Memory pressure monitoring
	relayTrafficMonitor   *RelayTrafficMonitor         // Monitor relay traffic for billing
	relayPeerSource       *TopicAwareRelayPeerSource   // Discover relay peers dynamically
	relayAdvertiser       *RelayServiceAdvertiser      // Advertise our relay service
	jobManager            *JobManager                  // Job management - single instance
	p2pResourceManager    *utils.P2PResourceManager    // Enhanced resource management with HTTP stats
}

func NewP2PManager(ctx context.Context, ui ui.UI, cm *utils.ConfigManager) *P2PManager {
	// Create cancellable context for clean shutdown
	ctx, cancel := context.WithCancel(ctx)

	// Create a database connection
	sqlm := database.NewSQLiteManager(cm)
	db, err := sqlm.CreateConnection()
	if err != nil {
		panic(err)
	}

	// Init log manager
	lm := utils.NewLogsManager(cm)

	// Load bootstrap peers addresses from configs
	var bootstrapAddrs []string
	bootstrapAddresses, exist := cm.GetConfig("bootstrap_addrs")
	if exist {
		bas := strings.SplitSeq(bootstrapAddresses, ",")
		for ba := range bas {
			bootstrapAddrs = append(bootstrapAddrs, strings.TrimSpace(ba))
		}
	}

	// Load relay peers addresses from configs
	var relayAddrs []string
	relayAddresses, exist := cm.GetConfig("relay_addrs")
	if exist {
		bas := strings.SplitSeq(relayAddresses, ",")
		for ba := range bas {
			relayAddrs = append(relayAddrs, strings.TrimSpace(ba))
		}
	}

	p2pm := &P2PManager{
		daemon:             false,
		public:             false,
		relay:              false,
		streamWorkerPool:   make(chan chan streamJob, cm.GetConfigInt("stream_worker_pool_size", 10, 1, 100)),
		streamWorkers:      make([]chan streamJob, 0, cm.GetConfigInt("stream_worker_pool_size", 10, 1, 100)),
		activeStreams:      make(map[string]*streamInfo),
		closingStreams:     make(map[string]bool),
		maxGoroutines:      int64(cm.GetConfigInt("max_goroutines", 200, 50, 1000)),
		bootstrapAddrs:     bootstrapAddrs,
		relayAddrs:         relayAddrs,
		topicNames:         []string{LOOKUP_SERVICE},
		completeTopicNames: []string{},
		topicsSubscribed:   make(map[string]*pubsub.Topic),
		subscriptions:      []*pubsub.Subscription{},
		protocolID:         "",
		idht:               nil,
		ps:                 nil,
		h:                  nil,
		ctx:                ctx,
		cancel:             cancel,
		tcm:                nil,
		DB:                 db,
		cm:                 cm,
		Lm:                 lm,
		leakDetector:       utils.NewLeakDetector(),
		wm:                 workflow.NewWorkflowManager(db, lm, cm),
		sc:                 node_types.NewServiceOffersCache(),
		UI:                 ui,
	}

	p2pm.ctx = ctx

	// Initialize memory leak prevention components
	p2pm.resourceTracker = utils.NewResourceTracker(p2pm.ctx, lm, p2pm.GetGoroutineTracker())
	p2pm.cleanupManager = utils.NewCleanupManager(p2pm.ctx, lm)

	// Create memory pressure monitor with configurable threshold
	memoryThreshold := cm.GetConfigFloat64("memory_pressure_threshold", 85.0, 50.0, 95.0)
	p2pm.memoryMonitor = utils.NewMemoryPressureMonitor(memoryThreshold, p2pm.cleanupManager, lm, p2pm.GetGoroutineTracker())

	// Start memory monitoring with configurable interval
	monitorInterval := cm.GetConfigDuration("memory_monitor_interval", 30*time.Second)
	p2pm.memoryMonitor.Start(monitorInterval)

	// Initialize context tracking for memory leak detection
	utils.InitializeContextTracking(lm, p2pm.GetGoroutineTracker())

	// Initialize buffer pools with configuration values
	utils.InitializeBufferPools(cm)

	// Register P2P cleanup functions
	p2pm.cleanupManager.RegisterCleanup("p2p-streams", func() error {
		p2pm.cleanupOldStreams()
		return nil
	})

	// Start periodic cleanup routine with critical priority
	gt := p2pm.GetGoroutineTracker()
	if !gt.SafeStartCritical("periodic-cleanup", p2pm.startPeriodicCleanup) {
		p2pm.Lm.Log("error", "CRITICAL: Failed to start P2P cleanup routine - system may leak memory!", "p2p")
	}

	// Initialize stream worker pool
	p2pm.initializeStreamWorkers()

	// Initialize enhanced P2P resource manager for monitoring and stats
	p2pm.p2pResourceManager = utils.NewP2PResourceManager(p2pm.ctx, lm)

	return p2pm
}

func (p2pm *P2PManager) Close() error {
	p2pm.Lm.Log("info", "Starting P2P manager shutdown with memory leak prevention", "p2p")

	// Cancel context first to signal all goroutines to stop
	if p2pm.cancel != nil {
		p2pm.cancel()
		p2pm.Lm.Log("info", "Context canceled - signaling all goroutines to stop", "p2p")
		// Reset context and cancel function after final shutdown
		p2pm.ctx = nil
		p2pm.cancel = nil
	}

	// Call Stop() to ensure proper P2P shutdown sequence
	if err := p2pm.Stop(); err != nil {
		p2pm.Lm.Log("warn", fmt.Sprintf("Error during P2P stop in Close(): %v", err), "p2p")
	}

	// Stop memory pressure monitor
	if p2pm.memoryMonitor != nil {
		p2pm.memoryMonitor.Stop()
	}

	// Shutdown enhanced P2P resource manager
	if p2pm.p2pResourceManager != nil {
		if err := p2pm.p2pResourceManager.Shutdown(); err != nil {
			p2pm.Lm.Log("warn", fmt.Sprintf("P2P resource manager shutdown error: %v", err), "p2p")
		}
	}

	// Run final cleanup before shutdown
	if p2pm.cleanupManager != nil {
		cleanupErrors := p2pm.cleanupManager.Shutdown()
		if len(cleanupErrors) > 0 {
			p2pm.Lm.Log("warn", fmt.Sprintf("Cleanup errors during shutdown: %v", cleanupErrors), "p2p")
		}
	}

	// Shutdown resource tracker
	if p2pm.resourceTracker != nil {
		if err := p2pm.resourceTracker.Shutdown(); err != nil {
			p2pm.Lm.Log("warn", fmt.Sprintf("Resource tracker shutdown error: %v", err), "p2p")
		}
	}

	// Force garbage collection before closing resources
	utils.ForceGC()

	// Close DB connection with timeout to prevent hanging
	p2pm.Lm.Log("info", "Closing database connection...", "p2p")
	dbClosed := make(chan error, 1)
	go func() {
		dbClosed <- p2pm.DB.Close()
	}()

	select {
	case err := <-dbClosed:
		if err != nil {
			p2pm.Lm.Log("error", fmt.Sprintf("Database close error: %v", err), "p2p")
			return err
		}
		p2pm.Lm.Log("info", "Database closed successfully", "p2p")
	case <-time.After(5 * time.Second):
		p2pm.Lm.Log("warn", "Database close timeout - proceeding with shutdown", "p2p")
	}

	// Close logs with timeout
	if p2pm.Lm != nil {
		p2pm.Lm.Log("info", "P2P manager shutdown completed", "p2p")

		logsClosed := make(chan error, 1)
		go func() {
			logsClosed <- p2pm.Lm.Close()
		}()

		select {
		case err := <-logsClosed:
			return err
		case <-time.After(3 * time.Second):
			// Log manager hanging, force exit
			fmt.Printf("Warning: Log manager close timeout, forcing exit\n")
			return nil
		}
	}
	return nil
}

// initializeStreamWorkers creates and starts the stream worker pool
func (p2pm *P2PManager) initializeStreamWorkers() {
	workerCount := 10 // Match the worker pool capacity

	// Get goroutine tracker
	gt := p2pm.GetGoroutineTracker()

	// Create individual worker channels and start workers
	for i := 0; i < workerCount; i++ {
		workerChan := make(chan streamJob, 1) // Buffer of 1 for non-blocking dispatch
		p2pm.streamWorkers = append(p2pm.streamWorkers, workerChan)

		// Add worker channel to the pool
		p2pm.streamWorkerPool <- workerChan

		// Start persistent worker goroutine with tracking
		p2pm.streamWorkersWg.Add(1)
		workerID := i // Capture loop variable
		if !gt.SafeStartCritical(fmt.Sprintf("stream-worker-%d", workerID), func() {
			p2pm.streamWorker(workerID, workerChan)
		}) {
			p2pm.Lm.Log("error", fmt.Sprintf("Failed to start stream worker %d", workerID), "p2p")
			p2pm.streamWorkersWg.Done() // Decrement since SafeStart failed
		}
	}

	p2pm.Lm.Log("info", fmt.Sprintf("Initialized %d stream workers", workerCount), "p2p")
}

// streamWorker is a persistent goroutine that processes stream jobs
func (p2pm *P2PManager) streamWorker(workerID int, jobChan chan streamJob) {
	defer p2pm.streamWorkersWg.Done()
	defer func() {
		if r := recover(); r != nil {
			p2pm.Lm.Log("error", fmt.Sprintf("Stream worker %d panic recovered: %v", workerID, r), "p2p")
		}
	}()

	p2pm.Lm.Log("debug", fmt.Sprintf("Stream worker %d started", workerID), "p2p")

	for {
		select {
		case job := <-jobChan:
			// Process the stream job
			p2pm.processStreamJob(job)

			// Return worker to pool for next job
			select {
			case p2pm.streamWorkerPool <- jobChan:
				// Worker returned to pool successfully
			case <-p2pm.cronStopChan:
				p2pm.Lm.Log("debug", fmt.Sprintf("Stream worker %d stopping (cronStopChan)", workerID), "p2p")
				return
			case <-p2pm.ctx.Done():
				p2pm.Lm.Log("debug", fmt.Sprintf("Stream worker %d stopping (context done)", workerID), "p2p")
				return
			}

		case <-p2pm.cronStopChan:
			p2pm.Lm.Log("debug", fmt.Sprintf("Stream worker %d stopping (cronStopChan)", workerID), "p2p")
			return

		case <-p2pm.ctx.Done():
			p2pm.Lm.Log("debug", fmt.Sprintf("Stream worker %d stopping (context done)", workerID), "p2p")
			return
		}
	}
}

// processStreamJob handles different types of stream jobs
func (p2pm *P2PManager) processStreamJob(job streamJob) {
	streamID := job.stream.ID()

	// Check if context is canceled before processing
	select {
	case <-p2pm.ctx.Done():
		p2pm.Lm.Log("debug", fmt.Sprintf("Skipping stream job %s - context canceled", streamID), "p2p")
		return
	default:
	}

	// Track stream processing start
	p2pm.Lm.Log("debug", fmt.Sprintf("Processing stream job %s (type: %s)", streamID, job.jobType), "p2p")

	defer func() {
		if r := recover(); r != nil {
			p2pm.Lm.Log("error", fmt.Sprintf("Stream job panic recovered for %s: %v", streamID, r), "p2p")
			// Use safe close instead of direct reset
			p2pm.safeCloseStream(job.stream, "panic recovery")
		}
	}()

	switch job.jobType {
	case "proposal":
		p2pm.streamProposalResponse(job.stream)
	case "message":
		// Register stream with resource tracker for monitoring only
		p2pm.resourceTracker.TrackResourceWithCleanup(streamID, utils.ResourceStream, func() error {
			p2pm.Lm.Log("debug", fmt.Sprintf("Resource tracker cleanup for stream %s", streamID), "p2p")
			return nil // Don't close here, let receivedStream handle it properly
		})
		defer p2pm.resourceTracker.ReleaseResource(streamID)

		p2pm.receivedStream(job.stream, job.streamData)
	default:
		p2pm.Lm.Log("warn", fmt.Sprintf("Unknown stream job type: %s", job.jobType), "p2p")
		p2pm.safeCloseStream(job.stream, "unknown job type")
	}
}

// submitStreamJob submits a stream job to the worker pool with timeout
func (p2pm *P2PManager) submitStreamJob(job streamJob) bool {
	// Try to get a worker with configurable timeout to reduce rejections
	dispatchTimeout := p2pm.cm.GetConfigDuration("stream_dispatch_timeout", 50*time.Millisecond)
	timeout := time.NewTimer(dispatchTimeout)
	defer timeout.Stop()

	select {
	case workerChan := <-p2pm.streamWorkerPool:
		// Got a worker, submit the job with timeout
		select {
		case workerChan <- job:
			return true // Job submitted successfully
		case <-timeout.C:
			// Worker timeout, return worker to pool and reject job
			p2pm.streamWorkerPool <- workerChan
			p2pm.Lm.Log("debug", fmt.Sprintf("Stream job timeout for %s", job.stream.ID()), "p2p")
			return false
		}
	case <-timeout.C:
		// No workers available within timeout
		return false
	}
}

// getGoroutineStats returns current goroutine statistics
func (p2pm *P2PManager) getGoroutineStats() (int64, int64) {
	active := atomic.LoadInt64(&p2pm.activeGoroutines)
	return active, p2pm.maxGoroutines
}

// GoroutineInfo tracks individual goroutine lifecycle
type GoroutineInfo struct {
	ID         string
	Name       string
	StartTime  time.Time
	LastPing   time.Time // For heartbeat tracking
	Critical   bool
	StackTrace string // Stack trace when started
}

// GoroutineTracker provides global goroutine tracking across all components
type GoroutineTracker struct {
	activeGoroutines *int64
	maxGoroutines    int64
	lm               *utils.LogsManager
	mu               sync.RWMutex
	goroutines       map[string]*GoroutineInfo // Track individual goroutines
}

// GetGoroutineTracker returns a goroutine tracker for use by other components
func (p2pm *P2PManager) GetGoroutineTracker() *GoroutineTracker {
	return &GoroutineTracker{
		activeGoroutines: &p2pm.activeGoroutines,
		maxGoroutines:    p2pm.maxGoroutines,
		lm:               p2pm.Lm,
		goroutines:       make(map[string]*GoroutineInfo),
	}
}

// SafeStart safely starts a goroutine with global tracking and limits
func (gt *GoroutineTracker) SafeStart(name string, fn func()) bool {
	return gt.SafeStartWithPriority(name, fn, false)
}

// SafeStartCritical starts a critical system goroutine that should always succeed
func (gt *GoroutineTracker) SafeStartCritical(name string, fn func()) bool {
	return gt.SafeStartWithPriority(name, fn, true)
}

// SafeStartWithPriority safely starts a goroutine with priority handling
func (gt *GoroutineTracker) SafeStartWithPriority(name string, fn func(), critical bool) bool {
	// Critical services get higher limit (reserve 30 goroutines for critical tasks)
	effectiveLimit := gt.maxGoroutines
	if !critical {
		effectiveLimit = gt.maxGoroutines - 30 // Reserve 30 slots for critical services
	}

	current := atomic.LoadInt64(gt.activeGoroutines)
	if current >= effectiveLimit {
		if critical {
			gt.lm.Log("warn", fmt.Sprintf("Critical goroutine %s started despite limit (%d active)", name, current), "goroutines")
		} else {
			gt.lm.Log("warn", fmt.Sprintf("Global goroutine limit reached (%d), rejecting %s", effectiveLimit, name), "goroutines")
			return false
		}
	}

	if atomic.AddInt64(gt.activeGoroutines, 1) > gt.maxGoroutines && !critical {
		atomic.AddInt64(gt.activeGoroutines, -1)
		gt.lm.Log("warn", fmt.Sprintf("Global goroutine limit exceeded, rejecting %s", name), "goroutines")
		return false
	}

	// Generate unique ID and capture stack trace
	goroutineID := fmt.Sprintf("%s_%d_%d", name, time.Now().UnixNano(), current)
	stackBuf := make([]byte, 2048)
	stackLen := runtime.Stack(stackBuf, false)
	stackTrace := string(stackBuf[:stackLen])

	// Register goroutine
	gt.mu.Lock()
	gt.goroutines[goroutineID] = &GoroutineInfo{
		ID:         goroutineID,
		Name:       name,
		StartTime:  time.Now(),
		LastPing:   time.Now(),
		Critical:   critical,
		StackTrace: stackTrace,
	}
	gt.mu.Unlock()

	gt.lm.Log("debug", fmt.Sprintf("Started goroutine %s (ID: %s)", name, goroutineID), "goroutines")

	// Execute function directly using helper method to avoid anonymous wrapper goroutines
	go gt.executeTrackedFunction(goroutineID, name, fn)

	return true
}

// executeTrackedFunction runs a function with tracking cleanup - separate method to avoid nested closures
func (gt *GoroutineTracker) executeTrackedFunction(goroutineID, name string, fn func()) {
	defer atomic.AddInt64(gt.activeGoroutines, -1)
	defer func() {
		gt.mu.Lock()
		delete(gt.goroutines, goroutineID)
		gt.mu.Unlock()
		gt.lm.Log("debug", fmt.Sprintf("Ended goroutine %s (ID: %s)", name, goroutineID), "goroutines")

		if r := recover(); r != nil {
			gt.lm.Log("error", fmt.Sprintf("Goroutine %s panic recovered: %v", name, r), "goroutines")
		}
	}()
	
	// Execute the actual function directly
	fn()
}

// Heartbeat updates the last ping time for a goroutine (optional heartbeat system)
func (gt *GoroutineTracker) Heartbeat(name string) {
	gt.mu.Lock()
	defer gt.mu.Unlock()

	for _, info := range gt.goroutines {
		if info.Name == name {
			info.LastPing = time.Now()
		}
	}
}

// GetStuckGoroutines returns goroutines that have been running longer than maxAge
func (gt *GoroutineTracker) GetStuckGoroutines(maxAge time.Duration) []*GoroutineInfo {
	gt.mu.RLock()
	defer gt.mu.RUnlock()

	var stuck []*GoroutineInfo
	cutoff := time.Now().Add(-maxAge)

	for _, info := range gt.goroutines {
		if info.StartTime.Before(cutoff) {
			stuck = append(stuck, info)
		}
	}

	return stuck
}

// GetGoroutineStats returns current goroutine tracking statistics
func (gt *GoroutineTracker) GetGoroutineStats() map[string]interface{} {
	gt.mu.RLock()
	defer gt.mu.RUnlock()

	stats := make(map[string]interface{})
	stats["total_tracked"] = len(gt.goroutines)
	stats["atomic_count"] = atomic.LoadInt64(gt.activeGoroutines)

	// Count by type
	typeCounts := make(map[string]int)
	criticalCount := 0

	for _, info := range gt.goroutines {
		typeCounts[info.Name]++
		if info.Critical {
			criticalCount++
		}
	}

	stats["by_type"] = typeCounts
	stats["critical_count"] = criticalCount

	return stats
}

// CheckGoroutineHealth performs health check and logs warnings about stuck goroutines
func (gt *GoroutineTracker) CheckGoroutineHealth() {
	// Check for goroutines stuck for more than 30 minutes
	stuckGoroutines := gt.GetStuckGoroutines(30 * time.Minute)

	if len(stuckGoroutines) > 0 {
		gt.lm.Log("warn", fmt.Sprintf("Found %d potentially stuck goroutines", len(stuckGoroutines)), "goroutines")

		for _, stuck := range stuckGoroutines {
			age := time.Since(stuck.StartTime)
			gt.lm.Log("warn", fmt.Sprintf("Stuck goroutine: %s (ID: %s) running for %v",
				stuck.Name, stuck.ID, age), "goroutines")

			// Log stack trace for critical stuck goroutines
			if stuck.Critical {
				gt.lm.Log("error", fmt.Sprintf("CRITICAL stuck goroutine %s stack trace:\n%s",
					stuck.Name, stuck.StackTrace), "goroutines")
			}
		}
	}

	// Log general statistics
	stats := gt.GetGoroutineStats()
	gt.lm.Log("debug", fmt.Sprintf("Goroutine health check - Tracked: %v, Atomic: %v, By type: %v",
		stats["total_tracked"], stats["atomic_count"], stats["by_type"]), "goroutines")
}

// safeCloseStream safely closes a stream with context-aware handling
func (p2pm *P2PManager) safeCloseStream(stream network.Stream, reason string) {
	streamID := stream.ID()

	// Check if stream is already being closed
	p2pm.closingMutex.Lock()
	if p2pm.closingStreams[streamID] {
		p2pm.closingMutex.Unlock()
		p2pm.Lm.Log("debug", fmt.Sprintf("Stream %s already being closed, skipping", streamID), "p2p")
		return
	}
	p2pm.closingStreams[streamID] = true
	p2pm.closingMutex.Unlock()

	// Check if context is already canceled before attempting close
	select {
	case <-p2pm.ctx.Done():
		// Context canceled - stream is already being closed by libp2p
		p2pm.Lm.Log("debug", fmt.Sprintf("Stream %s not closed - context already canceled (%s)", streamID, reason), "p2p")
	default:
		// Context active - safe to attempt graceful close
		if err := stream.Close(); err != nil {
			// Check if error is due to canceled context/stream
			errStr := err.Error()
			if strings.Contains(errStr, "canceled") || strings.Contains(errStr, "closed") {
				p2pm.Lm.Log("debug", fmt.Sprintf("Stream %s already canceled/closed (%s): %v", streamID, reason, err), "p2p")
			} else {
				p2pm.Lm.Log("debug", fmt.Sprintf("Stream %s close failed (%s), using reset: %v", streamID, reason, err), "p2p")
				// Only reset if it's not a cancellation error
				if !strings.Contains(errStr, "canceled") {
					stream.Reset()
				}
			}
		} else {
			p2pm.Lm.Log("debug", fmt.Sprintf("Stream %s closed gracefully (%s)", streamID, reason), "p2p")
		}
	}

	// Clean up tracking
	p2pm.closingMutex.Lock()
	delete(p2pm.closingStreams, streamID)
	p2pm.closingMutex.Unlock()

	p2pm.streamsMutex.Lock()
	delete(p2pm.activeStreams, streamID)
	p2pm.streamsMutex.Unlock()
}

// Start all P2P periodic tasks
func (p2pm *P2PManager) StartPeriodicTasks() error {
	// Get ticker intervals from config
	peerDiscoveryInterval, connectionHealthInterval, err := p2pm.getTickerIntervals()
	if err != nil {
		p2pm.Lm.Log("error", fmt.Sprintf("Can not read configs file. (%s)", err.Error()), "p2p")
		return err
	}

	// Initialize stop channel
	p2pm.cronStopChan = make(chan struct{})

	// Start peer discovery periodic task
	p2pm.peerDiscoveryTicker = time.NewTicker(peerDiscoveryInterval)
	p2pm.cronWg.Add(1)
	gt := p2pm.GetGoroutineTracker()
	gt.SafeStart("peer-discovery-ticker", func() {
		defer p2pm.cronWg.Done()
		defer p2pm.peerDiscoveryTicker.Stop()

		p2pm.Lm.Log("info", fmt.Sprintf("Started peer discovery every %v", peerDiscoveryInterval), "p2p")

		for {
			select {
			case <-p2pm.peerDiscoveryTicker.C:
				p2pm.DiscoverPeers()
			case <-p2pm.cronStopChan:
				p2pm.Lm.Log("info", "Stopped peer discovery periodic task", "p2p")
				return
			case <-p2pm.ctx.Done():
				p2pm.Lm.Log("info", "Context cancelled, stopping peer discovery", "p2p")
				return
			}
		}
	})

	// Start connection maintenance periodic task
	p2pm.connectionMaintTicker = time.NewTicker(connectionHealthInterval)
	p2pm.cronWg.Add(1)
	gt.SafeStart("connection-maintenance", func() {
		defer p2pm.cronWg.Done()
		defer p2pm.connectionMaintTicker.Stop()

		p2pm.Lm.Log("info", fmt.Sprintf("Started connection maintenance every %v", connectionHealthInterval), "p2p")

		for {
			select {
			case <-p2pm.connectionMaintTicker.C:
				p2pm.MaintainConnections()
			case <-p2pm.cronStopChan:
				p2pm.Lm.Log("info", "Stopped connection maintenance periodic task", "p2p")
				return
			case <-p2pm.ctx.Done():
				p2pm.Lm.Log("info", "Context cancelled, stopping connection maintenance", "p2p")
				return
			}
		}
	})

	// Start stream cleanup periodic task with configurable interval
	streamCleanupInterval := p2pm.cm.GetConfigDuration("stream_cleanup_interval", 1*time.Minute)

	streamCleanupTicker := time.NewTicker(streamCleanupInterval)
	p2pm.cronWg.Add(1)
	gt.SafeStart("stream-cleanup", func() {
		defer p2pm.cronWg.Done()
		defer streamCleanupTicker.Stop()

		p2pm.Lm.Log("info", fmt.Sprintf("Started stream cleanup every %v", streamCleanupInterval), "p2p")

		for {
			select {
			case <-streamCleanupTicker.C:
				p2pm.cleanupStaleStreams()
			case <-p2pm.cronStopChan:
				p2pm.Lm.Log("info", "Stopped stream cleanup periodic task", "p2p")
				return
			case <-p2pm.ctx.Done():
				p2pm.Lm.Log("info", "Context cancelled, stopping stream cleanup", "p2p")
				return
			}
		}
	})

	// Start service offers cache cleanup periodic task with configurable interval
	cacheCleanupInterval := p2pm.cm.GetConfigDuration("cache_cleanup_interval", 30*time.Minute)

	cacheCleanupTicker := time.NewTicker(cacheCleanupInterval)
	p2pm.cronWg.Add(1)
	gt.SafeStart("cache-cleanup", func() {
		defer p2pm.cronWg.Done()
		defer cacheCleanupTicker.Stop()

		p2pm.Lm.Log("info", fmt.Sprintf("Started cache cleanup every %v", cacheCleanupInterval), "p2p")

		for {
			select {
			case <-cacheCleanupTicker.C:
				p2pm.cleanupCaches()
			case <-p2pm.cronStopChan:
				p2pm.Lm.Log("info", "Stopped cache cleanup periodic task", "p2p")
				return
			case <-p2pm.ctx.Done():
				p2pm.Lm.Log("info", "Context cancelled, stopping cache cleanup", "p2p")
				return
			}
		}
	})

	p2pm.Lm.Log("info", "Started all P2P periodic tasks", "p2p")
	return nil
}

// Stop all P2P periodic tasks
func (p2pm *P2PManager) StopPeriodicTasks() {
	if p2pm.cronStopChan != nil {
		select {
		case <-p2pm.cronStopChan:
			// Channel already closed
			return
		default:
			close(p2pm.cronStopChan)
		}

		// Wait for periodic tasks (cron jobs) to finish first
		cronDone := make(chan struct{})
		gt := p2pm.GetGoroutineTracker()
		gt.SafeStart("stop-cron-waitgroup", func() {
			p2pm.cronWg.Wait()
			close(cronDone)
		})

		select {
		case <-cronDone:
			p2pm.Lm.Log("info", "All periodic tasks stopped gracefully", "p2p")
		case <-time.After(3 * time.Second):
			p2pm.Lm.Log("warn", "Timeout waiting for periodic tasks to stop, proceeding with stream worker cleanup", "p2p")
		}

		// Clean up worker pool resources (signals stream workers to stop)
		p2pm.cleanupStreamWorkers()

		// Wait for stream workers to finish with separate timeout
		streamsDone := make(chan struct{})
		gt.SafeStart("stop-streams-waitgroup", func() {
			p2pm.streamWorkersWg.Wait()
			close(streamsDone)
		})

		select {
		case <-streamsDone:
			p2pm.Lm.Log("info", "All stream workers stopped gracefully", "p2p")
		case <-time.After(5 * time.Second):
			p2pm.Lm.Log("warn", "Timeout waiting for stream workers to stop, proceeding with shutdown", "p2p")
		}

		p2pm.Lm.Log("info", "Stopped all P2P periodic tasks and stream workers", "p2p")
		p2pm.cronStopChan = nil // Prevent future close attempts
	}
}

// cleanupStreamWorkers closes all worker channels and clears the pool
func (p2pm *P2PManager) cleanupStreamWorkers() {
	// Close all individual worker channels
	for _, workerChan := range p2pm.streamWorkers {
		select {
		case <-workerChan:
			// Channel already closed
		default:
			close(workerChan)
		}
	}

	// Clear the worker slice
	p2pm.streamWorkers = p2pm.streamWorkers[:0]

	// Drain the worker pool channel
	for len(p2pm.streamWorkerPool) > 0 {
		<-p2pm.streamWorkerPool
	}

	p2pm.Lm.Log("debug", "Stream worker pool cleanup completed", "p2p")
}

// getMaxStreamAgeForType returns the configured max age for a specific stream data type
func (p2pm *P2PManager) getMaxStreamAgeForType(dataType uint16) time.Duration {
	switch dataType {
	case 0: // *[]node_types.ServiceOffer
		return p2pm.cm.GetConfigDuration("max_stream_age_service_offer", 2*time.Minute)
	case 1: // *node_types.JobRunRequest
		return p2pm.cm.GetConfigDuration("max_stream_age_job_run_request", 5*time.Minute)
	case 2: // *[]byte
		return p2pm.cm.GetConfigDuration("max_stream_age_byte_array", 2*time.Hour)
	case 3: // *os.File
		return p2pm.cm.GetConfigDuration("max_stream_age_file", 4*time.Hour)
	case 4: // *node_types.ServiceRequest
		return p2pm.cm.GetConfigDuration("max_stream_age_service_request", 1*time.Minute)
	case 5: // *node_types.ServiceResponse
		return p2pm.cm.GetConfigDuration("max_stream_age_service_response", 2*time.Minute)
	case 6: // *node_types.JobRunResponse
		return p2pm.cm.GetConfigDuration("max_stream_age_job_run_response", 3*time.Minute)
	case 7: // *node_types.JobRunStatus
		return p2pm.cm.GetConfigDuration("max_stream_age_job_run_status", 1*time.Minute)
	case 8: // *node_types.JobRunStatusRequest
		return p2pm.cm.GetConfigDuration("max_stream_age_job_run_status_request", 1*time.Minute)
	case 9, 10, 11, 12: // Other message types
		return p2pm.cm.GetConfigDuration("max_stream_age_message", 2*time.Minute)
	case 13: // *node_types.ServiceLookup
		return p2pm.cm.GetConfigDuration("max_stream_age_service_lookup", 30*time.Second)
	default:
		// Fallback to general max_stream_age config
		return p2pm.cm.GetConfigDuration("max_stream_age", 10*time.Minute)
	}
}

// cleanupStaleStreams removes streams that have been active for too long and actually closes them
func (p2pm *P2PManager) cleanupStaleStreams() {
	p2pm.streamsMutex.Lock()
	defer p2pm.streamsMutex.Unlock()

	now := time.Now()
	staleStreams := make([]string, 0)

	// Find stale streams based on their individual data type timeouts
	for streamID, streamInfo := range p2pm.activeStreams {
		maxAge := p2pm.getMaxStreamAgeForType(streamInfo.dataType)
		if now.Sub(streamInfo.startTime) > maxAge {
			staleStreams = append(staleStreams, streamID)
		}
	}

	// Close stale streams before removing from tracking
	for _, streamID := range staleStreams {
		streamInfo := p2pm.activeStreams[streamID]
		p2pm.Lm.Log("debug", fmt.Sprintf("Closing stale stream %s (type: %d, age: %v)", 
			streamID, streamInfo.dataType, now.Sub(streamInfo.startTime)), "p2p")
		
		// Close the stream using the stored reference
		if streamInfo.stream != nil {
			p2pm.safeCloseStream(streamInfo.stream, "stale stream cleanup")
		}
		
		// Remove from tracking
		delete(p2pm.activeStreams, streamID)
	}

	if len(staleStreams) > 0 {
		p2pm.Lm.Log("info", fmt.Sprintf("Cleaned up %d stale streams (total active: %d)",
			len(staleStreams), len(p2pm.activeStreams)), "p2p")
	}
}

// cleanupCaches performs periodic cleanup of various caches to prevent memory leaks
func (p2pm *P2PManager) cleanupCaches() {
	// Service Offers Cache cleanup
	cacheTTL := p2pm.cm.GetConfigDuration("cache_ttl", 1*time.Hour)

	// Get count before cleanup for logging
	initialCount := len(p2pm.sc.ServiceOffers)
	p2pm.sc.PruneExpired(cacheTTL)
	finalCount := len(p2pm.sc.ServiceOffers)

	if initialCount > finalCount {
		p2pm.Lm.Log("info",
			fmt.Sprintf("Cleaned up %d expired service offers (%d -> %d)",
				initialCount-finalCount, initialCount, finalCount),
			"p2p")
	}

	// Add other cache cleanups here as needed
	// For example, if there are other caches that need periodic cleanup
}

// GetActiveStreamsCount returns the current number of active streams for monitoring
func (p2pm *P2PManager) GetActiveStreamsCount() int {
	p2pm.streamsMutex.RLock()
	defer p2pm.streamsMutex.RUnlock()
	return len(p2pm.activeStreams)
}

// GetServiceCacheCount returns the current number of cached service offers for monitoring
func (p2pm *P2PManager) GetServiceCacheCount() int {
	p2pm.sc.Lock()
	defer p2pm.sc.Unlock()
	return len(p2pm.sc.ServiceOffers)
}

// GetMemoryUsageInfo returns a diagnostic summary for memory leak monitoring
func (p2pm *P2PManager) GetMemoryUsageInfo() map[string]int {
	info := make(map[string]int)

	// Active streams count
	info["active_streams"] = p2pm.GetActiveStreamsCount()

	// Service offers cache count
	info["service_offers"] = p2pm.GetServiceCacheCount()

	// Connection manager cache counts
	if p2pm.tcm != nil {
		info["topic_peers"] = len(p2pm.tcm.topicPeers)
		info["routing_peers"] = len(p2pm.tcm.routingPeers)
		info["peer_stats_cache"] = p2pm.tcm.peerStatsCache.Len()
	}

	// Relay traffic monitor
	if p2pm.relayTrafficMonitor != nil {
		info["relay_circuits"] = len(p2pm.relayTrafficMonitor.activeCircuits)
	}

	// Worker pool utilization
	info["stream_workers_available"] = len(p2pm.streamWorkerPool)                         // Available workers in pool
	info["stream_workers_busy"] = cap(p2pm.streamWorkerPool) - len(p2pm.streamWorkerPool) // Busy workers
	info["stream_workers_total"] = cap(p2pm.streamWorkerPool)                             // Total workers

	// Goroutine tracking
	active, max := p2pm.getGoroutineStats()
	info["goroutines_active"] = int(active)
	info["goroutines_max"] = int(max)
	info["goroutines_usage_percent"] = int(float64(active) / float64(max) * 100)

	return info
}

// GetEnhancedResourceStats returns detailed resource statistics from P2PResourceManager
func (p2pm *P2PManager) GetEnhancedResourceStats() map[string]any {
	if p2pm.p2pResourceManager == nil {
		return map[string]any{"error": "P2P resource manager not initialized"}
	}

	// Get detailed stats from the resource manager
	return p2pm.p2pResourceManager.GetResourceStats()
}

// Get ticker intervals from config (no cron parsing needed)
func (p2pm *P2PManager) getTickerIntervals() (time.Duration, time.Duration, error) {
	// Use simple duration strings in config instead of cron
	peerDiscoveryInterval := 5 * time.Minute // Default
	if val, exists := p2pm.cm.GetConfig("peer_discovery_interval"); exists {
		if duration, err := time.ParseDuration(val); err == nil {
			peerDiscoveryInterval = duration
		}
	}

	connectionHealthInterval := 2 * time.Minute // Default
	if val, exists := p2pm.cm.GetConfig("connection_health_interval"); exists {
		if duration, err := time.ParseDuration(val); err == nil {
			connectionHealthInterval = duration
		}
	}

	return peerDiscoveryInterval, connectionHealthInterval, nil
}

// IsHostRunning checks if the provided host is actively running
func (p2pm *P2PManager) IsHostRunning() bool {
	if p2pm.h == nil {
		return false
	}
	// Check if the host is listening on any network addresses
	return len(p2pm.h.Network().ListenAddresses()) > 0
}

// WaitForHostReady tries multiple times to check if the host is initialized and running
func (p2pm *P2PManager) WaitForHostReady(interval time.Duration, maxAttempts int) bool {
	for range maxAttempts {
		if p2pm.IsHostRunning() {
			return true // success
		}
		time.Sleep(interval)
	}
	return false
}

// Start p2p node
func (p2pm *P2PManager) Start(ctx context.Context, port uint16, daemon bool, public bool, relay bool) error {
	p2pm.daemon = daemon
	p2pm.public = public
	p2pm.relay = relay

	// Use the provided context (needed for Wails GUI runtime calls)
	p2pm.ctx = ctx

	// Reinitialize stream workers if they were shut down
	if len(p2pm.streamWorkers) == 0 {
		p2pm.initializeStreamWorkers()
	}

	blacklistManager, err := blacklist_node.NewBlacklistNodeManager(p2pm.DB, p2pm.UI, p2pm.Lm)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	// Read port number from configs
	if port == 0 {
		p := p2pm.cm.GetConfigWithDefault("node_port", "30609")
		p64, err := strconv.ParseUint(p, 10, 16)
		if err != nil {
			port = 30609
		} else {
			port = uint16(p64)
		}
	}

	// Read topics' names to subscribe
	p2pm.completeTopicNames = p2pm.completeTopicNames[:0]
	topicNamePrefix := p2pm.cm.GetConfigWithDefault("topic_name_prefix", "trustflow.network.")
	for _, topicName := range p2pm.topicNames {
		topicName = topicNamePrefix + strings.TrimSpace(topicName)
		p2pm.completeTopicNames = append(p2pm.completeTopicNames, topicName)
	}

	// Read streaming protocol
	p2pm.protocolID = protocol.ID(p2pm.cm.GetConfigWithDefault("protocol_id", "/trustflow-network/1.0.0"))

	// Create or get previously created node key
	keystoreManager := keystore.NewKeyStoreManager(p2pm.DB, p2pm.Lm)
	priv, _, err := keystoreManager.ProvideKey()
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	// Get botstrap & relay hosts addr info
	var bootstrapAddrsInfo []peer.AddrInfo
	var relayAddrsInfo []peer.AddrInfo
	for _, relayAddr := range p2pm.relayAddrs {
		relayAddrInfo, err := p2pm.makeRelayPeerInfo(relayAddr)
		if err != nil {
			p2pm.Lm.Log("error", err.Error(), "p2p")
			continue
		}
		relayAddrsInfo = append(relayAddrsInfo, relayAddrInfo)
	}
	for _, bootstrapAddr := range p2pm.bootstrapAddrs {
		bootstrapAddrInfo, err := p2pm.makeRelayPeerInfo(bootstrapAddr)
		if err != nil {
			p2pm.Lm.Log("error", err.Error(), "p2p")
			continue
		}
		bootstrapAddrsInfo = append(bootstrapAddrsInfo, bootstrapAddrInfo)
	}

	// Default DHT bootstrap peers + ours
	var bootstrapPeers []peer.AddrInfo
	for _, peerAddr := range dht.DefaultBootstrapPeers {
		peerinfo, _ := peer.AddrInfoFromP2pAddr(peerAddr)
		bootstrapPeers = append(bootstrapPeers, *peerinfo)
	}
	bootstrapPeers = append(bootstrapPeers, bootstrapAddrsInfo...)

	maxConnections := 400

	// Create resource manager with strict limits to prevent memory/goroutine leaks
	// Use conservative autoscaled limits to control resource usage
	limits := rcmgr.DefaultLimits.AutoScale()

	// Use a fixed limiter with autoscaled limits for better control
	limiter := rcmgr.NewFixedLimiter(limits)
	resourceManager, err := rcmgr.NewResourceManager(limiter)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	var hst host.Host
	if p2pm.public {
		maxConnections = 800
		// Configure connection manager with stricter limits to prevent goroutine leaks
		connMgr, err := connmgr.NewConnManager(
			50,             // Low water mark - reduced to limit baseline connections
			maxConnections, // High water mark - maximum connections before pruning
			connmgr.WithGracePeriod(30*time.Second),   // Shorter grace period for faster cleanup
			connmgr.WithSilencePeriod(15*time.Second), // Add silence period to avoid connection churn
		)
		if err != nil {
			p2pm.Lm.Log("error", err.Error(), "p2p")
			return err
		}
		hst, err = p2pm.createPublicHost(priv, port, resourceManager, connMgr, blacklistManager, bootstrapPeers, relayAddrsInfo)
		if err != nil {
			p2pm.Lm.Log("error", err.Error(), "p2p")
			return err
		}
	} else {
		// Configure connection manager with stricter limits to prevent goroutine leaks
		connMgr, err := connmgr.NewConnManager(
			50,             // Low water mark - reduced to limit baseline connections
			maxConnections, // High water mark - maximum connections before pruning
			connmgr.WithGracePeriod(30*time.Second),   // Shorter grace period for faster cleanup
			connmgr.WithSilencePeriod(15*time.Second), // Add silence period to avoid connection churn
		)
		if err != nil {
			p2pm.Lm.Log("error", err.Error(), "p2p")
			return err
		}
		hst, err = p2pm.createPrivateHost(priv, port, resourceManager, connMgr, blacklistManager, bootstrapPeers, relayAddrsInfo)
		if err != nil {
			p2pm.Lm.Log("error", err.Error(), "p2p")
			return err
		}
	}

	// Set peer ID prop
	p2pm.PeerId = hst.ID()

	// Set up identify service
	_, err = identify.NewIDService(hst)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		p2pm.UI.Print(err.Error())
	}

	message := fmt.Sprintf("Your Peer ID: %s", hst.ID())
	p2pm.Lm.Log("info", message, "p2p")
	p2pm.UI.Print(message)
	for _, addr := range hst.Addrs() {
		fullAddr := addr.Encapsulate(ma.StringCast("/p2p/" + hst.ID().String()))
		message := fmt.Sprintf("Listening on %s", fullAddr)
		p2pm.Lm.Log("info", message, "p2p")
		p2pm.UI.Print(message)
	}

	// Setup a stream handler with bounded concurrency.
	// This gets called every time a peer connects and opens a stream to this node.
	p2pm.h.SetStreamHandler(p2pm.protocolID, func(s network.Stream) {
		message := fmt.Sprintf("Remote peer `%s` started streaming to our node `%s` in stream id `%s`, using protocol id: `%s`",
			s.Conn().RemotePeer().String(), hst.ID(), s.ID(), p2pm.protocolID)
		p2pm.Lm.Log("info", message, "p2p")

		// Submit stream job to worker pool instead of creating new goroutine
		job := streamJob{
			stream:  s,
			jobType: "proposal",
		}

		if !p2pm.submitStreamJob(job) {
			// Worker pool full, handle gracefully
			p2pm.Lm.Log("warn", "Stream worker pool at capacity, gracefully closing stream "+s.ID(), "p2p")
			p2pm.safeCloseStream(s, "worker pool full")
		}
	})

	routingDiscovery := drouting.NewRoutingDiscovery(p2pm.idht)
	p2pm.ps, err = pubsub.NewGossipSub(p2pm.ctx, p2pm.h,
		pubsub.WithPeerExchange(true),
		pubsub.WithFloodPublish(true),
		pubsub.WithDiscovery(routingDiscovery),
	)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	tcm, err := NewTopicAwareConnectionManager(p2pm, maxConnections, p2pm.completeTopicNames)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}
	p2pm.tcm = tcm

	gt := p2pm.GetGoroutineTracker()
	gt.SafeStart("topic-subscription", func() {
		p2pm.subscriptions = p2pm.subscriptions[:0]
		for _, completeTopicName := range p2pm.completeTopicNames {
			sub, topic, err := p2pm.joinSubscribeTopic(p2pm.ctx, p2pm.ps, completeTopicName)
			if err != nil {
				p2pm.Lm.Log("error", err.Error(), "p2p")
				panic(fmt.Sprintf("%v", err))
			}
			p2pm.subscriptions = append(p2pm.subscriptions, sub)

			notifyManager := NewTopicAwareNotifiee(p2pm.ps, topic, completeTopicName, bootstrapPeers, tcm, p2pm.Lm)

			// Attach the notifiee to the host's network
			p2pm.h.Network().Notify(notifyManager)
		}
	})

	// Run initial peer discovery immediately, then ticker will handle periodic calls
	p2pm.DiscoverPeers()

	// Start P2P periodic tasks
	err = p2pm.StartPeriodicTasks()
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	// Start relay billing logger if relay is enabled
	if p2pm.relay && p2pm.relayTrafficMonitor != nil {
		if !gt.SafeStart("relay-billing", func() { p2pm.StartRelayBillingLogger(p2pm.ctx) }) {
			p2pm.Lm.Log("warn", "Failed to start relay billing logger due to goroutine limit", "p2p")
		}
	}

	// Initialize and start job periodic tasks
	p2pm.jobManager = NewJobManager(p2pm)
	err = p2pm.jobManager.StartPeriodicTasks()
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "jobs")
		return err
	}

	// Start pprof server for memory profiling and heap metrics with critical priority
	pprofPort := p2pm.cm.GetConfigWithDefault("pprof_port", "6060")
	tracker := p2pm.GetGoroutineTracker()
	if !tracker.SafeStartCritical("pprof-server", func() {
		p2pm.Lm.Log("info", fmt.Sprintf("Starting pprof server on port %s", pprofPort), "pprof")

		// Try multiple ports with proper error handling
		ports := []string{pprofPort, "6061", "6062"} // Fallback ports
		var server *http.Server
		var listener net.Listener
		var err error

		// Setup custom mux with both pprof and resource stats endpoints
		mux := http.NewServeMux()

		// Add pprof endpoints manually since we're using a custom mux
		mux.HandleFunc("/debug/pprof/", func(w http.ResponseWriter, r *http.Request) {
			http.DefaultServeMux.ServeHTTP(w, r)
		})

		// Add resource stats endpoints
		mux.HandleFunc("/stats/resources", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")

			// Get basic memory usage info
			basicStats := p2pm.GetMemoryUsageInfo()

			// Get enhanced resource stats from P2PResourceManager
			enhancedStats := p2pm.GetEnhancedResourceStats()

			// Combine both sets of statistics
			combined := map[string]any{
				"basic":     basicStats,
				"enhanced":  enhancedStats,
				"timestamp": time.Now().Format(time.RFC3339),
			}

			if err := json.NewEncoder(w).Encode(combined); err != nil {
				p2pm.Lm.Log("error", fmt.Sprintf("Failed to encode resource stats: %v", err), "http-stats")
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
		})

		// Add goroutine leak report endpoint
		mux.HandleFunc("/stats/leaks", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			report := p2pm.GetGoroutineLeakReport()
			w.Write([]byte(report))
		})

		// Add health check endpoint
		mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")

			health := map[string]any{
				"status":         "ok",
				"host_running":   p2pm.IsHostRunning(),
				"active_streams": p2pm.GetActiveStreamsCount(),
				"timestamp":      time.Now().Format(time.RFC3339),
			}

			json.NewEncoder(w).Encode(health)
		})

		for i, port := range ports {
			// Create server with context for graceful shutdown
			server = &http.Server{
				Addr:    ":" + port,
				Handler: mux, // Use custom mux with stats endpoints
			}

			// Try to listen on this port
			listener, err = net.Listen("tcp", ":"+port)
			if err != nil {
				if i < len(ports)-1 {
					p2pm.Lm.Log("warn", fmt.Sprintf("pprof port %s unavailable, trying next port: %v", port, err), "pprof")
					continue
				} else {
					p2pm.Lm.Log("error", fmt.Sprintf("All pprof ports failed, last error: %v", err), "pprof")
					return
				}
			}

			p2pm.Lm.Log("info", fmt.Sprintf("HTTP server with pprof and resource stats successfully bound to port %s", port), "pprof")
			p2pm.Lm.Log("info", fmt.Sprintf("Available endpoints: /debug/pprof/, /stats/resources, /stats/leaks, /health on port %s", port), "pprof")
			break
		}

		// Start server with graceful shutdown handling using tracked goroutine
		shutdownTracker := p2pm.GetGoroutineTracker()
		shutdownTracker.SafeStart("pprof-shutdown", func() {
			<-p2pm.ctx.Done()
			p2pm.Lm.Log("info", "Shutting down pprof server...", "pprof")

			// First try graceful shutdown with shorter timeout
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			if err := server.Shutdown(shutdownCtx); err != nil {
				p2pm.Lm.Log("warn", fmt.Sprintf("Graceful pprof server shutdown failed: %v, forcing close", err), "pprof")
				// Force close the server if graceful shutdown fails
				if err := server.Close(); err != nil {
					p2pm.Lm.Log("error", fmt.Sprintf("Force close pprof server failed: %v", err), "pprof")
				}
			} else {
				p2pm.Lm.Log("info", "pprof server shutdown gracefully", "pprof")
			}
		})

		// Serve with the pre-established listener
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			p2pm.Lm.Log("error", fmt.Sprintf("pprof server error: %v", err), "pprof")
		}
	}) {
		p2pm.Lm.Log("error", "CRITICAL: Failed to start pprof server due to goroutine limit - monitoring will be limited", "pprof")
	}

	// Set goroutine leak detection baseline after startup
	p2pm.leakDetector.SetBaseline()
	p2pm.Lm.Log("info", fmt.Sprintf("P2P manager started successfully - %s", p2pm.leakDetector.GetCurrentStats()), "p2p")

	return nil
}

func (p2pm *P2PManager) createPublicHost(
	priv crypto.PrivKey,
	port uint16,
	resourceManager network.ResourceManager,
	connMgr *connmgr.BasicConnMgr,
	blacklistManager *blacklist_node.BlacklistNodeManager,
	bootstrapAddrInfo []peer.AddrInfo,
	relayAddrsInfo []peer.AddrInfo,
) (host.Host, error) {
	message := "Creating public p2p host"
	p2pm.Lm.Log("info", message, "p2p")
	p2pm.UI.Print(message)

	hst, err := p2pm.createHost(
		priv,
		port,
		resourceManager,
		connMgr,
		blacklistManager,
		relayAddrsInfo,
	)
	if err != nil {
		return nil, err
	}

	// let this host use the DHT to find other hosts
	//p2pm.idht, err = p2pm.initDHT("server")
	p2pm.idht, err = p2pm.initDHT("", bootstrapAddrInfo)
	if err != nil {
		p2pm.h.Close()
		return nil, err
	}

	// Initialize relay components after DHT is ready
	p2pm.initRelayComponents()

	if p2pm.relay {
		message := "Starting relay server"
		p2pm.Lm.Log("info", message, "p2p")
		p2pm.UI.Print(message)

		// Initialize relay traffic monitor for billing with database storage
		p2pm.relayTrafficMonitor = NewRelayTrafficMonitor(hst.Network(), p2pm.DB)

		// Register relay connection notifee for billing
		relayNotifee := &RelayConnectionNotifee{
			monitor: p2pm.relayTrafficMonitor,
			p2pm:    p2pm,
		}
		hst.Network().Notify(relayNotifee)

		// Start relay service with resource limits (this is for high-bandwidth relay servers)
		// TODO, make this configurable
		_, err = relayv2.New(hst,
			relayv2.WithResources(relayv2.Resources{
				Limit: &relayv2.RelayLimit{
					Duration: 60 * time.Minute,       // 1 hour
					Data:     2 * 1024 * 1024 * 1024, // 2GB per circuit
				},
				ReservationTTL:  30 * time.Second,
				MaxReservations: 5000,
				MaxCircuits:     100,
				BufferSize:      256 * 1024, // 256KB buffer
			}),
		)
		if err != nil {
			hst.Close()
			return nil, err
		}

		// Auto-start relay service advertising for public relay nodes
		gt := p2pm.GetGoroutineTracker()
		gt.SafeStart("relay-advertising", func() {
			// Wait a bit for DHT to bootstrap before advertising
			time.Sleep(30 * time.Second)

			config := p2pm.createRelayConfigFromNodeConfig()
			err := p2pm.StartRelayAdvertising(p2pm.ctx, config)
			if err != nil {
				p2pm.Lm.Log("warning",
					fmt.Sprintf("Failed to start relay advertising: %v", err),
					"relay-advertiser")
			} else {
				p2pm.Lm.Log("info",
					fmt.Sprintf("🚀 Auto-started relay marketplace advertising for topics: %v", config.Topics),
					"relay-advertiser")
			}
		})

		// Relay diagnostics, DEBAG mode
		//		diagnostics := NewRelayDiagnostics(p2pm)
		//		diagnostics.RunFullDiagnostic()
	}

	return hst, nil
}

func (p2pm *P2PManager) createPrivateHost(
	priv crypto.PrivKey,
	port uint16,
	resourceManager network.ResourceManager,
	connMgr *connmgr.BasicConnMgr,
	blacklistManager *blacklist_node.BlacklistNodeManager,
	bootstrapAddrInfo []peer.AddrInfo,
	relayAddrsInfo []peer.AddrInfo,
) (host.Host, error) {
	message := "Creating private p2p host (behind NAT)"
	p2pm.Lm.Log("info", message, "p2p")
	p2pm.UI.Print(message)

	hst, err := p2pm.createHost(
		priv,
		port,
		resourceManager,
		connMgr,
		blacklistManager,
		relayAddrsInfo,
	)
	if err != nil {
		return nil, err
	}

	// let this host use the DHT to find other hosts
	//p2pm.idht, err = p2pm.initDHT("client")
	p2pm.idht, err = p2pm.initDHT("", bootstrapAddrInfo)
	if err != nil {
		p2pm.h.Close()
		return nil, err
	}

	// Initialize relay components after DHT is ready
	p2pm.initRelayComponents()

	// Auto-configure relay discovery for private nodes
	gt := p2pm.GetGoroutineTracker()
	gt.SafeStart("relay-discovery-config", func() {
		// Wait for DHT to bootstrap before configuring relay discovery
		time.Sleep(30 * time.Second)

		// Configure relay discovery parameters from config
		maxRelays := 5
		minReputation := 3.0
		maxPricePerGB := 0.50 // Max $0.50/GB
		currency := "USD"

		// Read configuration from node config if available
		if val, exists := p2pm.cm.GetConfig("relay_max_price_gb"); exists {
			if price, err := strconv.ParseFloat(val, 64); err == nil {
				maxPricePerGB = price
			}
		}
		if val, exists := p2pm.cm.GetConfig("relay_min_reputation"); exists {
			if rep, err := strconv.ParseFloat(val, 64); err == nil {
				minReputation = rep
			}
		}
		if val, exists := p2pm.cm.GetConfig("relay_currency"); exists {
			currency = val
		}

		p2pm.ConfigureRelayDiscovery(maxRelays, minReputation, maxPricePerGB, currency)

		p2pm.Lm.Log("info",
			fmt.Sprintf("🔍 Auto-configured relay discovery: max %d relays, min rating %.1f, max $%.2f/GB",
				maxRelays, minReputation, maxPricePerGB),
			"relay-discovery")
	})

	return hst, nil
}

func (p2pm *P2PManager) createHost(
	priv crypto.PrivKey,
	port uint16,
	resourceManager network.ResourceManager,
	connMgr *connmgr.BasicConnMgr,
	blacklistManager *blacklist_node.BlacklistNodeManager,
	relayAddrsInfo []peer.AddrInfo,
) (host.Host, error) {
	// Configure yamux for large transfers
	yamuxConfig := p2pm.Lm.CreateYamuxConfigWithLogger()

	hst, err := libp2p.New(
		// Use the keypair we generated
		libp2p.Identity(priv),
		// Listening addresses
		libp2p.ListenAddrStrings(
			fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", port),
			fmt.Sprintf("/ip6/::1/tcp/%d", port+1),
			fmt.Sprintf("/ip4/0.0.0.0/udp/%d/quic-v1", port+2),
			fmt.Sprintf("/ip6/::1/udp/%d/quic-v1", port+3),
			fmt.Sprintf("/ip4/0.0.0.0/tcp/%d/ws", port+4),
			fmt.Sprintf("/ip6/::1/udp/%d/ws", port+5),
			"/p2p-circuit",
		),
		// support TLS connections
		libp2p.Security(libp2ptls.ID, libp2ptls.New),
		// support noise connections
		libp2p.Security(noise.ID, noise.New),
		// support default transports
		libp2p.DefaultTransports,
		// support default muxers
		//		libp2p.DefaultMuxers,
		libp2p.Muxer(yamux.ID, yamuxConfig),
		// connection management
		libp2p.ConnectionManager(connMgr),
		libp2p.ResourceManager(resourceManager),
		// attempt to open ports using uPNP for NATed hosts.
		libp2p.EnableNATService(),
		libp2p.NATPortMap(),
		// control which nodes we allow to connect
		libp2p.ConnectionGater(blacklistManager.Gater),
		// Use dynamic relay discovery for topic-aware relay selection
		func() libp2p.Option {
			if p2pm.relayPeerSource != nil {
				return libp2p.EnableAutoRelayWithPeerSource(p2pm.relayPeerSource.PeerSourceFunc())
			}
			// Fallback to static relays if dynamic source not available
			return libp2p.EnableAutoRelayWithStaticRelays(relayAddrsInfo)
		}(),
		libp2p.EnableRelay(),
		// enable AutoNAT v2 for automatic reachability detection
		libp2p.EnableAutoNATv2(),
		// enable hole punching for direct connections when possible
		libp2p.EnableHolePunching(),
		// Add bandwidth reporting for relay traffic billing (only for relay nodes)
		func() libp2p.Option {
			if p2pm.relay {
				relayReporter := NewRelayBandwidthReporter(p2pm.DB, p2pm.Lm, p2pm)
				return libp2p.BandwidthReporter(relayReporter)
			}
			return func(*libp2p.Config) error { return nil } // No-op for non-relay nodes
		}(),
	)
	if err != nil {
		return nil, err
	}
	p2pm.h = hst

	return hst, nil
}

// createRelayConfigFromNodeConfig creates relay service configuration from node configuration
func (p2pm *P2PManager) createRelayConfigFromNodeConfig() RelayServiceConfig {
	config := CreateDefaultRelayConfig(p2pm.completeTopicNames)

	// Read pricing configuration
	if val, exists := p2pm.cm.GetConfig("relay_price_per_gb"); exists {
		if price, err := strconv.ParseFloat(val, 64); err == nil {
			config.PricePerGB = price
		}
	}

	// Read currency configuration
	if val, exists := p2pm.cm.GetConfig("relay_currency"); exists {
		config.Currency = val
	}

	// Read bandwidth limits
	if val, exists := p2pm.cm.GetConfig("relay_max_bandwidth_mbps"); exists {
		if mbps, err := strconv.ParseInt(val, 10, 64); err == nil {
			config.MaxBandwidth = mbps * 1024 * 1024 // Convert to bytes/second
		}
	}

	// Read contact information
	if val, exists := p2pm.cm.GetConfig("relay_contact_info"); exists {
		config.ContactInfo = val
	}

	// Read availability target
	if val, exists := p2pm.cm.GetConfig("relay_availability_percent"); exists {
		if availability, err := strconv.ParseFloat(val, 64); err == nil {
			config.Availability = availability
		}
	}

	return config
}

func (p2pm *P2PManager) Stop() error {
	p2pm.Lm.Log("info", "Stopping P2P Manager...", "p2p")

	// NOTE: Do NOT cancel context here - this is for stop/restart, not shutdown
	// Context should remain available for potential restart

	// Stop periodic tasks
	p2pm.StopPeriodicTasks()

	// Cancel subscriptions
	for _, subscription := range p2pm.subscriptions {
		subscription.Cancel()
	}

	// Close topics
	for key, topic := range p2pm.topicsSubscribed {
		if err := topic.Close(); err != nil {
			p2pm.UI.Print(fmt.Sprintf("Could not close topic %s: %s\n", key, err.Error()))
		}
		delete(p2pm.topicsSubscribed, key)
	}

	// Stop p2p node with timeout to prevent hanging
	p2pm.Lm.Log("info", "Closing libp2p host...", "p2p")

	hostClosed := make(chan error, 1)
	go func() {
		hostClosed <- p2pm.h.Close()
	}()

	select {
	case err := <-hostClosed:
		if err != nil {
			p2pm.Lm.Log("error", fmt.Sprintf("Host close error: %v", err), "p2p")
			return err
		}
		p2pm.Lm.Log("info", "libp2p host closed successfully", "p2p")
	case <-time.After(10 * time.Second):
		p2pm.Lm.Log("warn", "Host close timeout - force terminating", "p2p")
		// Don't return error, continue with cleanup
	}

	// Stop job periodic tasks when shutting down
	if p2pm.jobManager != nil {
		p2pm.jobManager.StopPeriodicTasks()
		// Also stop the worker manager
		if p2pm.jobManager.wm != nil {
			p2pm.jobManager.wm.Shutdown()
			p2pm.Lm.Log("info", "Stopped WorkerManager", "p2p")
		}
		p2pm.Lm.Log("info", "Stopped JobManager periodic tasks", "p2p")
	}

	// Wait for all periodic tasks and workers to stop with aggressive timeout
	p2pm.Lm.Log("info", "Waiting for P2P goroutines to complete...", "p2p")

	// Wait for both periodic tasks and stream workers
	allDone := make(chan struct{})
	gt := p2pm.GetGoroutineTracker()
	gt.SafeStart("all-waitgroups-completion", func() {
		// Wait for periodic tasks first
		p2pm.cronWg.Wait()
		p2pm.Lm.Log("debug", "All periodic tasks completed", "p2p")

		// Then wait for stream workers
		p2pm.streamWorkersWg.Wait()
		p2pm.Lm.Log("debug", "All stream workers completed", "p2p")

		close(allDone)
	})

	select {
	case <-allDone:
		p2pm.Lm.Log("info", "All P2P goroutines stopped gracefully", "p2p")
	case <-time.After(5 * time.Second): // Increased timeout since we wait for both
		p2pm.Lm.Log("warn", "Timeout waiting for remaining P2P goroutines, forcing shutdown", "p2p")

		// Capture final goroutine snapshot to identify stuck goroutines
		if p2pm.leakDetector != nil {
			finalSnapshot := p2pm.leakDetector.TakeSnapshot()
			p2pm.Lm.Log("warn", fmt.Sprintf("Final goroutine count at forced shutdown: %d", finalSnapshot.TotalCount), "leak-detector")

			// Log potential stuck goroutines
			leaks := p2pm.leakDetector.DetectLeaks()
			if len(leaks) > 0 {
				p2pm.Lm.Log("error", fmt.Sprintf("SHUTDOWN HANGING due to %d stuck goroutine types:", len(leaks)), "leak-detector")
				for i, leak := range leaks {
					if i >= 3 { // Show top 3 worst offenders
						break
					}
					p2pm.Lm.Log("error", fmt.Sprintf("STUCK: %s", leak.String()), "leak-detector")
				}
			}
		}
		// Force exit - don't wait for goroutines that won't exit
	}

	return nil
}

func (p2pm *P2PManager) joinSubscribeTopic(cntx context.Context, ps *pubsub.PubSub, completeTopicName string) (*pubsub.Subscription, *pubsub.Topic, error) {
	topic, err := ps.Join(completeTopicName)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return nil, nil, err
	}

	sub, err := topic.Subscribe()
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return nil, nil, err
	}

	p2pm.topicsSubscribed[completeTopicName] = topic

	// Start message receiving goroutine with tracking
	gt := p2pm.GetGoroutineTracker()
	if !gt.SafeStart(fmt.Sprintf("message-receiver-%s", completeTopicName), func() {
		p2pm.receivedMessage(cntx, sub)
	}) {
		p2pm.Lm.Log("error", fmt.Sprintf("Failed to start message receiver for topic %s", completeTopicName), "p2p")
	}

	return sub, topic, nil
}

func (p2pm *P2PManager) initDHT(mode string, bootstrapPeers []peer.AddrInfo) (*dht.IpfsDHT, error) {
	var dhtMode dht.Option

	switch mode {
	case "client":
		dhtMode = dht.Mode(dht.ModeClient)
	case "server":
		dhtMode = dht.Mode(dht.ModeServer)
	default:
		dhtMode = dht.Mode(dht.ModeAuto)
	}

	// Configure DHT with limits to prevent excessive goroutine creation
	dhtOpts := []dht.Option{
		dhtMode,
		dht.Concurrency(10),         // Limit concurrent DHT queries to prevent goroutine explosion
		dht.MaxRecordAge(time.Hour), // Expire DHT records faster
	}
	kademliaDHT, err := dht.New(p2pm.ctx, p2pm.h, dhtOpts...)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return nil, errors.New(err.Error())
	}

	if err = kademliaDHT.Bootstrap(p2pm.ctx); err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return nil, errors.New(err.Error())
	}

	// Connect to bootstrap peers with goroutine tracking
	gt := p2pm.GetGoroutineTracker()
	if !gt.SafeStart("bootstrap-connect", func() { p2pm.ConnectNodesAsync(bootstrapPeers, 3, 2*time.Second) }) {
		p2pm.Lm.Log("warn", "Failed to start bootstrap connection due to goroutine limit", "p2p")
	}

	return kademliaDHT, nil
}

func (p2pm *P2PManager) DiscoverPeers() {
	const maxDiscoveredPeers = 50 // Limit memory growth
	var discoveredPeers []peer.AddrInfo

	// Track discovery operation
	discoveryID := fmt.Sprintf("discovery-%d", time.Now().Unix())
	p2pm.resourceTracker.TrackResourceWithCleanup(discoveryID, utils.ResourceConnection, func() error {
		p2pm.Lm.Log("debug", "Peer discovery cleanup completed", "p2p")
		return nil
	})
	defer p2pm.resourceTracker.ReleaseResource(discoveryID)

	routingDiscovery := drouting.NewRoutingDiscovery(p2pm.idht)

	// Advertise all topics
	for _, completeTopicName := range p2pm.completeTopicNames {
		dutil.Advertise(p2pm.ctx, routingDiscovery, completeTopicName)
	}

	// Look for others who have announced and attempt to connect to them
	for _, completeTopicName := range p2pm.completeTopicNames {
		peerChan, err := routingDiscovery.FindPeers(p2pm.ctx, completeTopicName)
		if err != nil {
			p2pm.Lm.Log("warn", err.Error(), "p2p")
			continue
		}

		for peer := range peerChan {
			if peer.ID == "" || peer.ID == p2pm.h.ID() {
				continue
			}

			// Check if we already have this peer
			found := false
			for _, existing := range discoveredPeers {
				if existing.ID == peer.ID {
					found = true
					break
				}
			}

			if !found {
				discoveredPeers = append(discoveredPeers, peer)

				// Prevent unbounded memory growth
				if len(discoveredPeers) >= maxDiscoveredPeers {
					p2pm.Lm.Log("warn", fmt.Sprintf("Discovery limit reached (%d peers), stopping", maxDiscoveredPeers), "p2p")
					break
				}
			}
		}

		// Break outer loop if limit reached
		if len(discoveredPeers) >= maxDiscoveredPeers {
			break
		}
	}

	p2pm.Lm.Log("info", fmt.Sprintf("Discovered %d peers for connection attempts", len(discoveredPeers)), "p2p")

	// Log memory usage info
	memInfo := p2pm.GetMemoryUsageInfo()
	if len(memInfo) > 0 {
		p2pm.Lm.Log("info", fmt.Sprintf("Memory usage: %+v", memInfo), "p2p")
	}

	// Connect to peers synchronously (we're already in the ticker goroutine)
	if len(discoveredPeers) > 0 {
		p2pm.ConnectNodesAsync(discoveredPeers, 3, 2*time.Second)
	}
}

// Monitor connection health and reconnect (cron)
func (p2pm *P2PManager) MaintainConnections() {
	for _, peerID := range p2pm.h.Network().Peers() {
		if p2pm.h.Network().Connectedness(peerID) != network.Connected {
			// Attempt to reconnect
			if err := peerID.Validate(); err != nil {
				continue
			}
			p, err := p2pm.GeneratePeerAddrInfo(peerID.String())
			if err != nil {
				p2pm.Lm.Log("error", err.Error(), "p2p")
				continue
			}

			// Reconnect synchronously (we're already in the ticker goroutine)
			p2pm.ConnectNodeWithRetry(p2pm.ctx, p, 3, 2*time.Second)
		}
	}
}

func (p2pm *P2PManager) IsNodeConnected(peer peer.AddrInfo) (bool, error) {
	// Try every 500ms, up to 10 times (i.e. 5 seconds total)
	running := p2pm.WaitForHostReady(500*time.Millisecond, 10)
	if !running {
		err := fmt.Errorf("host is not running")
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return false, err
	}

	connected := false
	// Check for connected peers
	connectedPeers := p2pm.h.Network().Peers()
	if slices.Contains(connectedPeers, peer.ID) {
		connected = true
	}

	return connected, nil
}

func (p2pm *P2PManager) ConnectNode(p peer.AddrInfo) error {

	connected, err := p2pm.IsNodeConnected(p)
	if err != nil {
		msg := err.Error()
		p2pm.Lm.Log("error", msg, "p2p")
		return err
	}
	if connected {
		err := fmt.Errorf("node %s is already connected", p.ID.String())
		p2pm.Lm.Log("debug", err.Error(), "p2p")
		return nil // Consider this as success
	}

	if p.ID == p2pm.h.ID() {
		err := fmt.Errorf("can not connect to itself %s == %s", p.ID, p2pm.h.ID())
		p2pm.Lm.Log("debug", err.Error(), "p2p")
		return nil // Consider this as success
	}

	connCtx, cancel := utils.TrackContextWithTimeout(p2pm.ctx, 10*time.Second, "node_connection")
	err = p2pm.h.Connect(connCtx, p)
	cancel()

	if err != nil {
		// This might be usefull when node chabges its ID
		//		p2pm.PurgeNode(p)

		return err
	}

	p2pm.Lm.Log("debug", fmt.Sprintf("Connected to: %s", p.ID.String()), "p2p")

	for _, ma := range p.Addrs {
		p2pm.Lm.Log("debug", fmt.Sprintf("Connected peer's multiaddr is %s", ma.String()), "p2p")
	}

	return nil
}

func (p2pm *P2PManager) ConnectNodeWithRetry(ctx context.Context, peer peer.AddrInfo, maxRetries int, baseDelay time.Duration) error {
	for attempt := range maxRetries {
		select {
		case <-ctx.Done():
			return fmt.Errorf("missing context / context already done")
		default:
		}

		// Wait for host to be ready
		if !p2pm.WaitForHostReady(500*time.Millisecond, 10) {
			err := fmt.Errorf("host not ready, abandoning connection attempt")
			p2pm.Lm.Log("warn", err.Error(), "p2p")
			return err
		}

		// Attempt connection
		err := p2pm.ConnectNode(peer)
		if err != nil {
			// Exponential backoff
			if attempt < maxRetries-1 {
				delay := baseDelay * time.Duration(1<<attempt)
				select {
				case <-time.After(delay):
					continue
				case <-ctx.Done():
					return fmt.Errorf("missing context / context already done")
				}
			}
			continue
		}

		// Successfully connected
		return nil
	}

	err := fmt.Errorf("failed to connect to peer %s after %d attempts", peer.ID, maxRetries)
	p2pm.Lm.Log("warn", err.Error(), "p2p")
	return err
}

func (p2pm *P2PManager) ConnectNodesAsync(peers []peer.AddrInfo, maxRetries int, baseDelay time.Duration) []peer.AddrInfo {
	var mu sync.Mutex
	var connectedPeers []peer.AddrInfo

	// Use proper worker pool with job queue
	maxWorkers := 20
	peerChan := make(chan peer.AddrInfo, len(peers))

	// Send peers to job queue
	for _, peer := range peers {
		peerChan <- peer
	}
	close(peerChan)

	var wg sync.WaitGroup

	// Start workers that consume from job queue
	gt := p2pm.GetGoroutineTracker()
	for i := range maxWorkers {
		wg.Add(1)
		if !gt.SafeStart(fmt.Sprintf("peer-connect-worker-%d", i), func() {
			defer wg.Done()

			for peer := range peerChan {
				// Track connection attempt
				connID := fmt.Sprintf("connect-%s", peer.ID.String())
				p2pm.resourceTracker.TrackResourceWithCleanup(connID, utils.ResourceConnection, func() error {
					p2pm.Lm.Log("debug", fmt.Sprintf("Connection attempt cleanup: %s", peer.ID.String()), "p2p")
					return nil
				})

				if err := p2pm.ConnectNodeWithRetry(p2pm.ctx, peer, maxRetries, baseDelay); err == nil {
					mu.Lock()
					connectedPeers = append(connectedPeers, peer)
					mu.Unlock()
				}

				// Release resource tracking
				p2pm.resourceTracker.ReleaseResource(connID)
			}
		}) {
			wg.Done() // If goroutine couldn't start, reduce wait group count
			p2pm.Lm.Log("warn", fmt.Sprintf("Failed to start peer connection worker %d due to goroutine limit", i), "p2p")
		}
	}

	wg.Wait()
	return connectedPeers
}

func (p2pm *P2PManager) PurgeNode(p peer.AddrInfo) {
	p2pm.h.Network().ClosePeer(p.ID)
	p2pm.h.Network().Peerstore().RemovePeer(p.ID)
	p2pm.h.Peerstore().ClearAddrs(p.ID)
	p2pm.h.Peerstore().RemovePeer(p.ID)
	p2pm.Lm.Log("debug", fmt.Sprintf("Removed peer %s from a peer store", p.ID), "p2p")
}

func StreamData[T any](
	p2pm *P2PManager,
	receivingPeer peer.AddrInfo,
	data T,
	job *node_types.Job,
	existingStream network.Stream,
) error {
	var workflowId int64 = int64(0)
	var jobId int64 = int64(0)
	var orderingPeer255 [255]byte
	var interfaceId int64 = int64(0)
	var s network.Stream
	var ctx context.Context
	var cancel context.CancelFunc
	var deadline time.Time

	var t uint16 = uint16(0)
	switch v := any(data).(type) {
	case *node_types.ServiceLookup:
		t = 13
	case *[]node_types.ServiceOffer:
		t = 0
	case *node_types.JobRunRequest:
		t = 1
	case *[]byte:
		t = 2
	case *os.File:
		t = 3
	case *node_types.ServiceRequest:
		t = 4
	case *node_types.ServiceResponse:
		t = 5
	case *node_types.JobRunResponse:
		t = 6
	case *node_types.JobRunStatus:
		t = 7
	case *node_types.JobRunStatusRequest:
		t = 8
	case *node_types.JobDataReceiptAcknowledgement:
		t = 9
	case *node_types.JobDataRequest:
		t = 10
	case *node_types.ServiceRequestCancellation:
		t = 11
	case *node_types.ServiceResponseCancellation:
		t = 12
	default:
		err := fmt.Errorf("data type %v is not allowed in this context (streaming data)", v)
		p2pm.Lm.Log("error", err.Error(), "p2p")
		if existingStream != nil {
			existingStream.Reset()
		}
		return err
	}

	// Timeout context with tracking
	if t == 3 {
		// File transfers - use tracked context for 5-hour timeout
		ctx, cancel = utils.TrackContextWithTimeout(p2pm.ctx, 5*time.Hour, "file_transfer")
	} else {
		// Other operations - 1 minute timeout
		ctx, cancel = utils.TrackContextWithTimeout(p2pm.ctx, 1*time.Minute, "stream_operation")
	}
	defer cancel()

	err := p2pm.ConnectNodeWithRetry(ctx, receivingPeer, 3, 2*time.Second)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	if existingStream != nil {
		existingStream.SetDeadline(time.Now().Add(100 * time.Millisecond))
		_, err := existingStream.Write([]byte{})
		existingStream.SetDeadline(time.Time{}) // reset deadline

		if err == nil {
			// Stream is alive, reuse it
			s = existingStream
			p2pm.Lm.Log("debug", fmt.Sprintf("Reusing existing stream with %s", receivingPeer.ID), "p2p")
		} else {
			// Stream is broken, discard and create a new one
			existingStream.Reset()
			p2pm.Lm.Log("debug", fmt.Sprintf("Existing stream with %s is not usable: %v", receivingPeer.ID, err), "p2p")
		}
	}

	// Create new stream
	if s == nil {
		s, err = p2pm.h.NewStream(ctx, receivingPeer.ID, p2pm.protocolID)
		if err != nil {
			p2pm.Lm.Log("error", err.Error(), "p2p")
			return err
		}
		
		// Track this outbound stream with its data type
		streamID := s.ID()
		p2pm.streamsMutex.Lock()
		p2pm.activeStreams[streamID] = &streamInfo{
			startTime: time.Now(),
			dataType:  t, // We know the data type from the switch statement above
			stream:    s,
		}
		p2pm.streamsMutex.Unlock()
	}
	// Ensure stream is always cleaned up
	defer func() {
		if s != nil {
			// Remove from tracking before closing
			streamID := s.ID()
			p2pm.streamsMutex.Lock()
			delete(p2pm.activeStreams, streamID)
			p2pm.streamsMutex.Unlock()
			s.Close()
		}
	}()

	// Set timeouts for all stream operations
	//	if t == 2 || t == 3 {
	if t == 3 {
		deadline = time.Now().Add(5 * time.Hour) // TODO, put in configs
		s.SetDeadline(deadline)
	} else {
		deadline = time.Now().Add(1 * time.Minute)
		s.SetDeadline(deadline)
	}
	defer s.SetDeadline(time.Time{}) // Clear deadline

	if job != nil {
		var orderingPeer []byte = []byte(job.OrderingNodeId)
		copy(orderingPeer255[:], orderingPeer)
		workflowId = job.WorkflowId
		jobId = job.Id
		if len(job.JobInterfaces) > 0 {
			interfaceId = job.JobInterfaces[0].InterfaceId
		}
	}

	var sendingPeer []byte = []byte(p2pm.h.ID().String())
	var sendingPeer255 [255]byte
	copy(sendingPeer255[:], sendingPeer)

	// Use channels for proper goroutine coordination
	proposalDone := make(chan error, 1)
	streamDone := make(chan error, 1)

	gt := p2pm.GetGoroutineTracker()

	if !gt.SafeStart("stream-proposal", func() {
		proposalDone <- p2pm.streamProposal(s, sendingPeer255, t, orderingPeer255, workflowId, jobId, interfaceId)
	}) {
		return fmt.Errorf("failed to start stream proposal goroutine due to limit")
	}

	if !gt.SafeStart("stream-send", func() {
		streamDone <- sendStream(p2pm, s, data)
	}) {
		return fmt.Errorf("failed to start stream send goroutine due to limit")
	}

	// Wait for both operations with timeout
	select {
	case err := <-proposalDone:
		if err != nil {
			return err
		}
	case <-ctx.Done():
		return ctx.Err()
	}

	select {
	case err := <-streamDone:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p2pm *P2PManager) streamProposal(
	s network.Stream,
	p [255]byte,
	t uint16,
	onode [255]byte,
	wid int64,
	jid int64,
	iid int64,
) error {
	// Create an instance of StreamData to write
	streamData := node_types.StreamData{
		Type:           t,
		PeerId:         p,
		OrderingPeerId: onode,
		WorkflowId:     wid,
		JobId:          jid,
		InterfaceId:    iid,
	}

	// Send stream data
	if err := binary.Write(s, binary.BigEndian, streamData); err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		if closeErr := s.Close(); closeErr != nil {
			s.Reset() // Fallback to reset if close fails
		}
		return err
	}
	message := fmt.Sprintf("Stream proposal type %d has been sent in stream %s", streamData.Type, s.ID())
	p2pm.Lm.Log("debug", message, "p2p")

	return nil
}

func (p2pm *P2PManager) streamProposalResponse(s network.Stream) {
	// Create timeout context for the entire stream processing
	ctx, cancel := utils.TrackContextWithTimeout(p2pm.ctx, 15*time.Minute, "proposal_response")
	defer cancel()

	// Set reasonable timeouts to prevent indefinite blocking without closing the stream prematurely
	s.SetReadDeadline(time.Now().Add(10 * time.Minute))  // TODO, put in configs
	s.SetWriteDeadline(time.Now().Add(10 * time.Minute)) // TODO, put in configs

	// Recovery function to handle panics and reset stream on errors
	defer func() {
		if r := recover(); r != nil {
			p2pm.Lm.Log("error", fmt.Sprintf("Stream handler panic: %v", r), "p2p")
			s.Reset() // Reset only on panic to free resources
		}
	}()

	// Track this stream for cleanup
	streamID := s.ID()
	p2pm.streamsMutex.Lock()
	p2pm.activeStreams[streamID] = &streamInfo{
		startTime: time.Now(),
		dataType:  0, // Default to 0, will be updated when StreamData is received
		stream:    s,
	}
	p2pm.streamsMutex.Unlock()

	// Enhanced resource tracking with P2PResourceManager (non-intrusive)
	if p2pm.p2pResourceManager != nil {
		// Just track the stream resource without interfering with processing
		p2pm.resourceTracker.TrackResourceWithCleanup(streamID, utils.ResourceStream, func() error {
			p2pm.Lm.Log("debug", fmt.Sprintf("Stream %s tracked by resource manager", streamID), "p2p")
			return nil
		})
	}

	// Ensure stream is removed from tracking when function exits
	defer func() {
		p2pm.streamsMutex.Lock()
		delete(p2pm.activeStreams, streamID)
		p2pm.streamsMutex.Unlock()

		// Release from resource tracker as well
		if p2pm.resourceTracker != nil {
			p2pm.resourceTracker.ReleaseResource(streamID)
		}
	}()

	// Check if context is cancelled before processing
	select {
	case <-ctx.Done():
		p2pm.Lm.Log("warn", "Stream processing cancelled due to timeout", "p2p")
		s.Reset()
		return
	default:
		// Continue processing
	}
	// Prepare to read the stream data
	var streamData node_types.StreamData
	err := binary.Read(s, binary.BigEndian, &streamData)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		if closeErr := s.Close(); closeErr != nil {
			s.Reset() // Fallback to reset if close fails
		}
		return
	}

	// Update stream data type in tracking info now that we know the actual type
	p2pm.streamsMutex.Lock()
	if streamInfo, exists := p2pm.activeStreams[streamID]; exists {
		streamInfo.dataType = streamData.Type
	}
	p2pm.streamsMutex.Unlock()

	message := fmt.Sprintf("Received stream data type %d from %s in stream %s",
		streamData.Type, string(bytes.Trim(streamData.PeerId[:], "\x00")), s.ID())
	p2pm.Lm.Log("debug", message, "p2p")

	// Check settings, do we want to accept receiving service catalogues and updates
	accepted := p2pm.streamProposalAssessment(streamData.Type)
	if accepted {
		p2pm.streamAccepted(s)

		// Submit stream job to worker pool instead of creating new goroutine
		job := streamJob{
			stream:     s,
			jobType:    "message",
			streamData: streamData,
		}

		if !p2pm.submitStreamJob(job) {
			// Worker pool full, handle gracefully to prevent "canceled stream" errors
			p2pm.Lm.Log("warn", "Stream worker pool at capacity, gracefully closing stream "+s.ID(), "p2p")
			p2pm.safeCloseStream(s, "worker pool full")
		}
	} else {
		// Stream rejected by local policy - close gracefully instead of reset
		p2pm.safeCloseStream(s, "rejected by local policy")
	}
}

func (p2pm *P2PManager) streamProposalAssessment(streamDataType uint16) bool {
	var accepted bool
	settingsManager := settings.NewSettingsManager(p2pm.DB, p2pm.Lm)
	// Check what the stream proposal is about
	switch streamDataType {
	case 13:
		// Allow Service Catalogue request
		accepted = settingsManager.ReadBoolSetting("accept_service_catalogue_request")
	case 0:
		// Allow receiving Service Catalogue
		accepted = settingsManager.ReadBoolSetting("accept_service_catalogue")
	case 1:
		// Allow job run request
		accepted = settingsManager.ReadBoolSetting("accept_job_run_request")
	case 2:
		// Allow receiving binary stream
		accepted = settingsManager.ReadBoolSetting("accept_binary_stream")
	case 3:
		// Allow receiving file
		accepted = settingsManager.ReadBoolSetting("accept_file")
	case 4:
		// Allow Service Request
		accepted = settingsManager.ReadBoolSetting("accept_service_request")
	case 5:
		// Allow Service Response
		accepted = settingsManager.ReadBoolSetting("accept_service_response")
	case 6:
		// Allow Job Run Response
		accepted = settingsManager.ReadBoolSetting("accept_job_run_response")
	case 7:
		// Allow Job Run Status
		accepted = settingsManager.ReadBoolSetting("accept_job_run_status")
	case 8:
		// Allow Job Run Status update
		accepted = settingsManager.ReadBoolSetting("accept_job_run_status_request")
	case 9:
		// Allow Job Data Receipt Acknowledgment
		accepted = settingsManager.ReadBoolSetting("accept_job_data_receipt_acknowledgement")
	case 10:
		// Allow Job Data Request
		accepted = settingsManager.ReadBoolSetting("accept_job_data_request")
	case 11:
		// Allow Service Request Cancllation
		accepted = settingsManager.ReadBoolSetting("accept_service_request_cancellation")
	case 12:
		// Allow Service Response Cancellation
		accepted = settingsManager.ReadBoolSetting("accept_service_response_cancellation")
	default:
		message := fmt.Sprintf("Unknown stream type %d is proposed", streamDataType)
		p2pm.Lm.Log("debug", message, "p2p")
		return false
	}
	if accepted {
		message := fmt.Sprintf("As per local settings stream data type %d is accepted.", streamDataType)
		p2pm.Lm.Log("debug", message, "p2p")
	} else {
		message := fmt.Sprintf("As per local settings stream data type %d is not accepted", streamDataType)
		p2pm.Lm.Log("debug", message, "p2p")
	}

	return accepted
}

func (p2pm *P2PManager) streamAccepted(s network.Stream) {
	data := [7]byte{'T', 'F', 'R', 'E', 'A', 'D', 'Y'}
	err := binary.Write(s, binary.BigEndian, data)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		if closeErr := s.Close(); closeErr != nil {
			s.Reset() // Fallback to reset if close fails
		}
	}

	message := fmt.Sprintf("Sent TFREADY successfully in stream %s", s.ID())
	p2pm.Lm.Log("debug", message, "p2p")
}

func sendStream[T any](p2pm *P2PManager, s network.Stream, data T) error {
	// Ensure stream is properly closed on all exit paths
	defer func() {
		if r := recover(); r != nil {
			p2pm.Lm.Log("error", fmt.Sprintf("Stream panic recovered: %v", r), "p2p")
			s.Reset()
		}
	}()

	var ready [7]byte
	var expected [7]byte = [7]byte{'T', 'F', 'R', 'E', 'A', 'D', 'Y'}

	// Timeout for reading ready signal
	s.SetReadDeadline(time.Now().Add(30 * time.Second))
	err := binary.Read(s, binary.BigEndian, &ready)
	s.SetReadDeadline(time.Time{}) // Clear deadline
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		if closeErr := s.Close(); closeErr != nil {
			s.Reset() // Fallback to reset if close fails
		}
		return err
	}

	// Check if received data matches TFREADY signal
	if !bytes.Equal(expected[:], ready[:]) {
		err := errors.New("did not get expected TFREADY signal")
		p2pm.Lm.Log("error", err.Error(), "p2p")
		if closeErr := s.Close(); closeErr != nil {
			s.Reset() // Fallback to reset if close fails
		}
		return err
	}

	message := fmt.Sprintf("Received %s from %s", string(ready[:]), s.ID())
	p2pm.Lm.Log("debug", message, "p2p")

	var chunkSize uint64 = 4096
	var pointer uint64 = 0

	// Read chunk size form configs
	cs := p2pm.cm.GetConfigWithDefault("chunk_size", "81920")
	chunkSize, err = strconv.ParseUint(cs, 10, 64)
	if err != nil {
		message := fmt.Sprintf("Invalid chunk size in configs file. Will set to the default chunk size (%s)", err.Error())
		p2pm.Lm.Log("warn", message, "p2p")
	}

	switch v := any(data).(type) {
	case *node_types.ServiceLookup, *[]node_types.ServiceOffer, *node_types.ServiceRequest,
		*node_types.ServiceResponse, *node_types.JobRunRequest, *node_types.JobRunResponse,
		*node_types.JobRunStatus, *node_types.JobRunStatusRequest,
		*node_types.JobDataReceiptAcknowledgement, *node_types.JobDataRequest,
		*node_types.ServiceRequestCancellation, *node_types.ServiceResponseCancellation:
		b, err := json.Marshal(data)
		if err != nil {
			p2pm.Lm.Log("error", err.Error(), "p2p")
			if closeErr := s.Close(); closeErr != nil {
				s.Reset() // Fallback to reset if close fails
			}
			return err
		}

		err = p2pm.sendStreamChunks(b, pointer, chunkSize, s)
		if err != nil {
			p2pm.Lm.Log("error", err.Error(), "p2p")
			if closeErr := s.Close(); closeErr != nil {
				s.Reset() // Fallback to reset if close fails
			}
			return err
		}
	case *[]byte:
		err = p2pm.sendStreamChunks(*v, pointer, chunkSize, s)
		if err != nil {
			p2pm.Lm.Log("error", err.Error(), "p2p")
			if closeErr := s.Close(); closeErr != nil {
				s.Reset() // Fallback to reset if close fails
			}
			return err
		}
	case *os.File:
		// Ensure file is always closed
		defer func() {
			if closeErr := v.Close(); closeErr != nil {
				p2pm.Lm.Log("error", fmt.Sprintf("Failed to close file: %v", closeErr), "p2p")
			}
		}()

		buffer := utils.P2PBufferPool.Get()
		defer utils.P2PBufferPool.Put(buffer)

		writer := utils.GlobalWriterPool.Get(s)
		defer utils.GlobalWriterPool.Put(writer)

		for {
			n, err := v.Read(buffer)
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				p2pm.Lm.Log("error", err.Error(), "p2p")
				if closeErr := s.Close(); closeErr != nil {
					s.Reset() // Fallback to reset if close fails
				}
				return err
			}

			// Write timeout
			s.SetWriteDeadline(time.Now().Add(1 * time.Minute)) // TODO, put in configs
			_, err = writer.Write(buffer[:n])
			s.SetWriteDeadline(time.Time{})
			if err != nil {
				p2pm.Lm.Log("error", err.Error(), "p2p")
				if closeErr := s.Close(); closeErr != nil {
					s.Reset() // Fallback to reset if close fails
				}
				return err
			}
		}

		if err = writer.Flush(); err != nil {
			p2pm.Lm.Log("error", err.Error(), "p2p")
			if closeErr := s.Close(); closeErr != nil {
				s.Reset() // Fallback to reset if close fails
			}
			return err
		}

		// Stream closure
		return s.CloseWrite()

	default:
		msg := fmt.Sprintf("Data type %v is not allowed in this context (streaming data)", v)
		p2pm.Lm.Log("error", msg, "p2p")
		if closeErr := s.Close(); closeErr != nil {
			s.Reset() // Fallback to reset if close fails
		}
		return err
	}

	message = fmt.Sprintf("Sending ended %s", s.ID())
	p2pm.Lm.Log("debug", message, "p2p")

	return nil
}

func (p2pm *P2PManager) sendStreamChunks(b []byte, pointer uint64, chunkSize uint64, s network.Stream) error {
	// Use buffered writer for better performance and memory management
	return utils.SafeWriterOperation(s, func(writer *bufio.Writer) error {
		for {
			if len(b)-int(pointer) < int(chunkSize) {
				chunkSize = uint64(len(b) - int(pointer))
			}

			err := binary.Write(writer, binary.BigEndian, chunkSize)
			if err != nil {
				p2pm.Lm.Log("error", err.Error(), "p2p")
				return err
			}
			message := fmt.Sprintf("Chunk [%d bytes] successfully sent in stream %s", chunkSize, s.ID())
			p2pm.Lm.Log("debug", message, "p2p")

			if chunkSize == 0 {
				break
			}

			err = binary.Write(writer, binary.BigEndian, (b)[pointer:pointer+chunkSize])
			if err != nil {
				p2pm.Lm.Log("error", err.Error(), "p2p")
				return err
			}
			message = fmt.Sprintf("Chunk [%d bytes] successfully sent in stream %s", len((b)[pointer:pointer+chunkSize]), s.ID())
			p2pm.Lm.Log("debug", message, "p2p")

			pointer += chunkSize
		}

		// Ensure data is flushed
		return writer.Flush()
	})
}

func (p2pm *P2PManager) receiveStreamChunks(s network.Stream, streamData node_types.StreamData) ([]byte, error) {
	var data []byte

	// Safety limits to prevent memory exhaustion attacks
	maxChunkSize := uint64(10 * 1024 * 1024)  // 10MB max per chunk
	maxTotalSize := uint64(100 * 1024 * 1024) // 100MB max total
	if val, exists := p2pm.cm.GetConfig("max_chunk_size_mb"); exists {
		if size, err := strconv.ParseUint(val, 10, 64); err == nil {
			maxChunkSize = size * 1024 * 1024
		}
	}
	if val, exists := p2pm.cm.GetConfig("max_stream_size_mb"); exists {
		if size, err := strconv.ParseUint(val, 10, 64); err == nil {
			maxTotalSize = size * 1024 * 1024
		}
	}

	for {
		// Receive chunk size
		var chunkSize uint64
		err := binary.Read(s, binary.BigEndian, &chunkSize)
		if err != nil {
			p2pm.Lm.Log("error", err.Error(), "p2p")
			return nil, err
		}

		message := fmt.Sprintf("Data from %s will be received in chunks of %d bytes", s.ID(), chunkSize)
		p2pm.Lm.Log("debug", message, "p2p")

		if chunkSize == 0 {
			break
		}

		// Validate chunk size
		if chunkSize > maxChunkSize {
			err := fmt.Errorf("chunk size %d exceeds maximum allowed %d bytes", chunkSize, maxChunkSize)
			p2pm.Lm.Log("error", err.Error(), "p2p")
			return nil, err
		}

		// Check total size limit
		if uint64(len(data))+chunkSize > maxTotalSize {
			err := fmt.Errorf("total stream size would exceed maximum allowed %d bytes", maxTotalSize)
			p2pm.Lm.Log("error", err.Error(), "p2p")
			return nil, err
		}

		chunk := make([]byte, chunkSize)

		// Receive a data chunk
		err = binary.Read(s, binary.BigEndian, &chunk)
		if err != nil {
			p2pm.Lm.Log("error", err.Error(), "p2p")
			return nil, err
		}

		// Update resource tracker that stream is still active
		if len(data)%1048576 == 0 { // Update every 1MB received
			p2pm.resourceTracker.UpdateLastUsed(s.ID())
		}

		message = fmt.Sprintf("Received chunk [%d bytes] of type %d from %s", len(chunk), streamData.Type, s.ID())
		p2pm.Lm.Log("debug", message, "p2p")

		// Concatenate received chunk
		data = append(data, chunk...)
	}

	return data, nil
}

func (p2pm *P2PManager) receivedStream(s network.Stream, streamData node_types.StreamData) {
	streamID := s.ID()

	// Check if context is canceled before processing stream
	select {
	case <-p2pm.ctx.Done():
		p2pm.Lm.Log("debug", fmt.Sprintf("Skipping stream processing %s - context canceled", streamID), "p2p")
		return
	default:
	}

	// Update resource tracker that stream is being actively used
	p2pm.resourceTracker.UpdateLastUsed(streamID)

	// Add small delay to prevent race conditions with resource tracker
	time.Sleep(10 * time.Millisecond)
	// Determine local and remote peer Ids
	localPeerId := s.Conn().LocalPeer()
	remotePeerId := s.Conn().RemotePeer()

	// Determine data type
	switch streamData.Type {
	case 13, 0, 1, 4, 5, 6, 7, 8, 9, 10, 11, 12:
		// Prepare to read back the data
		data, err := p2pm.receiveStreamChunks(s, streamData)
		if err != nil {
			p2pm.Lm.Log("error", fmt.Sprintf("Failed to receive stream chunks: %v", err), "p2p")
			p2pm.safeCloseStream(s, "receive chunks error")
			return
		}

		message := fmt.Sprintf("Receiving completed. Data [%d bytes] of type %d received in stream %s from %s ",
			len(data), streamData.Type, s.ID(), string(bytes.Trim(streamData.PeerId[:], "\x00")))
		p2pm.Lm.Log("debug", message, "p2p")

		switch streamData.Type {
		case 13:
			// Received a Service Catalogue Request from the remote peer
			if err := p2pm.serviceCatalogueRequestReceived(data, remotePeerId); err != nil {
				p2pm.Lm.Log("error", fmt.Sprintf("Service catalogue request processing failed: %v", err), "p2p")
				if closeErr := s.Close(); closeErr != nil {
					s.Reset() // Fallback to reset if close fails
				}
				return
			}
		case 0:
			// Received a Service Offer from the remote peer
			if err := p2pm.serviceOfferReceived(data, localPeerId); err != nil {
				p2pm.handleStreamError(s, "Service offer processing failed", err)
				return
			}
		case 1:
			// Received Job Run Request from remote peer
			if err := p2pm.jobRunRequestReceived(data, remotePeerId); err != nil {
				p2pm.handleStreamError(s, "Stream processing failed", err)
				return
			}
		case 4:
			// Received Service Request from remote peer
			if err := p2pm.serviceRequestReceived(data, remotePeerId); err != nil {
				p2pm.handleStreamError(s, "Stream processing failed", err)
				return
			}
		case 5:
			// Received Service Response from remote peer
			if err := p2pm.serviceResponseReceived(data); err != nil {
				p2pm.handleStreamError(s, "Stream processing failed", err)
				return
			}
		case 6:
			// Received Job Run Response from remote peer
			if err := p2pm.jobRunResponseReceived(data); err != nil {
				p2pm.handleStreamError(s, "Stream processing failed", err)
				return
			}
		case 7:
			// Received Job Run Status Update from remote peer
			if err := p2pm.jobRunStatusUpdateReceived(data); err != nil {
				p2pm.handleStreamError(s, "Stream processing failed", err)
				return
			}
		case 8:
			// Received Job Run Status Request from remote peer
			if err := p2pm.jobRunStatusRequestReceived(data, remotePeerId); err != nil {
				p2pm.handleStreamError(s, "Stream processing failed", err)
				return
			}
		case 9:
			// Received Job Data Acknowledgement Receipt from remote peer
			if err := p2pm.jobDataAcknowledgementReceiptReceived(data, remotePeerId); err != nil {
				p2pm.handleStreamError(s, "Stream processing failed", err)
				return
			}
		case 10:
			// Received Job Data Request from remote peer
			if err := p2pm.jobDataRequestReceived(data, remotePeerId); err != nil {
				p2pm.handleStreamError(s, "Stream processing failed", err)
				return
			}
		case 11:
			// Received Service Request Cancelation from remote peer
			if err := p2pm.serviceRequestCancellationReceived(data, remotePeerId); err != nil {
				p2pm.handleStreamError(s, "Stream processing failed", err)
				return
			}
		case 12:
			// Received Service Request Cancellation Response from remote peer
			if err := p2pm.serviceRequestCancellationResponseReceived(data); err != nil {
				p2pm.handleStreamError(s, "Stream processing failed", err)
				return
			}
		}
	case 2:
		// Received binary stream from the remote peer	// OBSOLETE, to remove
	case 3:
		// Received file from remote peer
		if err := p2pm.fileReceived(s, remotePeerId, streamData); err != nil {
			p2pm.handleStreamError(s, "File reception failed", err)
			return
		}
	default:
		message := fmt.Sprintf("Unknown stream type %d is received", streamData.Type)
		p2pm.Lm.Log("warn", message, "p2p")
	}

	message := fmt.Sprintf("Receiving ended %s", s.ID())
	p2pm.Lm.Log("debug", message, "p2p")

	// Proper stream cleanup using safe close
	p2pm.safeCloseStream(s, "normal completion")
}

// Received file from remote peer
func (p2pm *P2PManager) fileReceived(
	s network.Stream,
	remotePeerId peer.ID,
	streamData node_types.StreamData,
) error {
	var file *os.File
	var err error
	var fdir string
	var fpath string

	remotePeer := remotePeerId.String()

	// Allow node to receive remote files
	// Data should be accepted in following cases:
	// a) this is ordering node (OrderingPeerId == h.ID())
	// b) the data is sent to our job (IDLE WorkflowId/JobId exists in jobs table)
	jobManager := p2pm.jobManager
	orderingNodeId := string(bytes.Trim(streamData.OrderingPeerId[:], "\x00"))
	receivingNodeId := p2pm.h.ID().String()

	// Is this ordering node?
	allowed := receivingNodeId == string(orderingNodeId)
	if !allowed {
		allowed, err = jobManager.JobExpectingInputsFrom(streamData.JobId, remotePeer)
		if err != nil {
			p2pm.Lm.Log("error", err.Error(), "p2p")
			return err
		}
	}

	// Allow it if there is a job expecting this input (not at ordering node)
	if !allowed {
		err := fmt.Errorf("we haven't ordered this data from `%s`. We are also not expecting it as an input for a job",
			orderingNodeId)
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	sWorkflowId := strconv.FormatInt(streamData.WorkflowId, 10)
	sJobId := strconv.FormatInt(streamData.JobId, 10)
	fdir = filepath.Join(p2pm.cm.GetConfigWithDefault("local_storage", "./local_storage/"), "workflows",
		orderingNodeId, sWorkflowId, "job", sJobId, "input", remotePeer)
	fpath = fdir + utils.RandomString(32)

	// Note: chunkSize is now handled by buffer pool, no need to parse it here
	if err = os.MkdirAll(fdir, 0755); err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}
	file, err = os.Create(fpath)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}
	defer file.Close()

	reader := utils.GlobalReaderPool.Get(s)
	defer utils.GlobalReaderPool.Put(reader)

	buf := utils.P2PBufferPool.Get()
	defer utils.P2PBufferPool.Put(buf)

	for {
		n, err := reader.Read(buf)
		if err != nil {
			if err == io.EOF {
				break
			}
			p2pm.Lm.Log("error", err.Error(), "p2p")
			return err
		}
		file.Write(buf[:n])
	}

	// Uncompress received file
	err = utils.Uncompress(fpath, fdir)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	// Send acknowledgement receipt with goroutine tracking
	gt := p2pm.GetGoroutineTracker()
	workflowId := streamData.WorkflowId
	jobId := streamData.JobId
	interfaceId := streamData.InterfaceId

	if !gt.SafeStart(fmt.Sprintf("receipt-ack-%d-%d", workflowId, jobId), func() {
		p2pm.sendReceiptAcknowledgement(
			workflowId,
			jobId,
			interfaceId,
			orderingNodeId,
			receivingNodeId,
			remotePeer,
		)
	}) {
		p2pm.Lm.Log("warn", fmt.Sprintf("Failed to start receipt acknowledgement goroutine for workflow %d job %d", workflowId, jobId), "p2p")
	}

	err = os.RemoveAll(fpath)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	return nil
}

// Received Serviced Catalogue Request from remote peer
func (p2pm *P2PManager) serviceCatalogueRequestReceived(data []byte, remotePeerId peer.ID) error {
	err := p2pm.sendServiceOffer(data, remotePeerId)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	return nil
}

// Received Service Offer from remote peer
func (p2pm *P2PManager) serviceOfferReceived(data []byte, localPeerId peer.ID) error {
	var serviceOffer []node_types.ServiceOffer
	err := json.Unmarshal(data, &serviceOffer)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	for _, service := range serviceOffer {
		// Remote node ID
		nodeId := service.NodeId
		localNodeId := localPeerId.String()
		// Skip if this is our service advertized on remote node
		if localNodeId == nodeId {
			continue
		}

		// Add/Update Service Offers Cache
		p2pm.sc.PruneExpired(time.Hour)
		service = p2pm.sc.AddOrUpdate(service)

		// If we are in interactive mode print Service Offer to CLI
		uiType, err := ui.DetectUIType(p2pm.UI)
		if err != nil {
			p2pm.Lm.Log("error", err.Error(), "p2p")
			return err
		}

		switch uiType {
		case "CLI":
			menuManager := NewMenuManager(p2pm)
			menuManager.printOfferedService(service)
		case "GUI":
			// Push message
			p2pm.UI.ServiceOffer(service)
		default:
			err := fmt.Errorf("unknown UI type %s", uiType)
			p2pm.Lm.Log("error", err.Error(), "p2p")
			return err
		}
	}

	return nil
}

// Received Service Request from remote peer
func (p2pm *P2PManager) serviceRequestReceived(data []byte, remotePeerId peer.ID) error {
	var serviceRequest node_types.ServiceRequest

	remotePeer := remotePeerId.String()
	peer, err := p2pm.GeneratePeerAddrInfo(remotePeer)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	err = json.Unmarshal(data, &serviceRequest)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	serviceResponse := node_types.ServiceResponse{
		JobId:          int64(0),
		Accepted:       false,
		Message:        "",
		OrderingNodeId: remotePeer,
		ServiceRequest: serviceRequest,
	}

	// Check if it is existing and active service
	serviceManager := NewServiceManager(p2pm)
	err, exist := serviceManager.Exists(serviceRequest.ServiceId)
	if err != nil {
		serviceResponse.Message = err.Error()

		// Stream error message
		errs := StreamData(p2pm, peer, &serviceResponse, nil, nil)
		if errs != nil {
			p2pm.Lm.Log("error", errs.Error(), "p2p")
			return errs
		}

		return err
	}

	if !exist {
		err := fmt.Errorf("could not find service Id %d", serviceRequest.ServiceId)
		p2pm.Lm.Log("error", err.Error(), "p2p")

		serviceResponse.Message = err.Error()

		// Stream error message
		errs := StreamData(p2pm, peer, &serviceResponse, nil, nil)
		if errs != nil {
			p2pm.Lm.Log("error", errs.Error(), "p2p")
			return errs
		}

		return err
	}

	// Get service
	service, err := serviceManager.Get(serviceRequest.ServiceId)
	if err != nil {
		serviceResponse.Message = err.Error()

		// Stream error message
		errs := StreamData(p2pm, peer, &serviceResponse, nil, nil)
		if errs != nil {
			p2pm.Lm.Log("error", errs.Error(), "p2p")
			return errs
		}

		return err
	}

	// If service type is DATA populate file hash(es) in response message
	if service.Type == "DATA" {
		dataService, err := serviceManager.GetData(serviceRequest.ServiceId)
		if err != nil {
			serviceResponse.Message = err.Error()

			// Stream error message
			errs := StreamData(p2pm, peer, &serviceResponse, nil, nil)
			if errs != nil {
				p2pm.Lm.Log("error", errs.Error(), "p2p")
				return errs
			}

			return err
		}

		// Set data path in response message
		serviceResponse.Message = dataService.Path
	} else {
		serviceResponse.Message = ""
	}

	// TODO, service price and payment

	// Create a job
	jobManager := p2pm.jobManager
	job, err := jobManager.CreateJob(serviceRequest, remotePeer)
	if err != nil {
		serviceResponse.Message = err.Error()

		// Stream error message
		errs := StreamData(p2pm, peer, &serviceResponse, nil, nil)
		if errs != nil {
			p2pm.Lm.Log("error", errs.Error(), "p2p")
			return errs
		}

		return err
	}

	serviceResponse.JobId = job.Id
	serviceResponse.Accepted = true

	// Send successfull service response message
	err = StreamData(p2pm, peer, &serviceResponse, nil, nil)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	return nil
}

// Received Service Response from remote peer
func (p2pm *P2PManager) serviceResponseReceived(data []byte) error {
	var serviceResponse node_types.ServiceResponse

	err := json.Unmarshal(data, &serviceResponse)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	if !serviceResponse.Accepted {
		err := fmt.Errorf("service request (service request Id %d) for node Id %s is not accepted with the following reason: %s",
			serviceResponse.ServiceId, serviceResponse.NodeId, serviceResponse.Message)
		p2pm.Lm.Log("error", err.Error(), "p2p")
	} else {
		// Add job to workflow
		err := p2pm.wm.RegisteredWorkflowJob(serviceResponse.WorkflowId, serviceResponse.WorkflowJobId,
			serviceResponse.NodeId, serviceResponse.ServiceId, serviceResponse.JobId, serviceResponse.Message)
		if err != nil {
			p2pm.Lm.Log("error", err.Error(), "p2p")
			return err
		}
	}

	// Draw output / push output message
	uiType, err := ui.DetectUIType(p2pm.UI)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	switch uiType {
	case "CLI":
		menuManager := NewMenuManager(p2pm)
		menuManager.printServiceResponse(serviceResponse)
	case "GUI":
		// TODO, GUI push message
	default:
		err := fmt.Errorf("unknown UI type %s", uiType)
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	return nil
}

// Received Job Run Request from remote peer
func (p2pm *P2PManager) jobRunRequestReceived(data []byte, remotePeerId peer.ID) error {
	var jobRunRequest node_types.JobRunRequest

	remotePeer := remotePeerId.String()
	peer, err := p2pm.GeneratePeerAddrInfo(remotePeer)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	err = json.Unmarshal(data, &jobRunRequest)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	jobRunResponse := node_types.JobRunResponse{
		Accepted:      false,
		Message:       "",
		JobRunRequest: jobRunRequest,
	}

	// Check if job is existing
	jobManager := p2pm.jobManager
	if err, exists := jobManager.JobExists(jobRunRequest.JobId); err != nil || !exists {
		jobRunResponse.Message = err.Error()

		// Stream error message
		errs := StreamData(p2pm, peer, &jobRunResponse, nil, nil)
		if errs != nil {
			p2pm.Lm.Log("error", errs.Error(), "p2p")
			return errs
		}

		return err
	}

	// Check if job is owned by requesting peer
	job, err := jobManager.GetJob(jobRunRequest.JobId)
	if err != nil {
		jobRunResponse.Message = err.Error()

		// Stream error message
		errs := StreamData(p2pm, peer, &jobRunResponse, nil, nil)
		if errs != nil {
			p2pm.Lm.Log("error", errs.Error(), "p2p")
			return errs
		}

		return err
	}

	if job.OrderingNodeId != remotePeer {
		err := fmt.Errorf("job run request for job Id %d is made by node %s who is not owning the job",
			jobRunRequest.JobId, remotePeer)
		p2pm.Lm.Log("error", err.Error(), "p2p")

		jobRunResponse.Message = err.Error()

		// Stream error message
		errs := StreamData(p2pm, peer, &jobRunResponse, nil, nil)
		if errs != nil {
			p2pm.Lm.Log("error", errs.Error(), "p2p")
			return errs
		}

		return err
	}

	// Set READY flag for the job
	err = jobManager.UpdateJobStatus(job.Id, "READY")
	if err != nil {
		jobRunResponse.Message = err.Error()

		// Stream error message
		errs := StreamData(p2pm, peer, &jobRunResponse, nil, nil)
		if errs != nil {
			p2pm.Lm.Log("error", errs.Error(), "p2p")
			return errs
		}

		return err
	}

	jobRunResponse.Message = fmt.Sprintf("READY flag is set for job Id %d.", jobRunRequest.JobId)
	jobRunResponse.Accepted = true

	// Stream job run accept message
	errs := StreamData(p2pm, peer, &jobRunResponse, nil, nil)
	if errs != nil {
		p2pm.Lm.Log("error", errs.Error(), "p2p")
		return errs
	}

	return nil
}

// Received Job Run Response from remote peer
func (p2pm *P2PManager) jobRunResponseReceived(data []byte) error {
	var jobRunResponse node_types.JobRunResponse

	err := json.Unmarshal(data, &jobRunResponse)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	// Update workflow job status
	workflowManager := workflow.NewWorkflowManager(p2pm.DB, p2pm.Lm, p2pm.cm)
	if !jobRunResponse.Accepted {
		err := fmt.Errorf("job run request Id %d for node Id %s is not accepted with the following reason: %s",
			jobRunResponse.JobId, jobRunResponse.NodeId, jobRunResponse.Message)
		p2pm.Lm.Log("error", err.Error(), "p2p")

		err = workflowManager.UpdateWorkflowJobStatus(jobRunResponse.WorkflowId, jobRunResponse.NodeId, jobRunResponse.JobId, "ERRORED")
		if err != nil {
			p2pm.Lm.Log("error", err.Error(), "p2p")
			return err
		}
	}

	// Draw output / push mesage
	uiType, err := ui.DetectUIType(p2pm.UI)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	switch uiType {
	case "CLI":
		menuManager := NewMenuManager(p2pm)
		menuManager.printJobRunResponse(jobRunResponse)
	case "GUI":
		// TODO, GUI push message
	default:
		err := fmt.Errorf("unknown UI type %s", uiType)
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	return nil
}

// Received Job Run Status Request from remote peer
func (p2pm *P2PManager) jobRunStatusRequestReceived(data []byte, remotePeerId peer.ID) error {
	var jobRunStatusRequest node_types.JobRunStatusRequest

	err := json.Unmarshal(data, &jobRunStatusRequest)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	remotePeer := remotePeerId.String()
	peer, err := p2pm.GeneratePeerAddrInfo(remotePeer)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	jobRunStatusResponse := node_types.JobRunStatus{
		JobRunStatusRequest: jobRunStatusRequest,
		Status:              "",
	}

	// Check if job is existing
	jobManager := p2pm.jobManager
	if err, exists := jobManager.JobExists(jobRunStatusRequest.JobId); err != nil || !exists {
		err := fmt.Errorf("could not find job Id %d (%s)", jobRunStatusRequest.JobId, err.Error())
		p2pm.Lm.Log("error", err.Error(), "p2p")

		jobRunStatusResponse.Status = "ERRORED"

		// Stream error message
		errs := StreamData(p2pm, peer, &jobRunStatusResponse, nil, nil)
		if errs != nil {
			p2pm.Lm.Log("error", errs.Error(), "p2p")
			return errs
		}

		return err
	}

	// Check if job is owned by requesting peer
	job, err := jobManager.GetJob(jobRunStatusRequest.JobId)
	if err != nil {
		jobRunStatusResponse.Status = "ERRORED"

		// Stream error message
		errs := StreamData(p2pm, peer, &jobRunStatusResponse, nil, nil)
		if errs != nil {
			p2pm.Lm.Log("error", errs.Error(), "p2p")
			return errs
		}

		return err
	}

	if job.OrderingNodeId != remotePeer {
		err := fmt.Errorf("job run status request for job Id %d is made by node %s who is not owning the job",
			jobRunStatusRequest.JobId, remotePeer)
		p2pm.Lm.Log("error", err.Error(), "p2p")

		jobRunStatusResponse.Status = "ERRORED"

		// Stream error message
		errs := StreamData(p2pm, peer, &jobRunStatusResponse, nil, nil)
		if errs != nil {
			p2pm.Lm.Log("error", errs.Error(), "p2p")
			return errs
		}

		return err
	}

	// Stream status update message
	err = jobManager.StatusUpdate(job.Id, job.Status)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	return nil
}

// Received Job Run Status Update from remote peer
func (p2pm *P2PManager) jobRunStatusUpdateReceived(data []byte) error {
	var jobRunStatus node_types.JobRunStatus

	err := json.Unmarshal(data, &jobRunStatus)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	// Update workflow job status
	workflowManager := workflow.NewWorkflowManager(p2pm.DB, p2pm.Lm, p2pm.cm)
	err = workflowManager.UpdateWorkflowJobStatus(jobRunStatus.WorkflowId, jobRunStatus.NodeId, jobRunStatus.JobId, jobRunStatus.Status)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	return nil
}

// Received Job Data Request from remote peer
func (p2pm *P2PManager) jobDataRequestReceived(data []byte, remotePeerId peer.ID) error {
	var jobDataRequest node_types.JobDataRequest

	err := json.Unmarshal(data, &jobDataRequest)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	// Send the data if peer is eligible for it
	jobManager := p2pm.jobManager
	err = jobManager.SendIfPeerEligible(
		jobDataRequest.JobId,
		jobDataRequest.InterfaceId,
		remotePeerId,
		jobDataRequest.WhatData,
	)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	return nil
}

// Received Job Data Acknowledgment Receipt from remote peer
func (p2pm *P2PManager) jobDataAcknowledgementReceiptReceived(data []byte, remotePeerId peer.ID) error {
	var jobDataReceiptAcknowledgement node_types.JobDataReceiptAcknowledgement

	err := json.Unmarshal(data, &jobDataReceiptAcknowledgement)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	// Acknowledge receipt
	jobManager := p2pm.jobManager
	err = jobManager.AcknowledgeReceipt(
		jobDataReceiptAcknowledgement.JobId,
		jobDataReceiptAcknowledgement.InterfaceId,
		remotePeerId,
		"OUTPUT",
	)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	return nil
}

// Received Service Request Cancelation from remote peer
func (p2pm *P2PManager) serviceRequestCancellationReceived(data []byte, remotePeerId peer.ID) error {
	var serviceRequestCancellation node_types.ServiceRequestCancellation

	remotePeer := remotePeerId.String()
	peer, err := p2pm.GeneratePeerAddrInfo(remotePeer)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	err = json.Unmarshal(data, &serviceRequestCancellation)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	serviceResponseCancellation := node_types.ServiceResponseCancellation{
		JobId:                      serviceRequestCancellation.JobId,
		Accepted:                   false,
		Message:                    "",
		OrderingNodeId:             remotePeer,
		ServiceRequestCancellation: serviceRequestCancellation,
	}

	// Get job
	jobManager := p2pm.jobManager
	job, err := jobManager.GetJob(serviceRequestCancellation.JobId)
	if err != nil {
		serviceResponseCancellation.Message = err.Error()

		// Stream error message
		errs := StreamData(p2pm, peer, &serviceResponseCancellation, nil, nil)
		if errs != nil {
			p2pm.Lm.Log("error", errs.Error(), "p2p")
			return errs
		}

		return err
	}

	// Check if job is owned by ordering peer
	if job.OrderingNodeId != remotePeer {
		err := fmt.Errorf("job cancellation request for job Id %d is made by node %s who is not owning the job",
			serviceRequestCancellation.JobId, remotePeer)
		p2pm.Lm.Log("error", err.Error(), "p2p")

		serviceResponseCancellation.Message = err.Error()

		// Stream error message
		errs := StreamData(p2pm, peer, &serviceResponseCancellation, nil, nil)
		if errs != nil {
			p2pm.Lm.Log("error", errs.Error(), "p2p")
			return errs
		}

		return err
	}

	// Remove a job
	err = jobManager.RemoveJob(serviceRequestCancellation.JobId)
	if err != nil {
		serviceResponseCancellation.Message = err.Error()

		// Stream error message
		errs := StreamData(p2pm, peer, &serviceResponseCancellation, nil, nil)
		if errs != nil {
			p2pm.Lm.Log("error", errs.Error(), "p2p")
			return errs
		}

		return err
	}

	// Stream successfull cancellation message
	serviceResponseCancellation.Accepted = true
	err = StreamData(p2pm, peer, &serviceResponseCancellation, nil, nil)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	return nil
}

// Received Service Request Cancellation Response from remote peer
func (p2pm *P2PManager) serviceRequestCancellationResponseReceived(data []byte) error {
	var serviceResponseCancellation node_types.ServiceResponseCancellation

	err := json.Unmarshal(data, &serviceResponseCancellation)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	if !serviceResponseCancellation.Accepted {
		err := fmt.Errorf("service request cancellation (job Id %d) for node Id %s is not accepted with the following reason: %s",
			serviceResponseCancellation.JobId, serviceResponseCancellation.NodeId, serviceResponseCancellation.Message)
		p2pm.Lm.Log("error", err.Error(), "p2p")
	} else {
		// Remove a workflow job
		err := p2pm.wm.RemoveWorkflowJob(serviceResponseCancellation.WorkflowJobId)
		if err != nil {
			p2pm.Lm.Log("error", err.Error(), "p2p")
			return err
		}
	}

	// Draw table / push message
	uiType, err := ui.DetectUIType(p2pm.UI)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}
	switch uiType {
	case "CLI":
		// TODO, CLI
	case "GUI":
		// TODO, GUI push message
	default:
		err := fmt.Errorf("unknown UI type %s", uiType)
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	return nil
}

func (p2pm *P2PManager) sendReceiptAcknowledgement(
	workflowId int64,
	jobId int64,
	interfaceId int64,
	orderingNodeId string,
	receivingNodeId string,
	jobRunningNodeId string,
) {
	receiptAcknowledgement := node_types.JobDataReceiptAcknowledgement{
		JobRunStatusRequest: node_types.JobRunStatusRequest{
			WorkflowId: workflowId,
			NodeId:     jobRunningNodeId,
			JobId:      jobId,
		},
		InterfaceId:     interfaceId,
		OrderingNodeId:  orderingNodeId,
		ReceivingNodeId: receivingNodeId,
	}

	peer, err := p2pm.GeneratePeerAddrInfo(jobRunningNodeId)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return
	}

	err = StreamData(p2pm, peer, &receiptAcknowledgement, nil, nil)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return
	}
}

func BroadcastMessage[T any](p2pm *P2PManager, message T) error {
	var m []byte
	var err error
	var topic *pubsub.Topic

	switch v := any(message).(type) {
	case node_types.ServiceLookup:
		m, err = json.Marshal(message)
		if err != nil {
			p2pm.Lm.Log("error", err.Error(), "p2p")
			return err
		}

		topicKey := p2pm.cm.GetConfigWithDefault("topic_name_prefix", "trustflow.network.") + LOOKUP_SERVICE
		topic = p2pm.topicsSubscribed[topicKey]

	default:
		msg := fmt.Sprintf("Message type %v is not allowed in this context (broadcasting message)", v)
		p2pm.Lm.Log("error", msg, "p2p")
		err := errors.New(msg)
		return err
	}

	if err := topic.Publish(p2pm.ctx, m); err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	return nil
}

func (p2pm *P2PManager) receivedMessage(ctx context.Context, sub *pubsub.Subscription) {
	for {
		if sub == nil {
			err := fmt.Errorf("subscription is nil. Will stop listening")
			p2pm.Lm.Log("error", err.Error(), "p2p")
			break
		}

		m, err := sub.Next(ctx)
		if err != nil {
			p2pm.Lm.Log("error", err.Error(), "p2p")
		}
		if m == nil {
			err := fmt.Errorf("message received is nil. Will stop listening")
			p2pm.Lm.Log("error", err.Error(), "p2p")
			break
		}

		message := string(m.Message.Data)
		peerId := m.GetFrom()
		p2pm.Lm.Log("debug", fmt.Sprintf("Message %s received from %s via %s", message, peerId.String(), m.ReceivedFrom), "p2p")

		topic := string(*m.Topic)
		p2pm.Lm.Log("debug", fmt.Sprintf("Message topic is %s", topic), "p2p")

		switch topic {
		case p2pm.cm.GetConfigWithDefault("topic_name_prefix", "trustflow.network.") + LOOKUP_SERVICE:
			err := p2pm.sendServiceOffer(m.Message.Data, peerId)
			if err != nil {
				p2pm.Lm.Log("error", err.Error(), "p2p")
				continue
			}
		default:
			msg := fmt.Sprintf("Unknown topic %s", topic)
			p2pm.Lm.Log("error", msg, "p2p")
			continue
		}
	}
}

func (p2pm *P2PManager) sendServiceOffer(data []byte, peerId peer.ID) error {
	services, err := p2pm.ServiceLookup(data, true)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	// Retrieve known multiaddresses from the peerstore
	peerAddrInfo, err := p2pm.GeneratePeerAddrInfo(peerId.String())
	if err != nil {
		msg := err.Error()
		p2pm.Lm.Log("error", msg, "p2p")
		return err
	}

	// Stream back offered services with prices
	// (only if there we have matching services to offer)
	if len(services) > 0 {
		err = StreamData(p2pm, peerAddrInfo, &services, nil, nil)
		if err != nil {
			msg := err.Error()
			p2pm.Lm.Log("error", msg, "p2p")
			return err
		}
	}

	return nil
}

func (p2pm *P2PManager) RequestServiceCatalogue(searchPhrases string, serviceType string, peer string) error {
	var serviceLookup node_types.ServiceLookup = node_types.ServiceLookup{
		Phrases: searchPhrases,
		Type:    serviceType,
	}

	peerAddrInfo, err := p2pm.GeneratePeerAddrInfo(peer)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	// Send successfull service response message
	err = StreamData(p2pm, peerAddrInfo, &serviceLookup, nil, nil)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	return nil
}

func (p2pm *P2PManager) ServiceLookup(data []byte, active bool) ([]node_types.ServiceOffer, error) {
	var services []node_types.ServiceOffer
	var lookup node_types.ServiceLookup

	if len(data) == 0 {
		return services, nil
	}

	err := json.Unmarshal(data, &lookup)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return nil, err
	}

	var searchService node_types.SearchService = node_types.SearchService{
		Phrases: lookup.Phrases,
		Type:    lookup.Type,
		Active:  active,
	}

	// Search services
	var offset uint32 = 0
	var limit uint32 = 1
	l := p2pm.cm.GetConfigWithDefault("search_results", "10")
	l64, err := strconv.ParseUint(l, 10, 32)
	if err != nil {
		limit = 10
	} else {
		limit = uint32(l64)
	}
	serviceManager := NewServiceManager(p2pm)

	for {
		servicesBatch, err := serviceManager.SearchServices(searchService, offset, limit)
		if err != nil {
			p2pm.Lm.Log("error", err.Error(), "p2p")
			break
		}

		services = append(services, servicesBatch...)

		if len(servicesBatch) == 0 {
			break
		}

		offset += uint32(len(servicesBatch))
	}

	return services, nil
}

func (p2pm *P2PManager) GeneratePeerAddrInfo(peerId string) (peer.AddrInfo, error) {
	var p peer.AddrInfo

	// Convert the string to a peer.ID
	pID, err := p2pm.GeneratePeerId(peerId)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return p, err
	}

	addrInfo, err := p2pm.idht.FindPeer(p2pm.ctx, pID)
	if err != nil {
		msg := err.Error()
		p2pm.Lm.Log("error", msg, "p2p")
		return p, err
	}

	// Create peer.AddrInfo from peer.ID
	p = peer.AddrInfo{
		ID: pID,
		// Add any multiaddresses if known (leave blank here if unknown)
		Addrs: addrInfo.Addrs,
	}

	return p, nil
}

func (p2pm *P2PManager) makeRelayPeerInfo(peerAddrStr string) (peer.AddrInfo, error) {
	maddr, err := ma.NewMultiaddr(peerAddrStr)
	if err != nil {
		return peer.AddrInfo{}, err
	}
	peerAddrInfo, err := peer.AddrInfoFromP2pAddr(maddr)
	if err != nil {
		return peer.AddrInfo{}, err
	}
	return *peerAddrInfo, nil
}

func (p2pm *P2PManager) GeneratePeerId(peerId string) (peer.ID, error) {
	// Convert the string to a peer.ID
	pID, err := peer.Decode(peerId)
	if err != nil {
		return pID, err
	}

	return pID, nil
}

// cleanupOldStreams removes old/stale streams from tracking
func (p2pm *P2PManager) cleanupOldStreams() {
	p2pm.streamsMutex.Lock()
	defer p2pm.streamsMutex.Unlock()

	cutoff := time.Now().Add(-30 * time.Minute) // Remove streams older than 30 minutes
	for streamID, streamInfo := range p2pm.activeStreams {
		if streamInfo.startTime.Before(cutoff) {
			delete(p2pm.activeStreams, streamID)
			p2pm.Lm.Log("debug", fmt.Sprintf("Cleaned up old stream tracking: %s", streamID), "p2p")
		}
	}
}

// handleStreamError handles stream errors with graceful close or reset fallback
func (p2pm *P2PManager) handleStreamError(s network.Stream, message string, err error) {
	p2pm.Lm.Log("error", fmt.Sprintf("%s: %v", message, err), "p2p")
	if closeErr := s.Close(); closeErr != nil {
		s.Reset() // Fallback to reset if close fails
	}
}

// startPeriodicCleanup runs periodic cleanup tasks
func (p2pm *P2PManager) startPeriodicCleanup() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-p2pm.ctx.Done():
			return
		case <-ticker.C:
			p2pm.Lm.Log("debug", "Running periodic cleanup cycle", "p2p")
			// Clean up old resources
			if cleaned := p2pm.resourceTracker.CleanupOldResources(30 * time.Minute); cleaned > 0 {
				p2pm.Lm.Log("info", fmt.Sprintf("Cleaned up %d old resources", cleaned), "p2p")
			}

			// Clean up old stream tracking
			p2pm.cleanupOldStreams()

			// Run P2PResourceManager periodic cleanup
			if p2pm.p2pResourceManager != nil {
				p2pm.p2pResourceManager.PeriodicCleanup()
			}

			// Force GC after cleanup to reclaim memory
			memStats := utils.ForceGC()
			p2pm.Lm.Log("debug", fmt.Sprintf("Post-cleanup memory - HeapInuse: %d MB, NumGC: %d",
				memStats.HeapInuse/1024/1024, memStats.NumGC), "p2p")

			// Periodic goroutine leak detection with detailed reports
			if p2pm.leakDetector != nil {
				// Log basic leak warnings
				p2pm.leakDetector.LogLeaks(func(level, msg, category string) {
					p2pm.Lm.Log(level, msg, category)
				})

				// Generate detailed leak report every 30 minutes (3 cleanup cycles)
				// Use a simple counter to track cycles
				if p2pm.cleanupCycleCount == 0 {
					p2pm.cleanupCycleCount = 1
				}

				if p2pm.cleanupCycleCount%3 == 0 {
					report := p2pm.GetGoroutineLeakReport()
					p2pm.Lm.Log("info", fmt.Sprintf("Detailed goroutine analysis:\n%s", report), "leak-detector")
				}
				p2pm.cleanupCycleCount++
			}

			// Check goroutine health and log stuck goroutines
			gt := p2pm.GetGoroutineTracker()
			gt.CheckGoroutineHealth()

			// Log basic resource statistics
			stats := p2pm.resourceTracker.GetResourceStats()
			if len(stats) > 0 {
				p2pm.Lm.Log("debug", fmt.Sprintf("Basic resource stats: %+v", stats), "p2p")
			}

			// Log enhanced resource statistics every cleanup cycle (10 minutes)
			enhancedStats := p2pm.GetEnhancedResourceStats()
			if enhancedStats != nil {
				p2pm.Lm.Log("info", fmt.Sprintf("Enhanced resource stats: %+v", enhancedStats), "p2p-enhanced")
			}

		}
	}
}

// GetGoroutineLeakReport returns a detailed goroutine leak analysis
func (p2pm *P2PManager) GetGoroutineLeakReport() string {
	if p2pm.leakDetector == nil {
		return "Leak detector not initialized"
	}

	currentStats := p2pm.leakDetector.GetCurrentStats()
	leaks := p2pm.leakDetector.DetectLeaks()

	var report strings.Builder
	report.WriteString("=== GOROUTINE LEAK ANALYSIS ===\n")
	report.WriteString(currentStats)

	if len(leaks) > 0 {
		report.WriteString(fmt.Sprintf("\n=== DETECTED LEAKS (%d) ===\n", len(leaks)))
		for i, leak := range leaks {
			if i >= 10 { // Limit to top 10 for readability
				report.WriteString(fmt.Sprintf("... and %d more leaks\n", len(leaks)-i))
				break
			}
			report.WriteString(fmt.Sprintf("%d. %s\n", i+1, leak.String()))
		}
	} else {
		report.WriteString("\n=== NO LEAKS DETECTED ===\n")
	}

	return report.String()
}
