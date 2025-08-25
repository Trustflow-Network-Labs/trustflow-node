package node

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

// RelayConnectionNotifee implements network.Notifee to monitor relay connections for billing
type RelayConnectionNotifee struct {
	monitor *RelayTrafficMonitor
	p2pm    *P2PManager
}

// Listen is called when network starts listening on an addr
func (rcn *RelayConnectionNotifee) Listen(network.Network, multiaddr.Multiaddr) {}

// ListenClose is called when network stops listening on an addr
func (rcn *RelayConnectionNotifee) ListenClose(network.Network, multiaddr.Multiaddr) {}

// Connected is called when a connection is opened
func (rcn *RelayConnectionNotifee) Connected(n network.Network, conn network.Conn) {
	// Check if this is a relay connection by examining the multiaddr
	relayAddr := conn.RemoteMultiaddr().String()
	peerID := conn.RemotePeer()
	
	// Log all connections for debugging
	rcn.p2pm.Lm.Log("debug", 
		fmt.Sprintf("Connection opened: peer %s via %s", 
			peerID.String()[:8], relayAddr), 
		"relay-billing")
	
	if strings.Contains(relayAddr, "/p2p-circuit") {
		// Extract circuit information for billing
		circuitID := fmt.Sprintf("circuit-%s-%d", peerID.String()[:8], time.Now().Unix())
		
		// Start monitoring this relay circuit
		rcn.monitor.StartCircuitMonitoring(circuitID, peerID)
		
		rcn.p2pm.Lm.Log("info", 
			fmt.Sprintf("🔗 RELAY CIRCUIT DETECTED: peer %s via %s (circuit: %s)", 
				peerID.String()[:8], relayAddr, circuitID), 
			"relay-billing")
	}
}

// Disconnected is called when a connection is closed
func (rcn *RelayConnectionNotifee) Disconnected(n network.Network, conn network.Conn) {
	// Check if this was a relay connection
	relayAddr := conn.RemoteMultiaddr().String()
	if strings.Contains(relayAddr, "/p2p-circuit/") {
		peerID := conn.RemotePeer()
		
		// Determine connection type based on remaining connections
		connectionType := "relay_only"
		remainingConns := n.ConnsToPeer(peerID)
		hasDirectConnection := false
		
		for _, c := range remainingConns {
			if !strings.Contains(c.RemoteMultiaddr().String(), "/p2p-circuit/") {
				hasDirectConnection = true
				break
			}
		}
		
		if hasDirectConnection {
			connectionType = "relay_then_direct"
		}
		
		// Find and end the circuit monitoring
		activeCircuits := rcn.monitor.GetActiveCircuits()
		for circuitID, record := range activeCircuits {
			if record.PeerID == peerID {
				rcn.monitor.EndCircuitMonitoring(circuitID, connectionType)
				break
			}
		}
		
		rcn.p2pm.Lm.Log("info", 
			fmt.Sprintf("Relay circuit disconnected: peer %s (Type: %s)", 
				peerID.String()[:8], connectionType), 
			"relay-billing")
	}
}

// GetRelayUsageReport generates a billing report for relay usage
func (p2pm *P2PManager) GetRelayUsageReport(since time.Time) map[peer.ID]RelayUsageSummary {
	if p2pm.relayTrafficMonitor == nil {
		return make(map[peer.ID]RelayUsageSummary)
	}
	return p2pm.relayTrafficMonitor.ExportBillingData(since)
}

// GetActiveRelayCircuits returns currently active relay circuits
func (p2pm *P2PManager) GetActiveRelayCircuits() map[string]*RelayTrafficRecord {
	if p2pm.relayTrafficMonitor == nil {
		return make(map[string]*RelayTrafficRecord)
	}
	return p2pm.relayTrafficMonitor.GetActiveCircuits()
}

// StartRelayBillingLogger starts periodic logging for relay billing
func (p2pm *P2PManager) StartRelayBillingLogger(ctx context.Context) {
	if p2pm.relayTrafficMonitor == nil {
		return
	}
	
	// Set up billing callbacks
	p2pm.relayTrafficMonitor.SetBillingCallbacks(
		// On circuit start
		func(record *RelayTrafficRecord) {
			p2pm.Lm.Log("info", 
				fmt.Sprintf("Billing: Circuit started for peer %s", record.PeerID.String()[:8]), 
				"relay-billing")
		},
		// On circuit end
		func(record *RelayTrafficRecord) {
			p2pm.Lm.Log("info", 
				fmt.Sprintf("Billing: Circuit completed for peer %s - Duration: %v, Ingress: %d bytes, Egress: %d bytes", 
					record.PeerID.String()[:8], record.Duration, record.BytesIngress, record.BytesEgress), 
				"relay-billing")
		},
		// On traffic update (every 30s)
		func(record *RelayTrafficRecord) {
			if record.BytesIngress+record.BytesEgress > 10*1024*1024 { // Log if > 10MB total
				p2pm.Lm.Log("info", 
					fmt.Sprintf("Billing: High usage for peer %s - Total: %d bytes", 
						record.PeerID.String()[:8], record.BytesIngress+record.BytesEgress), 
					"relay-billing")
			}
		},
	)
	
	// Start periodic logging with goroutine tracking
	tracker := p2pm.GetGoroutineTracker()
	if !tracker.SafeStart("relay-traffic-periodic-logging", func() {
		p2pm.relayTrafficMonitor.StartPeriodicLogging(ctx, func(records []RelayTrafficRecord) {
			totalTraffic := int64(0)
			for _, record := range records {
				totalTraffic += record.BytesIngress + record.BytesEgress
			}
			
			if totalTraffic > 0 {
				p2pm.Lm.Log("info", 
					fmt.Sprintf("Relay billing summary: %d active circuits, %d total bytes", 
						len(records), totalTraffic), 
					"relay-billing")
			}
		})
	}) {
		p2pm.Lm.Log("error", "Failed to start relay traffic periodic logging due to goroutine limit", "relay-billing")
	}
}