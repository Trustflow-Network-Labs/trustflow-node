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
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"slices"

	blacklist_node "github.com/adgsm/trustflow-node/blacklist-node"
	"github.com/adgsm/trustflow-node/database"
	"github.com/adgsm/trustflow-node/keystore"
	"github.com/adgsm/trustflow-node/node_types"
	"github.com/adgsm/trustflow-node/settings"
	"github.com/adgsm/trustflow-node/utils"
	"github.com/adgsm/trustflow-node/workflow"
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
	daemon             bool
	topicNames         []string
	completeTopicNames []string
	topicsSubscribed   map[string]*pubsub.Topic
	protocolID         protocol.ID
	idht               *dht.IpfsDHT
	h                  host.Host
	ctx                context.Context
	db                 *sql.DB
	lm                 *utils.LogsManager
	wm                 *workflow.WorkflowManager
	sc                 *node_types.ServiceOffersCache
}

func NewP2PManager() *P2PManager {
	// Create a database connection
	sqlm := database.NewSQLiteManager()
	db, err := sqlm.CreateConnection()
	if err != nil {
		panic(err)
	}

	return &P2PManager{
		daemon:             false,
		topicNames:         []string{"lookup.service", "dummy.service"},
		completeTopicNames: []string{},
		topicsSubscribed:   make(map[string]*pubsub.Topic),
		protocolID:         "",
		idht:               nil,
		h:                  nil,
		ctx:                nil,
		db:                 db,
		lm:                 utils.NewLogsManager(),
		wm:                 workflow.NewWorkflowManager(db),
		sc:                 node_types.NewServiceOffersCache(),
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
	p2pm.daemon = daemon

	// Read configs
	configManager := utils.NewConfigManager("")
	config, err := configManager.ReadConfigs()
	if err != nil {
		message := fmt.Sprintf("Can not read configs file. (%s)", err.Error())
		p2pm.lm.Log("error", message, "p2p")
		return
	}

	blacklistManager, err := blacklist_node.NewBlacklistNodeManager(p2pm.db)
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
	keystoreManager := keystore.NewKeyStoreManager(p2pm.db)
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

	// Start crons
	cronManager := NewCronManager(p2pm)
	err = cronManager.JobQueue()
	if err != nil {
		p2pm.lm.Log("panic", err.Error(), "p2p")
		panic(fmt.Sprintf("%v", err))
	}

	if !daemon {
		// Print interactive menu
		menuManager := NewMenuManager(p2pm)
		menuManager.Run()
	} else {
		// Running as a daemon never ends
		<-cntx.Done()
	}

	// When host is stopped close DB connection
	if err = p2pm.db.Close(); err != nil {
		p2pm.lm.Log("error", err.Error(), "p2p")
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

func StreamData[T any](p2pm *P2PManager, peer peer.AddrInfo, data T, job *node_types.Job, existingStream network.Stream) error {
	_, err := p2pm.ConnectNode(peer)
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
			p2pm.lm.Log("debug", fmt.Sprintf("Reusing existing stream with %s", peer.ID), "p2p")
		} else {
			// Stream is broken, discard and create a new one
			p2pm.lm.Log("debug", fmt.Sprintf("Existing stream with %s is not usable: %v", peer.ID, err), "p2p")
		}
	}

	if s == nil {
		s, err = p2pm.h.NewStream(p2pm.ctx, peer.ID, p2pm.protocolID)
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
	default:
		msg := fmt.Sprintf("Data type %v is not allowed in this context (streaming data)", v)
		p2pm.lm.Log("error", msg, "p2p")
		s.Reset()
		return errors.New(msg)
	}

	var str []byte = []byte(p2pm.h.ID().String())
	var str255 [255]byte
	copy(str255[:], str)
	workflowId := int64(0)
	jobId := int64(0)
	if job != nil {
		workflowId = job.WorkflowId
		jobId = job.Id
	}
	go p2pm.streamProposal(s, str255, t, workflowId, jobId)
	go sendStream(p2pm, s, data)

	return nil
}

func (p2pm *P2PManager) streamProposal(s network.Stream, p [255]byte, t uint16, wid int64, jid int64) {
	// Create an instance of StreamData to write
	streamData := node_types.StreamData{
		Type:       t,
		PeerId:     p,
		WorkflowId: wid,
		JobId:      jid,
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
	settingsManager := settings.NewSettingsManager(p2pm.db)
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
		*node_types.JobRunStatus, *node_types.JobRunStatusRequest:
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
	case 0, 1, 4, 5, 6, 7, 8:
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
				p2pm.sc.AddOrUpdate(service)
				// If we are in interactive mode print Service Offer to CLI
				if !p2pm.daemon {
					menuManager := NewMenuManager(p2pm)
					menuManager.printOfferedService(service)
				} else {
					// TODO, Otherwise, if we are connected with other client (web, gui) push message
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
				err = p2pm.wm.AddWorkflowJob(serviceResponse.WorkflowId, serviceResponse.NodeId,
					serviceResponse.JobId, serviceResponse.Message)
				if err != nil {
					msg := fmt.Sprintf("Could not add workflow job (%s-%d) to workflow %d.\n\n%s",
						serviceResponse.NodeId, serviceResponse.JobId, serviceResponse.WorkflowId, err.Error())
					p2pm.lm.Log("error", msg, "p2p")
					s.Reset()
					return
				}
			}

			// Draw table output
			// If we are in interactive mode print Service Offer to CLI
			if !p2pm.daemon {
				menuManager := NewMenuManager(p2pm)
				menuManager.printServiceResponse(serviceResponse)
			} else {
				// TODO, Otherwise, if we are connected with other client (web, gui) push message
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
			workflowManager := workflow.NewWorkflowManager(p2pm.db)
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
			if !p2pm.daemon {
				menuManager := NewMenuManager(p2pm)
				menuManager.printJobRunResponse(jobRunResponse)
			} else {
				// TODO, Otherwise, if we are connected with other client (web, gui) push message
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
			workflowManager := workflow.NewWorkflowManager(p2pm.db)
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

			err = jobManager.StatusUpdate(job, job.Status)
			if err != nil {
				p2pm.lm.Log("error", err.Error(), "p2p")
				s.Close()
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

		// Read chunk size form configs
		configManager := utils.NewConfigManager("")
		configs, err := configManager.ReadConfigs()
		if err != nil {
			p2pm.lm.Log("error", err.Error(), "p2p")
			s.Reset()
			return
		}

		peerId := s.Conn().RemotePeer().String()
		//		fdir = configs["received_files_storage"] + fmt.Sprintf("%d", streamData.WorkflowId) + "/" + peerId + "/" + fmt.Sprintf("%d", streamData.JobId) + "/"
		fdir = configs["received_files_storage"] + peerId + "/" + fmt.Sprintf("%d", streamData.WorkflowId) + "/"
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

		/* TODO, allow remote nodes to receive files per requests from a workflow
		// Create CID
		cid, err := utils.HashFileToCID(fpath)
		if err != nil {
			p2pm.lm.Log("error", err.Error(), "p2p")
			return
		}
		// Check are we expecting this file
		expected, err := p2pm.wm.ExpectedOutputFound(peerId, cid)
		if err != nil || !expected {
			p2pm.lm.Log("error", err.Error(), "p2p")
			os.RemoveAll(fpath)
			return
		}
		*/
		// Uncompress received file
		//		err = utils.Uncompress(fpath, fdir+cid+"/")
		err = utils.Uncompress(fpath, fdir)
		if err != nil {
			p2pm.lm.Log("error", err.Error(), "p2p")
			s.Close()
			return
		}

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
		m, err := sub.Next(ctx)
		if err != nil {
			p2pm.lm.Log("error", err.Error(), "p2p")
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
		p2pm.lm.Log("warn", err.Error(), "p2p")
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

func (p2pm *P2PManager) GeneratePeerId(peerId string) (peer.ID, error) {
	// Convert the string to a peer.ID
	pID, err := peer.Decode(peerId)
	if err != nil {
		return pID, err
	}

	return pID, nil
}
