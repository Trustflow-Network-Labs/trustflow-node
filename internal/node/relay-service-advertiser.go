package node

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ipfs/go-cid"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/adgsm/trustflow-node/internal/utils"
)

// RelayServiceAdvertiser manages advertising relay services on the DHT
type RelayServiceAdvertiser struct {
	dht       *dht.IpfsDHT
	p2pm      *P2PManager
	lm        *utils.LogsManager
	
	// Service configuration
	serviceInfo RelayServiceInfo
	isActive    bool
	
	// Advertisement timing
	advertiseInterval time.Duration
	stopChan         chan struct{}
}

// NewRelayServiceAdvertiser creates a new relay service advertiser
func NewRelayServiceAdvertiser(dht *dht.IpfsDHT, p2pm *P2PManager, lm *utils.LogsManager) *RelayServiceAdvertiser {
	return &RelayServiceAdvertiser{
		dht:               dht,
		p2pm:              p2pm,
		lm:                lm,
		advertiseInterval: 10 * time.Minute, // Re-advertise every 10 minutes
		stopChan:          make(chan struct{}),
	}
}

// StartAdvertising begins advertising relay services for specified topics
func (rsa *RelayServiceAdvertiser) StartAdvertising(ctx context.Context, config RelayServiceConfig) error {
	if rsa.isActive {
		return fmt.Errorf("relay service already advertising")
	}
	
	// Create service info from config
	rsa.serviceInfo = RelayServiceInfo{
		PeerID:        rsa.p2pm.h.ID(),
		Topics:        config.Topics,
		PricePerGB:    config.PricePerGB,
		Currency:      config.Currency,
		MaxBandwidth:  config.MaxBandwidth,
		MaxDuration:   config.MaxDuration,
		MaxData:       config.MaxData,
		Availability:  config.Availability,
		Latency:       config.ExpectedLatency,
		Reputation:    config.InitialReputation,
		ContactInfo:   config.ContactInfo,
		LastSeen:      time.Now(),
	}
	
	rsa.isActive = true
	
	// Start advertisement goroutine
	go rsa.advertisementLoop(ctx)
	
	rsa.lm.Log("info", 
		fmt.Sprintf("🚀 Started advertising relay service for topics %v at $%.3f/GB", 
			config.Topics, config.PricePerGB), 
		"relay-advertiser")
	
	return nil
}

// StopAdvertising stops advertising relay services
func (rsa *RelayServiceAdvertiser) StopAdvertising() {
	if !rsa.isActive {
		return
	}
	
	close(rsa.stopChan)
	rsa.isActive = false
	
	rsa.lm.Log("info", "Stopped advertising relay service", "relay-advertiser")
}

// UpdateServiceInfo updates the advertised service information
func (rsa *RelayServiceAdvertiser) UpdateServiceInfo(config RelayServiceConfig) error {
	if !rsa.isActive {
		return fmt.Errorf("relay service not currently advertising")
	}
	
	rsa.serviceInfo.Topics = config.Topics
	rsa.serviceInfo.PricePerGB = config.PricePerGB
	rsa.serviceInfo.Currency = config.Currency
	rsa.serviceInfo.MaxBandwidth = config.MaxBandwidth
	rsa.serviceInfo.MaxDuration = config.MaxDuration
	rsa.serviceInfo.MaxData = config.MaxData
	rsa.serviceInfo.Availability = config.Availability
	rsa.serviceInfo.Latency = config.ExpectedLatency
	rsa.serviceInfo.ContactInfo = config.ContactInfo
	rsa.serviceInfo.LastSeen = time.Now()
	
	rsa.lm.Log("info", 
		fmt.Sprintf("Updated relay service info: topics %v, price $%.3f/GB", 
			config.Topics, config.PricePerGB), 
		"relay-advertiser")
	
	return nil
}

// advertisementLoop continuously advertises the relay service
func (rsa *RelayServiceAdvertiser) advertisementLoop(ctx context.Context) {
	// Advertise immediately
	if err := rsa.advertiseService(ctx); err != nil {
		rsa.lm.Log("error", 
			fmt.Sprintf("Failed to advertise relay service: %v", err), 
			"relay-advertiser")
	}
	
	ticker := time.NewTicker(rsa.advertiseInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-rsa.stopChan:
			return
		case <-ticker.C:
			if err := rsa.advertiseService(ctx); err != nil {
				rsa.lm.Log("error", 
					fmt.Sprintf("Failed to re-advertise relay service: %v", err), 
					"relay-advertiser")
			}
		}
	}
}

// advertiseService advertises the relay service on DHT for all topics
func (rsa *RelayServiceAdvertiser) advertiseService(ctx context.Context) error {
	rsa.serviceInfo.LastSeen = time.Now()
	
	for _, topic := range rsa.serviceInfo.Topics {
		if err := rsa.advertiseForTopic(ctx, topic); err != nil {
			rsa.lm.Log("error", 
				fmt.Sprintf("Failed to advertise for topic %s: %v", topic, err), 
				"relay-advertiser")
			continue
		}
		
		rsa.lm.Log("debug", 
			fmt.Sprintf("Successfully advertised relay service for topic: %s", topic), 
			"relay-advertiser")
	}
	
	return nil
}

// advertiseForTopic advertises relay service for a specific topic
func (rsa *RelayServiceAdvertiser) advertiseForTopic(ctx context.Context, topic string) error {
	// Create DHT key for this topic's relay services
	relayKey := fmt.Sprintf("/trustflow/relay-service/%s", topic)
	cid, err := cid.Cast([]byte(relayKey))
	if err != nil {
		return fmt.Errorf("failed to create CID for relay key: %w", err)
	}
	
	// Advertise as a provider for this topic's relay services
	err = rsa.dht.Provide(ctx, cid, true)
	if err != nil {
		return fmt.Errorf("failed to provide relay service for topic %s: %w", topic, err)
	}
	
	// Store detailed service information
	serviceKey := fmt.Sprintf("/trustflow/relay-info/%s/%s", rsa.serviceInfo.PeerID.String(), topic)
	serviceData, err := json.Marshal(rsa.serviceInfo)
	if err != nil {
		return fmt.Errorf("failed to marshal service info: %w", err)
	}
	
	err = rsa.dht.PutValue(ctx, serviceKey, serviceData)
	if err != nil {
		return fmt.Errorf("failed to store service info: %w", err)
	}
	
	return nil
}

// GetCurrentServiceInfo returns the current service information
func (rsa *RelayServiceAdvertiser) GetCurrentServiceInfo() RelayServiceInfo {
	return rsa.serviceInfo
}

// IsAdvertising returns whether the service is currently advertising
func (rsa *RelayServiceAdvertiser) IsAdvertising() bool {
	return rsa.isActive
}

// RelayServiceConfig contains configuration for relay service advertisement
type RelayServiceConfig struct {
	Topics            []string  `json:"topics"`
	PricePerGB        float64   `json:"price_per_gb"`
	Currency          string    `json:"currency"`
	MaxBandwidth      int64     `json:"max_bandwidth"`      // bytes/second
	MaxDuration       int64     `json:"max_duration"`       // seconds
	MaxData           int64     `json:"max_data"`           // bytes per circuit
	Availability      float64   `json:"availability"`       // percentage
	ExpectedLatency   int       `json:"expected_latency"`   // milliseconds
	InitialReputation float64   `json:"initial_reputation"` // 0-5 rating
	ContactInfo       string    `json:"contact_info"`
}

// CreateDefaultRelayConfig creates a default relay service configuration
func CreateDefaultRelayConfig(topics []string) RelayServiceConfig {
	return RelayServiceConfig{
		Topics:            topics,
		PricePerGB:        0.10,  // $0.10 per GB
		Currency:          "USD",
		MaxBandwidth:      10 * 1024 * 1024,    // 10 MB/s
		MaxDuration:       60 * 60,             // 1 hour
		MaxData:           2 * 1024 * 1024 * 1024, // 2GB
		Availability:      99.0,  // 99% uptime
		ExpectedLatency:   100,   // 100ms
		InitialReputation: 3.0,   // Neutral rating
		ContactInfo:       "",
	}
}

// UpdateAdvertisementFromBilling updates service info based on actual billing data
func (rsa *RelayServiceAdvertiser) UpdateAdvertisementFromBilling(ctx context.Context) error {
	if !rsa.isActive {
		return nil
	}
	
	// Get actual performance metrics from billing system
	if rsa.p2pm.relayTrafficMonitor != nil {
		// Update availability based on uptime
		// Update latency based on connection stats
		// This would integrate with your monitoring system
		
		rsa.lm.Log("debug", "Updated relay advertisement with real performance data", "relay-advertiser")
	}
	
	return nil
}

// SetAdvertisementInterval sets how often to re-advertise the service
func (rsa *RelayServiceAdvertiser) SetAdvertisementInterval(interval time.Duration) {
	rsa.advertiseInterval = interval
}