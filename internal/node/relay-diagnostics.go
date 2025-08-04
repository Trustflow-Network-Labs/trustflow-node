package node

import (
	"fmt"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// RelayDiagnostics provides debugging tools for relay traffic monitoring
type RelayDiagnostics struct {
	p2pm *P2PManager
}

// NewRelayDiagnostics creates diagnostic tools
func NewRelayDiagnostics(p2pm *P2PManager) *RelayDiagnostics {
	return &RelayDiagnostics{p2pm: p2pm}
}

// CheckRelayConnections shows current connections and identifies relay circuits
func (rd *RelayDiagnostics) CheckRelayConnections() {
	if rd.p2pm.h == nil {
		fmt.Println("❌ Host not initialized")
		return
	}

	network := rd.p2pm.h.Network()
	allConns := network.Conns()
	
	fmt.Printf("🔍 Total connections: %d\n", len(allConns))
	
	relayCount := 0
	directCount := 0
	
	for i, conn := range allConns {
		remoteAddr := conn.RemoteMultiaddr().String()
		peerID := conn.RemotePeer()
		
		isRelay := strings.Contains(remoteAddr, "/p2p-circuit")
		if isRelay {
			relayCount++
			fmt.Printf("🔗 [%d] RELAY: %s -> %s\n", i, peerID.String()[:8], remoteAddr)
		} else {
			directCount++
			fmt.Printf("📡 [%d] DIRECT: %s -> %s\n", i, peerID.String()[:8], remoteAddr)
		}
	}
	
	fmt.Printf("\n📊 Summary: %d relay, %d direct connections\n", relayCount, directCount)
}

// CheckDatabaseRecords shows current relay traffic records in database
func (rd *RelayDiagnostics) CheckDatabaseRecords() {
	if rd.p2pm.DB == nil {
		fmt.Println("❌ Database not available")
		return
	}

	query := `
		SELECT peer_id, ingress_bytes, egress_bytes, protocol, direction, 
		       recorded_at, circuit_id, connection_type
		FROM relay_traffic_log 
		ORDER BY recorded_at DESC 
		LIMIT 20
	`
	
	rows, err := rd.p2pm.DB.Query(query)
	if err != nil {
		fmt.Printf("❌ Database query error: %v\n", err)
		return
	}
	defer rows.Close()
	
	fmt.Println("🗄️ Recent relay traffic records:")
	count := 0
	
	for rows.Next() {
		var peerID, protocol, direction, circuitID, connectionType string
		var ingressBytes, egressBytes, recordedAt int64
		
		err := rows.Scan(&peerID, &ingressBytes, &egressBytes, &protocol, &direction, &recordedAt, &circuitID, &connectionType)
		if err != nil {
			continue
		}
		
		timestamp := time.Unix(recordedAt, 0)
		fmt.Printf("  [%d] %s: peer %s, %d/%d bytes, %s, %s\n", 
			count+1, timestamp.Format("15:04:05"), peerID[:8], ingressBytes, egressBytes, direction, connectionType)
		count++
	}
	
	if count == 0 {
		fmt.Println("  ❌ No records found")
		
		// Check if table exists
		tableQuery := "SELECT name FROM sqlite_master WHERE type='table' AND name='relay_traffic_log'"
		var tableName string
		err = rd.p2pm.DB.QueryRow(tableQuery).Scan(&tableName)
		if err != nil {
			fmt.Println("  ❌ relay_traffic_log table does not exist!")
		} else {
			fmt.Println("  ✅ relay_traffic_log table exists but is empty")
		}
	} else {
		fmt.Printf("  ✅ Found %d records\n", count)
	}
}

// CheckBandwidthReporter verifies if bandwidth reporter is working
func (rd *RelayDiagnostics) CheckBandwidthReporter() {
	fmt.Println("🔧 Bandwidth Reporter Status:")
	
	if rd.p2pm.relay {
		fmt.Println("  ✅ Node configured as relay")
	} else {
		fmt.Println("  ❌ Node NOT configured as relay - bandwidth reporter disabled")
		return
	}
	
	if rd.p2pm.DB != nil {
		fmt.Println("  ✅ Database available")
	} else {
		fmt.Println("  ❌ Database not available")
	}
	
	if rd.p2pm.Lm != nil {
		fmt.Println("  ✅ Logger available")
	} else {
		fmt.Println("  ❌ Logger not available")
	}
	
	// Check recent logs for bandwidth reporter activity
	fmt.Println("\n📝 Check your logs for these messages:")
	fmt.Println("  - '🚀 RelayBandwidthReporter initialized for traffic billing'")
	fmt.Println("  - 'BandwidthReporter: LogSentMessage called'")
	fmt.Println("  - 'BandwidthReporter: LogRecvMessage called'")
	fmt.Println("  - '🔗 RELAY CIRCUIT DETECTED'")
	fmt.Println("  - 'Recording relay EGRESS/INGRESS traffic'")
}

// RunFullDiagnostic runs all diagnostic checks
func (rd *RelayDiagnostics) RunFullDiagnostic() {
	fmt.Println("🚀 Relay Traffic Billing Diagnostics")
	fmt.Println("=====================================")
	
	rd.CheckBandwidthReporter()
	fmt.Println()
	
	rd.CheckRelayConnections()
	fmt.Println()
	
	rd.CheckDatabaseRecords()
	fmt.Println()
	
	fmt.Println("💡 Troubleshooting Tips:")
	fmt.Println("1. If no relay connections: Private nodes might be using direct connections (hole punching working!)")
	fmt.Println("2. If no bandwidth reporter logs: Check if node is configured with relay=true")
	fmt.Println("3. If connections exist but no DB records: Check isRelayTraffic() logic")
	fmt.Println("4. If everything looks good: Relay traffic might just be very low volume")
}

// GetPeerConnectionTypes shows connection types for each peer
func (rd *RelayDiagnostics) GetPeerConnectionTypes() map[peer.ID][]string {
	result := make(map[peer.ID][]string)
	
	if rd.p2pm.h == nil {
		return result
	}
	
	for _, conn := range rd.p2pm.h.Network().Conns() {
		peerID := conn.RemotePeer()
		addr := conn.RemoteMultiaddr().String()
		
		if strings.Contains(addr, "/p2p-circuit") {
			result[peerID] = append(result[peerID], "relay")
		} else {
			result[peerID] = append(result[peerID], "direct")
		}
	}
	
	return result
}