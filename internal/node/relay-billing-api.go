package node

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// RelayBillingReport represents a comprehensive billing report for relay usage
type RelayBillingReport struct {
	ReportPeriod    ReportPeriod                  `json:"report_period"`
	TotalUsage      RelayUsageSummary             `json:"total_usage"`
	PeerUsage       map[string]RelayUsageSummary  `json:"peer_usage"`
	ActiveCircuits  map[string]*RelayTrafficRecord `json:"active_circuits"`
	CompletedCircuits []RelayTrafficRecord        `json:"completed_circuits"`
	GeneratedAt     time.Time                     `json:"generated_at"`
}

// ReportPeriod defines the time range for the billing report
type ReportPeriod struct {
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Duration  string    `json:"duration"`
}

// RelayBillingAPI provides methods to generate billing reports and export data
type RelayBillingAPI struct {
	p2pm *P2PManager
}

// NewRelayBillingAPI creates a new billing API instance
func NewRelayBillingAPI(p2pm *P2PManager) *RelayBillingAPI {
	return &RelayBillingAPI{p2pm: p2pm}
}

// GenerateUsageReport creates a comprehensive billing report for the specified time period
func (rba *RelayBillingAPI) GenerateUsageReport(since time.Time) (*RelayBillingReport, error) {
	if rba.p2pm.relayTrafficMonitor == nil {
		return nil, fmt.Errorf("relay traffic monitor not initialized")
	}

	endTime := time.Now()
	
	// Get usage data from monitor
	peerUsageMap := rba.p2pm.relayTrafficMonitor.ExportBillingData(since)
	activeCircuits := rba.p2pm.relayTrafficMonitor.GetActiveCircuits()
	completedTraffic := rba.p2pm.relayTrafficMonitor.GetCompletedTraffic(since)

	// Convert peer.ID keys to strings for JSON serialization
	peerUsageStr := make(map[string]RelayUsageSummary)
	var totalUsage RelayUsageSummary
	totalUsage.PeerID = peer.ID("TOTAL")

	for peerID, usage := range peerUsageMap {
		peerUsageStr[peerID.String()] = usage
		totalUsage.TotalIngress += usage.TotalIngress
		totalUsage.TotalEgress += usage.TotalEgress
		totalUsage.TotalDuration += usage.TotalDuration
		totalUsage.CircuitCount += usage.CircuitCount
	}

	report := &RelayBillingReport{
		ReportPeriod: ReportPeriod{
			StartTime: since,
			EndTime:   endTime,
			Duration:  endTime.Sub(since).String(),
		},
		TotalUsage:        totalUsage,
		PeerUsage:         peerUsageStr,
		ActiveCircuits:    activeCircuits,
		CompletedCircuits: completedTraffic,
		GeneratedAt:       time.Now(),
	}

	return report, nil
}

// ExportUsageReportJSON exports the usage report as JSON
func (rba *RelayBillingAPI) ExportUsageReportJSON(since time.Time) ([]byte, error) {
	report, err := rba.GenerateUsageReport(since)
	if err != nil {
		return nil, err
	}

	return json.MarshalIndent(report, "", "  ")
}

// ExportUsageReportCSV exports peer usage data as CSV format
func (rba *RelayBillingAPI) ExportUsageReportCSV(since time.Time) (string, error) {
	report, err := rba.GenerateUsageReport(since)
	if err != nil {
		return "", err
	}

	// Create CSV header
	csv := "PeerID,TotalIngressBytes,TotalEgressBytes,TotalBytes,TotalDurationSeconds,CircuitCount,AvgBytesPerCircuit\n"

	// Add peer usage data
	for peerID, usage := range report.PeerUsage {
		totalBytes := usage.TotalIngress + usage.TotalEgress
		avgBytesPerCircuit := int64(0)
		if usage.CircuitCount > 0 {
			avgBytesPerCircuit = totalBytes / int64(usage.CircuitCount)
		}

		csv += fmt.Sprintf("%s,%d,%d,%d,%.2f,%d,%d\n",
			peerID,
			usage.TotalIngress,
			usage.TotalEgress,
			totalBytes,
			usage.TotalDuration.Seconds(),
			usage.CircuitCount,
			avgBytesPerCircuit,
		)
	}

	return csv, nil
}

// GetTopUsageByTraffic returns the top N peers by total traffic usage
func (rba *RelayBillingAPI) GetTopUsageByTraffic(since time.Time, limit int) ([]RelayUsageSummary, error) {
	report, err := rba.GenerateUsageReport(since)
	if err != nil {
		return nil, err
	}

	// Convert to slice for sorting
	usageList := make([]RelayUsageSummary, 0, len(report.PeerUsage))
	for _, usage := range report.PeerUsage {
		usageList = append(usageList, usage)
	}

	// Sort by total traffic (ingress + egress)
	for i := 0; i < len(usageList)-1; i++ {
		for j := i + 1; j < len(usageList); j++ {
			totalI := usageList[i].TotalIngress + usageList[i].TotalEgress
			totalJ := usageList[j].TotalIngress + usageList[j].TotalEgress
			if totalJ > totalI {
				usageList[i], usageList[j] = usageList[j], usageList[i]
			}
		}
	}

	// Return top N
	if limit > len(usageList) {
		limit = len(usageList)
	}
	return usageList[:limit], nil
}

// GetUsageForPeer returns detailed usage information for a specific peer
func (rba *RelayBillingAPI) GetUsageForPeer(peerID peer.ID, since time.Time) (*RelayUsageSummary, []RelayTrafficRecord, error) {
	if rba.p2pm.relayTrafficMonitor == nil {
		return nil, nil, fmt.Errorf("relay traffic monitor not initialized")
	}

	// Get aggregated usage
	totalIngress, totalEgress, totalDuration := rba.p2pm.relayTrafficMonitor.GetPeerUsage(peerID, since)
	
	usage := &RelayUsageSummary{
		PeerID:        peerID,
		TotalIngress:  totalIngress,
		TotalEgress:   totalEgress,
		TotalDuration: totalDuration,
	}

	// Get individual traffic records
	completedTraffic := rba.p2pm.relayTrafficMonitor.GetCompletedTraffic(since)
	var peerRecords []RelayTrafficRecord
	
	for _, record := range completedTraffic {
		if record.PeerID == peerID {
			peerRecords = append(peerRecords, record)
			usage.CircuitCount++
		}
	}

	// Check active circuits
	activeCircuits := rba.p2pm.relayTrafficMonitor.GetActiveCircuits()
	for _, record := range activeCircuits {
		if record.PeerID == peerID {
			peerRecords = append(peerRecords, *record)
			usage.CircuitCount++
		}
	}

	return usage, peerRecords, nil
}

// GenerateInvoiceData creates invoice-ready data for a specific peer
func (rba *RelayBillingAPI) GenerateInvoiceData(peerID peer.ID, since time.Time, ratePerGB float64) (*InvoiceData, error) {
	usage, records, err := rba.GetUsageForPeer(peerID, since)
	if err != nil {
		return nil, err
	}

	totalGB := float64(usage.TotalIngress+usage.TotalEgress) / (1024 * 1024 * 1024)
	totalCost := totalGB * ratePerGB

	invoice := &InvoiceData{
		PeerID:        peerID.String(),
		BillingPeriod: ReportPeriod{
			StartTime: since,
			EndTime:   time.Now(),
			Duration:  time.Since(since).String(),
		},
		Usage: *usage,
		RatePerGB: ratePerGB,
		TotalGB:   totalGB,
		TotalCost: totalCost,
		Records:   records,
		GeneratedAt: time.Now(),
	}

	return invoice, nil
}

// InvoiceData represents billing data ready for invoice generation
type InvoiceData struct {
	PeerID        string                 `json:"peer_id"`
	BillingPeriod ReportPeriod          `json:"billing_period"`
	Usage         RelayUsageSummary     `json:"usage"`
	RatePerGB     float64               `json:"rate_per_gb"`
	TotalGB       float64               `json:"total_gb"`
	TotalCost     float64               `json:"total_cost"`
	Records       []RelayTrafficRecord  `json:"traffic_records"`
	GeneratedAt   time.Time             `json:"generated_at"`
}

// AddRelayBillingAPI adds billing API methods to P2PManager
func (p2pm *P2PManager) GetRelayBillingAPI() *RelayBillingAPI {
	return NewRelayBillingAPI(p2pm)
}