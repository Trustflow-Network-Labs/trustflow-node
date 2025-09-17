package node

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p/core/metrics"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/Trustflow-Network-Labs/trustflow-node/internal/utils"
)

// RelayBandwidthReporter implements metrics.Reporter to capture real relay traffic
type RelayBandwidthReporter struct {
	db   *sql.DB
	lm   *utils.LogsManager
	p2pm *P2PManager
}

// NewRelayBandwidthReporter creates a bandwidth reporter for relay traffic measurement
func NewRelayBandwidthReporter(db *sql.DB, lm *utils.LogsManager, p2pm *P2PManager) *RelayBandwidthReporter {
	if lm != nil {
		lm.Log("info", "🚀 RelayBandwidthReporter initialized for traffic billing", "relay-billing")
	}
	
	return &RelayBandwidthReporter{
		db:   db,
		lm:   lm,
		p2pm: p2pm,
	}
}

// LogSentMessage records outbound traffic through relay connections
func (rbr *RelayBandwidthReporter) LogSentMessage(size int64) {
	// This captures all outbound traffic - we'll filter for relay in LogSentMessageStream
	if rbr.lm != nil {
		rbr.lm.Log("debug", fmt.Sprintf("BandwidthReporter: LogSentMessage called with %d bytes", size), "relay-billing")
	}
}

// LogRecvMessage records inbound traffic through relay connections  
func (rbr *RelayBandwidthReporter) LogRecvMessage(size int64) {
	// This captures all inbound traffic - we'll filter for relay in LogRecvMessageStream
	if rbr.lm != nil {
		rbr.lm.Log("debug", fmt.Sprintf("BandwidthReporter: LogRecvMessage called with %d bytes", size), "relay-billing")
	}
}

// LogSentMessageStream records outbound traffic for specific peer/protocol
func (rbr *RelayBandwidthReporter) LogSentMessageStream(size int64, proto protocol.ID, p peer.ID) {
	if rbr.lm != nil {
		rbr.lm.Log("debug", fmt.Sprintf("BandwidthReporter: LogSentMessageStream called - peer: %s, proto: %s, size: %d", 
			p.String()[:8], proto, size), "relay-billing")
	}
	
	if rbr.isRelayTraffic(p) {
		if rbr.lm != nil {
			rbr.lm.Log("info", fmt.Sprintf("Recording relay EGRESS traffic: peer %s, protocol %s, %d bytes", 
				p.String()[:8], proto, size), "relay-billing")
		}
		rbr.recordTraffic(p, 0, size, string(proto), "egress")
	}
}

// LogRecvMessageStream records inbound traffic for specific peer/protocol
func (rbr *RelayBandwidthReporter) LogRecvMessageStream(size int64, proto protocol.ID, p peer.ID) {
	if rbr.lm != nil {
		rbr.lm.Log("debug", fmt.Sprintf("BandwidthReporter: LogRecvMessageStream called - peer: %s, proto: %s, size: %d", 
			p.String()[:8], proto, size), "relay-billing")
	}
	
	if rbr.isRelayTraffic(p) {
		if rbr.lm != nil {
			rbr.lm.Log("info", fmt.Sprintf("Recording relay INGRESS traffic: peer %s, protocol %s, %d bytes", 
				p.String()[:8], proto, size), "relay-billing")
		}
		rbr.recordTraffic(p, size, 0, string(proto), "ingress")
	}
}

// GetBandwidthForPeer returns total bandwidth used by a peer (unused - implemented for interface)
func (rbr *RelayBandwidthReporter) GetBandwidthForPeer(peer.ID) metrics.Stats {
	return metrics.Stats{} // Not used, we store in database
}

// GetBandwidthForProtocol returns total bandwidth for protocol (unused - implemented for interface)  
func (rbr *RelayBandwidthReporter) GetBandwidthForProtocol(protocol.ID) metrics.Stats {
	return metrics.Stats{} // Not used, we store in database
}

// GetBandwidthTotals returns total bandwidth (unused - implemented for interface)
func (rbr *RelayBandwidthReporter) GetBandwidthTotals() metrics.Stats {
	return metrics.Stats{} // Not used, we store in database
}

// GetBandwidthByPeer returns bandwidth per peer (unused - implemented for interface)
func (rbr *RelayBandwidthReporter) GetBandwidthByPeer() map[peer.ID]metrics.Stats {
	return make(map[peer.ID]metrics.Stats) // Not used, we store in database
}

// GetBandwidthByProtocol returns bandwidth per protocol (unused - implemented for interface)
func (rbr *RelayBandwidthReporter) GetBandwidthByProtocol() map[protocol.ID]metrics.Stats {
	return make(map[protocol.ID]metrics.Stats) // Not used, we store in database
}

// isRelayTraffic determines if traffic with a peer is going through relay
func (rbr *RelayBandwidthReporter) isRelayTraffic(peerID peer.ID) bool {
	if rbr.p2pm == nil || rbr.p2pm.h == nil {
		return false
	}

	// Check all connections to this peer
	conns := rbr.p2pm.h.Network().ConnsToPeer(peerID)
	for _, conn := range conns {
		remoteAddr := conn.RemoteMultiaddr().String()
		if strings.Contains(remoteAddr, "/p2p-circuit") {
			return true
		}
	}
	return false
}

// recordTraffic stores actual measured traffic in the database
func (rbr *RelayBandwidthReporter) recordTraffic(peerID peer.ID, ingressBytes, egressBytes int64, protocol, direction string) {
	if rbr.db == nil {
		return
	}

	// Insert traffic record into database
	query := `
		INSERT INTO relay_traffic_log (
			peer_id, ingress_bytes, egress_bytes, protocol, direction, 
			recorded_at, relay_node_id
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	relayNodeID := ""
	if rbr.p2pm != nil && rbr.p2pm.h != nil {
		relayNodeID = rbr.p2pm.h.ID().String()
	}

	_, err := rbr.db.Exec(query, 
		peerID.String(), 
		ingressBytes, 
		egressBytes, 
		protocol, 
		direction,
		time.Now().Unix(),
		relayNodeID,
	)

	if err != nil && rbr.lm != nil {
		rbr.lm.Log("error", 
			fmt.Sprintf("Failed to record relay traffic: %v", err), 
			"relay-billing")
	} else if rbr.lm != nil {
		// Log significant transfers
		totalBytes := ingressBytes + egressBytes
		if totalBytes > 1024*1024 { // > 1MB
			rbr.lm.Log("info", 
				fmt.Sprintf("Recorded relay traffic: peer %s, %d bytes (%s)", 
					peerID.String()[:8], totalBytes, direction), 
				"relay-billing")
		}
	}
}

// GetRelayTrafficFromDB retrieves relay traffic data from database
func (rbr *RelayBandwidthReporter) GetRelayTrafficFromDB(peerID peer.ID, since time.Time) (int64, int64, error) {
	if rbr.db == nil {
		return 0, 0, fmt.Errorf("database not available")
	}

	query := `
		SELECT 
			COALESCE(SUM(ingress_bytes), 0) as total_ingress,
			COALESCE(SUM(egress_bytes), 0) as total_egress
		FROM relay_traffic_log 
		WHERE peer_id = ? AND recorded_at >= ?
	`

	var totalIngress, totalEgress int64
	err := rbr.db.QueryRow(query, peerID.String(), since.Unix()).Scan(&totalIngress, &totalEgress)
	
	return totalIngress, totalEgress, err
}

// GetTopRelayUsers returns top users by traffic volume from database
func (rbr *RelayBandwidthReporter) GetTopRelayUsers(since time.Time, limit int) ([]RelayUsageSummary, error) {
	if rbr.db == nil {
		return nil, fmt.Errorf("database not available")
	}

	query := `
		SELECT 
			peer_id,
			COALESCE(SUM(ingress_bytes), 0) as total_ingress,
			COALESCE(SUM(egress_bytes), 0) as total_egress,
			COUNT(DISTINCT DATE(recorded_at, 'unixepoch')) as active_days,
			MIN(recorded_at) as first_seen,
			MAX(recorded_at) as last_seen
		FROM relay_traffic_log 
		WHERE recorded_at >= ?
		GROUP BY peer_id
		ORDER BY (total_ingress + total_egress) DESC
		LIMIT ?
	`

	rows, err := rbr.db.Query(query, since.Unix(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []RelayUsageSummary
	for rows.Next() {
		var peerIDStr string
		var totalIngress, totalEgress, activeDays, firstSeen, lastSeen int64

		err := rows.Scan(&peerIDStr, &totalIngress, &totalEgress, &activeDays, &firstSeen, &lastSeen)
		if err != nil {
			continue
		}

		peerID, err := peer.Decode(peerIDStr)
		if err != nil {
			continue
		}

		duration := time.Unix(lastSeen, 0).Sub(time.Unix(firstSeen, 0))

		users = append(users, RelayUsageSummary{
			PeerID:        peerID,
			TotalIngress:  totalIngress,
			TotalEgress:   totalEgress,
			TotalDuration: duration,
			CircuitCount:  int(activeDays), // Using active days as proxy for circuit count
		})
	}

	return users, nil
}

// GetRelayTrafficHistory returns time-series traffic data for analysis
func (rbr *RelayBandwidthReporter) GetRelayTrafficHistory(peerID peer.ID, since time.Time, intervalMinutes int) ([]TrafficDataPoint, error) {
	if rbr.db == nil {
		return nil, fmt.Errorf("database not available")
	}

	query := `
		SELECT 
			(recorded_at / (? * 60)) * (? * 60) as time_bucket,
			COALESCE(SUM(ingress_bytes), 0) as total_ingress,
			COALESCE(SUM(egress_bytes), 0) as total_egress,
			COUNT(*) as transfer_count
		FROM relay_traffic_log 
		WHERE peer_id = ? AND recorded_at >= ?
		GROUP BY time_bucket
		ORDER BY time_bucket
	`

	rows, err := rbr.db.Query(query, intervalMinutes, intervalMinutes, peerID.String(), since.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dataPoints []TrafficDataPoint
	for rows.Next() {
		var timeBucket, totalIngress, totalEgress, transferCount int64

		err := rows.Scan(&timeBucket, &totalIngress, &totalEgress, &transferCount)
		if err != nil {
			continue
		}

		dataPoints = append(dataPoints, TrafficDataPoint{
			Timestamp:     time.Unix(timeBucket, 0),
			IngressBytes:  totalIngress,
			EgressBytes:   totalEgress,
			TransferCount: int(transferCount),
		})
	}

	return dataPoints, nil
}

// TrafficDataPoint represents a point in time for traffic analysis
type TrafficDataPoint struct {
	Timestamp     time.Time `json:"timestamp"`
	IngressBytes  int64     `json:"ingress_bytes"`
	EgressBytes   int64     `json:"egress_bytes"`
	TransferCount int       `json:"transfer_count"`
}