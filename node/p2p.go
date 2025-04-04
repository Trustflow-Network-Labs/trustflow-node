package node

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"slices"

	blacklist_node "github.com/adgsm/trustflow-node/blacklist-node"
	"github.com/adgsm/trustflow-node/keystore"
	"github.com/adgsm/trustflow-node/node_types"
	"github.com/adgsm/trustflow-node/settings"
	"github.com/adgsm/trustflow-node/utils"
	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/core/routing"
	drouting "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	dutil "github.com/libp2p/go-libp2p/p2p/discovery/util"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	libp2ptls "github.com/libp2p/go-libp2p/p2p/security/tls"
)

type P2PManager struct {
	topicNames         []string
	completeTopicNames []string
	topicsSubscribed   map[string]*pubsub.Topic
	protocolID         protocol.ID
	idht               *dht.IpfsDHT
	h                  host.Host
	ctx                context.Context
	lm                 *utils.LogsManager
}

func NewP2PManager() *P2PManager {
	return &P2PManager{
		topicNames:         []string{"lookup.service", "dummy.service"},
		completeTopicNames: []string{},
		topicsSubscribed:   make(map[string]*pubsub.Topic),
		protocolID:         "",
		idht:               nil,
		h:                  nil,
		ctx:                nil,
		lm:                 utils.NewLogsManager(),
	}
}

// IsHostRunning checks if the provided host is actively running
func (p2pm *P2PManager) IsHostRunning() bool {
	if p2pm.h == nil {
		return false
	}
	// Check if the host is listening on any network addresses
	return len(p2pm.h.Network().ListenAddresses()) > 0
}

// Set host context
func (p2pm *P2PManager) SetHostContext(c context.Context, hst host.Host) {
	p2pm.ctx = c
	p2pm.h = hst
}

func (p2pm *P2PManager) Start(port uint16, daemon bool) {

	// Read configs
	configManager := utils.NewConfigManager("")
	config, err := configManager.ReadConfigs()
	if err != nil {
		message := fmt.Sprintf("Can not read configs file. (%s)", err.Error())
		p2pm.lm.Log("error", message, "p2p")
		return
	}

	blacklistManager, err := blacklist_node.NewBlacklistNodeManager()
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
	topicNamePrefix := config["topic_name_prefix"]
	for _, topicName := range p2pm.topicNames {
		topicName = topicNamePrefix + strings.TrimSpace(topicName)
		p2pm.completeTopicNames = append(p2pm.completeTopicNames, topicName)
	}

	// Read streaming protocol
	p2pm.protocolID = protocol.ID(config["protocol_id"])

	cntx := context.Background()

	// Create or get previously created node key
	keystoreManager := keystore.NewKeyStoreManager()
	priv, _, err := keystoreManager.ProvideKey()
	if err != nil {
		p2pm.lm.Log("panic", err.Error(), "p2p")
		panic(fmt.Sprintf("%v", err))
	}

	hst, err := libp2p.New(
		// Use the keypair we generated
		libp2p.Identity(priv),
		// Multiple listen addresses
		libp2p.ListenAddrStrings(
			fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", port),
			fmt.Sprintf("/ip4/0.0.0.0/udp/%d/quic", port),
			fmt.Sprintf("/ip6/::1/tcp/%d", port),
			fmt.Sprintf("/ip6/::1/udp/%d/quic", port),
		),
		// support TLS connections
		libp2p.Security(libp2ptls.ID, libp2ptls.New),
		// support noise connections
		libp2p.Security(noise.ID, noise.New),
		// support any other default transports (TCP)
		libp2p.DefaultTransports,
		// Attempt to open ports using uPNP for NATed hosts.
		libp2p.NATPortMap(),
		// Let this host use the DHT to find other hosts
		libp2p.Routing(func(hst host.Host) (routing.PeerRouting, error) {
			p2pm.SetHostContext(cntx, hst)
			p2pm.idht, err = p2pm.initDHT()
			return p2pm.idht, err
		}),
		// If you want to help other peers to figure out if they are behind
		// NATs, you can launch the server-side of AutoNAT too (AutoRelay
		// already runs the client)
		//
		// This service is highly rate-limited and should not cause any
		// performance issues.
		libp2p.EnableNATService(),
		libp2p.ConnectionGater(blacklistManager.Gater),
	)
	if err != nil {
		p2pm.lm.Log("panic", err.Error(), "p2p")
		panic(fmt.Sprintf("%v", err))
	}

	message := fmt.Sprintf("Node ID is %s", hst.ID())
	p2pm.lm.Log("info", message, "p2p")
	for _, ma := range hst.Addrs() {
		message := fmt.Sprintf("Multiaddr is %s", ma.String())
		p2pm.lm.Log("info", message, "p2p")
	}

	// Setup a stream handler.
	// This gets called every time a peer connects and opens a stream to this node.
	p2pm.h.SetStreamHandler(p2pm.protocolID, func(s network.Stream) {
		message := fmt.Sprintf("Stream [protocol: %s] %s has been openned on node %s from node %s",
			p2pm.protocolID, s.ID(), hst.ID(), s.Conn().RemotePeer().String())
		p2pm.lm.Log("info", message, "p2p")
		go p2pm.streamProposalResponse(s)
	})

	peerChannel := make(chan []peer.AddrInfo)

	ps, err := pubsub.NewGossipSub(cntx, hst)
	if err != nil {
		p2pm.lm.Log("panic", err.Error(), "p2p")
		panic(fmt.Sprintf("%v", err))
	}

	go p2pm.discoverPeers(peerChannel)

	for _, completeTopicName := range p2pm.completeTopicNames {
		_, topic, err := p2pm.joinSubscribeTopic(cntx, ps, completeTopicName)
		if err != nil {
			p2pm.lm.Log("panic", err.Error(), "p2p")
			panic(fmt.Sprintf("%v", err))
		}

		notifyManager := utils.NewTopicAwareNotifiee(ps, topic, completeTopicName, peerChannel)

		// Attach the notifiee to the host's network
		p2pm.h.Network().Notify(notifyManager)
	}

	if !daemon {
		// Print interactive menu
		menuManager := NewMenuManager(p2pm)
		menuManager.Run()
	} else {
		// Running as a daemon never ends
		<-cntx.Done()
	}
}

func (p2pm *P2PManager) Stop(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}

	if runtime.GOOS == "windows" {
		// On Windows
		return process.Kill()
	} else {
		// On Unix-like systems
		//return process.Signal(syscall.SIGKILL)
		// Or for graceful termination:
		return process.Signal(syscall.SIGTERM)
	}
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

func (p2pm *P2PManager) initDHT() (*dht.IpfsDHT, error) {
	kademliaDHT, err := dht.New(p2pm.ctx, p2pm.h)
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
			if err := p2pm.h.Connect(p2pm.ctx, *peerinfo); err != nil {
				p2pm.lm.Log("warn", err.Error(), "p2p")
			}
		}()
	}

	return kademliaDHT, nil
}

func (p2pm *P2PManager) discoverPeers(peerChannel chan []peer.AddrInfo) {
	//serviceManager := NewServiceManager(p2pm)

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

				skip, err := p2pm.ConnectNode(peer)
				if err != nil {
					//p2pm.lm.Log("warn", err.Error(), "p2p")
					continue
				}
				if skip {
					continue
				}

				peerChannel <- discoveredPeers
				/*
					// Read list of active services (service offers with pricing)
					var data []byte
					var catalogueLookup node_types.ServiceLookup = node_types.ServiceLookup{
						Name:        "",
						Description: "",
						NodeId:      "",
						Type:        "",
						Repo:        "",
					}

					data, err = json.Marshal(catalogueLookup)
					if err != nil {
						p2pm.lm.Log("error", err.Error(), "p2p")
						continue
					}

					serviceCatalogue, err := p2pm.ServiceLookup(data, true)
					if err != nil {
						p2pm.lm.Log("error", err.Error(), "p2p")
						continue
					}

					// Stream it's own Service Catalogue
					err = StreamData(p2pm, peer, &serviceCatalogue)
					if err != nil {
						msg := err.Error()
						p2pm.lm.Log("error", msg, "p2p")
						continue
					}

					// Request connected peer's Service Catalogue
					//serviceManager.LookupRemoteService("", "", peer.ID.String(), "", "")
				*/
			}
		}
	}
	close(peerChannel)
	p2pm.lm.Log("debug", "Peer discovery complete", "p2p")
}

func (p2pm *P2PManager) IsNodeConnected(peer peer.AddrInfo) (bool, error) {
	running := p2pm.IsHostRunning()
	if !running {
		msg := "host is not running"
		p2pm.lm.Log("error", msg, "p2p")
		err := errors.New(msg)
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

func (p2pm *P2PManager) ConnectNode(peer peer.AddrInfo) (bool, error) {

	connected, err := p2pm.IsNodeConnected(peer)
	if err != nil {
		msg := err.Error()
		p2pm.lm.Log("error", msg, "p2p")
		return false, err
	}
	if connected {
		msg := fmt.Sprintf("Node %s is already connected", peer.ID.String())
		p2pm.lm.Log("warn", msg, "p2p")
		return true, nil // skip node but do not panic
	}

	if peer.ID == p2pm.h.ID() {
		return true, nil // skip node but do not panic
	}

	err = p2pm.h.Connect(p2pm.ctx, peer)
	if err != nil {
		p2pm.h.Network().ClosePeer(peer.ID)
		p2pm.h.Network().Peerstore().RemovePeer(peer.ID)
		p2pm.h.Peerstore().ClearAddrs(peer.ID)
		p2pm.h.Peerstore().RemovePeer(peer.ID)
		p2pm.lm.Log("debug", fmt.Sprintf("Removed peer %s from a peer store", peer.ID), "p2p")

		return false, err
	}

	p2pm.lm.Log("debug", fmt.Sprintf("Connected to: %s", peer.ID.String()), "p2p")
	for _, ma := range peer.Addrs {
		p2pm.lm.Log("debug", fmt.Sprintf("Connected peer's multiaddr is %s", ma.String()), "p2p")
	}

	return false, nil
}

func (p2pm *P2PManager) RequestData(peer peer.AddrInfo, jobId int64) error {
	_, err := p2pm.ConnectNode(peer)
	if err != nil {
		msg := err.Error()
		p2pm.lm.Log("error", msg, "p2p")
		return err
	}

	s, err := p2pm.h.NewStream(p2pm.ctx, peer.ID, p2pm.protocolID)
	if err != nil {
		p2pm.lm.Log("error", err.Error(), "p2p")
		s.Reset()
		return err
	} else {
		var str []byte = []byte(p2pm.h.ID().String())
		var str255 [255]byte
		copy(str255[:], str)
		go p2pm.streamProposal(s, str255, 1, jobId)
	}

	return nil
}

func StreamData[T any](p2pm *P2PManager, peer peer.AddrInfo, data T) error {
	_, err := p2pm.ConnectNode(peer)
	if err != nil {
		msg := err.Error()
		p2pm.lm.Log("error", msg, "p2p")
		return err
	}

	s, err := p2pm.h.NewStream(p2pm.ctx, peer.ID, p2pm.protocolID)
	if err != nil {
		p2pm.lm.Log("error", err.Error(), "p2p")
		return err
	} else {
		t := uint16(0)
		id := int64(0)
		switch v := any(data).(type) {
		case *[]node_types.ServiceOffer:
			t = 0
			id = 0
		case *[]byte:
			t = 2
			id = 0
		case *os.File:
			t = 3
			id = 0
		default:
			msg := fmt.Sprintf("Data type %v is not allowed in this context (streaming data)", v)
			p2pm.lm.Log("error", msg, "p2p")
			s.Reset()
			return errors.New(msg)
		}

		var str []byte = []byte(p2pm.h.ID().String())
		var str255 [255]byte
		copy(str255[:], str)
		go p2pm.streamProposal(s, str255, t, id)
		go sendStream(p2pm, s, data)
	}

	return nil
}

func (p2pm *P2PManager) streamProposal(s network.Stream, p [255]byte, t uint16, id int64) {
	// Create an instance of StreamData to write
	streamData := node_types.StreamData{
		Type:   t,
		Id:     id,
		PeerId: p,
	}

	// Send stream data
	if err := binary.Write(s, binary.BigEndian, streamData); err != nil {
		p2pm.lm.Log("error", err.Error(), "p2p")
		s.Reset()
	}
	message := fmt.Sprintf("Sending stream proposal %d (%d) has ended in stream %s", streamData.Type, streamData.Id, s.ID())
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

	message := fmt.Sprintf("Received stream data type %d, id %d from %s in stream %s",
		streamData.Type, streamData.Id, string(bytes.Trim(streamData.PeerId[:], "\x00")), s.ID())
	p2pm.lm.Log("debug", message, "p2p")

	// Check what the stream proposal is
	switch streamData.Type {
	case 0, 2, 3:
		// Request to receive a Service Catalogue from the remote peer
		// Check settings, do we want to accept receiving service catalogues and updates
		accepted := p2pm.streamProposalAssessment(streamData.Type)
		if accepted {
			p2pm.streamAccepted(s)
			go p2pm.receivedStream(s, streamData)
		} else {
			s.Reset()
		}
	case 1:
		// Request to send data to the remote peer
		// Check settings, do we want to accept sending data
		accepted := p2pm.streamProposalAssessment(streamData.Type)
		if accepted {
			jobManager := NewJobManager(p2pm)
			go jobManager.RunJob(streamData.Id)
			s.Reset()
		} else {
			s.Reset()
		}
	default:
		message := fmt.Sprintf("Unknown stream type %d is poposed", streamData.Type)
		p2pm.lm.Log("debug", message, "p2p")
		s.Reset()
	}
}

func (p2pm *P2PManager) streamProposalAssessment(streamDataType uint16) bool {
	var accepted bool
	settingsManager := settings.NewSettingsManager()
	// Check what the stream proposal is about
	switch streamDataType {
	case 0:
		// Request to receive a Service Catalogue from the remote peer
		// Check settings
		accepted = settingsManager.ReadBoolSetting("accept_service_catalogue")
	case 1:
		// Request to send data to the remote peer
		// Check settings
		accepted = settingsManager.ReadBoolSetting("accept_sending_data")
	case 2:
		// Request to receive a binary stream from the remote peer
		// Check settings
		accepted = settingsManager.ReadBoolSetting("accept_binary_stream")
	case 3:
		// Request to receive a file from the remote peer
		// Check settings
		accepted = settingsManager.ReadBoolSetting("accept_file")
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

	message := fmt.Sprintf("Sending TFREADY ended %s", s.ID())
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
	case *[]node_types.ServiceOffer:
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
		buffer := make([]byte, chunkSize)

		for {
			n, err := v.Read(buffer)
			if err != nil {
				if err == io.EOF {
					break // End of file reached
				}
				p2pm.lm.Log("error", err.Error(), "p2p")
				s.Reset()
				return
			}

			if n-int(pointer) < int(chunkSize) {
				chunkSize = uint64(n - int(pointer))
			}

			err = binary.Write(s, binary.BigEndian, chunkSize)
			if err != nil {
				p2pm.lm.Log("error", err.Error(), "p2p")
				s.Reset()
				return
			}
			message := fmt.Sprintf("Sending chunk size %d ended %s", chunkSize, s.ID())
			p2pm.lm.Log("debug", message, "p2p")

			if chunkSize == 0 {
				break
			}

			err = binary.Write(s, binary.BigEndian, buffer[:n])
			if err != nil {
				p2pm.lm.Log("error", err.Error(), "p2p")
				s.Reset()
				return
			}
			message = fmt.Sprintf("Sending chunk %v ended %s", buffer[:n], s.ID())
			p2pm.lm.Log("debug", message, "p2p")

			pointer += chunkSize
		}
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
		message := fmt.Sprintf("Sending chunk size %d ended %s", chunkSize, s.ID())
		p2pm.lm.Log("debug", message, "p2p")

		if chunkSize == 0 {
			break
		}

		err = binary.Write(s, binary.BigEndian, (b)[pointer:pointer+chunkSize])
		if err != nil {
			p2pm.lm.Log("error", err.Error(), "p2p")
			return err
		}
		message = fmt.Sprintf("Sending chunk %v ended %s", (b)[pointer:pointer+chunkSize], s.ID())
		p2pm.lm.Log("debug", message, "p2p")

		pointer += chunkSize
	}
	return nil
}

func (p2pm *P2PManager) receivedStream(s network.Stream, streamData node_types.StreamData) {
	// Prepare to read back the data
	var data []byte
	for {
		var chunkSize uint64
		err := binary.Read(s, binary.BigEndian, &chunkSize)
		if err != nil {
			p2pm.lm.Log("error", err.Error(), "p2p")
			s.Reset()
		}

		message := fmt.Sprintf("Received chunk size %d from %s", chunkSize, s.ID())
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

		message = fmt.Sprintf("Received %v of type %d id %d from %s", chunk, streamData.Type, streamData.Id, s.ID())
		p2pm.lm.Log("debug", message, "p2p")

		// Concatenate receiving chunk
		data = append(data, chunk...)
	}

	message := fmt.Sprintf("Received data %v type from %s is %d, id %d, node %s", data, s.ID(),
		streamData.Type, streamData.Id, string(bytes.Trim(streamData.PeerId[:], "\x00")))
	p2pm.lm.Log("debug", message, "p2p")

	// Determine data type
	switch streamData.Type {
	case 0:
		// Received a Service Catalogue from the remote peer
		var serviceCatalogue []node_types.ServiceOffer
		err := json.Unmarshal(data, &serviceCatalogue)
		if err != nil {
			msg := fmt.Sprintf("Could not load received binary stream into a Service Catalogue struct.\n\n%s", err.Error())
			p2pm.lm.Log("error", msg, "p2p")
		}
		serviceManager := NewServiceManager(p2pm)
		for i, service := range serviceCatalogue {
			// Remote node ID
			nodeId := service.NodeId
			localNodeId := s.Conn().LocalPeer().String()
			// Skip if this is our service advertized on remote node
			if localNodeId == nodeId {
				continue
			}
			// Delete currently stored service catalogue (if existing)
			if i == 0 {
				services, err := serviceManager.GetServicesByNodeId(nodeId)
				if err != nil {
					msg := fmt.Sprintf("Error occured whilst looking for existing Service Catalogue of a foreign node %s.\n\n%s", nodeId, err.Error())
					p2pm.lm.Log("error", msg, "p2p")
				}
				for _, service := range services {
					serviceManager.Remove(service.Id)
					msg := fmt.Sprintf("Removing service %s of a foreign node %s.\n\n", service.Name, nodeId)
					p2pm.lm.Log("debug", msg, "p2p")
				}
			}
			// Store newly received Service Catalogue
			serviceManager.Add(service.Name, service.Description, service.Type, service.Active)
		}
	case 1:
		// Sent data to the remote peer
	case 2:
		// Received binary stream from the remote peer
	case 3:
		// Received a file from the remote peer
	default:
		message := fmt.Sprintf("Unknown stream type %d is received", streamData.Type)
		p2pm.lm.Log("warn", message, "p2p")
	}

	message = fmt.Sprintf("Receiving ended %s", s.ID())
	p2pm.lm.Log("debug", message, "p2p")

	s.Reset()
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

		fmt.Printf("topicKey: %s,\ntopic: %v\ntopicsSubscribed: %v\n", topicKey, topic, p2pm.topicsSubscribed)
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
		m, err := sub.Next(ctx)
		if err != nil {
			p2pm.lm.Log("error", err.Error(), "p2p")
		}

		message := string(m.Message.Data)
		fmt.Println(m.ReceivedFrom, " Data: ", message)
		p2pm.lm.Log("debug", fmt.Sprintf("Message %s received from %s", message, m.ReceivedFrom), "p2p")

		topic := string(*m.Topic)
		fmt.Println(m.ReceivedFrom, " Topic: ", topic)
		p2pm.lm.Log("debug", fmt.Sprintf("Message topic is %s", topic), "p2p")

		peerId := m.ReceivedFrom
		p := peerId.String()
		fmt.Println(m.ReceivedFrom, " Peer: ", p)
		p2pm.lm.Log("debug", fmt.Sprintf("From peer %s", p), "p2p")

		switch topic {
		case config["topic_name_prefix"] + "lookup.service":
			services, err := p2pm.ServiceLookup(m.Message.Data, true)
			if err != nil {
				p2pm.lm.Log("error", err.Error(), "p2p")
				continue
			}

			// Retrieve known multiaddresses from the peerstore
			peerAddrInfo, err := p2pm.GeneratePeerFromId(p)
			if err != nil {
				msg := err.Error()
				p2pm.lm.Log("error", msg, "p2p")
				continue
			}

			// Stream back offered services with prices
			// (only if there we have matching services to offer)
			if len(services) > 0 {
				err = StreamData(p2pm, peerAddrInfo, &services)
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

func (p2pm *P2PManager) GeneratePeerFromId(peerId string) (peer.AddrInfo, error) {
	var p peer.AddrInfo

	// Convert the string to a peer.ID
	pID, err := peer.Decode(peerId)
	if err != nil {
		msg := err.Error()
		p2pm.lm.Log("warn", msg, "p2p")
		return p, err
	}

	addrInfo, err := p2pm.idht.FindPeer(p2pm.ctx, pID)
	if err != nil {
		msg := err.Error()
		p2pm.lm.Log("warn", msg, "p2p")
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
