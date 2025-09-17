package node

import (
	"container/list"
	"context"
	"fmt"
	"maps"
	"math"
	"slices"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/adgsm/trustflow-node/internal/ui"
	"github.com/adgsm/trustflow-node/internal/utils"
	"github.com/ipfs/go-cid"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	mh "github.com/multiformats/go-multihash"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// LRUCache implements a simple LRU cache for peer statistics
type LRUCache struct {
	capacity int
	items    map[peer.ID]*list.Element
	order    *list.List
	mu       sync.RWMutex
}

type cacheItem struct {
	key   peer.ID
	value *PeerStats
}

func NewLRUCache(capacity int) *LRUCache {
	return &LRUCache{
		capacity: capacity,
		items:    make(map[peer.ID]*list.Element),
		order:    list.New(),
	}
}

func (c *LRUCache) Get(key peer.ID) (*PeerStats, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, exists := c.items[key]; exists {
		c.order.MoveToFront(elem)
		return elem.Value.(*cacheItem).value, true
	}
	return nil, false
}

func (c *LRUCache) Set(key peer.ID, value *PeerStats) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, exists := c.items[key]; exists {
		c.order.MoveToFront(elem)
		elem.Value.(*cacheItem).value = value
		return
	}

	// Add new item
	if c.order.Len() >= c.capacity {
		// Remove least recently used
		back := c.order.Back()
		if back != nil {
			c.order.Remove(back)
			delete(c.items, back.Value.(*cacheItem).key)
		}
	}

	elem := c.order.PushFront(&cacheItem{key: key, value: value})
	c.items[key] = elem
}

func (c *LRUCache) Remove(key peer.ID) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, exists := c.items[key]; exists {
		c.order.Remove(elem)
		delete(c.items, key)
	}
}

func (c *LRUCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

// PeerStats tracks historical data about a peer's usefulness
type PeerStats struct {
	DiscoveryAttempts       int
	SuccessfulDiscoveries   int
	LastDiscoveryAttempt    time.Time
	LastSuccessfulDiscovery time.Time
	ConnectionQuality       float64
}

// TopicAwareConnectionManager manages connections with topic and routing awareness
type TopicAwareConnectionManager struct {
	ctx          context.Context
	host         host.Host
	pubsub       *pubsub.PubSub
	dht          *dht.IpfsDHT
	targetTopics []string
	lm           *utils.LogsManager
	uiType       string
	p2pm         *P2PManager // Reference to access goroutine tracker

	// Connection priorities
	topicPeers     map[peer.ID]bool      // Peers subscribed to our topics
	routingPeers   map[peer.ID]float64   // Peers useful for routing (with score)
	lastEvaluation map[peer.ID]time.Time // Last time we evaluated peer usefulness
	lastTopicCheck map[peer.ID]time.Time // Last time we checked topic subscription
	peerStatsCache *LRUCache             // Historical statistics

	mu sync.RWMutex

	// Configuration
	maxConnections        int
	maxPeerTracking       int     // Limit tracked peers
	topicPeerRatio        float64 // % of connections reserved for topic peers
	routingPeerRatio      float64 // % for routing peers
	evaluationPeriod      time.Duration
	routingScoreThreshold float64
	cleanupInterval       time.Duration // Regular cleanup

	evaluationTicker *time.Ticker // Evaluation ticker
	cleanupTicker    *time.Ticker // Cleanup ticker
	stopChan         chan struct{}

	// Worker pool for peer evaluation with priority
	topicPeerQueue chan peer.ID   // Unbounded priority queue for topic peers
	regularQueue   chan peer.ID   // Bounded queue for regular peer evaluation
	workerPool     sync.WaitGroup // Track worker goroutines

	// New fields for improved management
	isEvaluationInProgress bool       // Prevent overlapping evaluations
	maxEvaluationsPerCycle int        // Rate limiting
	evaluationInProgressMu sync.Mutex // Protect isEvaluationInProgress

	// Semaphore for coordinating evaluation completion with trimming
	pendingEvaluations int32         // Atomic counter for pending evaluations
	evaluationComplete chan struct{} // Signal when all evaluations are done
	
	// Shutdown protection
	isShutdown int32 // Atomic flag to prevent multiple shutdowns
}

func NewTopicAwareConnectionManager(p2pm *P2PManager, maxConnections int,
	targetTopics []string) (*TopicAwareConnectionManager, error) {
	maxPeerTracking := maxConnections * 3 // Track 3x max connections

	// Detect UI type for emitting events to front-end
	uiType, err := ui.DetectUIType(p2pm.UI)
	if err != nil {
		return nil, err
	}

	tcm := &TopicAwareConnectionManager{
		ctx:                    p2pm.ctx,
		host:                   p2pm.h,
		pubsub:                 p2pm.ps,
		dht:                    p2pm.idht,
		lm:                     p2pm.Lm,
		uiType:                 uiType,
		p2pm:                   p2pm,
		targetTopics:           targetTopics,
		topicPeers:             make(map[peer.ID]bool),
		routingPeers:           make(map[peer.ID]float64),
		lastEvaluation:         make(map[peer.ID]time.Time),
		lastTopicCheck:         make(map[peer.ID]time.Time),
		peerStatsCache:         NewLRUCache(maxPeerTracking),
		maxConnections:         maxConnections,
		maxPeerTracking:        maxPeerTracking,
		topicPeerRatio:         0.9, // 90% target for topic peers (unlimited - never disconnected)
		routingPeerRatio:       0.1, // 10% for routing peers (capped at 50 max)
		evaluationPeriod:       time.Minute * 5,
		cleanupInterval:        time.Minute * 10,
		routingScoreThreshold:  0.1,
		stopChan:               make(chan struct{}),
		topicPeerQueue:         make(chan peer.ID, 1000), // Large buffer for topic peers - never drop
		regularQueue:           make(chan peer.ID, 100),  // Bounded buffer for regular peers
		maxEvaluationsPerCycle: 50,                       // Limit to 50 evaluations per 3-minute cycle
		evaluationComplete:     make(chan struct{}, 1),   // Buffered channel for completion signal
	}

	// Start periodic evaluation and cleanup
	tcm.startPeriodicEvaluation(3 * time.Minute)
	tcm.startPeriodicCleanup()

	// Start worker pool for peer evaluation
	tcm.startWorkerPool(3) // Start 3 worker goroutines

	return tcm, nil
}

// Ticker-based periodic peer evaluation
func (tcm *TopicAwareConnectionManager) startPeriodicEvaluation(interval time.Duration) {
	tcm.evaluationTicker = time.NewTicker(interval)

	gt := tcm.p2pm.GetGoroutineTracker()
	gt.SafeStart("tcm-peer-evaluation", func() {
		defer tcm.evaluationTicker.Stop()

		for {
			select {
			case <-tcm.evaluationTicker.C:
				tcm.lm.Log("debug", "Starting periodic peer evaluation", "p2p")
				tcm.peersEvaluation()
			case <-tcm.stopChan:
				tcm.lm.Log("debug", "Stopping periodic peer evaluation", "p2p")
				return
			case <-tcm.ctx.Done():
				tcm.lm.Log("debug", "Context cancelled, stopping peer evaluation", "p2p")
				return
			}
		}
	})
}

// Stop periodic peer evaluation
func (tcm *TopicAwareConnectionManager) StopPeriodicEvaluation() {
	close(tcm.stopChan)
}

// Periodic cleanup to prevent unbounded growth
func (tcm *TopicAwareConnectionManager) startPeriodicCleanup() {
	tcm.cleanupTicker = time.NewTicker(tcm.cleanupInterval)

	gt := tcm.p2pm.GetGoroutineTracker()
	gt.SafeStart("tcm-periodic-cleanup", func() {
		defer tcm.cleanupTicker.Stop()

		for {
			select {
			case <-tcm.cleanupTicker.C:
				tcm.cleanupStaleData()
			case <-tcm.stopChan:
				return
			case <-tcm.ctx.Done():
				return
			}
		}
	})
}

// Cleanup stale peer data
func (tcm *TopicAwareConnectionManager) cleanupStaleData() {
	tcm.mu.Lock()
	defer tcm.mu.Unlock()

	now := time.Now()
	staleThreshold := time.Hour * 2 // Remove data older than 2 hours

	// Clean up topicPeers
	topicPeersBeforeCleanup := len(tcm.topicPeers)
	if len(tcm.topicPeers) > tcm.maxPeerTracking {
		// Keep only connected peers if over limit
		newTopicPeers := make(map[peer.ID]bool)
		for peerID := range tcm.topicPeers {
			if tcm.host.Network().Connectedness(peerID) == network.Connected {
				newTopicPeers[peerID] = true
			}
		}
		tcm.topicPeers = newTopicPeers
		tcm.lm.Log("debug", fmt.Sprintf("Topic peers cleanup: %d -> %d (removed %d disconnected)",
			topicPeersBeforeCleanup, len(tcm.topicPeers), topicPeersBeforeCleanup-len(tcm.topicPeers)), "p2p")
	}

	// Clean up routingPeers
	if len(tcm.routingPeers) > tcm.maxPeerTracking {
		// Keep only recent and connected peers
		newRoutingPeers := make(map[peer.ID]float64)
		for peerID, score := range tcm.routingPeers {
			if tcm.host.Network().Connectedness(peerID) == network.Connected ||
				(now.Sub(tcm.lastEvaluation[peerID]) < staleThreshold) {
				newRoutingPeers[peerID] = score
			}
		}
		tcm.routingPeers = newRoutingPeers
	}

	// Clean up lastEvaluation and lastTopicCheck
	stalePeers := make([]peer.ID, 0)
	for peerID, lastEval := range tcm.lastEvaluation {
		if now.Sub(lastEval) > staleThreshold {
			delete(tcm.lastEvaluation, peerID)
			stalePeers = append(stalePeers, peerID)
		}
	}

	// Clean up topic check timestamps
	for peerID, lastCheck := range tcm.lastTopicCheck {
		if now.Sub(lastCheck) > staleThreshold {
			delete(tcm.lastTopicCheck, peerID)
		}
	}

	// Also cleanup stale entries from LRU cache
	// Do this outside the main lock to avoid holding it too long
	tcm.mu.Unlock()

	// Clean up stale entries from stats cache
	for _, peerID := range stalePeers {
		// Only remove if peer is disconnected and stale
		if tcm.host.Network().Connectedness(peerID) != network.Connected {
			tcm.peerStatsCache.Remove(peerID)
		}
	}

	tcm.mu.Lock() // Re-acquire for the defer unlock

	tcm.lm.Log("debug", fmt.Sprintf(
		"Cleanup completed. Tracking %d topic peers, %d routing peers, %d stats entries",
		len(tcm.topicPeers), len(tcm.routingPeers), tcm.peerStatsCache.Len()), "p2p")
}

// Periodic evaluation of peers
func (tcm *TopicAwareConnectionManager) peersEvaluation() {
	// Prevent overlapping evaluations
	tcm.evaluationInProgressMu.Lock()
	if tcm.isEvaluationInProgress {
		tcm.lm.Log("debug", "Skipping peer evaluation - previous cycle still in progress", "p2p")
		tcm.evaluationInProgressMu.Unlock()
		return
	}
	tcm.isEvaluationInProgress = true
	tcm.evaluationInProgressMu.Unlock()

	defer func() {
		tcm.evaluationInProgressMu.Lock()
		tcm.isEvaluationInProgress = false
		tcm.evaluationInProgressMu.Unlock()
	}()

	// Update topic peer subscriptions
	tcm.UpdateTopicPeersList(tcm.targetTopics)

	// Only re-evaluate stale peers instead of ALL peers
	tcm.reevaluateStalePeers()

	// Wait for all evaluations to complete before trimming
	tcm.waitForEvaluationComplete()

	// Now trim connections based on updated scores
	ctx, cancel := context.WithTimeout(tcm.ctx, time.Minute*2)
	defer cancel()
	tcm.TrimOpenConns(ctx)

	// Log statistics
	connStats := tcm.GetConnectionStats()
	msg := fmt.Sprintf("Connection stats => Total: %d, Topic peers: %d, Routing peers: %d",
		connStats["total"], connStats["topic_peers"], connStats["routing_peers"])
	tcm.lm.Log("debug", msg, "libp2p-events")
}

// Refresh the list of peers subscribed to our topics
func (tcm *TopicAwareConnectionManager) UpdateTopicPeersList(targetTopics []string) {
	tcm.mu.Lock()
	defer tcm.mu.Unlock()

	// Store previous topic peers for comparison
	previousTopicPeers := make(map[peer.ID]bool)
	maps.Copy(previousTopicPeers, tcm.topicPeers)

	// Update selectively to prevent churn
	currentTopicPeers := make(map[peer.ID]bool)

	// Get current topic subscribers using pubsub.ListPeers directly
	for _, topicName := range targetTopics {
		peers := tcm.pubsub.ListPeers(topicName)
		tcm.lm.Log("debug", fmt.Sprintf("Topic '%s' reports %d peers: %v", topicName, len(peers), peers), "p2p")
		for _, peerID := range peers {
			currentTopicPeers[peerID] = true
		}
	}

	tcm.lm.Log("debug", fmt.Sprintf("Total unique topic peers from pubsub: %d", len(currentTopicPeers)), "p2p")

	// Bounded update - only keep up to maxPeerTracking peers
	if len(currentTopicPeers) > tcm.maxPeerTracking {
		// If we have too many topic peers, prioritize currently connected ones
		boundedTopicPeers := make(map[peer.ID]bool)
		connectedCount := 0

		// First pass: add connected peers
		for peerID := range currentTopicPeers {
			if connectedCount >= tcm.maxPeerTracking {
				break
			}
			if tcm.host.Network().Connectedness(peerID) == network.Connected {
				boundedTopicPeers[peerID] = true
				connectedCount++
			}
		}

		// Second pass: fill remaining slots with any peers
		for peerID := range currentTopicPeers {
			if len(boundedTopicPeers) >= tcm.maxPeerTracking {
				break
			}
			if _, exists := boundedTopicPeers[peerID]; !exists {
				boundedTopicPeers[peerID] = true
			}
		}

		tcm.topicPeers = boundedTopicPeers
		tcm.lm.Log("debug", fmt.Sprintf("Topic peers list bounded to %d entries", len(boundedTopicPeers)), "p2p")
	} else {
		tcm.topicPeers = currentTopicPeers
	}

	// Debug: Show final topic peers count and some peer IDs for verification
	tcm.lm.Log("debug", fmt.Sprintf("Final topic peers count: %d", len(tcm.topicPeers)), "p2p")
	if len(tcm.topicPeers) > 0 {
		peerIDs := make([]string, 0, min(5, len(tcm.topicPeers)))
		count := 0
		for peerID := range tcm.topicPeers {
			if count >= 5 {
				break
			}
			peerIDs = append(peerIDs, peerID.String()[:12]+"...")
			count++
		}
		tcm.lm.Log("debug", fmt.Sprintf("Sample topic peers: %v", peerIDs), "p2p")
	}

	switch tcm.uiType {
	case "CLI":
		// Do nothing
	case "GUI":
		// Emit events for newly connected topic peers
		for peerID := range tcm.topicPeers {
			if !previousTopicPeers[peerID] && tcm.host.Network().Connectedness(peerID) == network.Connected {
				runtime.EventsEmit(tcm.ctx, "topicpeerconnectedlog-event", peerID.String())
			}
		}

		// Emit events for disconnected topic peers
		for peerID := range previousTopicPeers {
			if !tcm.topicPeers[peerID] {
				runtime.EventsEmit(tcm.ctx, "topicpeerdisconnectedlog-event", peerID.String())
			}
		}
	default:
		// Do nothing
	}
}

// Calculates comprehensive routing score
func (tcm *TopicAwareConnectionManager) evaluateRoutingUsefulness(peerID peer.ID) {
	// FIXED: Add context checking and timeout
	ctx, cancel := context.WithTimeout(tcm.ctx, time.Minute*1)
	defer cancel()

	// Check if we should still evaluate this peer
	select {
	case <-ctx.Done():
		return
	default:
	}

	defer func() {
		tcm.mu.Lock()
		tcm.lastEvaluation[peerID] = time.Now()
		tcm.mu.Unlock()
	}()

	score := 0.0

	// Method 1: Check peer's potential connections to topic-subscribed peers
	// Add context checking between expensive operations
	select {
	case <-ctx.Done():
		return
	default:
		score += tcm.scoreByTopicPeerConnections(peerID)
	}

	// Method 2: DHT routing table position
	select {
	case <-ctx.Done():
		return
	default:
		score += tcm.scoreDHTPosition(peerID)
	}

	// Method 3: Historical success in reaching topic peers
	select {
	case <-ctx.Done():
		return
	default:
		score += tcm.scoreHistoricalSuccess(peerID)
	}

	// Method 4: Connection quality and stability
	select {
	case <-ctx.Done():
		return
	default:
		score += tcm.scoreConnectionQuality(peerID)
	}

	// Bound the final score and update with bounds checking
	finalScore := math.Min(score, 1.0)

	tcm.mu.Lock()
	oldScore := tcm.routingPeers[peerID]
	// Only update if we haven't exceeded our tracking limits
	if len(tcm.routingPeers) < tcm.maxPeerTracking || tcm.routingPeers[peerID] != 0 {
		tcm.routingPeers[peerID] = finalScore

		switch tcm.uiType {
		case "CLI":
			// Do nothing
		case "GUI":
			// Emit event if peer becomes a useful routing peer
			if oldScore <= tcm.routingScoreThreshold && finalScore > tcm.routingScoreThreshold {
				if tcm.host.Network().Connectedness(peerID) == network.Connected {
					runtime.EventsEmit(tcm.ctx, "routingpeerconnectedlog-event", peerID.String())
				}
			}
			// Emit event if peer is no longer a useful routing peer
			if oldScore > tcm.routingScoreThreshold && finalScore <= tcm.routingScoreThreshold {
				runtime.EventsEmit(tcm.ctx, "routingpeerdisconnectedlog-event", peerID.String())
			}
		default:
			// Do nothing
		}
	}
	tcm.mu.Unlock()
}

// Evaluates potential to reach topic peers
func (tcm *TopicAwareConnectionManager) scoreByTopicPeerConnections(peerID peer.ID) float64 {
	// Get snapshot of topic peers without holding lock for long
	tcm.mu.RLock()
	topicPeersCopy := make(map[peer.ID]bool)
	maps.Copy(topicPeersCopy, tcm.topicPeers)
	tcm.mu.RUnlock()

	potentialReach := 0
	totalTopicPeers := len(topicPeersCopy)

	if totalTopicPeers == 0 {
		return 0.0
	}

	// Check connectivity without holding locks
	for topicPeerID := range topicPeersCopy {
		if tcm.host.Network().Connectedness(topicPeerID) != network.Connected {
			if tcm.isPeerLikelyConnectedTo(peerID, topicPeerID) {
				potentialReach++
			}
		}
	}

	return float64(potentialReach) / float64(totalTopicPeers) * 0.4
}

// Uses heuristics to estimate peer connectivity
func (tcm *TopicAwareConnectionManager) isPeerLikelyConnectedTo(peerA, peerB peer.ID) bool {
	// Heuristic 1: Check if peers are in similar network regions (using peer ID distance)
	distanceScore := tcm.calculatePeerDistance(peerA, peerB)
	if distanceScore < 0.3 { // Close peers are more likely to be connected
		return true
	}

	// Heuristic 2: Check if peerA has been seen in DHT queries related to peerB
	if tcm.hasSeenPeerInDHTContext(peerA, peerB) {
		return true
	}

	// Heuristic 3: Check if they share common neighbors
	commonNeighbors := tcm.getCommonNeighbors(peerA, peerB)

	return commonNeighbors > 2
}

// Computes XOR distance between peer IDs
func (tcm *TopicAwareConnectionManager) calculatePeerDistance(peerA, peerB peer.ID) float64 {
	// Convert peer IDs to bytes and calculate XOR distance
	bytesA := []byte(peerA)
	bytesB := []byte(peerB)

	if len(bytesA) != len(bytesB) {
		return 1.0 // Maximum distance for different lengths
	}

	xorSum := 0
	for i := 0; i < len(bytesA) && i < len(bytesB); i++ {
		xorSum += int(bytesA[i] ^ bytesB[i])
	}

	// Normalize to 0-1 range
	maxPossible := 255 * len(bytesA)
	return float64(xorSum) / float64(maxPossible)
}

// Checks if peerA is useful for routing to peerB via DHT
func (tcm *TopicAwareConnectionManager) hasSeenPeerInDHTContext(peerA, peerB peer.ID) bool {
	if tcm.dht == nil {
		return false
	}

	// Check if peerA and peerB are in similar DHT key space (closer peers are more likely to help route)
	// This is a practical heuristic: if peers have similar key distances, one might help route to the other
	distanceAToUs := tcm.calculatePeerDistance(peerA, tcm.host.ID())
	distanceBToUs := tcm.calculatePeerDistance(peerB, tcm.host.ID())
	distanceAtoB := tcm.calculatePeerDistance(peerA, peerB)

	// If A is closer to B than we are, A might be useful for routing to B
	return distanceAtoB < distanceBToUs || distanceAToUs < distanceBToUs
}

// Estimates shared connections based on network topology
func (tcm *TopicAwareConnectionManager) getCommonNeighbors(peerA, peerB peer.ID) int {
	// Check if we're actually connected to both peers
	connectedToA := tcm.host.Network().Connectedness(peerA) == network.Connected
	connectedToB := tcm.host.Network().Connectedness(peerB) == network.Connected

	if !connectedToA && !connectedToB {
		return 0 // Can't estimate if we're not connected to either
	}

	// Simple heuristic: if peers are close in ID space, they likely share neighbors
	distance := tcm.calculatePeerDistance(peerA, peerB)

	// Convert distance to estimated common neighbors
	// Closer peers (lower distance) have more common neighbors
	if distance < 0.1 {
		return 5 // Very close peers likely share many neighbors
	} else if distance < 0.3 {
		return 3 // Moderately close peers share some neighbors
	} else if distance < 0.5 {
		return 1 // Distant peers might share few neighbors
	}

	return 0 // Very distant peers unlikely to share neighbors
}

// Evaluates peer's position in DHT for topic-related queries
func (tcm *TopicAwareConnectionManager) scoreDHTPosition(peerID peer.ID) float64 {
	if tcm.dht == nil {
		return 0.0
	}

	score := 0.0

	// Use DHT to find peers close to topic-related keys
	for _, topic := range tcm.targetTopics {
		ctx, cancel := context.WithTimeout(tcm.ctx, time.Second*2)

		// Create a CID from the topic string
		hash, err := mh.Sum([]byte(topic), mh.SHA2_256, -1)
		if err != nil {
			cancel()
			continue
		}
		topicCID := cid.NewCidV1(cid.Raw, hash)

		// Try to find providers for the topic
		providerCh := tcm.dht.FindProvidersAsync(ctx, topicCID, 5)

		for provider := range providerCh {
			if provider.ID == peerID {
				score += 0.1 // This peer provides topic-related content
				break
			}
		}

		cancel()
	}

	return math.Min(score, 0.3)
}

// Evaluates peer based on past performance
func (tcm *TopicAwareConnectionManager) scoreHistoricalSuccess(peerID peer.ID) float64 {
	stats, exists := tcm.peerStatsCache.Get(peerID)
	if !exists || stats.DiscoveryAttempts == 0 {
		return 0.1 // Default neutral score for new peers
	}

	successRate := float64(stats.SuccessfulDiscoveries) / float64(stats.DiscoveryAttempts)

	// Consider recency of success
	timeSinceLastSuccess := time.Since(stats.LastSuccessfulDiscovery)
	recencyFactor := math.Exp(-timeSinceLastSuccess.Hours() / 24.0) // Decay over days

	return successRate * recencyFactor * 0.3
}

// Evaluates connection stability and quality
func (tcm *TopicAwareConnectionManager) scoreConnectionQuality(peerID peer.ID) float64 {
	stats, exists := tcm.peerStatsCache.Get(peerID)
	if !exists {
		return 0.1 // Default for new peers
	}

	return stats.ConnectionQuality * 0.2
}

// Connection pruning with topic peer protection
// POLICY: Topic peers are NEVER disconnected regardless of connection limits
// Priority order: 1) Disconnect other peers, 2) Limit routing peers, 3) Warn if over limit
func (tcm *TopicAwareConnectionManager) TrimOpenConns(ctx context.Context) {
	tcm.mu.Lock()
	defer tcm.mu.Unlock()

	allConns := tcm.host.Network().Conns()

	// Categorize current connections
	var topicPeers []network.Conn
	var routingPeers []network.Conn
	var otherPeers []network.Conn

	for _, conn := range allConns {
		peerID := conn.RemotePeer()

		// Use existing scores - don't re-evaluate during trimming
		isTopicPeer := tcm.topicPeers[peerID]
		routingScore, hasRoutingScore := tcm.routingPeers[peerID]

		if isTopicPeer {
			topicPeers = append(topicPeers, conn)
		} else if hasRoutingScore && routingScore > tcm.routingScoreThreshold {
			routingPeers = append(routingPeers, conn)
		} else {
			otherPeers = append(otherPeers, conn)
		}
	}

	// Calculate target numbers
	// Note: Topic peers have no limits - they are always protected
	// Cap routing peers at 50 maximum, regardless of total connection limit
	maxRoutingPeers := int(math.Min(float64(tcm.maxConnections)*tcm.routingPeerRatio, 50))

	// Check if trimming is needed
	overTotalLimit := len(allConns) > tcm.maxConnections
	hasOtherPeers := len(otherPeers) > 0
	overRoutingLimit := len(routingPeers) > maxRoutingPeers

	if !overTotalLimit && !hasOtherPeers && !overRoutingLimit {
		return // No pruning needed
	}

	tcm.lm.Log("debug", fmt.Sprintf("Trimming needed: total=%d/%d, routing=%d/%d, other=%d",
		len(allConns), tcm.maxConnections, len(routingPeers), maxRoutingPeers, len(otherPeers)), "p2p")

	// Always prune other peers first (lowest priority)
	if len(otherPeers) > 0 {
		tcm.lm.Log("debug", fmt.Sprintf("Pruning %d other peers (lowest priority)", len(otherPeers)), "p2p")
		for _, conn := range otherPeers {
			conn.Close()
		}
	}

	// Always enforce routing peer limits (keep most useful ones)
	if len(routingPeers) > maxRoutingPeers {
		excess := len(routingPeers) - maxRoutingPeers
		tcm.lm.Log("debug", fmt.Sprintf("Pruning %d excess routing peers (%d > %d)", excess, len(routingPeers), maxRoutingPeers), "p2p")
		tcm.pruneRoutingPeers(routingPeers, excess)
	}

	// IMPORTANT: Topic peers are NEVER disconnected regardless of connection limits
	remainingConns := len(topicPeers) + maxRoutingPeers // otherPeers = 0, routingPeers = maxRoutingPeers
	if remainingConns > tcm.maxConnections {
		// Still over limit but NEVER disconnect topic peers - only issue warning
		excessConnections := remainingConns - tcm.maxConnections
		tcm.lm.Log("warning", fmt.Sprintf(
			"Connection count exceeded limit: %d connections (%d topic + %d routing) > %d limit. "+
			"Topic peers are protected and will NOT be disconnected. "+
			"Consider increasing maxConnections or reducing routing peer ratio.",
			remainingConns, len(topicPeers), maxRoutingPeers, tcm.maxConnections), "p2p")
		
		// Log detailed breakdown for monitoring
		tcm.lm.Log("warning", fmt.Sprintf(
			"Protected topic peers: %d (cannot be disconnected), "+
			"Routing peers: %d (capped at %d), "+
			"Other peers: %d (already pruned), "+
			"Excess over limit: %d",
			len(topicPeers), len(routingPeers), maxRoutingPeers, len(otherPeers), excessConnections), "p2p")
	}
}

// Remove least useful routing peers
func (tcm *TopicAwareConnectionManager) pruneRoutingPeers(routingConns []network.Conn, toPrune int) {
	// Sort by routing score (ascending - worst first)
	sort.Slice(routingConns, func(i, j int) bool {
		scoreI := tcm.routingPeers[routingConns[i].RemotePeer()]
		scoreJ := tcm.routingPeers[routingConns[j].RemotePeer()]
		return scoreI < scoreJ
	})

	// Prune the worst ones
	for i := 0; i < toPrune && i < len(routingConns); i++ {
		routingConns[i].Close()
	}
}

// Re-evaluate only stale connected peers
func (tcm *TopicAwareConnectionManager) reevaluateStalePeers() {
	tcm.mu.RLock()
	stalePeers := make([]peer.ID, 0)
	now := time.Now()

	// Find peers with stale evaluation data
	for peerID, lastEval := range tcm.lastEvaluation {
		if now.Sub(lastEval) > tcm.evaluationPeriod { // 5 minutes
			if tcm.host.Network().Connectedness(peerID) == network.Connected {
				stalePeers = append(stalePeers, peerID)
			}
		}
	}

	// Also check peers that have never been evaluated
	for _, conn := range tcm.host.Network().Conns() {
		peerID := conn.RemotePeer()
		if _, hasEval := tcm.lastEvaluation[peerID]; !hasEval {
			stalePeers = append(stalePeers, peerID)
		}
	}

	tcm.mu.RUnlock()

	// Rate limiting - only evaluate up to maxEvaluationsPerCycle peers
	if len(stalePeers) > tcm.maxEvaluationsPerCycle {
		stalePeers = stalePeers[:tcm.maxEvaluationsPerCycle]
	}

	// Schedule stale peers for evaluation and track pending count
	atomic.StoreInt32(&tcm.pendingEvaluations, int32(len(stalePeers)))
	for _, peerID := range stalePeers {
		tcm.schedulePeerEvaluation(peerID)
	}

	tcm.lm.Log("debug", fmt.Sprintf("Scheduled %d stale peers for evaluation", len(stalePeers)), "p2p")
}

// Wait for all pending evaluations to complete before trimming
func (tcm *TopicAwareConnectionManager) waitForEvaluationComplete() {
	pending := atomic.LoadInt32(&tcm.pendingEvaluations)
	if pending == 0 {
		return // No evaluations pending
	}

	tcm.lm.Log("debug", fmt.Sprintf("Waiting for %d pending evaluations to complete", pending), "p2p")

	// Wait for completion signal with timeout
	select {
	case <-tcm.evaluationComplete:
		tcm.lm.Log("debug", "All peer evaluations completed, proceeding with trimming", "p2p")
	case <-time.After(time.Minute * 2):
		tcm.lm.Log("warning", "Timeout waiting for peer evaluations to complete, proceeding with trimming anyway", "p2p")
	case <-tcm.ctx.Done():
		tcm.lm.Log("debug", "Context cancelled while waiting for evaluations", "p2p")
		return
	}
}

// Handle new peer connections
func (tcm *TopicAwareConnectionManager) OnPeerConnected(peerID peer.ID) {
	// Initialize peer stats
	if _, exists := tcm.peerStatsCache.Get(peerID); !exists {
		stats := &PeerStats{
			ConnectionQuality: 0.5, // Start with neutral quality
		}
		tcm.peerStatsCache.Set(peerID, stats)
	}

	// Add to active tracking with bounds checking
	tcm.mu.Lock()
	// Check if we need to prevent unbounded growth
	if len(tcm.topicPeers) < tcm.maxPeerTracking {
		// We have room, initialize if needed
		if _, exists := tcm.topicPeers[peerID]; !exists {
			tcm.topicPeers[peerID] = false // Will be updated by topic discovery
		}
	}

	if len(tcm.routingPeers) < tcm.maxPeerTracking {
		// Initialize with default routing score
		if _, exists := tcm.routingPeers[peerID]; !exists {
			tcm.routingPeers[peerID] = 0.0 // Will be updated by evaluation
		}
	}
	tcm.mu.Unlock()

	// IMPORTANT: Check topic status FIRST before scheduling evaluation
	// This ensures proper priority routing from the start
	tcm.mu.RLock()
	_, hasRecentCheck := tcm.lastTopicCheck[peerID]
	tcm.mu.RUnlock()

	if !hasRecentCheck {
		// New connection - check topic status with delay, then schedule evaluation
		gt := tcm.p2pm.GetGoroutineTracker()
		gt.SafeStart(fmt.Sprintf("tcm-topic-check-%s", peerID.String()[:8]), func() {
			ctx, cancel := context.WithTimeout(tcm.ctx, 5*time.Second)
			defer cancel()

			select {
			case <-time.After(2 * time.Second):
				// Check if peer is still connected before doing expensive check
				if tcm.host.Network().Connectedness(peerID) == network.Connected {
					// First update topic status
					tcm.checkPeerTopicSubscriptionCached(peerID)
					// Then schedule evaluation with correct priority
					tcm.schedulePeerEvaluation(peerID)
				}
			case <-ctx.Done():
				// Context cancelled, exit goroutine
				return
			}
		})
	} else {
		// Has cache - immediate check then schedule
		tcm.checkPeerTopicSubscriptionCached(peerID)
		tcm.schedulePeerEvaluation(peerID)
	}
}

// Handle peer disconnections
func (tcm *TopicAwareConnectionManager) OnPeerDisconnected(peerID peer.ID) {
	tcm.mu.Lock()
	defer tcm.mu.Unlock()

	// Check if this was a topic or routing peer before removal
	wasTopicPeer := tcm.topicPeers[peerID]
	wasRoutingPeer := tcm.routingPeers[peerID] > tcm.routingScoreThreshold

	// Remove from active tracking
	delete(tcm.topicPeers, peerID)
	delete(tcm.routingPeers, peerID)
	delete(tcm.lastEvaluation, peerID)
	delete(tcm.lastTopicCheck, peerID)

	switch tcm.uiType {
	case "CLI":
		// Do nothing
	case "GUI":
		// Emit disconnection events
		if wasTopicPeer {
			runtime.EventsEmit(tcm.ctx, "topicpeerdisconnectedlog-event", peerID.String())
		}
		if wasRoutingPeer {
			runtime.EventsEmit(tcm.ctx, "routingpeerdisconnectedlog-event", peerID.String())
		}
	default:
		// Do nothing
	}
}

// Update historical performance data
func (tcm *TopicAwareConnectionManager) UpdatePeerStats(peerID peer.ID, discoverySuccess bool, connectionQuality float64) {
	stats, exists := tcm.peerStatsCache.Get(peerID)
	if !exists {
		stats = &PeerStats{}
	}

	stats.DiscoveryAttempts++
	stats.LastDiscoveryAttempt = time.Now()
	stats.ConnectionQuality = connectionQuality

	if discoverySuccess {
		stats.SuccessfulDiscoveries++
		stats.LastSuccessfulDiscovery = time.Now()
	}

	tcm.peerStatsCache.Set(peerID, stats)
}

// Additional utility methods for debugging and monitoring

// Return the current list of peers subscribed to our topics
func (tcm *TopicAwareConnectionManager) GetTopicPeers() []peer.ID {
	tcm.mu.RLock()
	defer tcm.mu.RUnlock()

	peers := make([]peer.ID, 0, len(tcm.topicPeers))
	for peerID := range tcm.topicPeers {
		peers = append(peers, peerID)
	}
	return peers
}

// Return the current list of useful routing peers
func (tcm *TopicAwareConnectionManager) GetRoutingPeers() map[peer.ID]float64 {
	tcm.mu.RLock()
	defer tcm.mu.RUnlock()

	result := make(map[peer.ID]float64)
	for peerID, score := range tcm.routingPeers {
		if score > tcm.routingScoreThreshold {
			result[peerID] = score
		}
	}
	return result
}

// Return statistics about current connections
func (tcm *TopicAwareConnectionManager) GetConnectionStats() map[string]int {
	tcm.mu.RLock()
	defer tcm.mu.RUnlock()

	stats := make(map[string]int)
	allConns := tcm.host.Network().Conns()

	stats["total"] = len(allConns)

	connectedTopicPeers := 0
	for peerID := range tcm.topicPeers {
		if tcm.host.Network().Connectedness(peerID) == network.Connected {
			connectedTopicPeers++
		}
	}
	stats["topic_peers"] = connectedTopicPeers

	connectedRoutingPeers := 0
	for peerID := range tcm.routingPeers {
		if tcm.host.Network().Connectedness(peerID) == network.Connected {
			connectedRoutingPeers++
		}
	}
	stats["routing_peers"] = connectedRoutingPeers

	return stats
}

func (tcm *TopicAwareConnectionManager) Shutdown() {
	// Check if already shutdown to prevent panic on double close
	if !atomic.CompareAndSwapInt32(&tcm.isShutdown, 0, 1) {
		// Already shutdown, return early
		return
	}

	// Signal shutdown
	close(tcm.stopChan)

	// Stop tickers
	if tcm.evaluationTicker != nil {
		tcm.evaluationTicker.Stop()
		tcm.evaluationTicker = nil
	}
	if tcm.cleanupTicker != nil {
		tcm.cleanupTicker.Stop()
		tcm.cleanupTicker = nil
	}

	// Clear all tracking data
	tcm.mu.Lock()
	tcm.topicPeers = make(map[peer.ID]bool)
	tcm.routingPeers = make(map[peer.ID]float64)
	tcm.lastEvaluation = make(map[peer.ID]time.Time)
	tcm.lastTopicCheck = make(map[peer.ID]time.Time)
	tcm.mu.Unlock()

	// Stop worker pool with timeout protection
	close(tcm.topicPeerQueue)
	close(tcm.regularQueue)
	
	// Wait for workers with timeout to prevent hanging
	workersDone := make(chan struct{})
	go func() {
		tcm.workerPool.Wait()
		close(workersDone)
	}()
	
	select {
	case <-workersDone:
		// All workers stopped gracefully
	case <-time.After(5 * time.Second):
		// Timeout - workers might be stuck, but continue shutdown
		tcm.lm.Log("warning", "Worker pool shutdown timeout - some evaluation workers may still be running", "p2p")
	}

	// Note: LRU cache will be garbage collected when tcm is destroyed
}

// Start worker pool for peer evaluation
func (tcm *TopicAwareConnectionManager) startWorkerPool(numWorkers int) {
	tracker := tcm.p2pm.GetGoroutineTracker()
	for i := range numWorkers {
		tcm.workerPool.Add(1)
		workerID := i // Capture loop variable
		if !tracker.SafeStart(fmt.Sprintf("peer-evaluation-worker-%d", workerID), func() {
			tcm.peerEvaluationWorker(workerID)
		}) {
			tcm.lm.Log("error", fmt.Sprintf("Failed to start peer evaluation worker %d due to goroutine limit", workerID), "connection-manager")
			tcm.workerPool.Done() // Decrement since SafeStart failed
		}
	}
}

// Worker goroutine that processes peer evaluation requests with priority
func (tcm *TopicAwareConnectionManager) peerEvaluationWorker(workerID int) {
	defer tcm.workerPool.Done()

	for {
		select {
		// Priority 1: Topic peers (never drop these)
		case peerID, ok := <-tcm.topicPeerQueue:
			if !ok {
				// Channel closed, worker should exit
				tcm.lm.Log("debug", fmt.Sprintf("Peer evaluation worker %d shutting down (topic queue)", workerID), "p2p")
				return
			}
			tcm.processPeerEvaluation(peerID, "topic-peer")

		// Priority 2: Regular peers (can be dropped if queue full)
		case peerID, ok := <-tcm.regularQueue:
			if !ok {
				// Channel closed, worker should exit
				tcm.lm.Log("debug", fmt.Sprintf("Peer evaluation worker %d shutting down (regular queue)", workerID), "p2p")
				return
			}
			tcm.processPeerEvaluation(peerID, "regular-peer")

		case <-tcm.ctx.Done():
			tcm.lm.Log("debug", fmt.Sprintf("Peer evaluation worker %d context cancelled", workerID), "p2p")
			return
		}
	}
}

// Process individual peer evaluation
func (tcm *TopicAwareConnectionManager) processPeerEvaluation(peerID peer.ID, peerType string) {
	// Create a bounded context for this evaluation
	ctx, cancel := context.WithTimeout(tcm.ctx, time.Minute*2)
	defer cancel()

	// Wait for peer to potentially subscribe to topics (shorter wait for topic peers)
	waitTime := time.Second * 5
	if peerType == "topic-peer" {
		waitTime = time.Second * 2 // Faster evaluation for known topic peers
	}

	select {
	case <-time.After(waitTime):
		// Continue with evaluation
	case <-ctx.Done():
		return // Context cancelled, skip this evaluation
	}

	// Perform the actual evaluation
	tcm.evaluateRoutingUsefulness(peerID)

	// Decrement pending counter and signal if all done
	remaining := atomic.AddInt32(&tcm.pendingEvaluations, -1)
	if remaining == 0 {
		// All evaluations complete - signal trimming can proceed
		select {
		case tcm.evaluationComplete <- struct{}{}:
		default: // Channel already has signal
		}
	}
}

// Schedule peer evaluation with priority routing
func (tcm *TopicAwareConnectionManager) schedulePeerEvaluation(peerID peer.ID) {
	// Check if this is a topic peer by looking at current subscriptions
	isTopicPeer := tcm.isKnownTopicPeer(peerID)
	
	if isTopicPeer {
		// Topic peers always get evaluated - use large buffered channel
		select {
		case tcm.topicPeerQueue <- peerID:
			tcm.lm.Log("debug", "Queued topic peer "+peerID.String()+" for priority evaluation", "p2p")
		default:
			// Even if topic queue is full (very unlikely), force evaluation immediately
			tcm.lm.Log("warning", "Topic peer queue full, evaluating immediately: "+peerID.String(), "p2p")
			go tcm.processPeerEvaluation(peerID, "topic-peer-immediate")
		}
	} else {
		// Regular peers use bounded queue and can be dropped
		select {
		case tcm.regularQueue <- peerID:
			tcm.lm.Log("debug", "Queued regular peer "+peerID.String()+" for evaluation", "p2p")
		default:
			// Queue is full - log warning and drop request for regular peers
			queueLen := len(tcm.regularQueue)
			tcm.lm.Log("warning", fmt.Sprintf("Regular peer evaluation queue full (%d/%d), dropping evaluation for %s",
				queueLen, cap(tcm.regularQueue), peerID.String()), "p2p")
		}
	}
}

// Check if peer is known to be subscribed to our topics
func (tcm *TopicAwareConnectionManager) isKnownTopicPeer(peerID peer.ID) bool {
	tcm.mu.RLock()
	defer tcm.mu.RUnlock()
	
	// First check our cached topic peers
	if isTopicPeer, exists := tcm.topicPeers[peerID]; exists && isTopicPeer {
		return true
	}
	
	// Also check current pubsub subscriptions directly for real-time accuracy
	for _, topicName := range tcm.targetTopics {
		peers := tcm.pubsub.ListPeers(topicName)
		if slices.Contains(peers, peerID) {
			return true
		}
	}
	
	return false
}

// Cache-first topic subscription check - eliminates goroutine overhead
func (tcm *TopicAwareConnectionManager) checkPeerTopicSubscriptionCached(peerID peer.ID) {
	tcm.mu.Lock()
	defer tcm.mu.Unlock()

	// Check if peer is still connected
	if tcm.host.Network().Connectedness(peerID) != network.Connected {
		return
	}

	now := time.Now()
	topicCheckStaleThreshold := time.Hour * 1 // Re-check after 1 hour (topic subscriptions rarely change)

	// Check if we have recent cache data
	lastCheck, hasRecentCheck := tcm.lastTopicCheck[peerID]
	wasTopicPeer := tcm.topicPeers[peerID]

	// If we have recent data (< 10 minutes), trust the cache but still emit connection events
	if hasRecentCheck && now.Sub(lastCheck) < topicCheckStaleThreshold {
		tcm.lm.Log("debug", fmt.Sprintf("Peer %s topic check: using cached result (topic_peer=%v)", peerID.String()[:12], wasTopicPeer), "p2p")

		// Emit connection event for topic peers (only if actually connected and not already emitted)
		if wasTopicPeer && tcm.host.Network().Connectedness(peerID) == network.Connected {
			switch tcm.uiType {
			case "CLI":
				// Do nothing
			case "GUI":
				// Only emit if this is a newly discovered connection (prevent duplicates)
				// Note: wasTopicPeer means we had this in cache, so event was likely already emitted
				// Skip emission for cached results to prevent GUI duplicates
			default:
				// Do nothing
			}
		}
		return
	}

	// Cache is stale or missing - do expensive pubsub query
	tcm.lm.Log("debug", fmt.Sprintf("Peer %s topic check: cache stale, doing pubsub query", peerID.String()[:12]), "p2p")

	// Check if peer is now subscribed to any target topics
	isTopicPeer := false
	for _, topicName := range tcm.targetTopics {
		peers := tcm.pubsub.ListPeers(topicName)
		if slices.Contains(peers, peerID) {
			isTopicPeer = true
			break
		}
	}

	// Update cache timestamp
	tcm.lastTopicCheck[peerID] = now

	// Update topic peer status and emit appropriate events
	if isTopicPeer && !wasTopicPeer {
		// Newly subscribed to topics
		tcm.topicPeers[peerID] = true

		// Only emit connection event if peer is actually connected AND this is truly a new discovery
		if tcm.host.Network().Connectedness(peerID) == network.Connected {
			switch tcm.uiType {
			case "CLI":
				// Do nothing
			case "GUI":
				// This is the RIGHT place to emit - truly newly discovered topic peer
				runtime.EventsEmit(tcm.ctx, "topicpeerconnectedlog-event", peerID.String())
			default:
				// Do nothing
			}
		}

		tcm.lm.Log("debug", "Cache-based topic peer detection: "+peerID.String(), "p2p")
	} else if !isTopicPeer && wasTopicPeer {
		// Peer no longer subscribes to topics - emit disconnection event
		tcm.topicPeers[peerID] = false

		switch tcm.uiType {
		case "CLI":
			// Do nothing
		case "GUI":
			runtime.EventsEmit(tcm.ctx, "topicpeerdisconnectedlog-event", peerID.String())
		default:
			// Do nothing
		}

		tcm.lm.Log("debug", "Peer unsubscribed from topics: "+peerID.String(), "p2p")
	} else {
		// Update cache entry even if status unchanged
		tcm.topicPeers[peerID] = isTopicPeer
	}
}

// Get queue statistics for monitoring
func (tcm *TopicAwareConnectionManager) GetQueueStats() map[string]int {
	topicQueueLen := len(tcm.topicPeerQueue)
	topicQueueCap := cap(tcm.topicPeerQueue)
	regularQueueLen := len(tcm.regularQueue)
	regularQueueCap := cap(tcm.regularQueue)
	
	topicUtilization := 0
	if topicQueueCap > 0 {
		topicUtilization = (topicQueueLen * 100) / topicQueueCap
	}
	
	regularUtilization := 0
	if regularQueueCap > 0 {
		regularUtilization = (regularQueueLen * 100) / regularQueueCap
	}

	return map[string]int{
		"topic_queue_length":              topicQueueLen,
		"topic_queue_capacity":            topicQueueCap,
		"topic_queue_utilization_percent": topicUtilization,
		"regular_queue_length":            regularQueueLen,
		"regular_queue_capacity":          regularQueueCap,
		"regular_queue_utilization_percent": regularUtilization,
		"total_pending":                   topicQueueLen + regularQueueLen,
	}
}
