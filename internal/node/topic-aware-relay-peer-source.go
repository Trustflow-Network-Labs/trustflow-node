package node

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p/core/peer"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	mh "github.com/multiformats/go-multihash"
	"github.com/Trustflow-Network-Labs/trustflow-node/internal/utils"
)

// RelayServiceInfo contains pricing and capability information for a relay service
type RelayServiceInfo struct {
	PeerID        peer.ID   `json:"peer_id"`
	Topics        []string  `json:"topics"`          // Topics this relay serves
	PricePerGB    float64   `json:"price_per_gb"`    // Price in units per GB
	Currency      string    `json:"currency"`        // Pricing currency (e.g., "USD", "TOKEN")
	MaxBandwidth  int64     `json:"max_bandwidth"`   // Max bytes/second
	MaxDuration   int64     `json:"max_duration"`    // Max seconds per circuit
	MaxData       int64     `json:"max_data"`        // Max bytes per circuit
	Availability  float64   `json:"availability"`    // Uptime percentage (0-100)
	Latency       int       `json:"latency_ms"`      // Average latency in ms
	Reputation    float64   `json:"reputation"`      // User rating (0-5)
	LastSeen      time.Time `json:"last_seen"`
	ContactInfo   string    `json:"contact_info"`    // Optional contact information
}

// TopicAwareRelayPeerSource discovers relay peers for specific topics through DHT
type TopicAwareRelayPeerSource struct {
	dht           *dht.IpfsDHT
	tcm           *TopicAwareConnectionManager
	lm            *utils.LogsManager
	topics        []string
	
	// Caching
	mu            sync.RWMutex
	cachedRelays  map[string][]RelayServiceInfo // topic -> relays
	lastUpdate    map[string]time.Time          // topic -> last update time
	cacheTimeout  time.Duration
	
	// Configuration
	maxRelaysPerTopic int
	minReputation     float64
	maxPricePerGB     float64
	preferredCurrency string
}

// NewTopicAwareRelayPeerSource creates a new topic-aware relay peer source
func NewTopicAwareRelayPeerSource(
	dht *dht.IpfsDHT, 
	tcm *TopicAwareConnectionManager, 
	lm *utils.LogsManager,
	topics []string,
) *TopicAwareRelayPeerSource {
	return &TopicAwareRelayPeerSource{
		dht:               dht,
		tcm:               tcm,
		lm:                lm,
		topics:            topics,
		cachedRelays:      make(map[string][]RelayServiceInfo),
		lastUpdate:        make(map[string]time.Time),
		cacheTimeout:      5 * time.Minute, // Cache for 5 minutes
		maxRelaysPerTopic: 10,
		minReputation:     3.0,
		maxPricePerGB:     1.0, // Max $1 per GB
		preferredCurrency: "USD",
	}
}

// PeerSourceFunc returns a PeerSource function for libp2p autorelay
func (tarps *TopicAwareRelayPeerSource) PeerSourceFunc() func(ctx context.Context, num int) <-chan peer.AddrInfo {
	return tarps.GetPeers
}

// GetPeers implements the libp2p PeerSource function type for relay discovery
func (tarps *TopicAwareRelayPeerSource) GetPeers(ctx context.Context, num int) <-chan peer.AddrInfo {
	peerChan := make(chan peer.AddrInfo, num)
	
	// Check for nil pointers to prevent panics during node restart
	if tarps.tcm == nil || tarps.tcm.p2pm == nil {
		if tarps.lm != nil {
			tarps.lm.Log("warning", "TopicAwareConnectionManager or P2PManager is nil during relay peer discovery", "relay-discovery")
		}
		close(peerChan)
		return peerChan
	}
	
	gt := tarps.tcm.p2pm.GetGoroutineTracker()
	if !gt.SafeStart("relay-peer-discovery", func() {
		defer close(peerChan)
		
		// Discover relays for all topics
		relays := tarps.discoverTopicRelays(ctx)
		
		// Sort by preference (price, reputation, latency)
		sortedRelays := tarps.sortRelaysByPreference(relays)
		
		// Return up to 'num' best relays
		count := 0
		for _, relay := range sortedRelays {
			if count >= num {
				break
			}
			
			// Convert RelayServiceInfo to peer.AddrInfo
			addrInfo, err := tarps.serviceInfoToAddrInfo(ctx, relay)
			if err != nil {
				tarps.lm.Log("warning", 
					fmt.Sprintf("Failed to convert relay info to AddrInfo: %v", err), 
					"relay-discovery")
				continue
			}
			
			select {
			case peerChan <- addrInfo:
				count++
				tarps.lm.Log("info", 
					fmt.Sprintf("Selected relay: peer %s, topics %v, price $%.3f/GB", 
						relay.PeerID.String()[:8], relay.Topics, relay.PricePerGB), 
					"relay-discovery")
			case <-ctx.Done():
				return
			}
		}
		
		tarps.lm.Log("info", 
			fmt.Sprintf("Provided %d relay peers from %d discovered", count, len(sortedRelays)), 
			"relay-discovery")
	}) {
		// If goroutine couldn't start, close channel immediately
		close(peerChan)
		tarps.lm.Log("warn", "Failed to start relay peer discovery goroutine due to limit", "relay-discovery")
	}
	
	return peerChan
}

// discoverTopicRelays discovers relay services for all configured topics
func (tarps *TopicAwareRelayPeerSource) discoverTopicRelays(ctx context.Context) []RelayServiceInfo {
	var allRelays []RelayServiceInfo
	
	for _, topic := range tarps.topics {
		relays := tarps.discoverRelaysForTopic(ctx, topic)
		allRelays = append(allRelays, relays...)
	}
	
	// Deduplicate relays (a relay might serve multiple topics)
	return tarps.deduplicateRelays(allRelays)
}

// discoverRelaysForTopic discovers relay services for a specific topic
func (tarps *TopicAwareRelayPeerSource) discoverRelaysForTopic(ctx context.Context, topic string) []RelayServiceInfo {
	tarps.mu.RLock()
	if cached, exists := tarps.cachedRelays[topic]; exists {
		if time.Since(tarps.lastUpdate[topic]) < tarps.cacheTimeout {
			tarps.mu.RUnlock()
			tarps.lm.Log("debug", 
				fmt.Sprintf("Using cached relays for topic %s: %d relays", topic, len(cached)), 
				"relay-discovery")
			return cached
		}
	}
	tarps.mu.RUnlock()
	
	tarps.lm.Log("info", 
		fmt.Sprintf("Discovering relay services for topic: %s", topic), 
		"relay-discovery")
	
	// Create DHT key for this topic's relay services
	relayKey := tarps.createRelayServiceKey(topic)
	
	// Create proper CID from relay key hash
	hash, err := mh.Sum([]byte(relayKey), mh.SHA2_256, -1)
	if err != nil {
		tarps.lm.Log("error", 
			fmt.Sprintf("Failed to create hash for relay key: %v", err), 
			"relay-discovery")
		return []RelayServiceInfo{}
	}
	relayCID := cid.NewCidV1(cid.Raw, hash)
	
	// Query DHT for relay service providers
	providers, err := tarps.dht.FindProviders(ctx, relayCID)
	if err != nil {
		tarps.lm.Log("error", 
			fmt.Sprintf("DHT FindProviders failed for topic %s: %v", topic, err), 
			"relay-discovery")
		return []RelayServiceInfo{}
	}
	
	var relays []RelayServiceInfo
	for _, provider := range providers {
		// Create default relay service info since detailed info is exchanged during direct communication
		// This follows libp2p best practices and avoids DHT record validation issues
		serviceInfo := RelayServiceInfo{
			PeerID:       provider.ID,
			Topics:       []string{topic},
			PricePerGB:   0.25,   // Default pricing - will be negotiated directly
			Currency:     "USD",
			MaxBandwidth: 10485760, // Default 10MB/s - actual capacity determined during handshake
			MaxDuration:  3600,   // 1 hour default
			MaxData:      1073741824, // 1GB default
			Availability: 95.0,   // Default availability assumption
			Latency:      50,     // Default 50ms assumption
			Reputation:   3.0,    // Neutral default rating
			ContactInfo:  "",     // Contact info exchanged during direct communication
			LastSeen:     time.Now(),
		}
		
		// Validate service meets our criteria
		if tarps.validateRelayService(serviceInfo) {
			relays = append(relays, serviceInfo)
		}
		
		tarps.lm.Log("debug", 
			fmt.Sprintf("Found relay provider %s for topic %s", provider.ID.String()[:8], topic), 
			"relay-discovery")
	}
	
	// Cache the results
	tarps.mu.Lock()
	tarps.cachedRelays[topic] = relays
	tarps.lastUpdate[topic] = time.Now()
	tarps.mu.Unlock()
	
	tarps.lm.Log("info", 
		fmt.Sprintf("Discovered %d relay services for topic %s", len(relays), topic), 
		"relay-discovery")
	
	return relays
}

// createRelayServiceKey creates a DHT key for discovering relay services for a topic
func (tarps *TopicAwareRelayPeerSource) createRelayServiceKey(topic string) string {
	return fmt.Sprintf("trustflow-relay-%s", topic)
}

// Note: Detailed relay service info is now exchanged during direct peer communication
// rather than stored in DHT to avoid record validation issues and follow libp2p best practices

// validateRelayService checks if a relay service meets our requirements
func (tarps *TopicAwareRelayPeerSource) validateRelayService(service RelayServiceInfo) bool {
	// Check reputation
	if service.Reputation < tarps.minReputation {
		return false
	}
	
	// Check price
	if service.Currency == tarps.preferredCurrency && service.PricePerGB > tarps.maxPricePerGB {
		return false
	}
	
	// Check if service is recent
	if time.Since(service.LastSeen) > 24*time.Hour {
		return false
	}
	
	// Check availability
	if service.Availability < 95.0 { // Require 95% uptime
		return false
	}
	
	return true
}

// sortRelaysByPreference sorts relays by preference (price, reputation, latency)
func (tarps *TopicAwareRelayPeerSource) sortRelaysByPreference(relays []RelayServiceInfo) []RelayServiceInfo {
	sort.Slice(relays, func(i, j int) bool {
		a, b := relays[i], relays[j]
		
		// Primary: Lower price (if same currency)
		if a.Currency == b.Currency {
			if a.PricePerGB != b.PricePerGB {
				return a.PricePerGB < b.PricePerGB
			}
		}
		
		// Secondary: Higher reputation
		if a.Reputation != b.Reputation {
			return a.Reputation > b.Reputation
		}
		
		// Tertiary: Lower latency
		if a.Latency != b.Latency {
			return a.Latency < b.Latency
		}
		
		// Quaternary: Higher availability
		return a.Availability > b.Availability
	})
	
	return relays
}

// deduplicateRelays removes duplicate relay services (same peer, keep best info)
func (tarps *TopicAwareRelayPeerSource) deduplicateRelays(relays []RelayServiceInfo) []RelayServiceInfo {
	seen := make(map[peer.ID]RelayServiceInfo)
	
	for _, relay := range relays {
		if existing, exists := seen[relay.PeerID]; exists {
			// Merge topics and keep better service info
			existingTopicsMap := make(map[string]bool)
			for _, topic := range existing.Topics {
				existingTopicsMap[topic] = true
			}
			
			for _, topic := range relay.Topics {
				if !existingTopicsMap[topic] {
					existing.Topics = append(existing.Topics, topic)
				}
			}
			
			// Keep better price/reputation
			if relay.PricePerGB < existing.PricePerGB || relay.Reputation > existing.Reputation {
				relay.Topics = existing.Topics
				seen[relay.PeerID] = relay
			} else {
				seen[relay.PeerID] = existing
			}
		} else {
			seen[relay.PeerID] = relay
		}
	}
	
	var result []RelayServiceInfo
	for _, relay := range seen {
		result = append(result, relay)
	}
	
	return result
}

// serviceInfoToAddrInfo converts RelayServiceInfo to peer.AddrInfo for libp2p
func (tarps *TopicAwareRelayPeerSource) serviceInfoToAddrInfo(ctx context.Context, service RelayServiceInfo) (peer.AddrInfo, error) {
	// Find peer addresses using DHT
	peerInfo, err := tarps.dht.FindPeer(ctx, service.PeerID)
	if err != nil {
		return peer.AddrInfo{}, fmt.Errorf("failed to find peer addresses: %w", err)
	}
	
	return peerInfo, nil
}

// SetConfiguration updates the relay selection configuration
func (tarps *TopicAwareRelayPeerSource) SetConfiguration(maxRelays int, minRep float64, maxPrice float64, currency string) {
	tarps.mu.Lock()
	defer tarps.mu.Unlock()
	
	tarps.maxRelaysPerTopic = maxRelays
	tarps.minReputation = minRep
	tarps.maxPricePerGB = maxPrice
	tarps.preferredCurrency = currency
	
	// Clear cache to force refresh with new criteria
	tarps.cachedRelays = make(map[string][]RelayServiceInfo)
	tarps.lastUpdate = make(map[string]time.Time)
}

// GetCachedRelays returns cached relay information for monitoring/debugging
func (tarps *TopicAwareRelayPeerSource) GetCachedRelays() map[string][]RelayServiceInfo {
	tarps.mu.RLock()
	defer tarps.mu.RUnlock()
	
	result := make(map[string][]RelayServiceInfo)
	for topic, relays := range tarps.cachedRelays {
		result[topic] = make([]RelayServiceInfo, len(relays))
		copy(result[topic], relays)
	}
	
	return result
}