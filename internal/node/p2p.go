package node

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"slices"

	blacklist_node "github.com/adgsm/trustflow-node/internal/blacklist-node"
	"github.com/adgsm/trustflow-node/internal/database"
	"github.com/adgsm/trustflow-node/internal/keystore"
	"github.com/adgsm/trustflow-node/internal/node_types"
	"github.com/adgsm/trustflow-node/internal/settings"
	"github.com/adgsm/trustflow-node/internal/ui"
	"github.com/adgsm/trustflow-node/internal/utils"
	"github.com/adgsm/trustflow-node/internal/workflow"
	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	drouting "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	dutil "github.com/libp2p/go-libp2p/p2p/discovery/util"
	rcmgr "github.com/libp2p/go-libp2p/p2p/host/resource-manager"
	"github.com/libp2p/go-libp2p/p2p/muxer/yamux"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	relayv2 "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
	identify "github.com/libp2p/go-libp2p/p2p/protocol/identify"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	libp2ptls "github.com/libp2p/go-libp2p/p2p/security/tls"
	ma "github.com/multiformats/go-multiaddr"
)

type P2PManager struct {
	daemon                bool
	public                bool
	relay                 bool
	bootstrapAddrs        []string
	relayAddrs            []string
	topicNames            []string
	completeTopicNames    []string
	topicsSubscribed      map[string]*pubsub.Topic
	subscriptions         []*pubsub.Subscription
	protocolID            protocol.ID
	ps                    *pubsub.PubSub
	idht                  *dht.IpfsDHT
	h                     host.Host
	ctx                   context.Context
	tcm                   *TopicAwareConnectionManager
	DB                    *sql.DB
	cm                    *utils.ConfigManager
	Lm                    *utils.LogsManager
	wm                    *workflow.WorkflowManager
	sc                    *node_types.ServiceOffersCache
	UI                    ui.UI
	peerDiscoveryTicker   *time.Ticker
	connectionMaintTicker *time.Ticker
	cronStopChan          chan struct{}
	cronWg                sync.WaitGroup
	streamSemaphore       chan struct{}        // Limit concurrent stream processing
	activeStreams         map[string]time.Time // Track active streams for cleanup
	streamsMutex          sync.RWMutex         // Protect activeStreams map
	relayTrafficMonitor   *RelayTrafficMonitor // Monitor relay traffic for billing
}

func NewP2PManager(ctx context.Context, ui ui.UI, cm *utils.ConfigManager) *P2PManager {
	// Create a database connection
	sqlm := database.NewSQLiteManager(cm)
	db, err := sqlm.CreateConnection()
	if err != nil {
		panic(err)
	}

	// Init log manager
	lm := utils.NewLogsManager(cm)

	p2pm := &P2PManager{
		daemon:          false,
		public:          false,
		relay:           false,
		streamSemaphore: make(chan struct{}, 10), // Allow max 10 concurrent streams
		activeStreams:   make(map[string]time.Time),
		bootstrapAddrs: []string{
			"/ip4/159.65.253.245/tcp/30609/p2p/QmRTYiSwrh4y5UozzTS5pors1jHPqsSSh7Sfd6dJ8kCgzF",
			"/ip4/159.65.253.245/udp/30611/quic-v1/p2p/QmRTYiSwrh4y5UozzTS5pors1jHPqsSSh7Sfd6dJ8kCgzF",
			//"/ip4/159.65.253.245/tcp/30613/ws/p2p/QmRTYiSwrh4y5UozzTS5pors1jHPqsSSh7Sfd6dJ8kCgzF",
			"/ip4/95.180.109.240/tcp/30609/p2p/QmSeoLQWMu48JGa2kj8bSvMrv59Rhkp2AveX3Yhf95ySeH",
			"/ip4/167.86.116.185/udp/30611/quic-v1/p2p/QmPpcuRSHmrjT2EEoHXhU5YT2zV9wF5N9LWuhPJofAhtci",
			"/ip4/167.86.116.185/tcp/30609/p2p/QmPpcuRSHmrjT2EEoHXhU5YT2zV9wF5N9LWuhPJofAhtci",
			//"/ip4/167.86.116.185/tcp/30613/ws/p2p/QmPpcuRSHmrjT2EEoHXhU5YT2zV9wF5N9LWuhPJofAhtci",
			"/ip4/85.237.211.221/udp/30611/quic-v1/p2p/QmaZffJXMWB1ifXP1c7U34NsgUZBSaA5QhXBwp269efHX9",
			"/ip4/85.237.211.221/tcp/30609/p2p/QmaZffJXMWB1ifXP1c7U34NsgUZBSaA5QhXBwp269efHX9",
			//"/ip4/85.237.211.221/tcp/30613/ws/p2p/QmaZffJXMWB1ifXP1c7U34NsgUZBSaA5QhXBwp269efHX9",
		},
		relayAddrs: []string{
			"/ip4/159.65.253.245/tcp/30609/p2p/QmRTYiSwrh4y5UozzTS5pors1jHPqsSSh7Sfd6dJ8kCgzF",
			"/ip4/159.65.253.245/udp/30611/quic-v1/p2p/QmRTYiSwrh4y5UozzTS5pors1jHPqsSSh7Sfd6dJ8kCgzF",
			//"/ip4/159.65.253.245/tcp/30613/ws/p2p/QmRTYiSwrh4y5UozzTS5pors1jHPqsSSh7Sfd6dJ8kCgzF",
			//"/ip4/95.180.109.240/tcp/30609/p2p/QmSeoLQWMu48JGa2kj8bSvMrv59Rhkp2AveX3Yhf95ySeH",
			"/ip4/167.86.116.185/udp/30611/quic-v1/p2p/QmPpcuRSHmrjT2EEoHXhU5YT2zV9wF5N9LWuhPJofAhtci",
			"/ip4/167.86.116.185/tcp/30609/p2p/QmPpcuRSHmrjT2EEoHXhU5YT2zV9wF5N9LWuhPJofAhtci",
			//"/ip4/167.86.116.185/tcp/30613/ws/p2p/QmPpcuRSHmrjT2EEoHXhU5YT2zV9wF5N9LWuhPJofAhtci",
			"/ip4/85.237.211.221/udp/30611/quic-v1/p2p/QmaZffJXMWB1ifXP1c7U34NsgUZBSaA5QhXBwp269efHX9",
			"/ip4/85.237.211.221/tcp/30609/p2p/QmaZffJXMWB1ifXP1c7U34NsgUZBSaA5QhXBwp269efHX9",
			//"/ip4/85.237.211.221/tcp/30613/ws/p2p/QmaZffJXMWB1ifXP1c7U34NsgUZBSaA5QhXBwp269efHX9",
		},
		topicNames:         []string{"lookup.service"},
		completeTopicNames: []string{},
		topicsSubscribed:   make(map[string]*pubsub.Topic),
		subscriptions:      []*pubsub.Subscription{},
		protocolID:         "",
		idht:               nil,
		ps:                 nil,
		h:                  nil,
		ctx:                nil,
		tcm:                nil,
		DB:                 db,
		cm:                 cm,
		Lm:                 lm,
		wm:                 workflow.NewWorkflowManager(db, lm, cm),
		sc:                 node_types.NewServiceOffersCache(),
		UI:                 ui,
	}

	if ctx != nil {
		p2pm.ctx = ctx
	}

	return p2pm
}

func (p2pm *P2PManager) Close() error {
	if p2pm.Lm != nil {
		return p2pm.Lm.Close()
	}
	return nil
}

// Start all P2P periodic tasks
func (p2pm *P2PManager) StartPeriodicTasks() error {
	// Get ticker intervals from config
	peerDiscoveryInterval, connectionHealthInterval, err := p2pm.getTickerIntervals()
	if err != nil {
		p2pm.Lm.Log("error", fmt.Sprintf("Can not read configs file. (%s)", err.Error()), "p2p")
		return err
	}

	// Initialize stop channel
	p2pm.cronStopChan = make(chan struct{})

	// Start peer discovery periodic task
	p2pm.peerDiscoveryTicker = time.NewTicker(peerDiscoveryInterval)
	p2pm.cronWg.Add(1)
	go func() {
		defer p2pm.cronWg.Done()
		defer p2pm.peerDiscoveryTicker.Stop()

		p2pm.Lm.Log("info", fmt.Sprintf("Started peer discovery every %v", peerDiscoveryInterval), "p2p")

		for {
			select {
			case <-p2pm.peerDiscoveryTicker.C:
				p2pm.DiscoverPeers()
			case <-p2pm.cronStopChan:
				p2pm.Lm.Log("info", "Stopped peer discovery periodic task", "p2p")
				return
			case <-p2pm.ctx.Done():
				p2pm.Lm.Log("info", "Context cancelled, stopping peer discovery", "p2p")
				return
			}
		}
	}()

	// Start connection maintenance periodic task
	p2pm.connectionMaintTicker = time.NewTicker(connectionHealthInterval)
	p2pm.cronWg.Add(1)
	go func() {
		defer p2pm.cronWg.Done()
		defer p2pm.connectionMaintTicker.Stop()

		p2pm.Lm.Log("info", fmt.Sprintf("Started connection maintenance every %v", connectionHealthInterval), "p2p")

		for {
			select {
			case <-p2pm.connectionMaintTicker.C:
				p2pm.MaintainConnections()
			case <-p2pm.cronStopChan:
				p2pm.Lm.Log("info", "Stopped connection maintenance periodic task", "p2p")
				return
			case <-p2pm.ctx.Done():
				p2pm.Lm.Log("info", "Context cancelled, stopping connection maintenance", "p2p")
				return
			}
		}
	}()

	p2pm.Lm.Log("info", "Started all P2P periodic tasks", "p2p")
	return nil
}

// Stop all P2P periodic tasks
func (p2pm *P2PManager) StopPeriodicTasks() {
	if p2pm.cronStopChan != nil {
		close(p2pm.cronStopChan)
		p2pm.cronWg.Wait() // Wait for all goroutines to finish
		p2pm.Lm.Log("info", "Stopped all P2P periodic tasks", "p2p")
	}
}

// Get ticker intervals from config (no cron parsing needed)
func (p2pm *P2PManager) getTickerIntervals() (time.Duration, time.Duration, error) {
	// Use simple duration strings in config instead of cron
	peerDiscoveryInterval := 5 * time.Minute // Default
	if val, exists := p2pm.cm.GetConfig("peer_discovery_interval"); exists {
		if duration, err := time.ParseDuration(val); err == nil {
			peerDiscoveryInterval = duration
		}
	}

	connectionHealthInterval := 2 * time.Minute // Default
	if val, exists := p2pm.cm.GetConfig("connection_health_interval"); exists {
		if duration, err := time.ParseDuration(val); err == nil {
			connectionHealthInterval = duration
		}
	}

	return peerDiscoveryInterval, connectionHealthInterval, nil
}

// IsHostRunning checks if the provided host is actively running
func (p2pm *P2PManager) IsHostRunning() bool {
	if p2pm.h == nil {
		return false
	}
	// Check if the host is listening on any network addresses
	return len(p2pm.h.Network().ListenAddresses()) > 0
}

// WaitForHostReady tries multiple times to check if the host is initialized and running
func (p2pm *P2PManager) WaitForHostReady(interval time.Duration, maxAttempts int) bool {
	for range maxAttempts {
		if p2pm.IsHostRunning() {
			return true // success
		}
		time.Sleep(interval)
	}
	return false
}

// Start p2p node
func (p2pm *P2PManager) Start(port uint16, daemon bool, public bool, relay bool) {
	p2pm.daemon = daemon
	p2pm.public = public
	p2pm.relay = relay

	if p2pm.ctx == nil {
		p2pm.ctx = context.Background()
	}

	blacklistManager, err := blacklist_node.NewBlacklistNodeManager(p2pm.DB, p2pm.UI, p2pm.Lm)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return
	}

	// Read port number from configs
	if port == 0 {
		p := p2pm.cm.GetConfigWithDefault("node_port", "30609")
		p64, err := strconv.ParseUint(p, 10, 16)
		if err != nil {
			port = 30609
		} else {
			port = uint16(p64)
		}
	}

	// Read topics' names to subscribe
	p2pm.completeTopicNames = p2pm.completeTopicNames[:0]
	topicNamePrefix := p2pm.cm.GetConfigWithDefault("topic_name_prefix", "trustflow.network.")
	for _, topicName := range p2pm.topicNames {
		topicName = topicNamePrefix + strings.TrimSpace(topicName)
		p2pm.completeTopicNames = append(p2pm.completeTopicNames, topicName)
	}

	// Read streaming protocol
	p2pm.protocolID = protocol.ID(p2pm.cm.GetConfigWithDefault("protocol_id", "/trustflow-network/1.0.0"))

	// Create or get previously created node key
	keystoreManager := keystore.NewKeyStoreManager(p2pm.DB, p2pm.Lm)
	priv, _, err := keystoreManager.ProvideKey()
	if err != nil {
		p2pm.Lm.Log("panic", err.Error(), "p2p")
		panic(fmt.Sprintf("%v", err))
	}

	// Get botstrap & relay hosts addr info
	var bootstrapAddrsInfo []peer.AddrInfo
	var relayAddrsInfo []peer.AddrInfo
	for _, relayAddr := range p2pm.relayAddrs {
		relayAddrInfo, err := p2pm.makeRelayPeerInfo(relayAddr)
		if err != nil {
			p2pm.Lm.Log("error", err.Error(), "p2p")
			continue
		}
		relayAddrsInfo = append(relayAddrsInfo, relayAddrInfo)
	}
	for _, bootstrapAddr := range p2pm.bootstrapAddrs {
		bootstrapAddrInfo, err := p2pm.makeRelayPeerInfo(bootstrapAddr)
		if err != nil {
			p2pm.Lm.Log("error", err.Error(), "p2p")
			continue
		}
		bootstrapAddrsInfo = append(bootstrapAddrsInfo, bootstrapAddrInfo)
	}

	// Default DHT bootstrap peers + ours
	var bootstrapPeers []peer.AddrInfo
	for _, peerAddr := range dht.DefaultBootstrapPeers {
		peerinfo, _ := peer.AddrInfoFromP2pAddr(peerAddr)
		bootstrapPeers = append(bootstrapPeers, *peerinfo)
	}
	bootstrapPeers = append(bootstrapPeers, bootstrapAddrsInfo...)

	maxConnections := 400

	// Create resource manager with moderate limits to prevent memory leaks
	// For now, use finite limits instead of infinite to control resource usage
	limits := rcmgr.DefaultLimits.AutoScale()
	limiter := rcmgr.NewFixedLimiter(limits)
	resourceManager, err := rcmgr.NewResourceManager(limiter)
	if err != nil {
		p2pm.Lm.Log("panic", err.Error(), "p2p")
		panic(fmt.Sprintf("%v", err))
	}

	var hst host.Host
	if p2pm.public {
		maxConnections = 800
		// Configure connection manager for large file transfers
		connMgr, err := connmgr.NewConnManager(
			100,                                  // Low water mark - minimum connections to maintain
			maxConnections,                       // High water mark - maximum connections before pruning
			connmgr.WithGracePeriod(time.Minute), // Grace period before pruning
		)
		if err != nil {
			p2pm.Lm.Log("panic", err.Error(), "p2p")
			panic(fmt.Sprintf("%v", err))
		}
		hst, err = p2pm.createPublicHost(priv, port, resourceManager, connMgr, blacklistManager, bootstrapPeers, relayAddrsInfo)
		if err != nil {
			p2pm.Lm.Log("error", err.Error(), "p2p")
			panic(fmt.Sprintf("%v", err))
		}
	} else {
		// Configure connection manager for large file transfers
		connMgr, err := connmgr.NewConnManager(
			100,                                  // Low water mark - minimum connections to maintain
			maxConnections,                       // High water mark - maximum connections before pruning
			connmgr.WithGracePeriod(time.Minute), // Grace period before pruning
		)
		if err != nil {
			p2pm.Lm.Log("panic", err.Error(), "p2p")
			panic(fmt.Sprintf("%v", err))
		}
		hst, err = p2pm.createPrivateHost(priv, port, resourceManager, connMgr, blacklistManager, bootstrapPeers, relayAddrsInfo)
		if err != nil {
			p2pm.Lm.Log("error", err.Error(), "p2p")
			panic(fmt.Sprintf("%v", err))
		}
	}

	// Set up identify service
	_, err = identify.NewIDService(hst)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		p2pm.UI.Print(err.Error())
	}

	message := fmt.Sprintf("Your Peer ID: %s", hst.ID())
	p2pm.Lm.Log("info", message, "p2p")
	p2pm.UI.Print(message)
	for _, addr := range hst.Addrs() {
		fullAddr := addr.Encapsulate(ma.StringCast("/p2p/" + hst.ID().String()))
		message := fmt.Sprintf("Listening on %s", fullAddr)
		p2pm.Lm.Log("info", message, "p2p")
		p2pm.UI.Print(message)
	}

	// Setup a stream handler with bounded concurrency.
	// This gets called every time a peer connects and opens a stream to this node.
	p2pm.h.SetStreamHandler(p2pm.protocolID, func(s network.Stream) {
		message := fmt.Sprintf("Remote peer `%s` started streaming to our node `%s` in stream id `%s`, using protocol id: `%s`",
			s.Conn().RemotePeer().String(), hst.ID(), s.ID(), p2pm.protocolID)
		p2pm.Lm.Log("info", message, "p2p")

		// Use semaphore to limit concurrent stream processing
		select {
		case p2pm.streamSemaphore <- struct{}{}:
			go func() {
				defer func() { <-p2pm.streamSemaphore }()
				p2pm.streamProposalResponse(s)
			}()
		default:
			// Semaphore full, reject stream
			p2pm.Lm.Log("warn", "Stream processing at capacity, rejecting stream "+s.ID(), "p2p")
			s.Reset()
		}
	})

	routingDiscovery := drouting.NewRoutingDiscovery(p2pm.idht)
	p2pm.ps, err = pubsub.NewGossipSub(p2pm.ctx, p2pm.h,
		pubsub.WithPeerExchange(true),
		pubsub.WithFloodPublish(true),
		pubsub.WithDiscovery(routingDiscovery),
	)
	if err != nil {
		p2pm.Lm.Log("panic", err.Error(), "p2p")
		panic(fmt.Sprintf("%v", err))
	}

	tcm, err := NewTopicAwareConnectionManager(p2pm, maxConnections, p2pm.completeTopicNames)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		panic(fmt.Sprintf("%v", err))
	}
	p2pm.tcm = tcm

	go func() {
		p2pm.subscriptions = p2pm.subscriptions[:0]
		for _, completeTopicName := range p2pm.completeTopicNames {
			sub, topic, err := p2pm.joinSubscribeTopic(p2pm.ctx, p2pm.ps, completeTopicName)
			if err != nil {
				p2pm.Lm.Log("panic", err.Error(), "p2p")
				panic(fmt.Sprintf("%v", err))
			}
			p2pm.subscriptions = append(p2pm.subscriptions, sub)

			notifyManager := NewTopicAwareNotifiee(p2pm.ps, topic, completeTopicName, bootstrapPeers, tcm, p2pm.Lm)

			// Attach the notifiee to the host's network
			p2pm.h.Network().Notify(notifyManager)
		}
	}()

	// Start topics aware peer discovery
	go p2pm.DiscoverPeers()

	// Start P2P periodic tasks
	err = p2pm.StartPeriodicTasks()
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		panic(fmt.Sprintf("%v", err))
	}

	// Start relay billing logger if relay is enabled
	if p2pm.relay && p2pm.relayTrafficMonitor != nil {
		go p2pm.StartRelayBillingLogger(p2pm.ctx)
	}

	// Start job periodic tasks
	jobManager := NewJobManager(p2pm)
	err = jobManager.StartPeriodicTasks()
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "jobs")
		panic(fmt.Sprintf("%v", err))
	}

	if !daemon {
		// Print interactive menu
		menuManager := NewMenuManager(p2pm)
		menuManager.Run()
	} else {
		// Running as a daemon never ends
		<-p2pm.ctx.Done()
	}

	// Stop job periodic tasks when shutting down
	jobManager.StopPeriodicTasks()

	// Close DB connection
	if err = p2pm.DB.Close(); err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
	}
}

func (p2pm *P2PManager) createPublicHost(
	priv crypto.PrivKey,
	port uint16,
	resourceManager network.ResourceManager,
	connMgr *connmgr.BasicConnMgr,
	blacklistManager *blacklist_node.BlacklistNodeManager,
	bootstrapAddrInfo []peer.AddrInfo,
	relayAddrsInfo []peer.AddrInfo,
) (host.Host, error) {
	message := "Creating public p2p host"
	p2pm.Lm.Log("info", message, "p2p")
	p2pm.UI.Print(message)

	hst, err := p2pm.createHost(
		priv,
		port,
		resourceManager,
		connMgr,
		blacklistManager,
		relayAddrsInfo,
	)
	if err != nil {
		return nil, err
	}

	// let this host use the DHT to find other hosts
	//p2pm.idht, err = p2pm.initDHT("server")
	p2pm.idht, err = p2pm.initDHT("", bootstrapAddrInfo)
	if err != nil {
		p2pm.h.Close()
		return nil, err
	}

	if p2pm.relay {
		message := "Starting relay server"
		p2pm.Lm.Log("info", message, "p2p")
		p2pm.UI.Print(message)

		// Initialize relay traffic monitor for billing with database storage
		p2pm.relayTrafficMonitor = NewRelayTrafficMonitor(hst.Network(), p2pm.DB)

		// Register relay connection notifee for billing
		relayNotifee := &RelayConnectionNotifee{
			monitor: p2pm.relayTrafficMonitor,
			p2pm:    p2pm,
		}
		hst.Network().Notify(relayNotifee)

		// Start relay service with resource limits (this is for high-bandwidth relay servers)
		// TODO, make this configurable
		_, err = relayv2.New(hst,
			relayv2.WithResources(relayv2.Resources{
				Limit: &relayv2.RelayLimit{
					Duration: 60 * time.Minute,       // 1 hour
					Data:     2 * 1024 * 1024 * 1024, // 2GB per circuit
				},
				ReservationTTL:  30 * time.Second,
				MaxReservations: 5000,
				MaxCircuits:     100,
				BufferSize:      256 * 1024, // 256KB buffer
			}),
		)
		if err != nil {
			hst.Close()
			return nil, err
		}

		// Relay diagnostics, DEBAG mode
		//		diagnostics := NewRelayDiagnostics(p2pm)
		//		diagnostics.RunFullDiagnostic()
	}

	return hst, nil
}

func (p2pm *P2PManager) createPrivateHost(
	priv crypto.PrivKey,
	port uint16,
	resourceManager network.ResourceManager,
	connMgr *connmgr.BasicConnMgr,
	blacklistManager *blacklist_node.BlacklistNodeManager,
	bootstrapAddrInfo []peer.AddrInfo,
	relayAddrsInfo []peer.AddrInfo,
) (host.Host, error) {
	message := "Creating private p2p host (behind NAT)"
	p2pm.Lm.Log("info", message, "p2p")
	p2pm.UI.Print(message)

	hst, err := p2pm.createHost(
		priv,
		port,
		resourceManager,
		connMgr,
		blacklistManager,
		relayAddrsInfo,
	)
	if err != nil {
		return nil, err
	}

	// let this host use the DHT to find other hosts
	//p2pm.idht, err = p2pm.initDHT("client")
	p2pm.idht, err = p2pm.initDHT("", bootstrapAddrInfo)
	if err != nil {
		p2pm.h.Close()
		return nil, err
	}

	return hst, nil
}

func (p2pm *P2PManager) createHost(
	priv crypto.PrivKey,
	port uint16,
	resourceManager network.ResourceManager,
	connMgr *connmgr.BasicConnMgr,
	blacklistManager *blacklist_node.BlacklistNodeManager,
	relayAddrsInfo []peer.AddrInfo,
) (host.Host, error) {
	// Configure yamux for large transfers
	yamuxConfig := p2pm.Lm.CreateYamuxConfigWithLogger()

	hst, err := libp2p.New(
		// Use the keypair we generated
		libp2p.Identity(priv),
		// Listening addresses
		libp2p.ListenAddrStrings(
			fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", port),
			fmt.Sprintf("/ip6/::1/tcp/%d", port+1),
			fmt.Sprintf("/ip4/0.0.0.0/udp/%d/quic-v1", port+2),
			fmt.Sprintf("/ip6/::1/udp/%d/quic-v1", port+3),
			fmt.Sprintf("/ip4/0.0.0.0/tcp/%d/ws", port+4),
			fmt.Sprintf("/ip6/::1/udp/%d/ws", port+5),
			"/p2p-circuit",
		),
		// support TLS connections
		libp2p.Security(libp2ptls.ID, libp2ptls.New),
		// support noise connections
		libp2p.Security(noise.ID, noise.New),
		// support default transports
		libp2p.DefaultTransports,
		// support default muxers
		//		libp2p.DefaultMuxers,
		libp2p.Muxer(yamux.ID, yamuxConfig),
		// connection management
		libp2p.ConnectionManager(connMgr),
		libp2p.ResourceManager(resourceManager),
		// attempt to open ports using uPNP for NATed hosts.
		libp2p.EnableNATService(),
		libp2p.NATPortMap(),
		// control which nodes we allow to connect
		libp2p.ConnectionGater(blacklistManager.Gater),
		// use static relays for more reliable relay selection
		libp2p.EnableAutoRelayWithStaticRelays(relayAddrsInfo),
		libp2p.EnableRelay(),
		// enable AutoNAT v2 for automatic reachability detection
		libp2p.EnableAutoNATv2(),
		// enable hole punching for direct connections when possible
		libp2p.EnableHolePunching(),
		// Add bandwidth reporting for relay traffic billing (only for relay nodes)
		func() libp2p.Option {
			if p2pm.relay {
				relayReporter := NewRelayBandwidthReporter(p2pm.DB, p2pm.Lm, p2pm)
				return libp2p.BandwidthReporter(relayReporter)
			}
			return func(*libp2p.Config) error { return nil } // No-op for non-relay nodes
		}(),
	)
	if err != nil {
		return nil, err
	}
	p2pm.h = hst

	return hst, nil
}

func (p2pm *P2PManager) Stop() error {
	// Stop periodic tasks
	p2pm.StopPeriodicTasks()

	// Cancel subscriptions
	for _, subscription := range p2pm.subscriptions {
		subscription.Cancel()
	}

	// Close topics
	for key, topic := range p2pm.topicsSubscribed {
		if err := topic.Close(); err != nil {
			p2pm.UI.Print(fmt.Sprintf("Could not close topic %s: %s\n", key, err.Error()))
		}
		delete(p2pm.topicsSubscribed, key)
	}

	// Stop p2p node
	if err := p2pm.h.Close(); err != nil {
		return err
	}

	return nil
}

func (p2pm *P2PManager) joinSubscribeTopic(cntx context.Context, ps *pubsub.PubSub, completeTopicName string) (*pubsub.Subscription, *pubsub.Topic, error) {
	topic, err := ps.Join(completeTopicName)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return nil, nil, err
	}

	sub, err := topic.Subscribe()
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return nil, nil, err
	}

	p2pm.topicsSubscribed[completeTopicName] = topic

	go p2pm.receivedMessage(cntx, sub)

	return sub, topic, nil
}

func (p2pm *P2PManager) initDHT(mode string, bootstrapPeers []peer.AddrInfo) (*dht.IpfsDHT, error) {
	var dhtMode dht.Option

	switch mode {
	case "client":
		dhtMode = dht.Mode(dht.ModeClient)
	case "server":
		dhtMode = dht.Mode(dht.ModeServer)
	default:
		dhtMode = dht.Mode(dht.ModeAuto)
	}

	kademliaDHT, err := dht.New(p2pm.ctx, p2pm.h, dhtMode)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return nil, errors.New(err.Error())
	}

	if err = kademliaDHT.Bootstrap(p2pm.ctx); err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return nil, errors.New(err.Error())
	}

	go p2pm.ConnectNodesAsync(bootstrapPeers, 3, 2*time.Second)

	return kademliaDHT, nil
}

func (p2pm *P2PManager) DiscoverPeers() {
	var discoveredPeers []peer.AddrInfo

	routingDiscovery := drouting.NewRoutingDiscovery(p2pm.idht)

	// Advertise all topics
	for _, completeTopicName := range p2pm.completeTopicNames {
		dutil.Advertise(p2pm.ctx, routingDiscovery, completeTopicName)
	}

	// Look for others who have announced and attempt to connect to them
	for _, completeTopicName := range p2pm.completeTopicNames {
		peerChan, err := routingDiscovery.FindPeers(p2pm.ctx, completeTopicName)
		if err != nil {
			p2pm.Lm.Log("warn", err.Error(), "p2p")
			continue
		}

		for peer := range peerChan {
			if peer.ID == "" || peer.ID == p2pm.h.ID() {
				continue
			}
			discoveredPeers = append(discoveredPeers, peer)
		}
	}

	// Connect to peers asynchronously
	go p2pm.ConnectNodesAsync(discoveredPeers, 3, 2*time.Second)
}

// Monitor connection health and reconnect (cron)
func (p2pm *P2PManager) MaintainConnections() {
	for _, peerID := range p2pm.h.Network().Peers() {
		if p2pm.h.Network().Connectedness(peerID) != network.Connected {
			// Attempt to reconnect
			if err := peerID.Validate(); err != nil {
				continue
			}
			p, err := p2pm.GeneratePeerAddrInfo(peerID.String())
			if err != nil {
				p2pm.Lm.Log("error", err.Error(), "p2p")
				continue
			}
			go p2pm.ConnectNodeWithRetry(p2pm.ctx, p, 3, 2*time.Second)
		}
	}
}

func (p2pm *P2PManager) IsNodeConnected(peer peer.AddrInfo) (bool, error) {
	// Try every 500ms, up to 10 times (i.e. 5 seconds total)
	running := p2pm.WaitForHostReady(500*time.Millisecond, 10)
	if !running {
		err := fmt.Errorf("host is not running")
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return false, err
	}

	connected := false
	// Check for connected peers
	connectedPeers := p2pm.h.Network().Peers()
	if slices.Contains(connectedPeers, peer.ID) {
		connected = true
	}

	return connected, nil
}

func (p2pm *P2PManager) ConnectNode(p peer.AddrInfo) error {

	connected, err := p2pm.IsNodeConnected(p)
	if err != nil {
		msg := err.Error()
		p2pm.Lm.Log("error", msg, "p2p")
		return err
	}
	if connected {
		err := fmt.Errorf("node %s is already connected", p.ID.String())
		p2pm.Lm.Log("debug", err.Error(), "p2p")
		return nil // Consider this as success
	}

	if p.ID == p2pm.h.ID() {
		err := fmt.Errorf("can not connect to itself %s == %s", p.ID, p2pm.h.ID())
		p2pm.Lm.Log("debug", err.Error(), "p2p")
		return nil // Consider this as success
	}

	connCtx, cancel := context.WithTimeout(p2pm.ctx, 10*time.Second)
	err = p2pm.h.Connect(connCtx, p)
	cancel()

	if err != nil {
		// This might be usefull when node chabges its ID
		//		p2pm.PurgeNode(p)

		return err
	}

	p2pm.Lm.Log("debug", fmt.Sprintf("Connected to: %s", p.ID.String()), "p2p")

	for _, ma := range p.Addrs {
		p2pm.Lm.Log("debug", fmt.Sprintf("Connected peer's multiaddr is %s", ma.String()), "p2p")
	}

	return nil
}

func (p2pm *P2PManager) ConnectNodeWithRetry(ctx context.Context, peer peer.AddrInfo, maxRetries int, baseDelay time.Duration) error {
	for attempt := range maxRetries {
		select {
		case <-ctx.Done():
			return fmt.Errorf("missing context / context already done")
		default:
		}

		// Wait for host to be ready
		if !p2pm.WaitForHostReady(500*time.Millisecond, 10) {
			err := fmt.Errorf("host not ready, abandoning connection attempt")
			p2pm.Lm.Log("warn", err.Error(), "p2p")
			return err
		}

		// Attempt connection
		err := p2pm.ConnectNode(peer)
		if err != nil {
			// Exponential backoff
			if attempt < maxRetries-1 {
				delay := baseDelay * time.Duration(1<<attempt)
				select {
				case <-time.After(delay):
					continue
				case <-ctx.Done():
					return fmt.Errorf("missing context / context already done")
				}
			}
			continue
		}

		// Successfully connected
		return nil
	}

	err := fmt.Errorf("failed to connect to peer %s after %d attempts", peer.ID, maxRetries)
	p2pm.Lm.Log("warn", err.Error(), "p2p")
	return err
}

func (p2pm *P2PManager) ConnectNodesAsync(peers []peer.AddrInfo, maxRetries int, baseDelay time.Duration) []peer.AddrInfo {
	var mu sync.Mutex
	var connectedPeers []peer.AddrInfo

	// Use a worker pool pattern
	maxWorkers := 20

	var wg sync.WaitGroup

	// Start workers
	for range maxWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for _, peer := range peers {
				if err := p2pm.ConnectNodeWithRetry(p2pm.ctx, peer, maxRetries, baseDelay); err == nil {
					mu.Lock()
					connectedPeers = append(connectedPeers, peer)
					mu.Unlock()
				}
			}
		}()
	}

	wg.Wait()
	return connectedPeers
}

func (p2pm *P2PManager) PurgeNode(p peer.AddrInfo) {
	p2pm.h.Network().ClosePeer(p.ID)
	p2pm.h.Network().Peerstore().RemovePeer(p.ID)
	p2pm.h.Peerstore().ClearAddrs(p.ID)
	p2pm.h.Peerstore().RemovePeer(p.ID)
	p2pm.Lm.Log("debug", fmt.Sprintf("Removed peer %s from a peer store", p.ID), "p2p")
}

func StreamData[T any](
	p2pm *P2PManager,
	receivingPeer peer.AddrInfo,
	data T,
	job *node_types.Job,
	existingStream network.Stream,
) error {
	var workflowId int64 = int64(0)
	var jobId int64 = int64(0)
	var orderingPeer255 [255]byte
	var interfaceId int64 = int64(0)
	var s network.Stream
	var ctx context.Context
	var cancel context.CancelFunc
	var deadline time.Time

	var t uint16 = uint16(0)
	switch v := any(data).(type) {
	case *[]node_types.ServiceOffer:
		t = 0
	case *node_types.JobRunRequest:
		t = 1
	case *[]byte:
		t = 2
	case *os.File:
		t = 3
	case *node_types.ServiceRequest:
		t = 4
	case *node_types.ServiceResponse:
		t = 5
	case *node_types.JobRunResponse:
		t = 6
	case *node_types.JobRunStatus:
		t = 7
	case *node_types.JobRunStatusRequest:
		t = 8
	case *node_types.JobDataReceiptAcknowledgement:
		t = 9
	case *node_types.JobDataRequest:
		t = 10
	case *node_types.ServiceRequestCancellation:
		t = 11
	case *node_types.ServiceResponseCancellation:
		t = 12
	default:
		err := fmt.Errorf("data type %v is not allowed in this context (streaming data)", v)
		p2pm.Lm.Log("error", err.Error(), "p2p")
		if existingStream != nil {
			existingStream.Reset()
		}
		return err
	}

	// Timeout context
	//	if t == 2 || t == 3 {
	if t == 3 {
		ctx, cancel = context.WithTimeout(p2pm.ctx, 5*time.Hour) // Allow loner periods, TODO put in config
	} else {
		ctx, cancel = context.WithTimeout(p2pm.ctx, 1*time.Minute)
	}
	defer cancel()

	err := p2pm.ConnectNodeWithRetry(ctx, receivingPeer, 3, 2*time.Second)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	if existingStream != nil {
		existingStream.SetDeadline(time.Now().Add(100 * time.Millisecond))
		_, err := existingStream.Write([]byte{})
		existingStream.SetDeadline(time.Time{}) // reset deadline

		if err == nil {
			// Stream is alive, reuse it
			s = existingStream
			p2pm.Lm.Log("debug", fmt.Sprintf("Reusing existing stream with %s", receivingPeer.ID), "p2p")
		} else {
			// Stream is broken, discard and create a new one
			existingStream.Reset()
			p2pm.Lm.Log("debug", fmt.Sprintf("Existing stream with %s is not usable: %v", receivingPeer.ID, err), "p2p")
		}
	}

	// Create new stream
	if s == nil {
		s, err = p2pm.h.NewStream(ctx, receivingPeer.ID, p2pm.protocolID)
		if err != nil {
			p2pm.Lm.Log("error", err.Error(), "p2p")
			return err
		}
	}
	// Ensure stream is always cleaned up
	defer func() {
		if s != nil {
			s.Close()
		}
	}()

	// Set timeouts for all stream operations
	//	if t == 2 || t == 3 {
	if t == 3 {
		deadline = time.Now().Add(5 * time.Hour) // TODO, put in configs
		s.SetDeadline(deadline)
	} else {
		deadline = time.Now().Add(1 * time.Minute)
		s.SetDeadline(deadline)
	}
	defer s.SetDeadline(time.Time{}) // Clear deadline

	if job != nil {
		var orderingPeer []byte = []byte(job.OrderingNodeId)
		copy(orderingPeer255[:], orderingPeer)
		workflowId = job.WorkflowId
		jobId = job.Id
		if len(job.JobInterfaces) > 0 {
			interfaceId = job.JobInterfaces[0].InterfaceId
		}
	}

	var sendingPeer []byte = []byte(p2pm.h.ID().String())
	var sendingPeer255 [255]byte
	copy(sendingPeer255[:], sendingPeer)

	// Use channels for proper goroutine coordination
	proposalDone := make(chan error, 1)
	streamDone := make(chan error, 1)

	go func() {
		proposalDone <- p2pm.streamProposal(s, sendingPeer255, t, orderingPeer255, workflowId, jobId, interfaceId)
	}()

	go func() {
		streamDone <- sendStream(p2pm, s, data)
	}()

	// Wait for both operations with timeout
	select {
	case err := <-proposalDone:
		if err != nil {
			return err
		}
	case <-ctx.Done():
		return ctx.Err()
	}

	select {
	case err := <-streamDone:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p2pm *P2PManager) streamProposal(
	s network.Stream,
	p [255]byte,
	t uint16,
	onode [255]byte,
	wid int64,
	jid int64,
	iid int64,
) error {
	// Create an instance of StreamData to write
	streamData := node_types.StreamData{
		Type:           t,
		PeerId:         p,
		OrderingPeerId: onode,
		WorkflowId:     wid,
		JobId:          jid,
		InterfaceId:    iid,
	}

	// Send stream data
	if err := binary.Write(s, binary.BigEndian, streamData); err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		s.Reset()
		return err
	}
	message := fmt.Sprintf("Stream proposal type %d has been sent in stream %s", streamData.Type, s.ID())
	p2pm.Lm.Log("debug", message, "p2p")

	return nil
}

func (p2pm *P2PManager) streamProposalResponse(s network.Stream) {
	// Create timeout context for the entire stream processing
	ctx, cancel := context.WithTimeout(p2pm.ctx, 15*time.Minute) // TODO, put in configs
	defer cancel()

	// Set reasonable timeouts to prevent indefinite blocking without closing the stream prematurely
	s.SetReadDeadline(time.Now().Add(10 * time.Minute))  // TODO, put in configs
	s.SetWriteDeadline(time.Now().Add(10 * time.Minute)) // TODO, put in configs

	// Recovery function to handle panics and reset stream on errors
	defer func() {
		if r := recover(); r != nil {
			p2pm.Lm.Log("error", fmt.Sprintf("Stream handler panic: %v", r), "p2p")
			s.Reset() // Reset only on panic to free resources
		}
	}()

	// Track this stream for cleanup
	streamID := s.ID()
	p2pm.streamsMutex.Lock()
	p2pm.activeStreams[streamID] = time.Now()
	p2pm.streamsMutex.Unlock()

	// Ensure stream is removed from tracking when function exits
	defer func() {
		p2pm.streamsMutex.Lock()
		delete(p2pm.activeStreams, streamID)
		p2pm.streamsMutex.Unlock()
	}()

	// Check if context is cancelled before processing
	select {
	case <-ctx.Done():
		p2pm.Lm.Log("warn", "Stream processing cancelled due to timeout", "p2p")
		s.Reset()
		return
	default:
		// Continue processing
	}
	// Prepare to read the stream data
	var streamData node_types.StreamData
	err := binary.Read(s, binary.BigEndian, &streamData)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		s.Reset()
	}

	message := fmt.Sprintf("Received stream data type %d from %s in stream %s",
		streamData.Type, string(bytes.Trim(streamData.PeerId[:], "\x00")), s.ID())
	p2pm.Lm.Log("debug", message, "p2p")

	// Check settings, do we want to accept receiving service catalogues and updates
	accepted := p2pm.streamProposalAssessment(streamData.Type)
	if accepted {
		p2pm.streamAccepted(s)
		go p2pm.receivedStream(s, streamData)
	} else {
		s.Reset()
	}
}

func (p2pm *P2PManager) streamProposalAssessment(streamDataType uint16) bool {
	var accepted bool
	settingsManager := settings.NewSettingsManager(p2pm.DB, p2pm.Lm)
	// Check what the stream proposal is about
	switch streamDataType {
	case 0:
		// Request to receive a Service Catalogue from the remote peer
		accepted = settingsManager.ReadBoolSetting("accept_service_catalogue")
	case 1:
		// Request to the remote peer to run a job
		accepted = settingsManager.ReadBoolSetting("accept_job_run_request")
	case 2:
		// Request to receive a binary stream from the remote peer
		accepted = settingsManager.ReadBoolSetting("accept_binary_stream")
	case 3:
		// Request to receive a file from the remote peer
		accepted = settingsManager.ReadBoolSetting("accept_file")
	case 4:
		// Request to receive a Service Request from the remote peer
		accepted = settingsManager.ReadBoolSetting("accept_service_request")
	case 5:
		// Request to receive a Service Response from the remote peer
		accepted = settingsManager.ReadBoolSetting("accept_service_response")
	case 6:
		// Request to receive a Job Run Response from the remote peer
		accepted = settingsManager.ReadBoolSetting("accept_job_run_response")
	case 7:
		// Request to send a Job Run Status to the remote peer
		accepted = settingsManager.ReadBoolSetting("accept_job_run_status")
	case 8:
		// Request to receive a Job Run Status update from the remote peer
		accepted = settingsManager.ReadBoolSetting("accept_job_run_status_request")
	case 9:
		// Request to receive a Job Data Receipt Acknowledgment from the remote peer
		accepted = settingsManager.ReadBoolSetting("accept_job_data_receipt_acknowledgement")
	case 10:
		// Request to receive a Job Data Request from the remote peer
		accepted = settingsManager.ReadBoolSetting("accept_job_data_request")
	case 11:
		// Request to receive a Service Request Cancllation from the remote peer
		accepted = settingsManager.ReadBoolSetting("accept_service_request_cancellation")
	case 12:
		// Request to receive a Service Response Cancellation from the remote peer
		accepted = settingsManager.ReadBoolSetting("accept_service_response_cancellation")
	default:
		message := fmt.Sprintf("Unknown stream type %d is proposed", streamDataType)
		p2pm.Lm.Log("debug", message, "p2p")
		return false
	}
	if accepted {
		message := fmt.Sprintf("As per local settings stream data type %d is accepted.", streamDataType)
		p2pm.Lm.Log("debug", message, "p2p")
	} else {
		message := fmt.Sprintf("As per local settings stream data type %d is not accepted", streamDataType)
		p2pm.Lm.Log("debug", message, "p2p")
	}

	return accepted
}

func (p2pm *P2PManager) streamAccepted(s network.Stream) {
	data := [7]byte{'T', 'F', 'R', 'E', 'A', 'D', 'Y'}
	err := binary.Write(s, binary.BigEndian, data)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		s.Reset()
	}

	message := fmt.Sprintf("Sent TFREADY successfully in stream %s", s.ID())
	p2pm.Lm.Log("debug", message, "p2p")
}

func sendStream[T any](p2pm *P2PManager, s network.Stream, data T) error {
	// Ensure stream is properly closed on all exit paths
	defer func() {
		if r := recover(); r != nil {
			p2pm.Lm.Log("error", fmt.Sprintf("Stream panic recovered: %v", r), "p2p")
			s.Reset()
		}
	}()

	var ready [7]byte
	var expected [7]byte = [7]byte{'T', 'F', 'R', 'E', 'A', 'D', 'Y'}

	// Timeout for reading ready signal
	s.SetReadDeadline(time.Now().Add(30 * time.Second))
	err := binary.Read(s, binary.BigEndian, &ready)
	s.SetReadDeadline(time.Time{}) // Clear deadline
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		s.Reset()
		return err
	}

	// Check if received data matches TFREADY signal
	if !bytes.Equal(expected[:], ready[:]) {
		err := errors.New("did not get expected TFREADY signal")
		p2pm.Lm.Log("error", err.Error(), "p2p")
		s.Reset()
		return err
	}

	message := fmt.Sprintf("Received %s from %s", string(ready[:]), s.ID())
	p2pm.Lm.Log("debug", message, "p2p")

	var chunkSize uint64 = 4096
	var pointer uint64 = 0

	// Read chunk size form configs
	cs := p2pm.cm.GetConfigWithDefault("chunk_size", "81920")
	chunkSize, err = strconv.ParseUint(cs, 10, 64)
	if err != nil {
		message := fmt.Sprintf("Invalid chunk size in configs file. Will set to the default chunk size (%s)", err.Error())
		p2pm.Lm.Log("warn", message, "p2p")
	}

	switch v := any(data).(type) {
	case *[]node_types.ServiceOffer, *node_types.ServiceRequest, *node_types.ServiceResponse,
		*node_types.JobRunRequest, *node_types.JobRunResponse,
		*node_types.JobRunStatus, *node_types.JobRunStatusRequest,
		*node_types.JobDataReceiptAcknowledgement, *node_types.JobDataRequest,
		*node_types.ServiceRequestCancellation, *node_types.ServiceResponseCancellation:
		b, err := json.Marshal(data)
		if err != nil {
			p2pm.Lm.Log("error", err.Error(), "p2p")
			s.Reset()
			return err
		}

		err = p2pm.sendStreamChunks(b, pointer, chunkSize, s)
		if err != nil {
			p2pm.Lm.Log("error", err.Error(), "p2p")
			s.Reset()
			return err
		}
	case *[]byte:
		err = p2pm.sendStreamChunks(*v, pointer, chunkSize, s)
		if err != nil {
			p2pm.Lm.Log("error", err.Error(), "p2p")
			s.Reset()
			return err
		}
	case *os.File:
		// Ensure file is always closed
		defer func() {
			if closeErr := v.Close(); closeErr != nil {
				p2pm.Lm.Log("error", fmt.Sprintf("Failed to close file: %v", closeErr), "p2p")
			}
		}()

		buffer := utils.P2PBufferPool.Get()
		defer utils.P2PBufferPool.Put(&buffer)

		writer := utils.GlobalWriterPool.Get(s)
		defer utils.GlobalWriterPool.Put(writer)

		for {
			n, err := v.Read(buffer)
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				p2pm.Lm.Log("error", err.Error(), "p2p")
				s.Reset()
				return err
			}

			// Write timeout
			s.SetWriteDeadline(time.Now().Add(1 * time.Minute)) // TODO, put in configs
			_, err = writer.Write(buffer[:n])
			s.SetWriteDeadline(time.Time{})
			if err != nil {
				p2pm.Lm.Log("error", err.Error(), "p2p")
				s.Reset()
				return err
			}
		}

		if err = writer.Flush(); err != nil {
			p2pm.Lm.Log("error", err.Error(), "p2p")
			s.Reset()
			return err
		}

		// Stream closure
		return s.CloseWrite()

	default:
		msg := fmt.Sprintf("Data type %v is not allowed in this context (streaming data)", v)
		p2pm.Lm.Log("error", msg, "p2p")
		s.Reset()
		return err
	}

	message = fmt.Sprintf("Sending ended %s", s.ID())
	p2pm.Lm.Log("debug", message, "p2p")

	return nil
}

func (p2pm *P2PManager) sendStreamChunks(b []byte, pointer uint64, chunkSize uint64, s network.Stream) error {
	for {
		if len(b)-int(pointer) < int(chunkSize) {
			chunkSize = uint64(len(b) - int(pointer))
		}

		err := binary.Write(s, binary.BigEndian, chunkSize)
		if err != nil {
			p2pm.Lm.Log("error", err.Error(), "p2p")
			return err
		}
		message := fmt.Sprintf("Chunk [%d bytes] successfully sent in stream %s", chunkSize, s.ID())
		p2pm.Lm.Log("debug", message, "p2p")

		if chunkSize == 0 {
			break
		}

		err = binary.Write(s, binary.BigEndian, (b)[pointer:pointer+chunkSize])
		if err != nil {
			p2pm.Lm.Log("error", err.Error(), "p2p")
			return err
		}
		message = fmt.Sprintf("Chunk [%d bytes] successfully sent in stream %s", len((b)[pointer:pointer+chunkSize]), s.ID())
		p2pm.Lm.Log("debug", message, "p2p")

		pointer += chunkSize

	}
	return nil
}

func (p2pm *P2PManager) receiveStreamChunks(s network.Stream, streamData node_types.StreamData) ([]byte, error) {
	var data []byte
	for {
		// Receive chunk size
		var chunkSize uint64
		err := binary.Read(s, binary.BigEndian, &chunkSize)
		if err != nil {
			p2pm.Lm.Log("error", err.Error(), "p2p")
			return nil, err
		}

		message := fmt.Sprintf("Data from %s will be received in chunks of %d bytes", s.ID(), chunkSize)
		p2pm.Lm.Log("debug", message, "p2p")

		if chunkSize == 0 {
			break
		}

		chunk := make([]byte, chunkSize)

		// Receive a data chunk
		err = binary.Read(s, binary.BigEndian, &chunk)
		if err != nil {
			p2pm.Lm.Log("error", err.Error(), "p2p")
			return nil, err
		}

		message = fmt.Sprintf("Received chunk [%d bytes] of type %d from %s", len(chunk), streamData.Type, s.ID())
		p2pm.Lm.Log("debug", message, "p2p")

		// Concatenate received chunk
		data = append(data, chunk...)
	}

	return data, nil
}

func (p2pm *P2PManager) receivedStream(s network.Stream, streamData node_types.StreamData) {
	// Determine local and remote peer Ids
	localPeerId := s.Conn().LocalPeer()
	remotePeerId := s.Conn().RemotePeer()

	// Determine data type
	switch streamData.Type {
	case 0, 1, 4, 5, 6, 7, 8, 9, 10, 11, 12:
		// Prepare to read back the data
		data, err := p2pm.receiveStreamChunks(s, streamData)
		if err != nil {
			s.Reset()
			return
		}

		message := fmt.Sprintf("Receiving completed. Data [%d bytes] of type %d received in stream %s from %s ",
			len(data), streamData.Type, s.ID(), string(bytes.Trim(streamData.PeerId[:], "\x00")))
		p2pm.Lm.Log("debug", message, "p2p")

		switch streamData.Type {
		case 0:
			// Received a Service Offer from the remote peer
			if err := p2pm.serviceOfferReceived(data, localPeerId); err != nil {
				s.Reset()
				return
			}
		case 1:
			// Received Job Run Request from remote peer
			if err := p2pm.jobRunRequestReceived(data, remotePeerId); err != nil {
				s.Reset()
				return
			}
		case 4:
			// Received Service Request from remote peer
			if err := p2pm.serviceRequestReceived(data, remotePeerId); err != nil {
				s.Reset()
				return
			}
		case 5:
			// Received Service Response from remote peer
			if err := p2pm.serviceResponseReceived(data); err != nil {
				s.Reset()
				return
			}
		case 6:
			// Received Job Run Response from remote peer
			if err := p2pm.jobRunResponseReceived(data); err != nil {
				s.Reset()
				return
			}
		case 7:
			// Received Job Run Status Update from remote peer
			if err := p2pm.jobRunStatusUpdateReceived(data); err != nil {
				s.Reset()
				return
			}
		case 8:
			// Received Job Run Status Request from remote peer
			if err := p2pm.jobRunStatusRequestReceived(data, remotePeerId); err != nil {
				s.Reset()
				return
			}
		case 9:
			// Received Job Data Acknowledgement Receipt from remote peer
			if err := p2pm.jobDataAcknowledgementReceiptReceived(data, remotePeerId); err != nil {
				s.Reset()
				return
			}
		case 10:
			// Received Job Data Request from remote peer
			if err := p2pm.jobDataRequestReceived(data, remotePeerId); err != nil {
				s.Reset()
				return
			}
		case 11:
			// Received Service Request Cancelation from remote peer
			if err := p2pm.serviceRequestCancellationReceived(data, remotePeerId); err != nil {
				s.Reset()
				return
			}
		case 12:
			// Received Service Request Cancellation Response from remote peer
			if err := p2pm.serviceRequestCancellationResponseReceived(data); err != nil {
				s.Reset()
				return
			}
		}
	case 2:
		// Received binary stream from the remote peer	// OBSOLETE, to remove
	case 3:
		// Received file from remote peer
		if err := p2pm.fileReceived(s, remotePeerId, streamData); err != nil {
			s.Reset()
			return
		}
	default:
		message := fmt.Sprintf("Unknown stream type %d is received", streamData.Type)
		p2pm.Lm.Log("warn", message, "p2p")
	}

	message := fmt.Sprintf("Receiving ended %s", s.ID())
	p2pm.Lm.Log("debug", message, "p2p")

	s.Reset()
}

// Received file from remote peer
func (p2pm *P2PManager) fileReceived(
	s network.Stream,
	remotePeerId peer.ID,
	streamData node_types.StreamData,
) error {
	var file *os.File
	var err error
	var fdir string
	var fpath string

	remotePeer := remotePeerId.String()

	// Allow node to receive remote files
	// Data should be accepted in following cases:
	// a) this is ordering node (OrderingPeerId == h.ID())
	// b) the data is sent to our job (IDLE WorkflowId/JobId exists in jobs table)
	jobManager := NewJobManager(p2pm)
	orderingNodeId := string(bytes.Trim(streamData.OrderingPeerId[:], "\x00"))
	receivingNodeId := p2pm.h.ID().String()

	// Is this ordering node?
	allowed := receivingNodeId == string(orderingNodeId)
	if !allowed {
		allowed, err = jobManager.JobExpectingInputsFrom(streamData.JobId, remotePeer)
		if err != nil {
			p2pm.Lm.Log("error", err.Error(), "p2p")
			return err
		}
	}

	// Allow it if there is a job expecting this input (not at ordering node)
	if !allowed {
		err := fmt.Errorf("we haven't ordered this data from `%s`. We are also not expecting it as an input for a job",
			orderingNodeId)
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	sWorkflowId := strconv.FormatInt(streamData.WorkflowId, 10)
	sJobId := strconv.FormatInt(streamData.JobId, 10)
	fdir = filepath.Join(p2pm.cm.GetConfigWithDefault("local_storage", "./local_storage/"), "workflows",
		orderingNodeId, sWorkflowId, "job", sJobId, "input", remotePeer)
	fpath = fdir + utils.RandomString(32)

	// Note: chunkSize is now handled by buffer pool, no need to parse it here
	if err = os.MkdirAll(fdir, 0755); err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}
	file, err = os.Create(fpath)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}
	defer file.Close()

	reader := utils.GlobalReaderPool.Get(s)
	defer utils.GlobalReaderPool.Put(reader)

	buf := utils.P2PBufferPool.Get()
	defer utils.P2PBufferPool.Put(&buf)

	for {
		n, err := reader.Read(buf)
		if err != nil {
			if err == io.EOF {
				break
			}
			p2pm.Lm.Log("error", err.Error(), "p2p")
			return err
		}
		file.Write(buf[:n])
	}

	// Uncompress received file
	err = utils.Uncompress(fpath, fdir)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	// Send acknowledgement receipt
	go p2pm.sendReceiptAcknowledgement(
		streamData.WorkflowId,
		streamData.JobId,
		streamData.InterfaceId,
		orderingNodeId,
		receivingNodeId,
		remotePeer,
	)

	err = os.RemoveAll(fpath)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	return nil
}

// Received Service Offer from remote peer
func (p2pm *P2PManager) serviceOfferReceived(data []byte, localPeerId peer.ID) error {
	var serviceOffer []node_types.ServiceOffer
	err := json.Unmarshal(data, &serviceOffer)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	for _, service := range serviceOffer {
		// Remote node ID
		nodeId := service.NodeId
		localNodeId := localPeerId.String()
		// Skip if this is our service advertized on remote node
		if localNodeId == nodeId {
			continue
		}

		// Add/Update Service Offers Cache
		p2pm.sc.PruneExpired(time.Hour)
		service = p2pm.sc.AddOrUpdate(service)

		// If we are in interactive mode print Service Offer to CLI
		uiType, err := ui.DetectUIType(p2pm.UI)
		if err != nil {
			p2pm.Lm.Log("error", err.Error(), "p2p")
			return err
		}

		switch uiType {
		case "CLI":
			menuManager := NewMenuManager(p2pm)
			menuManager.printOfferedService(service)
		case "GUI":
			// Push message
			p2pm.UI.ServiceOffer(service)
		default:
			err := fmt.Errorf("unknown UI type %s", uiType)
			p2pm.Lm.Log("error", err.Error(), "p2p")
			return err
		}
	}

	return nil
}

// Received Service Request from remote peer
func (p2pm *P2PManager) serviceRequestReceived(data []byte, remotePeerId peer.ID) error {
	var serviceRequest node_types.ServiceRequest

	remotePeer := remotePeerId.String()
	peer, err := p2pm.GeneratePeerAddrInfo(remotePeer)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	err = json.Unmarshal(data, &serviceRequest)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	serviceResponse := node_types.ServiceResponse{
		JobId:          int64(0),
		Accepted:       false,
		Message:        "",
		OrderingNodeId: remotePeer,
		ServiceRequest: serviceRequest,
	}

	// Check if it is existing and active service
	serviceManager := NewServiceManager(p2pm)
	err, exist := serviceManager.Exists(serviceRequest.ServiceId)
	if err != nil {
		serviceResponse.Message = err.Error()

		// Stream error message
		errs := StreamData(p2pm, peer, &serviceResponse, nil, nil)
		if errs != nil {
			p2pm.Lm.Log("error", errs.Error(), "p2p")
			return errs
		}

		return err
	}

	if !exist {
		err := fmt.Errorf("could not find service Id %d", serviceRequest.ServiceId)
		p2pm.Lm.Log("error", err.Error(), "p2p")

		serviceResponse.Message = err.Error()

		// Stream error message
		errs := StreamData(p2pm, peer, &serviceResponse, nil, nil)
		if errs != nil {
			p2pm.Lm.Log("error", errs.Error(), "p2p")
			return errs
		}

		return err
	}

	// Get service
	service, err := serviceManager.Get(serviceRequest.ServiceId)
	if err != nil {
		serviceResponse.Message = err.Error()

		// Stream error message
		errs := StreamData(p2pm, peer, &serviceResponse, nil, nil)
		if errs != nil {
			p2pm.Lm.Log("error", errs.Error(), "p2p")
			return errs
		}

		return err
	}

	// If service type is DATA populate file hash(es) in response message
	if service.Type == "DATA" {
		dataService, err := serviceManager.GetData(serviceRequest.ServiceId)
		if err != nil {
			serviceResponse.Message = err.Error()

			// Stream error message
			errs := StreamData(p2pm, peer, &serviceResponse, nil, nil)
			if errs != nil {
				p2pm.Lm.Log("error", errs.Error(), "p2p")
				return errs
			}

			return err
		}

		// Set data path in response message
		serviceResponse.Message = dataService.Path
	} else {
		serviceResponse.Message = ""
	}

	// TODO, service price and payment

	// Create a job
	jobManager := NewJobManager(p2pm)
	job, err := jobManager.CreateJob(serviceRequest, remotePeer)
	if err != nil {
		serviceResponse.Message = err.Error()

		// Stream error message
		errs := StreamData(p2pm, peer, &serviceResponse, nil, nil)
		if errs != nil {
			p2pm.Lm.Log("error", errs.Error(), "p2p")
			return errs
		}

		return err
	}

	serviceResponse.JobId = job.Id
	serviceResponse.Accepted = true

	// Send successfull service response message
	err = StreamData(p2pm, peer, &serviceResponse, nil, nil)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	return nil
}

// Received Service Response from remote peer
func (p2pm *P2PManager) serviceResponseReceived(data []byte) error {
	var serviceResponse node_types.ServiceResponse

	err := json.Unmarshal(data, &serviceResponse)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	if !serviceResponse.Accepted {
		err := fmt.Errorf("service request (service request Id %d) for node Id %s is not accepted with the following reason: %s",
			serviceResponse.ServiceId, serviceResponse.NodeId, serviceResponse.Message)
		p2pm.Lm.Log("error", err.Error(), "p2p")
	} else {
		// Add job to workflow
		err := p2pm.wm.RegisteredWorkflowJob(serviceResponse.WorkflowId, serviceResponse.WorkflowJobId,
			serviceResponse.NodeId, serviceResponse.ServiceId, serviceResponse.JobId, serviceResponse.Message)
		if err != nil {
			p2pm.Lm.Log("error", err.Error(), "p2p")
			return err
		}
	}

	// Draw output / push output message
	uiType, err := ui.DetectUIType(p2pm.UI)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	switch uiType {
	case "CLI":
		menuManager := NewMenuManager(p2pm)
		menuManager.printServiceResponse(serviceResponse)
	case "GUI":
		// TODO, GUI push message
	default:
		err := fmt.Errorf("unknown UI type %s", uiType)
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	return nil
}

// Received Job Run Request from remote peer
func (p2pm *P2PManager) jobRunRequestReceived(data []byte, remotePeerId peer.ID) error {
	var jobRunRequest node_types.JobRunRequest

	remotePeer := remotePeerId.String()
	peer, err := p2pm.GeneratePeerAddrInfo(remotePeer)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	err = json.Unmarshal(data, &jobRunRequest)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	jobRunResponse := node_types.JobRunResponse{
		Accepted:      false,
		Message:       "",
		JobRunRequest: jobRunRequest,
	}

	// Check if job is existing
	jobManager := NewJobManager(p2pm)
	if err, exists := jobManager.JobExists(jobRunRequest.JobId); err != nil || !exists {
		jobRunResponse.Message = err.Error()

		// Stream error message
		errs := StreamData(p2pm, peer, &jobRunResponse, nil, nil)
		if errs != nil {
			p2pm.Lm.Log("error", errs.Error(), "p2p")
			return errs
		}

		return err
	}

	// Check if job is owned by requesting peer
	job, err := jobManager.GetJob(jobRunRequest.JobId)
	if err != nil {
		jobRunResponse.Message = err.Error()

		// Stream error message
		errs := StreamData(p2pm, peer, &jobRunResponse, nil, nil)
		if errs != nil {
			p2pm.Lm.Log("error", errs.Error(), "p2p")
			return errs
		}

		return err
	}

	if job.OrderingNodeId != remotePeer {
		err := fmt.Errorf("job run request for job Id %d is made by node %s who is not owning the job",
			jobRunRequest.JobId, remotePeer)
		p2pm.Lm.Log("error", err.Error(), "p2p")

		jobRunResponse.Message = err.Error()

		// Stream error message
		errs := StreamData(p2pm, peer, &jobRunResponse, nil, nil)
		if errs != nil {
			p2pm.Lm.Log("error", errs.Error(), "p2p")
			return errs
		}

		return err
	}

	// Set READY flag for the job
	err = jobManager.UpdateJobStatus(job.Id, "READY")
	if err != nil {
		jobRunResponse.Message = err.Error()

		// Stream error message
		errs := StreamData(p2pm, peer, &jobRunResponse, nil, nil)
		if errs != nil {
			p2pm.Lm.Log("error", errs.Error(), "p2p")
			return errs
		}

		return err
	}

	jobRunResponse.Message = fmt.Sprintf("READY flag is set for job Id %d.", jobRunRequest.JobId)
	jobRunResponse.Accepted = true

	// Stream job run accept message
	errs := StreamData(p2pm, peer, &jobRunResponse, nil, nil)
	if errs != nil {
		p2pm.Lm.Log("error", errs.Error(), "p2p")
		return errs
	}

	return nil
}

// Received Job Run Response from remote peer
func (p2pm *P2PManager) jobRunResponseReceived(data []byte) error {
	var jobRunResponse node_types.JobRunResponse

	err := json.Unmarshal(data, &jobRunResponse)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	// Update workflow job status
	workflowManager := workflow.NewWorkflowManager(p2pm.DB, p2pm.Lm, p2pm.cm)
	if !jobRunResponse.Accepted {
		err := fmt.Errorf("job run request Id %d for node Id %s is not accepted with the following reason: %s",
			jobRunResponse.JobId, jobRunResponse.NodeId, jobRunResponse.Message)
		p2pm.Lm.Log("error", err.Error(), "p2p")

		err = workflowManager.UpdateWorkflowJobStatus(jobRunResponse.WorkflowId, jobRunResponse.NodeId, jobRunResponse.JobId, "ERRORED")
		if err != nil {
			p2pm.Lm.Log("error", err.Error(), "p2p")
			return err
		}
	}

	// Draw output / push mesage
	uiType, err := ui.DetectUIType(p2pm.UI)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	switch uiType {
	case "CLI":
		menuManager := NewMenuManager(p2pm)
		menuManager.printJobRunResponse(jobRunResponse)
	case "GUI":
		// TODO, GUI push message
	default:
		err := fmt.Errorf("unknown UI type %s", uiType)
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	return nil
}

// Received Job Run Status Request from remote peer
func (p2pm *P2PManager) jobRunStatusRequestReceived(data []byte, remotePeerId peer.ID) error {
	var jobRunStatusRequest node_types.JobRunStatusRequest

	err := json.Unmarshal(data, &jobRunStatusRequest)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	remotePeer := remotePeerId.String()
	peer, err := p2pm.GeneratePeerAddrInfo(remotePeer)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	jobRunStatusResponse := node_types.JobRunStatus{
		JobRunStatusRequest: jobRunStatusRequest,
		Status:              "",
	}

	// Check if job is existing
	jobManager := NewJobManager(p2pm)
	if err, exists := jobManager.JobExists(jobRunStatusRequest.JobId); err != nil || !exists {
		err := fmt.Errorf("could not find job Id %d (%s)", jobRunStatusRequest.JobId, err.Error())
		p2pm.Lm.Log("error", err.Error(), "p2p")

		jobRunStatusResponse.Status = "ERRORED"

		// Stream error message
		errs := StreamData(p2pm, peer, &jobRunStatusResponse, nil, nil)
		if errs != nil {
			p2pm.Lm.Log("error", errs.Error(), "p2p")
			return errs
		}

		return err
	}

	// Check if job is owned by requesting peer
	job, err := jobManager.GetJob(jobRunStatusRequest.JobId)
	if err != nil {
		jobRunStatusResponse.Status = "ERRORED"

		// Stream error message
		errs := StreamData(p2pm, peer, &jobRunStatusResponse, nil, nil)
		if errs != nil {
			p2pm.Lm.Log("error", errs.Error(), "p2p")
			return errs
		}

		return err
	}

	if job.OrderingNodeId != remotePeer {
		err := fmt.Errorf("job run status request for job Id %d is made by node %s who is not owning the job",
			jobRunStatusRequest.JobId, remotePeer)
		p2pm.Lm.Log("error", err.Error(), "p2p")

		jobRunStatusResponse.Status = "ERRORED"

		// Stream error message
		errs := StreamData(p2pm, peer, &jobRunStatusResponse, nil, nil)
		if errs != nil {
			p2pm.Lm.Log("error", errs.Error(), "p2p")
			return errs
		}

		return err
	}

	// Stream status update message
	err = jobManager.StatusUpdate(job.Id, job.Status)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	return nil
}

// Received Job Run Status Update from remote peer
func (p2pm *P2PManager) jobRunStatusUpdateReceived(data []byte) error {
	var jobRunStatus node_types.JobRunStatus

	err := json.Unmarshal(data, &jobRunStatus)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	// Update workflow job status
	workflowManager := workflow.NewWorkflowManager(p2pm.DB, p2pm.Lm, p2pm.cm)
	err = workflowManager.UpdateWorkflowJobStatus(jobRunStatus.WorkflowId, jobRunStatus.NodeId, jobRunStatus.JobId, jobRunStatus.Status)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	return nil
}

// Received Job Data Request from remote peer
func (p2pm *P2PManager) jobDataRequestReceived(data []byte, remotePeerId peer.ID) error {
	var jobDataRequest node_types.JobDataRequest

	err := json.Unmarshal(data, &jobDataRequest)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	// Send the data if peer is eligible for it
	jobManager := NewJobManager(p2pm)
	err = jobManager.SendIfPeerEligible(
		jobDataRequest.JobId,
		jobDataRequest.InterfaceId,
		remotePeerId,
		jobDataRequest.WhatData,
	)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	return nil
}

// Received Job Data Acknowledgment Receipt from remote peer
func (p2pm *P2PManager) jobDataAcknowledgementReceiptReceived(data []byte, remotePeerId peer.ID) error {
	var jobDataReceiptAcknowledgement node_types.JobDataReceiptAcknowledgement

	err := json.Unmarshal(data, &jobDataReceiptAcknowledgement)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	// Acknowledge receipt
	jobManager := NewJobManager(p2pm)
	err = jobManager.AcknowledgeReceipt(
		jobDataReceiptAcknowledgement.JobId,
		jobDataReceiptAcknowledgement.InterfaceId,
		remotePeerId,
		"OUTPUT",
	)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	return nil
}

// Received Service Request Cancelation from remote peer
func (p2pm *P2PManager) serviceRequestCancellationReceived(data []byte, remotePeerId peer.ID) error {
	var serviceRequestCancellation node_types.ServiceRequestCancellation

	remotePeer := remotePeerId.String()
	peer, err := p2pm.GeneratePeerAddrInfo(remotePeer)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	err = json.Unmarshal(data, &serviceRequestCancellation)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	serviceResponseCancellation := node_types.ServiceResponseCancellation{
		JobId:                      serviceRequestCancellation.JobId,
		Accepted:                   false,
		Message:                    "",
		OrderingNodeId:             remotePeer,
		ServiceRequestCancellation: serviceRequestCancellation,
	}

	// Get job
	jobManager := NewJobManager(p2pm)
	job, err := jobManager.GetJob(serviceRequestCancellation.JobId)
	if err != nil {
		serviceResponseCancellation.Message = err.Error()

		// Stream error message
		errs := StreamData(p2pm, peer, &serviceResponseCancellation, nil, nil)
		if errs != nil {
			p2pm.Lm.Log("error", errs.Error(), "p2p")
			return errs
		}

		return err
	}

	// Check if job is owned by ordering peer
	if job.OrderingNodeId != remotePeer {
		err := fmt.Errorf("job cancellation request for job Id %d is made by node %s who is not owning the job",
			serviceRequestCancellation.JobId, remotePeer)
		p2pm.Lm.Log("error", err.Error(), "p2p")

		serviceResponseCancellation.Message = err.Error()

		// Stream error message
		errs := StreamData(p2pm, peer, &serviceResponseCancellation, nil, nil)
		if errs != nil {
			p2pm.Lm.Log("error", errs.Error(), "p2p")
			return errs
		}

		return err
	}

	// Remove a job
	err = jobManager.RemoveJob(serviceRequestCancellation.JobId)
	if err != nil {
		serviceResponseCancellation.Message = err.Error()

		// Stream error message
		errs := StreamData(p2pm, peer, &serviceResponseCancellation, nil, nil)
		if errs != nil {
			p2pm.Lm.Log("error", errs.Error(), "p2p")
			return errs
		}

		return err
	}

	// Stream successfull cancellation message
	serviceResponseCancellation.Accepted = true
	err = StreamData(p2pm, peer, &serviceResponseCancellation, nil, nil)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	return nil
}

// Received Service Request Cancellation Response from remote peer
func (p2pm *P2PManager) serviceRequestCancellationResponseReceived(data []byte) error {
	var serviceResponseCancellation node_types.ServiceResponseCancellation

	err := json.Unmarshal(data, &serviceResponseCancellation)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	if !serviceResponseCancellation.Accepted {
		err := fmt.Errorf("service request cancellation (job Id %d) for node Id %s is not accepted with the following reason: %s",
			serviceResponseCancellation.JobId, serviceResponseCancellation.NodeId, serviceResponseCancellation.Message)
		p2pm.Lm.Log("error", err.Error(), "p2p")
	} else {
		// Remove a workflow job
		err := p2pm.wm.RemoveWorkflowJob(serviceResponseCancellation.WorkflowJobId)
		if err != nil {
			p2pm.Lm.Log("error", err.Error(), "p2p")
			return err
		}
	}

	// Draw table / push message
	uiType, err := ui.DetectUIType(p2pm.UI)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}
	switch uiType {
	case "CLI":
		// TODO, CLI
	case "GUI":
		// TODO, GUI push message
	default:
		err := fmt.Errorf("unknown UI type %s", uiType)
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	return nil
}

func (p2pm *P2PManager) sendReceiptAcknowledgement(
	workflowId int64,
	jobId int64,
	interfaceId int64,
	orderingNodeId string,
	receivingNodeId string,
	jobRunningNodeId string,
) {
	receiptAcknowledgement := node_types.JobDataReceiptAcknowledgement{
		JobRunStatusRequest: node_types.JobRunStatusRequest{
			WorkflowId: workflowId,
			NodeId:     jobRunningNodeId,
			JobId:      jobId,
		},
		InterfaceId:     interfaceId,
		OrderingNodeId:  orderingNodeId,
		ReceivingNodeId: receivingNodeId,
	}

	peer, err := p2pm.GeneratePeerAddrInfo(jobRunningNodeId)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return
	}

	err = StreamData(p2pm, peer, &receiptAcknowledgement, nil, nil)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return
	}
}

func BroadcastMessage[T any](p2pm *P2PManager, message T) error {
	var m []byte
	var err error
	var topic *pubsub.Topic

	switch v := any(message).(type) {
	case node_types.ServiceLookup:
		m, err = json.Marshal(message)
		if err != nil {
			p2pm.Lm.Log("error", err.Error(), "p2p")
			return err
		}

		topicKey := p2pm.cm.GetConfigWithDefault("topic_name_prefix", "trustflow.network.") + "lookup.service"
		topic = p2pm.topicsSubscribed[topicKey]

	default:
		msg := fmt.Sprintf("Message type %v is not allowed in this context (broadcasting message)", v)
		p2pm.Lm.Log("error", msg, "p2p")
		err := errors.New(msg)
		return err
	}

	if err := topic.Publish(p2pm.ctx, m); err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return err
	}

	return nil
}

func (p2pm *P2PManager) receivedMessage(ctx context.Context, sub *pubsub.Subscription) {
	for {
		if sub == nil {
			err := fmt.Errorf("subscription is nil. Will stop listening")
			p2pm.Lm.Log("error", err.Error(), "p2p")
			break
		}

		m, err := sub.Next(ctx)
		if err != nil {
			p2pm.Lm.Log("error", err.Error(), "p2p")
		}
		if m == nil {
			err := fmt.Errorf("message received is nil. Will stop listening")
			p2pm.Lm.Log("error", err.Error(), "p2p")
			break
		}

		message := string(m.Message.Data)
		peerId := m.GetFrom()
		p2pm.Lm.Log("debug", fmt.Sprintf("Message %s received from %s via %s", message, peerId.String(), m.ReceivedFrom), "p2p")

		topic := string(*m.Topic)
		p2pm.Lm.Log("debug", fmt.Sprintf("Message topic is %s", topic), "p2p")

		switch topic {
		case p2pm.cm.GetConfigWithDefault("topic_name_prefix", "trustflow.network.") + "lookup.service":
			services, err := p2pm.ServiceLookup(m.Message.Data, true)
			if err != nil {
				p2pm.Lm.Log("error", err.Error(), "p2p")
				continue
			}

			// Retrieve known multiaddresses from the peerstore
			peerAddrInfo, err := p2pm.GeneratePeerAddrInfo(peerId.String())
			if err != nil {
				msg := err.Error()
				p2pm.Lm.Log("error", msg, "p2p")
				continue
			}

			// Stream back offered services with prices
			// (only if there we have matching services to offer)
			if len(services) > 0 {
				err = StreamData(p2pm, peerAddrInfo, &services, nil, nil)
				if err != nil {
					msg := err.Error()
					p2pm.Lm.Log("error", msg, "p2p")
					continue
				}
			}
		default:
			msg := fmt.Sprintf("Unknown topic %s", topic)
			p2pm.Lm.Log("error", msg, "p2p")
			continue
		}
	}
}

func (p2pm *P2PManager) ServiceLookup(data []byte, active bool) ([]node_types.ServiceOffer, error) {
	var services []node_types.ServiceOffer
	var lookup node_types.ServiceLookup

	if len(data) == 0 {
		return services, nil
	}

	err := json.Unmarshal(data, &lookup)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return nil, err
	}

	var searchService node_types.SearchService = node_types.SearchService{
		Phrases: lookup.Phrases,
		Type:    lookup.Type,
		Active:  active,
	}

	// Search services
	var offset uint32 = 0
	var limit uint32 = 1
	l := p2pm.cm.GetConfigWithDefault("search_results", "10")
	l64, err := strconv.ParseUint(l, 10, 32)
	if err != nil {
		limit = 10
	} else {
		limit = uint32(l64)
	}
	serviceManager := NewServiceManager(p2pm)

	for {
		servicesBatch, err := serviceManager.SearchServices(searchService, offset, limit)
		if err != nil {
			p2pm.Lm.Log("error", err.Error(), "p2p")
			break
		}

		services = append(services, servicesBatch...)

		if len(servicesBatch) == 0 {
			break
		}

		offset += uint32(len(servicesBatch))
	}

	return services, nil
}

func (p2pm *P2PManager) GeneratePeerAddrInfo(peerId string) (peer.AddrInfo, error) {
	var p peer.AddrInfo

	// Convert the string to a peer.ID
	pID, err := p2pm.GeneratePeerId(peerId)
	if err != nil {
		p2pm.Lm.Log("error", err.Error(), "p2p")
		return p, err
	}

	addrInfo, err := p2pm.idht.FindPeer(p2pm.ctx, pID)
	if err != nil {
		msg := err.Error()
		p2pm.Lm.Log("error", msg, "p2p")
		return p, err
	}

	// Create peer.AddrInfo from peer.ID
	p = peer.AddrInfo{
		ID: pID,
		// Add any multiaddresses if known (leave blank here if unknown)
		Addrs: addrInfo.Addrs,
	}

	return p, nil
}

func (p2pm *P2PManager) makeRelayPeerInfo(peerAddrStr string) (peer.AddrInfo, error) {
	maddr, err := ma.NewMultiaddr(peerAddrStr)
	if err != nil {
		return peer.AddrInfo{}, err
	}
	peerAddrInfo, err := peer.AddrInfoFromP2pAddr(maddr)
	if err != nil {
		return peer.AddrInfo{}, err
	}
	return *peerAddrInfo, nil
}

func (p2pm *P2PManager) GeneratePeerId(peerId string) (peer.ID, error) {
	// Convert the string to a peer.ID
	pID, err := peer.Decode(peerId)
	if err != nil {
		return pID, err
	}

	return pID, nil
}
