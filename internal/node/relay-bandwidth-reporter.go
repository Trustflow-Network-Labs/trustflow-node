package node

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/metrics"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/Trustflow-Network-Labs/trustflow-node/internal/utils"
)

// TrafficRecord represents a single traffic measurement for batching
type TrafficRecord struct {
	PeerID       string
	IngressBytes int64
	EgressBytes  int64
	Protocol     string
	Direction    string
	RecordedAt   int64
	RelayNodeID  string
}

// RelayBandwidthReporter implements metrics.Reporter to capture real relay traffic
type RelayBandwidthReporter struct {
	db          *sql.DB
	lm          *utils.LogsManager
	p2pm        *P2PManager

	// Batching fields to reduce database overhead
	trafficBatch []TrafficRecord
	batchMutex   sync.Mutex
	batchTicker  *time.Ticker
	stopChan     chan struct{}
}

// NewRelayBandwidthReporter creates a bandwidth reporter for relay traffic measurement
func NewRelayBandwidthReporter(db *sql.DB, lm *utils.LogsManager, p2pm *P2PManager) *RelayBandwidthReporter {
	if lm != nil {
		lm.Log("info", "🚀 RelayBandwidthReporter initialized with batched operations for memory efficiency", "relay-billing")
	}

	rbr := &RelayBandwidthReporter{
		db:           db,
		lm:           lm,
		p2pm:         p2pm,
		trafficBatch: make([]TrafficRecord, 0, 100), // Pre-allocate batch capacity
		stopChan:     make(chan struct{}),
	}

	// Start batch processing to reduce database overhead
	var batchInterval time.Duration = 30 * time.Second // Default
	if p2pm != nil && p2pm.cm != nil {
		batchInterval = p2pm.cm.GetConfigDuration("relay_batch_interval", 30*time.Second)
		if lm != nil {
			lm.Log("info", fmt.Sprintf("🔧 Relay batch interval configured: %v", batchInterval), "relay-billing")
		}
	}
	rbr.batchTicker = time.NewTicker(batchInterval)
	go rbr.processBatchedTraffic()

	return rbr
}

// LogSentMessage records outbound traffic through relay connections
func (rbr *RelayBandwidthReporter) LogSentMessage(size int64) {
	// This captures all outbound traffic - we'll filter for relay in LogSentMessageStream
	// DEBUG LOGGING REMOVED: Was causing memory leaks with excessive string allocations
}

// LogRecvMessage records inbound traffic through relay connections
func (rbr *RelayBandwidthReporter) LogRecvMessage(size int64) {
	// This captures all inbound traffic - we'll filter for relay in LogRecvMessageStream
	// DEBUG LOGGING REMOVED: Was causing memory leaks with excessive string allocations
}

// LogSentMessageStream records outbound traffic for specific peer/protocol
func (rbr *RelayBandwidthReporter) LogSentMessageStream(size int64, proto protocol.ID, p peer.ID) {
	// DEBUG LOGGING REMOVED: Was causing massive memory leaks with 200k+ logs per day

	if rbr.isRelayTraffic(p) {
		// Keep only essential relay traffic logging, reduce frequency
		if rbr.lm != nil && size > 1000 { // Only log significant traffic
			rbr.lm.Log("info", fmt.Sprintf("Recording relay EGRESS traffic: peer %s, protocol %s, %d bytes",
				p.String()[:8], proto, size), "relay-billing")
		}
		rbr.recordTraffic(p, 0, size, string(proto), "egress")
	}
}

// LogRecvMessageStream records inbound traffic for specific peer/protocol
func (rbr *RelayBandwidthReporter) LogRecvMessageStream(size int64, proto protocol.ID, p peer.ID) {
	// DEBUG LOGGING REMOVED: Was causing massive memory leaks with 200k+ logs per day

	if rbr.isRelayTraffic(p) {
		// Keep only essential relay traffic logging, reduce frequency
		if rbr.lm != nil && size > 1000 { // Only log significant traffic
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

// recordTraffic adds traffic record to batch instead of immediate database write
func (rbr *RelayBandwidthReporter) recordTraffic(peerID peer.ID, ingressBytes, egressBytes int64, protocol, direction string) {
	if rbr.db == nil {
		return
	}

	relayNodeID := ""
	if rbr.p2pm != nil && rbr.p2pm.h != nil {
		relayNodeID = rbr.p2pm.h.ID().String()
	}

	// Add to batch instead of immediate database insert (reduces memory pressure)
	record := TrafficRecord{
		PeerID:       peerID.String(),
		IngressBytes: ingressBytes,
		EgressBytes:  egressBytes,
		Protocol:     protocol,
		Direction:    direction,
		RecordedAt:   time.Now().Unix(),
		RelayNodeID:  relayNodeID,
	}

	rbr.batchMutex.Lock()
	rbr.trafficBatch = append(rbr.trafficBatch, record)

	// If batch gets too large, trigger immediate flush to prevent memory buildup
	var batchSize int = 100 // Default
	if rbr.p2pm != nil && rbr.p2pm.cm != nil {
		batchSize = rbr.p2pm.cm.GetConfigInt("relay_traffic_batch_size", 100, 10, 1000)
	}
	if len(rbr.trafficBatch) >= batchSize {
		rbr.flushBatch() // Flush without releasing lock
	}
	rbr.batchMutex.Unlock()
}

// processBatchedTraffic runs periodic batch processing to reduce database overhead
func (rbr *RelayBandwidthReporter) processBatchedTraffic() {
	for {
		select {
		case <-rbr.batchTicker.C:
			rbr.batchMutex.Lock()
			if len(rbr.trafficBatch) > 0 {
				rbr.flushBatch()
			}
			rbr.batchMutex.Unlock()
		case <-rbr.stopChan:
			// Final flush before shutdown
			rbr.batchMutex.Lock()
			if len(rbr.trafficBatch) > 0 {
				rbr.flushBatch()
			}
			rbr.batchMutex.Unlock()
			return
		}
	}
}

// flushBatch performs batched database insert (must be called with mutex held)
func (rbr *RelayBandwidthReporter) flushBatch() {
	if len(rbr.trafficBatch) == 0 {
		return
	}

	// Prepare batch insert query
	query := `
		INSERT INTO relay_traffic_log (
			peer_id, ingress_bytes, egress_bytes, protocol, direction,
			recorded_at, relay_node_id
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	tx, err := rbr.db.Begin()
	if err != nil {
		if rbr.lm != nil {
			rbr.lm.Log("error", fmt.Sprintf("Failed to start batch transaction: %v", err), "relay-billing")
		}
		return
	}

	stmt, err := tx.Prepare(query)
	if err != nil {
		tx.Rollback()
		if rbr.lm != nil {
			rbr.lm.Log("error", fmt.Sprintf("Failed to prepare batch statement: %v", err), "relay-billing")
		}
		return
	}
	defer stmt.Close()

	// Execute all records in the batch
	for _, record := range rbr.trafficBatch {
		_, err := stmt.Exec(
			record.PeerID,
			record.IngressBytes,
			record.EgressBytes,
			record.Protocol,
			record.Direction,
			record.RecordedAt,
			record.RelayNodeID,
		)
		if err != nil {
			if rbr.lm != nil {
				rbr.lm.Log("error", fmt.Sprintf("Failed to execute batch record: %v", err), "relay-billing")
			}
			// Continue with other records instead of failing entire batch
		}
	}

	err = tx.Commit()
	if err != nil {
		if rbr.lm != nil {
			rbr.lm.Log("error", fmt.Sprintf("Failed to commit batch transaction: %v", err), "relay-billing")
		}
	} else if rbr.lm != nil {
		rbr.lm.Log("info", fmt.Sprintf("Successfully flushed %d relay traffic records to database", len(rbr.trafficBatch)), "relay-billing")
	}

	// Clear the batch (reuse slice to avoid allocations)
	rbr.trafficBatch = rbr.trafficBatch[:0]
}

// Shutdown stops the batch processing goroutine
func (rbr *RelayBandwidthReporter) Shutdown() {
	if rbr.batchTicker != nil {
		rbr.batchTicker.Stop()
	}
	close(rbr.stopChan)
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