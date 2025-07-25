package node

import (
	"context"
	"fmt"
	"maps"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/adgsm/trustflow-node/internal/utils"
	"github.com/ipfs/go-cid"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	mh "github.com/multiformats/go-multihash"
)

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

	// Connection priorities
	topicPeers     map[peer.ID]bool       // Peers subscribed to our topics
	routingPeers   map[peer.ID]float64    // Peers useful for routing (with score)
	lastEvaluation map[peer.ID]time.Time  // Last time we evaluated peer usefulness
	peerStats      map[peer.ID]*PeerStats // Historical statistics

	mu sync.RWMutex

	// Configuration
	maxConnections        int
	topicPeerRatio        float64 // % of connections reserved for topic peers
	routingPeerRatio      float64 // % for routing peers
	evaluationPeriod      time.Duration
	routingScoreThreshold float64

	evaluationSemaphore chan struct{}
	evaluationTicker    *time.Ticker
	stopChan            chan struct{}
}

func NewTopicAwareConnectionManager(p2pm *P2PManager, maxConnections int,
	targetTopics []string) (*TopicAwareConnectionManager, error) {

	tcm := &TopicAwareConnectionManager{
		ctx:                   p2pm.ctx,
		host:                  p2pm.h,
		pubsub:                p2pm.ps,
		dht:                   p2pm.idht,
		lm:                    p2pm.Lm,
		targetTopics:          targetTopics,
		topicPeers:            make(map[peer.ID]bool),
		routingPeers:          make(map[peer.ID]float64),
		lastEvaluation:        make(map[peer.ID]time.Time),
		peerStats:             make(map[peer.ID]*PeerStats),
		maxConnections:        maxConnections,
		topicPeerRatio:        0.6, // 60% for topic peers
		routingPeerRatio:      0.3, // 30% for routing peers
		evaluationPeriod:      time.Minute * 5,
		routingScoreThreshold: 0.1,
		evaluationSemaphore:   make(chan struct{}, 3),
		stopChan:              make(chan struct{}),
	}

	// Start periodic evaluation
	tcm.startPeriodicEvaluation(3 * time.Minute)

	return tcm, nil
}

// Ticker-based periodic peer evaluation
func (tcm *TopicAwareConnectionManager) startPeriodicEvaluation(interval time.Duration) {
	tcm.evaluationTicker = time.NewTicker(interval)

	go func() {
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
	}()
}

// Stop periodic peer evaluation
func (tcm *TopicAwareConnectionManager) StartPeriodicEvaluation() {
	close(tcm.stopChan)
}

// Periodic evaluation of peers
func (tcm *TopicAwareConnectionManager) peersEvaluation() {
	tcm.UpdateTopicPeersList(tcm.targetTopics)
	tcm.reevaluateAllPeers()
	// Use previously completed peers evaluation
	connStats := tcm.GetConnectionStats()
	msg := fmt.Sprintf("Connection stats:\nTotal connections: %d\nTopic peers connected: %d\nRouting peers connected: %d\nOther peers connected: %d\n",
		connStats["total"], connStats["topic_peers"], connStats["routing_peers"], connStats["other_peers"])
	tcm.lm.Log("debug", msg, "libp2p-events")
}

// Refresh the list of peers subscribed to our topics
func (tcm *TopicAwareConnectionManager) UpdateTopicPeersList(targetTopics []string) {
	tcm.mu.Lock()
	defer tcm.mu.Unlock()

	// Clear current topic peers
	newTopicPeers := make(map[peer.ID]bool)

	// Get current topic subscribers using pubsub.ListPeers directly
	for _, topicName := range targetTopics {
		peers := tcm.pubsub.ListPeers(topicName)
		for _, peerID := range peers {
			newTopicPeers[peerID] = true
		}
	}

	tcm.topicPeers = newTopicPeers
}

// Determines if a peer is topic-relevant or routing-useful
func (tcm *TopicAwareConnectionManager) classifyPeer(peerID peer.ID) (isTopicPeer bool, routingScore float64) {
	tcm.mu.RLock()

	// Check if peer is subscribed to our topics
	if tcm.topicPeers[peerID] {
		tcm.mu.RUnlock()
		return true, 1.0 // Topic peers get max routing score too
	}

	// Check if we have a recent cached score
	score, hasScore := tcm.routingPeers[peerID]
	lastEval, hasEval := tcm.lastEvaluation[peerID]
	needsEvaluation := !hasEval || time.Since(lastEval) > tcm.evaluationPeriod

	tcm.mu.RUnlock()

	if needsEvaluation {
		// Evaluate in the background to avoid blocking
		go func() {
			select {
			case tcm.evaluationSemaphore <- struct{}{}:
				defer func() { <-tcm.evaluationSemaphore }()
				tcm.evaluateRoutingUsefulness(peerID)
			default:
				// Semaphore full, skip evaluation for now
			}
		}()

		// Return cached score or default
		if hasScore {
			return false, score
		}
		return false, 0.0
	}

	return false, score
}

// Calculates comprehensive routing score
func (tcm *TopicAwareConnectionManager) evaluateRoutingUsefulness(peerID peer.ID) {
	defer func() {
		tcm.lastEvaluation[peerID] = time.Now()
	}()

	score := 0.0

	// Method 1: Check peer's potential connections to topic-subscribed peers
	score += tcm.scoreByTopicPeerConnections(peerID)

	// Method 2: DHT routing table position
	score += tcm.scoreDHTPosition(peerID)

	// Method 3: Historical success in reaching topic peers
	score += tcm.scoreHistoricalSuccess(peerID)

	// Method 4: Connection quality and stability
	score += tcm.scoreConnectionQuality(peerID)

	tcm.routingPeers[peerID] = math.Min(score, 1.0)
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
	stats, exists := tcm.peerStats[peerID]
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
	stats, exists := tcm.peerStats[peerID]
	if !exists {
		return 0.1 // Default for new peers
	}

	return stats.ConnectionQuality * 0.2
}

// Connection pruning
func (tcm *TopicAwareConnectionManager) TrimOpenConns(ctx context.Context) {
	tcm.mu.Lock()
	defer tcm.mu.Unlock()

	allConns := tcm.host.Network().Conns()
	if len(allConns) <= tcm.maxConnections {
		return // No pruning needed
	}

	// Categorize current connections
	var topicPeers []network.Conn
	var routingPeers []network.Conn
	var otherPeers []network.Conn

	for _, conn := range allConns {
		peerID := conn.RemotePeer()
		isTopicPeer, routingScore := tcm.classifyPeer(peerID)

		if isTopicPeer {
			topicPeers = append(topicPeers, conn)
		} else if routingScore > tcm.routingScoreThreshold {
			routingPeers = append(routingPeers, conn)
		} else {
			otherPeers = append(otherPeers, conn)
		}
	}

	// Calculate target numbers
	maxTopicPeers := int(float64(tcm.maxConnections) * tcm.topicPeerRatio)
	maxRoutingPeers := int(float64(tcm.maxConnections) * tcm.routingPeerRatio)

	// Prune in order: others first, then least useful routing peers, then excess topic peers
	toPrune := len(allConns) - tcm.maxConnections

	// Prune other peers first
	if len(otherPeers) > 0 && toPrune > 0 {
		pruneCount := int(math.Min(float64(len(otherPeers)), float64(toPrune)))
		for i := range pruneCount {
			otherPeers[i].Close()
		}
		toPrune -= pruneCount
	}

	// Prune excess routing peers (keep most useful ones)
	if toPrune > 0 && len(routingPeers) > maxRoutingPeers {
		excess := int(math.Min(float64(toPrune), float64(len(routingPeers)-maxRoutingPeers)))
		tcm.pruneRoutingPeers(routingPeers, excess)
		toPrune -= excess
	}

	// If we still need to prune and have too many topic peers, prune excess topic peers
	// (This should rarely happen since topic peers have highest priority)
	if toPrune > 0 && len(topicPeers) > maxTopicPeers {
		excess := int(math.Min(float64(toPrune), float64(len(topicPeers)-maxTopicPeers)))
		// For topic peers, just prune the excess without sophisticated scoring
		// since they're all equally valuable as topic peers
		for i := range excess {
			topicPeers[i].Close()
		}
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

// Periodically re-evaluate all connected peers
func (tcm *TopicAwareConnectionManager) reevaluateAllPeers() {
	tcm.mu.Lock()
	connectedPeers := make([]peer.ID, 0)
	for _, conn := range tcm.host.Network().Conns() {
		connectedPeers = append(connectedPeers, conn.RemotePeer())
	}
	tcm.mu.Unlock()

	// Process peers with controlled concurrency
	for _, peerID := range connectedPeers {
		go func(id peer.ID) {
			select {
			case tcm.evaluationSemaphore <- struct{}{}:
				defer func() { <-tcm.evaluationSemaphore }()
				tcm.evaluateRoutingUsefulness(id)
			default:
				// Semaphore full, skip this peer for now
			}
		}(peerID)
	}
}

// Handle new peer connections
func (tcm *TopicAwareConnectionManager) OnPeerConnected(peerID peer.ID) {
	// Initialize peer stats
	tcm.mu.Lock()
	if _, exists := tcm.peerStats[peerID]; !exists {
		tcm.peerStats[peerID] = &PeerStats{
			ConnectionQuality: 0.5, // Start with neutral quality
		}
	}
	tcm.mu.Unlock()

	// Evaluate peer after allowing time for topic subscription
	go func() {
		time.Sleep(time.Second * 5)

		// Use semaphore with timeout to prevent blocking
		select {
		case tcm.evaluationSemaphore <- struct{}{}:
			defer func() { <-tcm.evaluationSemaphore }()
			tcm.evaluateRoutingUsefulness(peerID)
		case <-time.After(time.Second * 10):
			// Timeout - skip evaluation to prevent blocking
			tcm.lm.Log("debug", "Peer evaluation timeout for "+peerID.String(), "p2p")
		}
	}()
}

// Handle peer disconnections
func (tcm *TopicAwareConnectionManager) OnPeerDisconnected(peerID peer.ID) {
	tcm.mu.Lock()
	defer tcm.mu.Unlock()

	// Remove from active tracking but keep historical stats
	delete(tcm.topicPeers, peerID)
	delete(tcm.routingPeers, peerID)
	delete(tcm.lastEvaluation, peerID)
}

// Update historical performance data
func (tcm *TopicAwareConnectionManager) UpdatePeerStats(peerID peer.ID, discoverySuccess bool, connectionQuality float64) {
	tcm.mu.Lock()
	defer tcm.mu.Unlock()

	stats, exists := tcm.peerStats[peerID]
	if !exists {
		stats = &PeerStats{}
		tcm.peerStats[peerID] = stats
	}

	stats.DiscoveryAttempts++
	stats.LastDiscoveryAttempt = time.Now()
	stats.ConnectionQuality = connectionQuality

	if discoverySuccess {
		stats.SuccessfulDiscoveries++
		stats.LastSuccessfulDiscovery = time.Now()
	}
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

	stats["other_peers"] = stats["total"] - (stats["topic_peers"] + stats["routing_peers"])

	return stats
}
