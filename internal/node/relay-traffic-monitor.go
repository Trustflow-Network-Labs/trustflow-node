package node

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/network"
)

// RelayTrafficRecord represents traffic usage for a single peer through relay
type RelayTrafficRecord struct {
	PeerID        peer.ID   `json:"peer_id"`
	CircuitID     string    `json:"circuit_id"`
	StartTime     time.Time `json:"start_time"`
	EndTime       *time.Time `json:"end_time,omitempty"`
	BytesIngress  int64     `json:"bytes_ingress"`
	BytesEgress   int64     `json:"bytes_egress"`
	Duration      time.Duration `json:"duration"`
	ConnectionType string   `json:"connection_type"` // "relay_only", "relay_then_direct", "direct_only"
	Status        string    `json:"status"` // "active", "completed", "failed"
}

// RelayTrafficMonitor tracks and logs traffic for relay services
type RelayTrafficMonitor struct {
	mu              sync.RWMutex
	activeCircuits  map[string]*RelayTrafficRecord // Keep active circuits in memory for performance
	host            network.Network
	db              *sql.DB // Database for persistent storage
	
	// Configuration
	logInterval     time.Duration
	maxActiveCircuits int // Limit active circuits in memory
	
	// Callbacks for billing integration
	onCircuitStart  func(*RelayTrafficRecord)
	onCircuitEnd    func(*RelayTrafficRecord)
	onTrafficUpdate func(*RelayTrafficRecord)
}

// NewRelayTrafficMonitor creates a new traffic monitor for relay services with database storage
func NewRelayTrafficMonitor(host network.Network, db *sql.DB) *RelayTrafficMonitor {
	return &RelayTrafficMonitor{
		activeCircuits:    make(map[string]*RelayTrafficRecord),
		host:             host,
		db:               db,
		logInterval:      30 * time.Second, // Log every 30 seconds
		maxActiveCircuits: 1000, // Limit active circuits in memory
	}
}

// StartCircuitMonitoring begins tracking a new relay circuit
func (rtm *RelayTrafficMonitor) StartCircuitMonitoring(circuitID string, peerID peer.ID) {
	rtm.mu.Lock()
	defer rtm.mu.Unlock()
	
	// Check if we're at the limit - if so, clean up oldest circuits first
	if len(rtm.activeCircuits) >= rtm.maxActiveCircuits {
		rtm.cleanupOldestCircuitsUnsafe(rtm.maxActiveCircuits / 4) // Remove 25% to free space
	}
	
	record := &RelayTrafficRecord{
		PeerID:         peerID,
		CircuitID:      circuitID,
		StartTime:      time.Now(),
		BytesIngress:   0,
		BytesEgress:    0,
		ConnectionType: "relay_only",
		Status:         "active",
	}
	
	rtm.activeCircuits[circuitID] = record
	
	if rtm.onCircuitStart != nil {
		rtm.onCircuitStart(record)
	}
}

// UpdateTraffic updates traffic counters for an active circuit
func (rtm *RelayTrafficMonitor) UpdateTraffic(circuitID string, ingressBytes, egressBytes int64) {
	rtm.mu.Lock()
	defer rtm.mu.Unlock()
	
	if record, exists := rtm.activeCircuits[circuitID]; exists {
		record.BytesIngress += ingressBytes
		record.BytesEgress += egressBytes
		record.Duration = time.Since(record.StartTime)
		
		if rtm.onTrafficUpdate != nil {
			rtm.onTrafficUpdate(record)
		}
	}
}

// EndCircuitMonitoring stops tracking a relay circuit and stores final data in database
func (rtm *RelayTrafficMonitor) EndCircuitMonitoring(circuitID string, connectionType string) {
	rtm.mu.Lock()
	defer rtm.mu.Unlock()
	
	if record, exists := rtm.activeCircuits[circuitID]; exists {
		endTime := time.Now()
		record.EndTime = &endTime
		record.Duration = endTime.Sub(record.StartTime)
		record.ConnectionType = connectionType
		record.Status = "completed"
		
		// Store final record in database
		if rtm.db != nil {
			rtm.storeCircuitRecord(record)
		}
		
		// Remove from active circuits
		delete(rtm.activeCircuits, circuitID)
		
		if rtm.onCircuitEnd != nil {
			rtm.onCircuitEnd(record)
		}
	}
}

// storeCircuitRecord stores a completed circuit record in the database
func (rtm *RelayTrafficMonitor) storeCircuitRecord(record *RelayTrafficRecord) {
	query := `
		INSERT INTO relay_traffic_log (
			peer_id, ingress_bytes, egress_bytes, protocol, direction,
			recorded_at, relay_node_id, circuit_id, connection_type
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	
	// Get relay node ID from host
	relayNodeID := ""
	if rtm.host != nil {
		// We need to get the host ID - this requires accessing the host through the network
		// For now, use placeholder - this will be set properly when integrated
		relayNodeID = "relay-node"
	}
	
	_, err := rtm.db.Exec(query,
		record.PeerID.String(),
		record.BytesIngress,
		record.BytesEgress,
		"circuit", // protocol placeholder
		"completed", // direction for completed circuits
		record.EndTime.Unix(),
		relayNodeID,
		record.CircuitID,
		record.ConnectionType,
	)
	
	if err != nil {
		// Log error but don't fail - billing data is critical
		fmt.Printf("Failed to store relay traffic record: %v\n", err)
	}
}

// GetActiveCircuits returns current active relay circuits
func (rtm *RelayTrafficMonitor) GetActiveCircuits() map[string]*RelayTrafficRecord {
	rtm.mu.RLock()
	defer rtm.mu.RUnlock()
	
	result := make(map[string]*RelayTrafficRecord)
	for id, record := range rtm.activeCircuits {
		// Create copy to avoid race conditions
		recordCopy := *record
		result[id] = &recordCopy
	}
	return result
}

// GetCompletedTraffic returns historical traffic records from database
func (rtm *RelayTrafficMonitor) GetCompletedTraffic(since time.Time) []RelayTrafficRecord {
	if rtm.db == nil {
		return []RelayTrafficRecord{}
	}
	
	query := `
		SELECT peer_id, ingress_bytes, egress_bytes, recorded_at, circuit_id, connection_type
		FROM relay_traffic_log 
		WHERE recorded_at >= ? AND direction = 'completed'
		ORDER BY recorded_at DESC
		LIMIT 10000
	`
	
	rows, err := rtm.db.Query(query, since.Unix())
	if err != nil {
		fmt.Printf("Failed to query completed traffic: %v\n", err)
		return []RelayTrafficRecord{}
	}
	defer rows.Close()
	
	var result []RelayTrafficRecord
	for rows.Next() {
		var peerIDStr, circuitID, connectionType string
		var ingressBytes, egressBytes, recordedAt int64
		
		err := rows.Scan(&peerIDStr, &ingressBytes, &egressBytes, &recordedAt, &circuitID, &connectionType)
		if err != nil {
			continue
		}
		
		peerID, err := peer.Decode(peerIDStr)
		if err != nil {
			continue
		}
		
		endTime := time.Unix(recordedAt, 0)
		record := RelayTrafficRecord{
			PeerID:         peerID,
			CircuitID:      circuitID,
			StartTime:      endTime, // We only have end time from DB
			EndTime:        &endTime,
			BytesIngress:   ingressBytes,
			BytesEgress:    egressBytes,
			ConnectionType: connectionType,
			Status:         "completed",
		}
		
		result = append(result, record)
	}
	
	return result
}

// GetPeerUsage returns total usage for a specific peer from both active circuits and database
func (rtm *RelayTrafficMonitor) GetPeerUsage(peerID peer.ID, since time.Time) (totalIngress, totalEgress int64, duration time.Duration) {
	rtm.mu.RLock()
	defer rtm.mu.RUnlock()
	
	// Check active circuits
	for _, record := range rtm.activeCircuits {
		if record.PeerID == peerID && record.StartTime.After(since) {
			totalIngress += record.BytesIngress
			totalEgress += record.BytesEgress
			duration += time.Since(record.StartTime)
		}
	}
	
	// Query database for completed traffic
	if rtm.db != nil {
		query := `
			SELECT COALESCE(SUM(ingress_bytes), 0), COALESCE(SUM(egress_bytes), 0)
			FROM relay_traffic_log 
			WHERE peer_id = ? AND recorded_at >= ?
		`
		
		var dbIngress, dbEgress int64
		err := rtm.db.QueryRow(query, peerID.String(), since.Unix()).Scan(&dbIngress, &dbEgress)
		if err == nil {
			totalIngress += dbIngress
			totalEgress += dbEgress
		}
	}
	
	return totalIngress, totalEgress, duration
}

// SetBillingCallbacks sets callbacks for billing system integration
func (rtm *RelayTrafficMonitor) SetBillingCallbacks(
	onStart func(*RelayTrafficRecord),
	onEnd func(*RelayTrafficRecord),
	onUpdate func(*RelayTrafficRecord),
) {
	rtm.mu.Lock()
	defer rtm.mu.Unlock()
	
	rtm.onCircuitStart = onStart
	rtm.onCircuitEnd = onEnd
	rtm.onTrafficUpdate = onUpdate
}

// StartPeriodicLogging begins periodic traffic logging
func (rtm *RelayTrafficMonitor) StartPeriodicLogging(ctx context.Context, logger func([]RelayTrafficRecord)) {
	ticker := time.NewTicker(rtm.logInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			activeRecords := make([]RelayTrafficRecord, 0, len(rtm.activeCircuits))
			rtm.mu.RLock()
			for _, record := range rtm.activeCircuits {
				activeRecords = append(activeRecords, *record)
			}
			rtm.mu.RUnlock()
			
			if len(activeRecords) > 0 && logger != nil {
				logger(activeRecords)
			}
		}
	}
}

// ExportBillingData exports traffic data in billing-friendly format from database
func (rtm *RelayTrafficMonitor) ExportBillingData(since time.Time) map[peer.ID]RelayUsageSummary {
	rtm.mu.RLock()
	defer rtm.mu.RUnlock()
	
	usage := make(map[peer.ID]RelayUsageSummary)
	
	// Add active circuits
	for _, record := range rtm.activeCircuits {
		if record.StartTime.After(since) {
			summary := usage[record.PeerID]
			summary.PeerID = record.PeerID
			summary.TotalIngress += record.BytesIngress
			summary.TotalEgress += record.BytesEgress
			summary.TotalDuration += time.Since(record.StartTime)
			summary.CircuitCount++
			usage[record.PeerID] = summary
		}
	}
	
	// Query database for completed traffic
	if rtm.db != nil {
		query := `
			SELECT peer_id, 
				   COALESCE(SUM(ingress_bytes), 0) as total_ingress,
				   COALESCE(SUM(egress_bytes), 0) as total_egress,
				   COUNT(DISTINCT circuit_id) as circuit_count
			FROM relay_traffic_log 
			WHERE recorded_at >= ?
			GROUP BY peer_id
		`
		
		rows, err := rtm.db.Query(query, since.Unix())
		if err == nil {
			defer rows.Close()
			
			for rows.Next() {
				var peerIDStr string
				var totalIngress, totalEgress, circuitCount int64
				
				err := rows.Scan(&peerIDStr, &totalIngress, &totalEgress, &circuitCount)
				if err != nil {
					continue
				}
				
				peerID, err := peer.Decode(peerIDStr)
				if err != nil {
					continue
				}
				
				// Merge with active circuit data
				summary := usage[peerID]
				summary.PeerID = peerID
				summary.TotalIngress += totalIngress
				summary.TotalEgress += totalEgress
				summary.CircuitCount += int(circuitCount)
				// Note: We don't have duration data in the current DB schema for completed circuits
				usage[peerID] = summary
			}
		}
	}
	
	return usage
}

// RelayUsageSummary aggregates usage data for billing
type RelayUsageSummary struct {
	PeerID        peer.ID       `json:"peer_id"`
	TotalIngress  int64         `json:"total_ingress_bytes"`
	TotalEgress   int64         `json:"total_egress_bytes"`
	TotalDuration time.Duration `json:"total_duration"`
	CircuitCount  int           `json:"circuit_count"`
}

// cleanupOldestCircuitsUnsafe removes the oldest active circuits (must be called with lock held)
func (rtm *RelayTrafficMonitor) cleanupOldestCircuitsUnsafe(count int) {
	if count <= 0 || len(rtm.activeCircuits) == 0 {
		return
	}
	
	// Sort circuits by start time (oldest first)
	type circuitAge struct {
		id        string
		startTime time.Time
	}
	
	circuits := make([]circuitAge, 0, len(rtm.activeCircuits))
	for id, record := range rtm.activeCircuits {
		circuits = append(circuits, circuitAge{id: id, startTime: record.StartTime})
	}
	
	// Sort by start time (oldest first)
	for i := 0; i < len(circuits)-1; i++ {
		for j := i + 1; j < len(circuits); j++ {
			if circuits[i].startTime.After(circuits[j].startTime) {
				circuits[i], circuits[j] = circuits[j], circuits[i]
			}
		}
	}
	
	// Remove oldest circuits up to count
	removed := 0
	for _, circuit := range circuits {
		if removed >= count {
			break
		}
		
		// Store final record in database before removing
		if record, exists := rtm.activeCircuits[circuit.id]; exists && rtm.db != nil {
			endTime := time.Now()
			record.EndTime = &endTime
			record.Duration = endTime.Sub(record.StartTime)
			record.Status = "timeout_cleanup"
			rtm.storeCircuitRecord(record)
		}
		
		delete(rtm.activeCircuits, circuit.id)
		removed++
	}
	
	if removed > 0 {
		fmt.Printf("Cleaned up %d oldest circuits due to memory limit (remaining: %d)\n",
			removed, len(rtm.activeCircuits))
	}
}

// CleanupStaleCircuits removes circuits that have been active longer than the specified duration
func (rtm *RelayTrafficMonitor) CleanupStaleCircuits(maxAge time.Duration) {
	rtm.mu.Lock()
	defer rtm.mu.Unlock()

	now := time.Now()
	staleCircuits := make([]string, 0)

	// Find stale circuits
	for circuitID, record := range rtm.activeCircuits {
		if now.Sub(record.StartTime) > maxAge {
			staleCircuits = append(staleCircuits, circuitID)
		}
	}

	// Remove stale circuits and save to database
	for _, circuitID := range staleCircuits {
		if record, exists := rtm.activeCircuits[circuitID]; exists && rtm.db != nil {
			// Mark as completed and save to database
			endTime := now
			record.EndTime = &endTime
			record.Duration = endTime.Sub(record.StartTime)
			record.Status = "completed"

			// Save to database
			rtm.storeCircuitRecord(record)
		}

		delete(rtm.activeCircuits, circuitID)
	}
}