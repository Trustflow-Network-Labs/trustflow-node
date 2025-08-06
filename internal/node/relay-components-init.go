package node

import (
	"context"
	"fmt"
)

// initRelayComponents initializes relay discovery and advertisement components
func (p2pm *P2PManager) initRelayComponents() {
	if p2pm.idht == nil {
		p2pm.Lm.Log("warning", "DHT not available, skipping relay component initialization", "relay-init")
		return
	}

	// Initialize relay peer source for discovering other relays
	p2pm.relayPeerSource = NewTopicAwareRelayPeerSource(
		p2pm.idht,
		p2pm.tcm,
		p2pm.Lm,
		p2pm.completeTopicNames,
	)

	// Initialize relay advertiser for advertising our relay service (if we're a relay)
	p2pm.relayAdvertiser = NewRelayServiceAdvertiser(p2pm.idht, p2pm, p2pm.Lm)

	p2pm.Lm.Log("info", 
		fmt.Sprintf("Initialized relay components for %d topics", len(p2pm.completeTopicNames)), 
		"relay-init")
}

// StartRelayAdvertising starts advertising relay service if configured as relay
func (p2pm *P2PManager) StartRelayAdvertising(ctx context.Context, config RelayServiceConfig) error {
	if !p2pm.relay {
		return fmt.Errorf("node not configured as relay")
	}

	if p2pm.relayAdvertiser == nil {
		return fmt.Errorf("relay advertiser not initialized")
	}

	return p2pm.relayAdvertiser.StartAdvertising(ctx, config)
}

// StopRelayAdvertising stops advertising relay service
func (p2pm *P2PManager) StopRelayAdvertising() {
	if p2pm.relayAdvertiser != nil {
		p2pm.relayAdvertiser.StopAdvertising()
	}
}

// UpdateRelayPricing updates relay service pricing
func (p2pm *P2PManager) UpdateRelayPricing(config RelayServiceConfig) error {
	if p2pm.relayAdvertiser == nil {
		return fmt.Errorf("relay advertiser not initialized")
	}

	return p2pm.relayAdvertiser.UpdateServiceInfo(config)
}

// GetDiscoveredRelays returns currently discovered relay services
func (p2pm *P2PManager) GetDiscoveredRelays() map[string][]RelayServiceInfo {
	if p2pm.relayPeerSource == nil {
		return make(map[string][]RelayServiceInfo)
	}

	return p2pm.relayPeerSource.GetCachedRelays()
}

// ConfigureRelayDiscovery configures relay discovery parameters
func (p2pm *P2PManager) ConfigureRelayDiscovery(maxRelays int, minRep float64, maxPrice float64, currency string) {
	if p2pm.relayPeerSource != nil {
		p2pm.relayPeerSource.SetConfiguration(maxRelays, minRep, maxPrice, currency)
	}
}

// GetRelayServiceStatus returns the current relay service status
func (p2pm *P2PManager) GetRelayServiceStatus() RelayServiceStatus {
	status := RelayServiceStatus{
		IsRelay:       p2pm.relay,
		IsAdvertising: false,
		DiscoveredRelays: 0,
	}

	if p2pm.relayAdvertiser != nil {
		status.IsAdvertising = p2pm.relayAdvertiser.IsAdvertising()
		if status.IsAdvertising {
			status.ServiceInfo = p2pm.relayAdvertiser.GetCurrentServiceInfo()
		}
	}

	if p2pm.relayPeerSource != nil {
		discovered := p2pm.relayPeerSource.GetCachedRelays()
		for _, relays := range discovered {
			status.DiscoveredRelays += len(relays)
		}
	}

	return status
}

// RelayServiceStatus contains information about relay service status
type RelayServiceStatus struct {
	IsRelay          bool              `json:"is_relay"`
	IsAdvertising    bool              `json:"is_advertising"`
	ServiceInfo      RelayServiceInfo  `json:"service_info,omitempty"`
	DiscoveredRelays int               `json:"discovered_relays"`
}