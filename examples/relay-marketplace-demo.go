package examples

import (
	"fmt"

	"github.com/Trustflow-Network-Labs/trustflow-node/internal/node"
)

// RelayMarketplaceDemo demonstrates the topic-aware relay marketplace
func RelayMarketplaceDemo() {
	fmt.Println("🚀 TrustFlow Relay Marketplace Demo")
	fmt.Println("==================================")

	// This demo shows how the new relay marketplace works
	demoRelayMarketplace()
}

func demoRelayMarketplace() {
	fmt.Println("\n🏪 Relay Marketplace Features:\n")

	// 1. Relay Service Advertisement
	fmt.Println("1. 📢 Relay Service Advertisement:")
	fmt.Println("   - Public nodes can advertise relay services")
	fmt.Println("   - Set pricing per GB (e.g., $0.10/GB)")
	fmt.Println("   - Specify supported topics")
	fmt.Println("   - Include performance metrics (latency, availability)")
	fmt.Printf(`
   Example usage:
   config := node.CreateDefaultRelayConfig([]string{"ai-training", "data-processing"})
   config.PricePerGB = 0.15  // $0.15 per GB
   config.Currency = "USD"
   config.MaxBandwidth = 50 * 1024 * 1024  // 50 MB/s
   
   err := p2pm.StartRelayAdvertising(ctx, config)
   `)

	// 2. Dynamic Relay Discovery
	fmt.Println("\n2. 🔍 Dynamic Relay Discovery:")
	fmt.Println("   - Private nodes discover available relays via DHT")
	fmt.Println("   - Topic-aware filtering (only find relays for your topics)")
	fmt.Println("   - Automatic sorting by price, reputation, latency")
	fmt.Println("   - Replaces static relay configuration")
	fmt.Printf(`
   DHT Keys Used:
   - "/trustflow/relay-service/{topic}"  -> Find providers
   - "/trustflow/relay-info/{peerID}/{topic}" -> Get service details
   `)

	// 3. Billing Integration
	fmt.Println("\n3. 💰 Billing Integration:")
	fmt.Println("   - Real-time traffic measurement via BandwidthReporter")
	fmt.Println("   - Database storage of usage data")
	fmt.Println("   - Per-peer billing with circuit tracking")
	fmt.Println("   - Connection type classification (relay vs direct)")
	fmt.Printf(`
   Billing Flow:
   1. RelayBandwidthReporter captures real traffic
   2. isRelayTraffic() filters for relay connections
   3. recordTraffic() stores in relay_traffic_log table
   4. RelayBillingAPI generates invoices
   `)

	// 4. Topic-Aware Architecture
	fmt.Println("\n4. 🎯 Topic-Aware Architecture:")
	fmt.Println("   - Relays advertise per topic (ai-training, data-processing, etc.)")
	fmt.Println("   - Clients discover relays for their specific topics")
	fmt.Println("   - Different pricing per topic/service type")
	fmt.Println("   - Better resource allocation and specialization")

	// 5. Implementation Status
	fmt.Println("\n5. ✅ Implementation Status:")
	fmt.Println("   ✅ TopicAwareRelayPeerSource - DHT-based relay discovery")
	fmt.Println("   ✅ RelayServiceAdvertiser - DHT advertisement system")
	fmt.Println("   ✅ RelayBandwidthReporter - Real traffic measurement")
	fmt.Println("   ✅ RelayTrafficMonitor - Database-backed billing")
	fmt.Println("   ✅ RelayBillingAPI - Invoice generation & export")
	fmt.Println("   ⚠️  PeerSource interface - Needs libp2p integration")

	// 6. Usage Examples
	fmt.Println("\n6. 📝 Usage Examples:")
	fmt.Printf(`
   // For Relay Operators (Public Nodes):
   config := node.CreateDefaultRelayConfig([]string{"ai-training"})
   config.PricePerGB = 0.20  // Premium pricing for AI workloads
   p2pm.StartRelayAdvertising(ctx, config)
   
   // For Relay Customers (Private Nodes):
   p2pm.ConfigureRelayDiscovery(5, 3.0, 0.50, "USD")  // Max 5 relays, min 3.0 rating, max $0.50/GB
   
   // Generate Bills:
   billingAPI := p2pm.GetRelayBillingAPI()
   report, _ := billingAPI.GenerateUsageReport(time.Now().Add(-24*time.Hour))
   csvData, _ := billingAPI.ExportUsageReportCSV(time.Now().Add(-24*time.Hour))
   `)

	// 7. Next Steps
	fmt.Println("\n7. 🔧 Next Steps:")
	fmt.Println("   1. Complete libp2p PeerSource interface implementation")
	fmt.Println("   2. Add reputation/rating system")
	fmt.Println("   3. Implement payment integration (crypto/fiat)")
	fmt.Println("   4. Add relay performance monitoring")
	fmt.Println("   5. Create web UI for relay marketplace")

	fmt.Println("\n🎉 Relay Marketplace Ready for Development!")
}

// Example relay configurations for different use cases
func showRelayConfigurations() {
	fmt.Println("\n💡 Example Relay Configurations:\n")

	// Budget relay
	budget := node.CreateDefaultRelayConfig([]string{"general", "testing"})
	budget.PricePerGB = 0.05              // $0.05/GB
	budget.MaxBandwidth = 5 * 1024 * 1024 // 5 MB/s
	fmt.Printf("Budget Relay: $%.2f/GB, %d MB/s\n", budget.PricePerGB, budget.MaxBandwidth/(1024*1024))

	// Premium relay
	premium := node.CreateDefaultRelayConfig([]string{"ai-training", "video-processing"})
	premium.PricePerGB = 0.25                // $0.25/GB
	premium.MaxBandwidth = 100 * 1024 * 1024 // 100 MB/s
	premium.Availability = 99.9              // 99.9% uptime
	fmt.Printf("Premium Relay: $%.2f/GB, %d MB/s, %.1f%% uptime\n",
		premium.PricePerGB, premium.MaxBandwidth/(1024*1024), premium.Availability)

	// Specialized relay
	specialized := node.CreateDefaultRelayConfig([]string{"blockchain", "crypto-mining"})
	specialized.PricePerGB = 0.15          // $0.15/GB
	specialized.MaxDuration = 12 * 60 * 60 // 12 hours max
	specialized.ContactInfo = "crypto-relay@example.com"
	fmt.Printf("Specialized Relay: $%.2f/GB, %d hour max duration\n",
		specialized.PricePerGB, specialized.MaxDuration/3600)
}
