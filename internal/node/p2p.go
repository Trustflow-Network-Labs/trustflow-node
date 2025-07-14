package node

import (
	"bufio"
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
	relayv2 "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
	identify "github.com/libp2p/go-libp2p/p2p/protocol/identify"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	libp2ptls "github.com/libp2p/go-libp2p/p2p/security/tls"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/robfig/cron"
)

type P2PManager struct {
	daemon             bool
	public             bool
	relay              bool
	bootstrapAddrs     []string
	relayAddrs         []string
	topicNames         []string
	completeTopicNames []string
	topicsSubscribed   map[string]*pubsub.Topic
	subscriptions      []*pubsub.Subscription
	protocolID         protocol.ID
	idht               *dht.IpfsDHT
	h                  host.Host
	ctx                context.Context
	DB                 *sql.DB
	crons              []*cron.Cron
	lm                 *utils.LogsManager
	wm                 *workflow.WorkflowManager
	sc                 *node_types.ServiceOffersCache
	UI                 ui.UI
}

func NewP2PManager(ctx context.Context, ui ui.UI) *P2PManager {
	// Create a database connection
	sqlm := database.NewSQLiteManager()
	db, err := sqlm.CreateConnection()
	if err != nil {
		panic(err)
	}

	p2pm := &P2PManager{
		daemon: false,
		public: false,
		relay:  false,
		bootstrapAddrs: []string{
			"/ip4/167.86.116.185/udp/30611/quic-v1/p2p/QmPpcuRSHmrjT2EEoHXhU5YT2zV9wF5N9LWuhPJofAhtci",
			"/ip4/167.86.116.185/tcp/30609/p2p/QmPpcuRSHmrjT2EEoHXhU5YT2zV9wF5N9LWuhPJofAhtci",
			"/ip4/167.86.116.185/tcp/30613/ws/p2p/QmPpcuRSHmrjT2EEoHXhU5YT2zV9wF5N9LWuhPJofAhtci",
			"/ip4/85.237.211.221/udp/30611/quic-v1/p2p/QmaZffJXMWB1ifXP1c7U34NsgUZBSaA5QhXBwp269efHX9",
			"/ip4/85.237.211.221/tcp/30609/p2p/QmaZffJXMWB1ifXP1c7U34NsgUZBSaA5QhXBwp269efHX9",
			"/ip4/85.237.211.221/tcp/30613/ws/p2p/QmaZffJXMWB1ifXP1c7U34NsgUZBSaA5QhXBwp269efHX9",
		},
		relayAddrs: []string{
			"/ip4/167.86.116.185/udp/30611/quic-v1/p2p/QmPpcuRSHmrjT2EEoHXhU5YT2zV9wF5N9LWuhPJofAhtci",
			"/ip4/167.86.116.185/tcp/30609/p2p/QmPpcuRSHmrjT2EEoHXhU5YT2zV9wF5N9LWuhPJofAhtci",
			"/ip4/167.86.116.185/tcp/30613/ws/p2p/QmPpcuRSHmrjT2EEoHXhU5YT2zV9wF5N9LWuhPJofAhtci",
			"/ip4/85.237.211.221/udp/30611/quic-v1/p2p/QmaZffJXMWB1ifXP1c7U34NsgUZBSaA5QhXBwp269efHX9",
			"/ip4/85.237.211.221/tcp/30609/p2p/QmaZffJXMWB1ifXP1c7U34NsgUZBSaA5QhXBwp269efHX9",
			"/ip4/85.237.211.221/tcp/30613/ws/p2p/QmaZffJXMWB1ifXP1c7U34NsgUZBSaA5QhXBwp269efHX9",
		},
		topicNames:         []string{"lookup.service"},
		completeTopicNames: []string{},
		topicsSubscribed:   make(map[string]*pubsub.Topic),
		subscriptions:      []*pubsub.Subscription{},
		protocolID:         "",
		idht:               nil,
		h:                  nil,
		ctx:                nil,
		DB:                 db,
		crons:              []*cron.Cron{},
		lm:                 utils.NewLogsManager(),
		wm:                 workflow.NewWorkflowManager(db),
		sc:                 node_types.NewServiceOffersCache(),
		UI:                 ui,
	}

	if ctx != nil {
		p2pm.ctx = ctx
	}

	return p2pm
}

// IsHostRunning checks if the provided host is actively running
func (p2pm *P2PManager) IsHostRunning() bool {
	if p2pm.h == nil {
		return false
	}
	// Check if the host is listening on any network addresses
	return len(p2pm.h.Network().ListenAddresses()) > 0
}

// Start p2p node
func (p2pm *P2PManager) Start(port uint16, daemon bool, public bool, relay bool) {
	p2pm.daemon = daemon
	p2pm.public = public
	p2pm.relay = relay

	if p2pm.ctx == nil {
		p2pm.ctx = context.Background()
	}

	// Read configs
	configManager := utils.NewConfigManager("")
	config, err := configManager.ReadConfigs()
	if err != nil {
		message := fmt.Sprintf("Can not read configs file. (%s)", err.Error())
		p2pm.lm.Log("error", message, "p2p")
		return
	}

	blacklistManager, err := blacklist_node.NewBlacklistNodeManager(p2pm.DB, p2pm.UI)
	if err != nil {
		p2pm.lm.Log("error", err.Error(), "p2p")
		return
	}

	// Read port number from configs
	if port == 0 {
		p := config["node_port"]
		p64, err := strconv.ParseUint(p, 10, 16)
		if err != nil {
			port = 30609
		} else {
			port = uint16(p64)
		}
	}

	// Read topics' names to subscribe
	p2pm.completeTopicNames = p2pm.completeTopicNames[:0]
	topicNamePrefix := config["topic_name_prefix"]
	for _, topicName := range p2pm.topicNames {
		topicName = topicNamePrefix + strings.TrimSpace(topicName)
		p2pm.completeTopicNames = append(p2pm.completeTopicNames, topicName)
	}

	// Read streaming protocol
	p2pm.protocolID = protocol.ID(config["protocol_id"])

	// Create or get previously created node key
	keystoreManager := keystore.NewKeyStoreManager(p2pm.DB)
	priv, _, err := keystoreManager.ProvideKey()
	if err != nil {
		p2pm.lm.Log("panic", err.Error(), "p2p")
		panic(fmt.Sprintf("%v", err))
	}

	// Get botstrap & relay hosts addr info
	var bootstrapAddrsInfo []peer.AddrInfo
	var relayAddrsInfo []peer.AddrInfo
	for _, relayAddr := range p2pm.relayAddrs {
		relayAddrInfo := p2pm.makeRelayPeerInfo(relayAddr)
		relayAddrsInfo = append(relayAddrsInfo, relayAddrInfo)
	}
	for _, bootstrapAddr := range p2pm.bootstrapAddrs {
		bootstrapAddrInfo := p2pm.makeRelayPeerInfo(bootstrapAddr)
		bootstrapAddrsInfo = append(bootstrapAddrsInfo, bootstrapAddrInfo)
	}

	var hst host.Host
	if p2pm.public {
		hst, err = p2pm.createPublicHost(priv, port, blacklistManager)
	} else {
		hst, err = p2pm.createPrivateHost(priv, port, blacklistManager, relayAddrsInfo)
	}
	if err != nil {
		p2pm.lm.Log("error", err.Error(), "p2p")
		panic(fmt.Sprintf("%v", err))
	}

	// Set up identify service
	_, err = identify.NewIDService(hst)
	if err != nil {
		p2pm.lm.Log("error", err.Error(), "p2p")
		p2pm.UI.Print(err.Error())
	}

	message := fmt.Sprintf("Your Peer ID: %s", hst.ID())
	p2pm.lm.Log("info", message, "p2p")
	p2pm.UI.Print(message)
	for _, addr := range hst.Addrs() {
		fullAddr := addr.Encapsulate(ma.StringCast("/p2p/" + hst.ID().String()))
		message := fmt.Sprintf("Listening on %s", fullAddr)
		p2pm.lm.Log("info", message, "p2p")
		p2pm.UI.Print(message)
	}

	// Connect bootstrap nodes
	p2pm.connectNodes(bootstrapAddrsInfo)

	// Connect relay nodes
	p2pm.connectNodes(relayAddrsInfo)

	// Setup a stream handler.
	// This gets called every time a peer connects and opens a stream to this node.
	p2pm.h.SetStreamHandler(p2pm.protocolID, func(s network.Stream) {
		message := fmt.Sprintf("Remote peer `%s` started streaming to our node `%s` in stream id `%s`, using protocol id: `%s`",
			s.Conn().RemotePeer().String(), hst.ID(), s.ID(), p2pm.protocolID)
		p2pm.lm.Log("info", message, "p2p")
		go p2pm.streamProposalResponse(s)
	})

	peerChannel := make(chan []peer.AddrInfo)

	routingDiscovery := drouting.NewRoutingDiscovery(p2pm.idht)
	ps, err := pubsub.NewGossipSub(p2pm.ctx, p2pm.h,
		pubsub.WithPeerExchange(true),
		pubsub.WithFloodPublish(true),
		pubsub.WithDiscovery(routingDiscovery),
	)
	if err != nil {
		p2pm.lm.Log("panic", err.Error(), "p2p")
		panic(fmt.Sprintf("%v", err))
	}

	go p2pm.discoverPeers(peerChannel)

	p2pm.subscriptions = p2pm.subscriptions[:0]
	for _, completeTopicName := range p2pm.completeTopicNames {
		sub, topic, err := p2pm.joinSubscribeTopic(p2pm.ctx, ps, completeTopicName)
		if err != nil {
			p2pm.lm.Log("panic", err.Error(), "p2p")
			panic(fmt.Sprintf("%v", err))
		}
		p2pm.subscriptions = append(p2pm.subscriptions, sub)

		notifyManager := utils.NewTopicAwareNotifiee(ps, topic, completeTopicName, peerChannel)

		// Attach the notifiee to the host's network
		p2pm.h.Network().Notify(notifyManager)
	}

	// Start crons
	cronManager := NewCronManager(p2pm)
	c, err := cronManager.JobQueue()
	if err != nil {
		p2pm.lm.Log("panic", err.Error(), "p2p")
		panic(fmt.Sprintf("%v", err))
	}
	p2pm.crons = append(p2pm.crons, c)

	if !daemon {
		// Print interactive menu
		menuManager := NewMenuManager(p2pm)
		menuManager.Run()
	} else {
		// Running as a daemon never ends
		<-p2pm.ctx.Done()
	}

	// When host is stopped close DB connection
	if err = p2pm.DB.Close(); err != nil {
		p2pm.lm.Log("error", err.Error(), "p2p")
	}
}

func (p2pm *P2PManager) createPublicHost(
	priv crypto.PrivKey,
	port uint16,
	blacklistManager *blacklist_node.BlacklistNodeManager,
) (host.Host, error) {
	message := "Creating public p2p host"
	p2pm.lm.Log("info", message, "p2p")
	p2pm.UI.Print(message)

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
		libp2p.DefaultMuxers,
		// let this host use the DHT to find other hosts
		//		libp2p.Routing(func(hst host.Host) (routing.PeerRouting, error) {
		//			var err error = nil
		//			p2pm.h = hst
		//			p2pm.idht, err = p2pm.initDHT("server")
		//			return p2pm.idht, err
		//		}),
		// attempt to open ports using uPNP for NATed hosts.
		libp2p.EnableNATService(),
		libp2p.NATPortMap(),
		// control which nodes we allow to connect
		libp2p.ConnectionGater(blacklistManager.Gater),
		// use static relays for more reliable relay selection
		libp2p.EnableRelay(),
		libp2p.EnableHolePunching(),
	)
	if err != nil {
		return nil, err
	}

	p2pm.h = hst
	p2pm.idht, err = p2pm.initDHT("server")
	if err != nil {
		p2pm.h.Close()
		return nil, err
	}

	if p2pm.relay {
		message := "Starting relay server"
		p2pm.lm.Log("info", message, "p2p")
		p2pm.UI.Print(message)

		// Start relay v2 as a relay server
		_, err = relayv2.New(hst)
		if err != nil {
			return nil, err
		}
		/* TODO
		// Start relay service with resource limits
		_, err = relayv2.New(hst,
			relayv2.WithResources(relayv2.Resources{
				Limit: &relayv2.RelayLimit{
					Duration: 2 * time.Minute,
					Data:     1024 * 1024, // 1MB
				},
				ReservationTTL: 30 * time.Second,
				MaxReservations: 1024,
				MaxCircuits:     16,
				BufferSize:      2048,
			}),
		)
		if err != nil {
			hst.Close()
			return nil, err
		}
		*/
	}

	return hst, nil
}

func (p2pm *P2PManager) createPrivateHost(
	priv crypto.PrivKey,
	port uint16,
	blacklistManager *blacklist_node.BlacklistNodeManager,
	relayAddrsInfo []peer.AddrInfo,
) (host.Host, error) {
	message := "Creating private p2p host (behind NAT)"
	p2pm.lm.Log("info", message, "p2p")
	p2pm.UI.Print(message)

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
		libp2p.DefaultMuxers,
		// let this host use the DHT to find other hosts
		//		libp2p.Routing(func(hst host.Host) (routing.PeerRouting, error) {
		//			var err error = nil
		//			p2pm.h = hst
		//			p2pm.idht, err = p2pm.initDHT("client")
		//			return p2pm.idht, err
		//		}),
		// enable NAT port mapping (UPnP/NAT-PMP)
		libp2p.NATPortMap(),
		// enable NAT service
		libp2p.EnableNATService(),
		// enable AutoNAT v2 for automatic reachability detection
		libp2p.EnableAutoNATv2(),
		// control which nodes we allow to connect
		libp2p.ConnectionGater(blacklistManager.Gater),
		// use static relays for more reliable relay selection
		libp2p.EnableAutoRelayWithStaticRelays(relayAddrsInfo),
		libp2p.EnableRelay(),
		// enable hole punching for direct connections when possible
		libp2p.EnableHolePunching(),
	)
	if err != nil {
		return nil, err
	}

	p2pm.h = hst
	p2pm.idht, err = p2pm.initDHT("client")
	if err != nil {
		p2pm.h.Close()
		return nil, err
	}

	return hst, nil
}

func (p2pm *P2PManager) connectNodes(addrsInfo []peer.AddrInfo) {
	for _, addrInfo := range addrsInfo {
		p2pm.ConnectNode(addrInfo)
	}
}

func (p2pm *P2PManager) Stop() error {
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

	// Stop crons
	for _, c := range p2pm.crons {
		c.Stop()
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
		p2pm.lm.Log("error", err.Error(), "p2p")
		return nil, nil, err
	}

	sub, err := topic.Subscribe()
	if err != nil {
		p2pm.lm.Log("error", err.Error(), "p2p")
		return nil, nil, err
	}

	p2pm.topicsSubscribed[completeTopicName] = topic

	go p2pm.receivedMessage(cntx, sub)

	return sub, topic, nil
}

func (p2pm *P2PManager) initDHT(mode string) (*dht.IpfsDHT, error) {
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
		p2pm.lm.Log("error", err.Error(), "p2p")
		return nil, errors.New(err.Error())
	}

	if err = kademliaDHT.Bootstrap(p2pm.ctx); err != nil {
		p2pm.lm.Log("error", err.Error(), "p2p")
		return nil, errors.New(err.Error())
	}

	for _, peerAddr := range dht.DefaultBootstrapPeers {
		peerinfo, _ := peer.AddrInfoFromP2pAddr(peerAddr)

		go func() {
			if _, err := p2pm.ConnectNode(*peerinfo); err != nil {
				p2pm.lm.Log("warn", err.Error(), "p2p")
			}
		}()
	}

	return kademliaDHT, nil
}

func (p2pm *P2PManager) discoverPeers(peerChannel chan []peer.AddrInfo) {
	routingDiscovery := drouting.NewRoutingDiscovery(p2pm.idht)
	for _, completeTopicName := range p2pm.completeTopicNames {
		dutil.Advertise(p2pm.ctx, routingDiscovery, completeTopicName)
	}

	// Look for others who have announced and attempt to connect to them
	anyConnected := false
	for !anyConnected {
		for _, completeTopicName := range p2pm.completeTopicNames {
			peerChan, err := routingDiscovery.FindPeers(p2pm.ctx, completeTopicName)
			if err != nil {
				p2pm.lm.Log("warn", err.Error(), "p2p")
				continue
			}

			var discoveredPeers []peer.AddrInfo

			for peer := range peerChan {
				if peer.ID != "" {
					discoveredPeers = append(discoveredPeers, peer)
				}

				running := p2pm.IsHostRunning()
				if !running {
					break
				}

				skip, err := p2pm.ConnectNode(peer)
				if err != nil {
					continue
				}
				if skip {
					continue
				}

				peerChannel <- discoveredPeers
			}
		}
	}
	close(peerChannel)
	p2pm.lm.Log("debug", "Peer discovery complete", "p2p")
}

func (p2pm *P2PManager) IsNodeConnected(peer peer.AddrInfo) (bool, error) {
	running := p2pm.IsHostRunning()
	if !running {
		err := fmt.Errorf("host is not running")
		p2pm.lm.Log("error", err.Error(), "p2p")
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

func (p2pm *P2PManager) ConnectNode(p peer.AddrInfo) (bool, error) {

	connected, err := p2pm.IsNodeConnected(p)
	if err != nil {
		msg := err.Error()
		p2pm.lm.Log("error", msg, "p2p")
		return false, err
	}
	if connected {
		msg := fmt.Sprintf("Node %s is already connected", p.ID.String())
		p2pm.lm.Log("debug", msg, "p2p")
		return true, nil // skip node but do not panic
	}

	if p.ID == p2pm.h.ID() {
		return true, fmt.Errorf("can not connect to itself %s == %s", p.ID, p2pm.h.ID())
	}

	/*
		result := make(chan struct {
			peer peer.AddrInfo
			err  error
		}, 1)

		go func(pi peer.AddrInfo) {
			connCtx, cancel := context.WithTimeout(p2pm.ctx, 15*time.Second)
			defer cancel()

			p2pm.lm.Log("debug", fmt.Sprintf("Attempting to connect to: %s\n", pi.ID.String()), "p2p")
			err := p2pm.h.Connect(connCtx, pi)

			result <- struct {
				peer peer.AddrInfo
				err  error
			}{pi, err}
		}(p)

		select {
		case res := <-result:
			if res.err != nil {
				p2pm.h.Network().ClosePeer(res.peer.ID)
				p2pm.h.Network().Peerstore().RemovePeer(res.peer.ID)
				p2pm.h.Peerstore().ClearAddrs(res.peer.ID)
				p2pm.h.Peerstore().RemovePeer(res.peer.ID)
				p2pm.lm.Log("debug", fmt.Sprintf("Failed to connect to %s: %v", res.peer.ID.String(), res.err), "p2p")
				p2pm.lm.Log("debug", fmt.Sprintf("Removed peer %s from a peer store", res.peer.ID), "p2p")
				return false, err
			} else {
				p2pm.lm.Log("debug", fmt.Sprintf("Connected to: %s", res.peer.ID.String()), "p2p")
				for _, ma := range res.peer.Addrs {
					p2pm.lm.Log("debug", fmt.Sprintf("Connected peer's multiaddr is %s", ma.String()), "p2p")
				}
				return false, nil
			}
		case <-time.After(20 * time.Second):
			return false, fmt.Errorf("timeout waiting for bootstrap connections")
		}
	*/
	connCtx, cancel := context.WithTimeout(p2pm.ctx, 10*time.Second)
	err = p2pm.h.Connect(connCtx, p)
	cancel()

	//	err = p2pm.h.Connect(p2pm.ctx, p)

	if err != nil {
		p2pm.h.Network().ClosePeer(p.ID)
		p2pm.h.Network().Peerstore().RemovePeer(p.ID)
		p2pm.h.Peerstore().ClearAddrs(p.ID)
		p2pm.h.Peerstore().RemovePeer(p.ID)
		p2pm.lm.Log("debug", fmt.Sprintf("Removed peer %s from a peer store", p.ID), "p2p")

		return false, err
	}

	p2pm.lm.Log("debug", fmt.Sprintf("Connected to: %s", p.ID.String()), "p2p")

	for _, ma := range p.Addrs {
		p2pm.lm.Log("debug", fmt.Sprintf("Connected peer's multiaddr is %s", ma.String()), "p2p")
	}

	return false, nil
}

func StreamData[T any](p2pm *P2PManager, receivingPeer peer.AddrInfo, data T, job *node_types.Job, existingStream network.Stream) error {
	_, err := p2pm.ConnectNode(receivingPeer)
	if err != nil {
		p2pm.lm.Log("error", err.Error(), "p2p")
		return err
	}

	var s network.Stream

	if existingStream != nil {
		// Try writing a ping to see if stream is alive
		existingStream.SetWriteDeadline(time.Now().Add(100 * time.Millisecond))
		_, err := existingStream.Write([]byte{})
		existingStream.SetWriteDeadline(time.Time{}) // reset deadline

		if err == nil {
			// Stream is alive, reuse it
			s = existingStream
			p2pm.lm.Log("debug", fmt.Sprintf("Reusing existing stream with %s", receivingPeer.ID), "p2p")
		} else {
			// Stream is broken, discard and create a new one
			p2pm.lm.Log("debug", fmt.Sprintf("Existing stream with %s is not usable: %v", receivingPeer.ID, err), "p2p")
		}
	}

	if s == nil {
		s, err = p2pm.h.NewStream(p2pm.ctx, receivingPeer.ID, p2pm.protocolID)
		if err != nil {
			p2pm.lm.Log("error", err.Error(), "p2p")
			return err
		}
	}

	t := uint16(0)
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
		msg := fmt.Sprintf("Data type %v is not allowed in this context (streaming data)", v)
		p2pm.lm.Log("error", msg, "p2p")
		s.Reset()
		return errors.New(msg)
	}

	var workflowId int64 = int64(0)
	var jobId int64 = int64(0)
	var orderingPeer255 [255]byte
	var interfaceId int64 = int64(0)

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

	go p2pm.streamProposal(s, sendingPeer255, t, orderingPeer255, workflowId, jobId, interfaceId)
	go sendStream(p2pm, s, data)

	return nil
}

func (p2pm *P2PManager) streamProposal(s network.Stream, p [255]byte, t uint16, onode [255]byte, wid int64, jid int64, iid int64) {
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
		p2pm.lm.Log("error", err.Error(), "p2p")
		s.Reset()
	}
	message := fmt.Sprintf("Stream proposal type %d has been sent in stream %s", streamData.Type, s.ID())
	p2pm.lm.Log("debug", message, "p2p")
}

func (p2pm *P2PManager) streamProposalResponse(s network.Stream) {
	// Prepare to read the stream data
	var streamData node_types.StreamData
	err := binary.Read(s, binary.BigEndian, &streamData)
	if err != nil {
		p2pm.lm.Log("error", err.Error(), "p2p")
		s.Reset()
	}

	message := fmt.Sprintf("Received stream data type %d from %s in stream %s",
		streamData.Type, string(bytes.Trim(streamData.PeerId[:], "\x00")), s.ID())
	p2pm.lm.Log("debug", message, "p2p")

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
	settingsManager := settings.NewSettingsManager(p2pm.DB)
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
		p2pm.lm.Log("debug", message, "p2p")
		return false
	}
	if accepted {
		message := fmt.Sprintf("As per local settings stream data type %d is accepted.", streamDataType)
		p2pm.lm.Log("debug", message, "p2p")
	} else {
		message := fmt.Sprintf("As per local settings stream data type %d is not accepted", streamDataType)
		p2pm.lm.Log("debug", message, "p2p")
	}

	return accepted
}

func (p2pm *P2PManager) streamAccepted(s network.Stream) {
	data := [7]byte{'T', 'F', 'R', 'E', 'A', 'D', 'Y'}
	err := binary.Write(s, binary.BigEndian, data)
	if err != nil {
		p2pm.lm.Log("error", err.Error(), "p2p")
		s.Reset()
	}

	message := fmt.Sprintf("Sent TFREADY successfully in stream %s", s.ID())
	p2pm.lm.Log("debug", message, "p2p")
}

func sendStream[T any](p2pm *P2PManager, s network.Stream, data T) {
	var ready [7]byte
	var expected [7]byte = [7]byte{'T', 'F', 'R', 'E', 'A', 'D', 'Y'}

	err := binary.Read(s, binary.BigEndian, &ready)
	if err != nil {
		p2pm.lm.Log("error", err.Error(), "p2p")
		s.Reset()
		return
	}

	// Check if received data matches TFREADY signal
	if !bytes.Equal(expected[:], ready[:]) {
		err := errors.New("did not get expected TFREADY signal")
		p2pm.lm.Log("error", err.Error(), "p2p")
		s.Reset()
		return
	}

	message := fmt.Sprintf("Received %s from %s", string(ready[:]), s.ID())
	p2pm.lm.Log("debug", message, "p2p")

	var chunkSize uint64 = 4096
	var pointer uint64 = 0

	// Load configs
	configManager := utils.NewConfigManager("")
	config, err := configManager.ReadConfigs()
	if err != nil {
		message := fmt.Sprintf("Can not read configs file. (%s)", err.Error())
		p2pm.lm.Log("error", message, "p2p")
		return
	}

	// Read chunk size form configs
	cs := config["chunk_size"]
	chunkSize, err = strconv.ParseUint(cs, 10, 64)
	if err != nil {
		message := fmt.Sprintf("Invalid chunk size in configs file. Will set to the default chunk size (%s)", err.Error())
		p2pm.lm.Log("warn", message, "p2p")
	}

	switch v := any(data).(type) {
	case *[]node_types.ServiceOffer, *node_types.ServiceRequest, *node_types.ServiceResponse,
		*node_types.JobRunRequest, *node_types.JobRunResponse,
		*node_types.JobRunStatus, *node_types.JobRunStatusRequest,
		*node_types.JobDataReceiptAcknowledgement, *node_types.JobDataRequest,
		*node_types.ServiceRequestCancellation, *node_types.ServiceResponseCancellation:
		b, err := json.Marshal(data)
		if err != nil {
			p2pm.lm.Log("error", err.Error(), "p2p")
			s.Reset()
			return
		}

		err = p2pm.sendStreamChunks(b, pointer, chunkSize, s)
		if err != nil {
			p2pm.lm.Log("error", err.Error(), "p2p")
			s.Reset()
			return
		}
	case *[]byte:
		err = p2pm.sendStreamChunks(*v, pointer, chunkSize, s)
		if err != nil {
			p2pm.lm.Log("error", err.Error(), "p2p")
			s.Reset()
			return
		}
	case *os.File:
		defer v.Close() // Ensure the file is closed after operations
		buffer := make([]byte, chunkSize)

		writer := bufio.NewWriter(s)

		for {
			n, err := v.Read(buffer)
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				p2pm.lm.Log("error", err.Error(), "p2p")
				s.Reset()
				return
			}
			_, err = writer.Write(buffer[:n])
			if err != nil {
				p2pm.lm.Log("error", err.Error(), "p2p")
				s.Reset()
				return
			}

		}

		writer.Flush()
		s.Close()

	default:
		msg := fmt.Sprintf("Data type %v is not allowed in this context (streaming data)", v)
		p2pm.lm.Log("error", msg, "p2p")
		s.Reset()
		return
	}

	message = fmt.Sprintf("Sending ended %s", s.ID())
	p2pm.lm.Log("debug", message, "p2p")
}

func (p2pm *P2PManager) sendStreamChunks(b []byte, pointer uint64, chunkSize uint64, s network.Stream) error {
	for {
		if len(b)-int(pointer) < int(chunkSize) {
			chunkSize = uint64(len(b) - int(pointer))
		}

		err := binary.Write(s, binary.BigEndian, chunkSize)
		if err != nil {
			p2pm.lm.Log("error", err.Error(), "p2p")
			return err
		}
		message := fmt.Sprintf("Chunk [%d bytes] successfully sent in stream %s", chunkSize, s.ID())
		p2pm.lm.Log("debug", message, "p2p")

		if chunkSize == 0 {
			break
		}

		err = binary.Write(s, binary.BigEndian, (b)[pointer:pointer+chunkSize])
		if err != nil {
			p2pm.lm.Log("error", err.Error(), "p2p")
			return err
		}
		message = fmt.Sprintf("Chunk [%d bytes] successfully sent in stream %s", len((b)[pointer:pointer+chunkSize]), s.ID())
		p2pm.lm.Log("debug", message, "p2p")

		pointer += chunkSize

	}
	return nil
}

func (p2pm *P2PManager) receivedStream(s network.Stream, streamData node_types.StreamData) {
	// Determine data type
	switch streamData.Type {
	case 0, 1, 4, 5, 6, 7, 8, 9, 10, 11, 12:
		// Prepare to read back the data
		var data []byte
		for {
			var chunkSize uint64
			err := binary.Read(s, binary.BigEndian, &chunkSize)
			if err != nil {
				p2pm.lm.Log("error", err.Error(), "p2p")
				s.Reset()
			}

			message := fmt.Sprintf("Data from %s will be received in chunks of %d bytes", s.ID(), chunkSize)
			p2pm.lm.Log("debug", message, "p2p")

			if chunkSize == 0 {
				break
			}

			chunk := make([]byte, chunkSize)

			err = binary.Read(s, binary.BigEndian, &chunk)
			if err != nil {
				p2pm.lm.Log("error", err.Error(), "p2p")
				s.Reset()
			}

			message = fmt.Sprintf("Received chunk [%d bytes] of type %d from %s", len(chunk), streamData.Type, s.ID())
			p2pm.lm.Log("debug", message, "p2p")

			// Concatenate received chunk
			data = append(data, chunk...)
		}

		message := fmt.Sprintf("Received completed. Data [%d bytes] of type %d received in stream %s from %s ",
			len(data), streamData.Type, s.ID(), string(bytes.Trim(streamData.PeerId[:], "\x00")))
		p2pm.lm.Log("debug", message, "p2p")

		if streamData.Type == 0 {
			// Received a Service Offer from the remote peer
			var serviceOffer []node_types.ServiceOffer
			err := json.Unmarshal(data, &serviceOffer)
			if err != nil {
				msg := fmt.Sprintf("Could not load received binary stream into a Service Offer struct.\n\n%s", err.Error())
				p2pm.lm.Log("error", msg, "p2p")
				s.Reset()
				return
			}

			// Draw table output
			for _, service := range serviceOffer {
				// Remote node ID
				nodeId := service.NodeId
				localNodeId := s.Conn().LocalPeer().String()
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
					p2pm.lm.Log("error", err.Error(), "p2p")
					s.Reset()
					return
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
					p2pm.lm.Log("error", err.Error(), "p2p")
					s.Reset()
					return
				}
			}
		} else if streamData.Type == 1 {
			// Received a Job Run Request from the remote peer
			var jobRunRequest node_types.JobRunRequest

			peerId := s.Conn().RemotePeer()
			peer, err := p2pm.GeneratePeerAddrInfo(peerId.String())
			if err != nil {
				msg := err.Error()
				p2pm.lm.Log("error", msg, "p2p")
				s.Close()
				return
			}

			err = json.Unmarshal(data, &jobRunRequest)
			if err != nil {
				msg := fmt.Sprintf("Could not load received binary stream into a Job Run Request struct.\n\n%s", err.Error())
				p2pm.lm.Log("error", msg, "p2p")
				s.Close()
				return
			}

			remotePeer := peerId.String()
			jobRunResponse := node_types.JobRunResponse{
				Accepted:      false,
				Message:       "",
				JobRunRequest: jobRunRequest,
			}

			// Check if job is existing
			jobManager := NewJobManager(p2pm)
			if err, exists := jobManager.JobExists(jobRunRequest.JobId); err != nil || !exists {
				msg := fmt.Sprintf("Could not find job Id %d.\n\n%s", jobRunRequest.JobId, err.Error())
				p2pm.lm.Log("error", msg, "p2p")

				jobRunResponse.Message = err.Error()
				err = StreamData(p2pm, peer, &jobRunResponse, nil, nil)
				if err != nil {
					p2pm.lm.Log("error", err.Error(), "p2p")
					s.Close()
					return
				}

				s.Close()
				return
			}

			// Check if job is owned by requesting peer
			job, err := jobManager.GetJob(jobRunRequest.JobId)
			if err != nil {
				msg := fmt.Sprintf("Could not retrieve job Id %d.\n\n%s", jobRunRequest.JobId, err.Error())
				p2pm.lm.Log("error", msg, "p2p")

				jobRunResponse.Message = err.Error()
				err = StreamData(p2pm, peer, &jobRunResponse, nil, nil)
				if err != nil {
					p2pm.lm.Log("error", err.Error(), "p2p")
					s.Close()
					return
				}

				s.Close()
				return
			}
			if job.OrderingNodeId != remotePeer {
				msg := fmt.Sprintf("Job run request for job Id %d is made by node %s who is not owning the job.\n\n", jobRunRequest.JobId, remotePeer)
				p2pm.lm.Log("error", msg, "p2p")

				jobRunResponse.Message = msg
				err = StreamData(p2pm, peer, &jobRunResponse, nil, nil)
				if err != nil {
					p2pm.lm.Log("error", err.Error(), "p2p")
					s.Close()
					return
				}

				s.Close()
				return
			}

			// Set READY flag for the job
			err = jobManager.UpdateJobStatus(job.Id, "READY")
			if err != nil {
				msg := fmt.Sprintf("Failed to set READY flag to job Id %d.\n\n%s", jobRunRequest.JobId, err.Error())
				p2pm.lm.Log("error", msg, "p2p")

				jobRunResponse.Message = err.Error()
				err = StreamData(p2pm, peer, &jobRunResponse, nil, nil)
				if err != nil {
					p2pm.lm.Log("error", err.Error(), "p2p")
					s.Close()
					return
				}

				s.Close()
				return
			}

			jobRunResponse.Message = fmt.Sprintf("READY flag is set for job Id %d.", jobRunRequest.JobId)
			jobRunResponse.Accepted = true
			err = StreamData(p2pm, peer, &jobRunResponse, nil, nil)
			if err != nil {
				p2pm.lm.Log("error", err.Error(), "p2p")
				s.Close()
				return
			}

		} else if streamData.Type == 4 {
			// Received a Service Request from the remote peer
			var serviceRequest node_types.ServiceRequest

			peerId := s.Conn().RemotePeer()
			peer, err := p2pm.GeneratePeerAddrInfo(peerId.String())
			if err != nil {
				msg := err.Error()
				p2pm.lm.Log("error", msg, "p2p")
				s.Close()
				return
			}

			err = json.Unmarshal(data, &serviceRequest)
			if err != nil {
				msg := fmt.Sprintf("Could not load received binary stream into a Service Request struct.\n\n%s", err.Error())
				p2pm.lm.Log("error", msg, "p2p")
				s.Close()
				return
			}

			remotePeer := peerId.String()
			serviceResponse := node_types.ServiceResponse{
				JobId:          int64(0),
				Accepted:       false,
				Message:        "",
				OrderingNodeId: remotePeer,
				ServiceRequest: serviceRequest,
			}

			// Check if it is existing service
			serviceManager := NewServiceManager(p2pm)
			err, exist := serviceManager.Exists(serviceRequest.ServiceId)
			if err != nil {
				p2pm.lm.Log("error", err.Error(), "p2p")

				serviceResponse.Message = err.Error()
				err = StreamData(p2pm, peer, &serviceResponse, nil, nil)
				if err != nil {
					p2pm.lm.Log("error", err.Error(), "p2p")
					s.Close()
					return
				}

				s.Close()
				return
			}
			if !exist {
				msg := fmt.Sprintf("Could not find service Id %d.", serviceRequest.ServiceId)
				p2pm.lm.Log("error", msg, "p2p")

				serviceResponse.Message = msg
				err = StreamData(p2pm, peer, &serviceResponse, nil, nil)
				if err != nil {
					p2pm.lm.Log("error", err.Error(), "p2p")
					s.Close()
					return
				}

				s.Close()
				return
			}

			// Acquire service
			service, err := serviceManager.Get(serviceRequest.ServiceId)
			if err != nil {
				p2pm.lm.Log("error", err.Error(), "p2p")

				serviceResponse.Message = err.Error()
				err = StreamData(p2pm, peer, &serviceResponse, nil, nil)
				if err != nil {
					p2pm.lm.Log("error", err.Error(), "p2p")
					s.Close()
					return
				}

				s.Close()
				return
			}
			// If service type is DATA populate file hash(es) in response message
			if service.Type == "DATA" {
				dataService, err := serviceManager.GetData(serviceRequest.ServiceId)
				if err != nil {
					p2pm.lm.Log("error", err.Error(), "p2p")

					serviceResponse.Message = err.Error()
					err = StreamData(p2pm, peer, &serviceResponse, nil, nil)
					if err != nil {
						p2pm.lm.Log("error", err.Error(), "p2p")
						s.Close()
						return
					}

					s.Close()
					return
				}
				serviceResponse.Message = dataService.Path
			} else {
				serviceResponse.Message = ""
			}

			// TODO, service price and payment

			// Create a job
			jobManager := NewJobManager(p2pm)
			job, err := jobManager.CreateJob(serviceRequest, remotePeer)
			if err != nil {
				p2pm.lm.Log("error", err.Error(), "p2p")

				serviceResponse.Message = err.Error()
				err = StreamData(p2pm, peer, &serviceResponse, nil, nil)
				if err != nil {
					p2pm.lm.Log("error", err.Error(), "p2p")
					s.Close()
					return
				}

				s.Close()
				return
			}

			// Send response
			serviceResponse.JobId = job.Id
			serviceResponse.Accepted = true
			err = StreamData(p2pm, peer, &serviceResponse, nil, nil)
			if err != nil {
				p2pm.lm.Log("error", err.Error(), "p2p")
				s.Close()
				return
			}
		} else if streamData.Type == 5 {
			// Received a Service Response from the remote peer
			var serviceResponse node_types.ServiceResponse
			err := json.Unmarshal(data, &serviceResponse)
			if err != nil {
				msg := fmt.Sprintf("Could not load received binary stream into a Service Response struct.\n\n%s", err.Error())
				p2pm.lm.Log("error", msg, "p2p")
				s.Reset()
				return
			}

			if !serviceResponse.Accepted {
				msg := fmt.Sprintf("Service Request (service request Id %d) for node Id %s is not accepted with the following reason: %s.\n\n",
					serviceResponse.ServiceId, serviceResponse.NodeId, serviceResponse.Message)
				p2pm.lm.Log("error", msg, "p2p")
			} else {
				// Add job to the workflow
				err = p2pm.wm.RegisteredWorkflowJob(serviceResponse.WorkflowId, serviceResponse.WorkflowJobId,
					serviceResponse.NodeId, serviceResponse.ServiceId, serviceResponse.JobId, serviceResponse.Message)
				if err != nil {
					msg := fmt.Sprintf("Workflow job acceptance (%s-%d) to workflow %d ended up with error.\n\n%s",
						serviceResponse.NodeId, serviceResponse.JobId, serviceResponse.WorkflowId, err.Error())
					p2pm.lm.Log("error", msg, "p2p")
					s.Reset()
					return
				}
			}

			// Draw table output
			// If we are in interactive mode print Service Offer to CLI
			uiType, err := ui.DetectUIType(p2pm.UI)
			if err != nil {
				p2pm.lm.Log("error", err.Error(), "p2p")
				s.Reset()
				return
			}
			switch uiType {
			case "CLI":
				menuManager := NewMenuManager(p2pm)
				menuManager.printServiceResponse(serviceResponse)
			case "GUI":
				// TODO, GUI push message
			default:
				err := fmt.Errorf("unknown UI type %s", uiType)
				p2pm.lm.Log("error", err.Error(), "p2p")
				s.Reset()
				return
			}
		} else if streamData.Type == 6 {
			// Received a Job Run Response from the remote peer
			var jobRunResponse node_types.JobRunResponse
			err := json.Unmarshal(data, &jobRunResponse)
			if err != nil {
				msg := fmt.Sprintf("Could not load received binary stream into a Job Run Response struct.\n\n%s", err.Error())
				p2pm.lm.Log("error", msg, "p2p")
				s.Reset()
				return
			}

			// Update workflow job status
			workflowManager := workflow.NewWorkflowManager(p2pm.DB)
			if !jobRunResponse.Accepted {
				msg := fmt.Sprintf("Job Run Request (job run request Id %d) for node Id %s is not accepted with the following reason: %s.\n\n",
					jobRunResponse.JobId, jobRunResponse.NodeId, jobRunResponse.Message)
				p2pm.lm.Log("error", msg, "p2p")

				err := workflowManager.UpdateWorkflowJobStatus(jobRunResponse.WorkflowId, jobRunResponse.NodeId, jobRunResponse.JobId, "ERRORED")
				if err != nil {
					msg := fmt.Sprintf("Could not update workflow job %d-%s-%d status to %s.\n\n%s",
						jobRunResponse.WorkflowId, jobRunResponse.NodeId, jobRunResponse.JobId, "ERRORED", err.Error())
					p2pm.lm.Log("error", msg, "p2p")
					s.Reset()
					return
				}
			}

			// Draw table output
			// If we are in interactive mode print Service Offer to CLI
			uiType, err := ui.DetectUIType(p2pm.UI)
			if err != nil {
				p2pm.lm.Log("error", err.Error(), "p2p")
				s.Reset()
				return
			}
			switch uiType {
			case "CLI":
				menuManager := NewMenuManager(p2pm)
				menuManager.printJobRunResponse(jobRunResponse)
			case "GUI":
				// TODO, GUI push message
			default:
				err := fmt.Errorf("unknown UI type %s", uiType)
				p2pm.lm.Log("error", err.Error(), "p2p")
				s.Reset()
				return
			}
		} else if streamData.Type == 7 {
			// Received a Job Run Status update from the remote peer
			var jobRunStatus node_types.JobRunStatus
			err := json.Unmarshal(data, &jobRunStatus)
			if err != nil {
				msg := fmt.Sprintf("Could not load received binary stream into a Job Run Status update struct.\n\n%s", err.Error())
				p2pm.lm.Log("error", msg, "p2p")
				s.Reset()
				return
			}

			// Update workflow job status
			workflowManager := workflow.NewWorkflowManager(p2pm.DB)
			err = workflowManager.UpdateWorkflowJobStatus(jobRunStatus.WorkflowId, jobRunStatus.NodeId, jobRunStatus.JobId, jobRunStatus.Status)
			if err != nil {
				msg := fmt.Sprintf("Could not update workflow job %d-%s-%d status to %s.\n\n%s",
					jobRunStatus.WorkflowId, jobRunStatus.NodeId, jobRunStatus.JobId, jobRunStatus.Status, err.Error())
				p2pm.lm.Log("error", msg, "p2p")
				s.Reset()
				return
			}
		} else if streamData.Type == 8 {
			// Received a Job Run Status Request from the remote peer
			var jobRunStatusRequest node_types.JobRunStatusRequest

			err := json.Unmarshal(data, &jobRunStatusRequest)
			if err != nil {
				msg := fmt.Sprintf("Could not load received binary stream into a Job Run Status Request struct.\n\n%s", err.Error())
				p2pm.lm.Log("error", msg, "p2p")
				s.Close()
				return
			}

			peerId := s.Conn().RemotePeer()
			peer, err := p2pm.GeneratePeerAddrInfo(peerId.String())
			if err != nil {
				msg := err.Error()
				p2pm.lm.Log("error", msg, "p2p")
				s.Close()
				return
			}

			remotePeer := peerId.String()
			jobRunStatusResponse := node_types.JobRunStatus{
				JobRunStatusRequest: jobRunStatusRequest,
				Status:              "",
			}

			// Check if job is existing
			jobManager := NewJobManager(p2pm)
			if err, exists := jobManager.JobExists(jobRunStatusRequest.JobId); err != nil || !exists {
				msg := fmt.Sprintf("Could not find job Id %d.\n\n%s", jobRunStatusRequest.JobId, err.Error())
				p2pm.lm.Log("error", msg, "p2p")

				jobRunStatusResponse.Status = "ERRORED"
				err = StreamData(p2pm, peer, &jobRunStatusResponse, nil, nil)
				if err != nil {
					p2pm.lm.Log("error", err.Error(), "p2p")
					s.Close()
					return
				}

				s.Close()
				return
			}

			// Check if job is owned by requesting peer
			job, err := jobManager.GetJob(jobRunStatusRequest.JobId)
			if err != nil {
				msg := fmt.Sprintf("Could not retrieve job Id %d.\n\n%s", jobRunStatusRequest.JobId, err.Error())
				p2pm.lm.Log("error", msg, "p2p")

				jobRunStatusResponse.Status = "ERRORED"
				err = StreamData(p2pm, peer, &jobRunStatusResponse, nil, nil)
				if err != nil {
					p2pm.lm.Log("error", err.Error(), "p2p")
					s.Close()
					return
				}

				s.Close()
				return
			}
			if job.OrderingNodeId != remotePeer {
				msg := fmt.Sprintf("Job run status request for job Id %d is made by node %s who is not owning the job.\n\n", jobRunStatusRequest.JobId, remotePeer)
				p2pm.lm.Log("error", msg, "p2p")

				jobRunStatusResponse.Status = "ERRORED"
				err = StreamData(p2pm, peer, &jobRunStatusResponse, nil, nil)
				if err != nil {
					p2pm.lm.Log("error", err.Error(), "p2p")
					s.Close()
					return
				}

				s.Close()
				return
			}

			err = jobManager.StatusUpdate(job.Id, job.Status)
			if err != nil {
				p2pm.lm.Log("error", err.Error(), "p2p")
				s.Close()
				return
			}
		} else if streamData.Type == 9 {
			// Received a Job Data Acknowledgement Receipt from the remote peer
			var jobDataReceiptAcknowledgement node_types.JobDataReceiptAcknowledgement

			err := json.Unmarshal(data, &jobDataReceiptAcknowledgement)
			if err != nil {
				msg := fmt.Sprintf("Could not load received binary stream into a Job Data Acknowledgement Receipt struct.\n\n%s", err.Error())
				p2pm.lm.Log("error", msg, "p2p")
				s.Close()
				return
			}

			// Acknowledge receipt
			remotePeer := s.Conn().RemotePeer()
			jobManager := NewJobManager(p2pm)
			err = jobManager.AcknowledgeReceipt(
				jobDataReceiptAcknowledgement.JobId,
				jobDataReceiptAcknowledgement.InterfaceId,
				remotePeer,
				"OUTPUT",
			)
			if err != nil {
				p2pm.lm.Log("error", err.Error(), "p2p")
				s.Close()
				return
			}
		} else if streamData.Type == 10 {
			// Received a Job Data Request from the remote peer
			var jobDataRequest node_types.JobDataRequest

			err := json.Unmarshal(data, &jobDataRequest)
			if err != nil {
				msg := fmt.Sprintf("Could not load received binary stream into a Job Data Request struct.\n\n%s", err.Error())
				p2pm.lm.Log("error", msg, "p2p")
				s.Close()
				return
			}

			// Send the data if peer is eligible for it
			remotePeer := s.Conn().RemotePeer()
			jobManager := NewJobManager(p2pm)
			err = jobManager.SendIfPeerEligible(
				jobDataRequest.JobId,
				jobDataRequest.InterfaceId,
				remotePeer,
				jobDataRequest.WhatData,
			)
			if err != nil {
				p2pm.lm.Log("error", err.Error(), "p2p")
				s.Close()
				return
			}
		} else if streamData.Type == 11 {
			// Received a Service Request Cancelation from the remote peer
			var serviceRequestCancellation node_types.ServiceRequestCancellation

			peerId := s.Conn().RemotePeer()
			peer, err := p2pm.GeneratePeerAddrInfo(peerId.String())
			if err != nil {
				msg := err.Error()
				p2pm.lm.Log("error", msg, "p2p")
				s.Close()
				return
			}

			err = json.Unmarshal(data, &serviceRequestCancellation)
			if err != nil {
				msg := fmt.Sprintf("Could not load received binary stream into a Service Request Cancellation struct.\n\n%s", err.Error())
				p2pm.lm.Log("error", msg, "p2p")
				s.Close()
				return
			}

			remotePeer := peerId.String()
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
				p2pm.lm.Log("error", err.Error(), "p2p")

				serviceResponseCancellation.Message = err.Error()
				err = StreamData(p2pm, peer, &serviceResponseCancellation, nil, nil)
				if err != nil {
					p2pm.lm.Log("error", err.Error(), "p2p")
					s.Close()
					return
				}

				s.Close()
				return
			}

			// Check if job is owned by ordering peer
			if job.OrderingNodeId != remotePeer {
				err := fmt.Errorf("job cancellation request for job Id %d is made by node %s who is not owning the job", serviceRequestCancellation.JobId, remotePeer)
				p2pm.lm.Log("error", err.Error(), "p2p")

				serviceResponseCancellation.Message = err.Error()
				err = StreamData(p2pm, peer, &serviceResponseCancellation, nil, nil)
				if err != nil {
					p2pm.lm.Log("error", err.Error(), "p2p")
					s.Close()
					return
				}

				s.Close()
				return
			}

			// Remove a job
			err = jobManager.RemoveJob(serviceRequestCancellation.JobId)
			if err != nil {
				p2pm.lm.Log("error", err.Error(), "p2p")

				serviceResponseCancellation.Message = err.Error()
				err = StreamData(p2pm, peer, &serviceResponseCancellation, nil, nil)
				if err != nil {
					p2pm.lm.Log("error", err.Error(), "p2p")
					s.Close()
					return
				}

				s.Close()
				return
			}

			// Send a response
			serviceResponseCancellation.Accepted = true
			err = StreamData(p2pm, peer, &serviceResponseCancellation, nil, nil)
			if err != nil {
				p2pm.lm.Log("error", err.Error(), "p2p")
				s.Close()
				return
			}
		} else if streamData.Type == 12 {
			// Received a Service Response Cancellation from the remote peer
			var serviceResponseCancellation node_types.ServiceResponseCancellation
			err := json.Unmarshal(data, &serviceResponseCancellation)
			if err != nil {
				msg := fmt.Sprintf("Could not load received binary stream into a Service Response Cancellation struct.\n\n%s", err.Error())
				p2pm.lm.Log("error", msg, "p2p")
				s.Reset()
				return
			}

			if !serviceResponseCancellation.Accepted {
				msg := fmt.Sprintf("Service Request Cancellation (job Id %d) for node Id %s is not accepted with the following reason: %s.\n\n",
					serviceResponseCancellation.JobId, serviceResponseCancellation.NodeId, serviceResponseCancellation.Message)
				p2pm.lm.Log("error", msg, "p2p")
			} else {
				// Remove a workflow job
				err = p2pm.wm.RemoveWorkflowJob(serviceResponseCancellation.WorkflowJobId)
				if err != nil {
					msg := fmt.Sprintf("Removing Workflow Job id %d from node id %s ended up with error.\n\n%s",
						serviceResponseCancellation.JobId, serviceResponseCancellation.NodeId, err.Error())
					p2pm.lm.Log("error", msg, "p2p")
					s.Reset()
					return
				}
			}

			// Draw table output
			// If we are in interactive mode print to CLI
			uiType, err := ui.DetectUIType(p2pm.UI)
			if err != nil {
				p2pm.lm.Log("error", err.Error(), "p2p")
				s.Reset()
				return
			}
			switch uiType {
			case "CLI":
				// TODO, CLI
			case "GUI":
				// TODO, GUI push message
			default:
				err := fmt.Errorf("unknown UI type %s", uiType)
				p2pm.lm.Log("error", err.Error(), "p2p")
				s.Reset()
				return
			}
		}
	case 2:
		// Received binary stream from the remote peer
	case 3:
		// Received a file from the remote peer
		var file *os.File
		var err error
		var fdir string
		var fpath string
		var chunkSize uint64 = 4096

		// Allow node to receive remote files
		// Data should be accepted in following cases:
		// a) this is ordering node (OrderingPeerId == h.ID())
		// b) the data is sent to our job (IDLE WorkflowId/JobId exists in jobs table)
		jobManager := NewJobManager(p2pm)
		orderingNodeId := string(bytes.Trim(streamData.OrderingPeerId[:], "\x00"))
		receivingNodeId := p2pm.h.ID().String()
		allowed := receivingNodeId == string(orderingNodeId)
		peerId := s.Conn().RemotePeer().String()
		if !allowed {
			allowed, err = jobManager.JobExpectingInputsFrom(streamData.JobId, peerId)
			if err != nil {
				p2pm.lm.Log("error", err.Error(), "p2p")
				s.Close()
				return
			}
		}
		if !allowed {
			err := fmt.Errorf("we haven't ordered this data from `%s`. We are also not expecting it as an input for a job",
				orderingNodeId)
			p2pm.lm.Log("error", err.Error(), "p2p")
			s.Close()
			return
		}

		// Read chunk size form configs
		configManager := utils.NewConfigManager("")
		configs, err := configManager.ReadConfigs()
		if err != nil {
			p2pm.lm.Log("error", err.Error(), "p2p")
			s.Reset()
			return
		}

		sWorkflowId := strconv.FormatInt(streamData.WorkflowId, 10)
		sJobId := strconv.FormatInt(streamData.JobId, 10)
		fdir = filepath.Join(configs["local_storage"], "workflows", orderingNodeId, sWorkflowId, "job", sJobId, "input", peerId)
		fpath = fdir + utils.RandomString(32)

		cs := configs["chunk_size"]
		chunkSize, err = strconv.ParseUint(cs, 10, 64)
		if err != nil {
			message := fmt.Sprintf("Invalid chunk size in configs file. Will set to the default chunk size (%s)", err.Error())
			p2pm.lm.Log("warn", message, "p2p")
		}
		if err = os.MkdirAll(fdir, 0755); err != nil {
			p2pm.lm.Log("error", err.Error(), "p2p")
			s.Reset()
			return
		}
		file, err = os.Create(fpath)
		if err != nil {
			p2pm.lm.Log("error", err.Error(), "p2p")
			s.Reset()
			return
		}
		defer file.Close()

		reader := bufio.NewReader(s)
		buf := make([]byte, chunkSize)

		for {
			n, err := reader.Read(buf)
			if err != nil {
				if err == io.EOF {
					break
				}
				p2pm.lm.Log("error", err.Error(), "p2p")
				s.Reset()
				return
			}
			file.Write(buf[:n])
		}

		// Uncompress received file
		err = utils.Uncompress(fpath, fdir)
		if err != nil {
			p2pm.lm.Log("error", err.Error(), "p2p")
			s.Close()
			return
		}

		// Send receipt acknowledgement
		go p2pm.sendReceiptAcknowledgement(streamData.WorkflowId, streamData.JobId, streamData.InterfaceId, orderingNodeId, receivingNodeId, peerId)

		err = os.RemoveAll(fpath)
		if err != nil {
			p2pm.lm.Log("error", err.Error(), "p2p")
			s.Close()
			return
		}
	default:
		message := fmt.Sprintf("Unknown stream type %d is received", streamData.Type)
		p2pm.lm.Log("warn", message, "p2p")
	}

	message := fmt.Sprintf("Receiving ended %s", s.ID())
	p2pm.lm.Log("debug", message, "p2p")

	s.Reset()
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
		p2pm.lm.Log("error", err.Error(), "p2p")
		return
	}

	err = StreamData(p2pm, peer, &receiptAcknowledgement, nil, nil)
	if err != nil {
		p2pm.lm.Log("error", err.Error(), "p2p")
		return
	}
}

func BroadcastMessage[T any](p2pm *P2PManager, message T) error {
	var m []byte
	var err error
	var topic *pubsub.Topic

	configManager := utils.NewConfigManager("")
	config, err := configManager.ReadConfigs()
	if err != nil {
		msg := fmt.Sprintf("Can not read configs file. (%s)", err.Error())
		p2pm.lm.Log("error", msg, "p2p")
		return err
	}

	switch v := any(message).(type) {
	case node_types.ServiceLookup:
		m, err = json.Marshal(message)
		if err != nil {
			p2pm.lm.Log("error", err.Error(), "p2p")
			return err
		}

		topicKey := config["topic_name_prefix"] + "lookup.service"
		topic = p2pm.topicsSubscribed[topicKey]

	default:
		msg := fmt.Sprintf("Message type %v is not allowed in this context (broadcasting message)", v)
		p2pm.lm.Log("error", msg, "p2p")
		err := errors.New(msg)
		return err
	}

	if err := topic.Publish(p2pm.ctx, m); err != nil {
		p2pm.lm.Log("error", err.Error(), "p2p")
		return err
	}

	return nil
}

func (p2pm *P2PManager) receivedMessage(ctx context.Context, sub *pubsub.Subscription) {
	configManager := utils.NewConfigManager("")
	config, err := configManager.ReadConfigs()
	if err != nil {
		message := fmt.Sprintf("Can not read configs file. (%s)", err.Error())
		p2pm.lm.Log("error", message, "p2p")
		return
	}

	for {
		if sub == nil {
			err := fmt.Errorf("subscription is nil. Will stop listening")
			p2pm.lm.Log("error", err.Error(), "p2p")
			break
		}

		m, err := sub.Next(ctx)
		if err != nil {
			p2pm.lm.Log("error", err.Error(), "p2p")
		}
		if m == nil {
			err := fmt.Errorf("message received is nil. Will stop listening")
			p2pm.lm.Log("error", err.Error(), "p2p")
			break
		}

		message := string(m.Message.Data)
		peerId := m.GetFrom()
		p2pm.lm.Log("debug", fmt.Sprintf("Message %s received from %s via %s", message, peerId.String(), m.ReceivedFrom), "p2p")

		topic := string(*m.Topic)
		p2pm.lm.Log("debug", fmt.Sprintf("Message topic is %s", topic), "p2p")

		switch topic {
		case config["topic_name_prefix"] + "lookup.service":
			services, err := p2pm.ServiceLookup(m.Message.Data, true)
			if err != nil {
				p2pm.lm.Log("error", err.Error(), "p2p")
				continue
			}

			// Retrieve known multiaddresses from the peerstore
			peerAddrInfo, err := p2pm.GeneratePeerAddrInfo(peerId.String())
			if err != nil {
				msg := err.Error()
				p2pm.lm.Log("error", msg, "p2p")
				continue
			}

			// Stream back offered services with prices
			// (only if there we have matching services to offer)
			if len(services) > 0 {
				err = StreamData(p2pm, peerAddrInfo, &services, nil, nil)
				if err != nil {
					msg := err.Error()
					p2pm.lm.Log("error", msg, "p2p")
					continue
				}
			}
		default:
			msg := fmt.Sprintf("Unknown topic %s", topic)
			p2pm.lm.Log("error", msg, "p2p")
			continue
		}
	}
}

func (p2pm *P2PManager) ServiceLookup(data []byte, active bool) ([]node_types.ServiceOffer, error) {
	configManager := utils.NewConfigManager("")
	config, err := configManager.ReadConfigs()
	if err != nil {
		message := fmt.Sprintf("Can not read configs file. (%s)", err.Error())
		p2pm.lm.Log("error", message, "p2p")
		return nil, err
	}

	var services []node_types.ServiceOffer
	var lookup node_types.ServiceLookup

	if len(data) == 0 {
		return services, nil
	}

	err = json.Unmarshal(data, &lookup)
	if err != nil {
		p2pm.lm.Log("error", err.Error(), "p2p")
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
	l := config["search_results"]
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
			p2pm.lm.Log("error", err.Error(), "p2p")
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
		p2pm.lm.Log("error", err.Error(), "p2p")
		return p, err
	}

	addrInfo, err := p2pm.idht.FindPeer(p2pm.ctx, pID)
	if err != nil {
		msg := err.Error()
		p2pm.lm.Log("error", msg, "p2p")
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

func (p2pm *P2PManager) makeRelayPeerInfo(peerAddrStr string) peer.AddrInfo {
	maddr, _ := ma.NewMultiaddr(peerAddrStr)
	peerAddrInfo, _ := peer.AddrInfoFromP2pAddr(maddr)
	return *peerAddrInfo
}

func (p2pm *P2PManager) GeneratePeerId(peerId string) (peer.ID, error) {
	// Convert the string to a peer.ID
	pID, err := peer.Decode(peerId)
	if err != nil {
		return pID, err
	}

	return pID, nil
}
