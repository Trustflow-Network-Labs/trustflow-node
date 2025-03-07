package shared

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"

	blacklist_node "github.com/adgsm/trustflow-node/cmd/blacklist-node"
	"github.com/adgsm/trustflow-node/cmd/settings"
	"github.com/adgsm/trustflow-node/keystore"
	"github.com/adgsm/trustflow-node/node_types"
	"github.com/adgsm/trustflow-node/tfnode"
	"github.com/adgsm/trustflow-node/utils"
	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/core/routing"
	drouting "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	dutil "github.com/libp2p/go-libp2p/p2p/discovery/util"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	libp2ptls "github.com/libp2p/go-libp2p/p2p/security/tls"
	"github.com/manifoldco/promptui"
	"github.com/multiformats/go-multiaddr"
)

var (
	topicNames       []string = []string{"lookup.service", "dummy.service"}
	topicNameFlags   []*string
	topicsSubscribed map[string]*pubsub.Topic = make(map[string]*pubsub.Topic)
	protocolID       protocol.ID
	idht             *dht.IpfsDHT
)

// provide configs file path
var configsPath string = "configs"

var h host.Host
var ctx context.Context

// IsHostRunning checks if the provided host is actively running
func IsHostRunning() bool {
	_, hst := GetHostContext()
	if h == nil {
		return false
	}
	// Check if the host is listening on any network addresses
	return len(hst.Network().ListenAddresses()) > 0
}

// Get host context
func GetHostContext() (context.Context, host.Host) {
	return ctx, h
}

// Set host context
func SetHostContext(c context.Context, hst host.Host) {
	ctx = c
	h = hst
}

func Start(port uint16, daemon bool) {
	// Read configs
	config, err := utils.ReadConfigs(configsPath)
	if err != nil {
		message := fmt.Sprintf("Can not read configs file. (%s)", err.Error())
		utils.Log("error", message, "p2p")
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
	for i, topicName := range topicNames {
		topicName = topicNamePrefix + strings.TrimSpace(topicName)
		topicNameFlag := flag.String(fmt.Sprintf("topicName_%d", i), topicName, fmt.Sprintf("topic no %d to join", i))
		topicNameFlags = append(topicNameFlags, topicNameFlag)
	}

	// Read streaming protocol
	protocolID = protocol.ID(config["protocol_id"])

	flag.Parse()

	cntx := context.Background()

	// Create or get previously created node key
	priv, _, err := tfnode.GetNodeKey()
	if err != nil {
		utils.Log("panic", err.Error(), "p2p")
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
			idht, err = dht.New(cntx, hst)
			return idht, err
		}),
		// If you want to help other peers to figure out if they are behind
		// NATs, you can launch the server-side of AutoNAT too (AutoRelay
		// already runs the client)
		//
		// This service is highly rate-limited and should not cause any
		// performance issues.
		libp2p.EnableNATService(),
	)
	if err != nil {
		utils.Log("panic", err.Error(), "p2p")
		panic(fmt.Sprintf("%v", err))
	}

	SetHostContext(context.Background(), hst)

	var multiaddrs []string
	message := fmt.Sprintf("Node ID is %s", hst.ID())
	utils.Log("info", message, "p2p")
	for _, ma := range hst.Addrs() {
		message := fmt.Sprintf("Multiaddr is %s", ma.String())
		utils.Log("info", message, "p2p")
		multiaddrs = append(multiaddrs, ma.String())
	}

	// Check if node is already existing in the DB
	_, err = tfnode.FindNode(hst.ID().String())
	if err != nil {
		// Add node
		err = tfnode.AddNode(hst.ID().String(), strings.Join(multiaddrs, ","), true)
		if err != nil {
			utils.Log("panic", err.Error(), "p2p")
			panic(fmt.Sprintf("%v", err))
		}

		// Add key
		key, err := crypto.MarshalPrivateKey(priv)
		if err != nil {
			utils.Log("panic", err.Error(), "p2p")
			panic(fmt.Sprintf("%v", err))
		}
		err = keystore.AddKey(hst.ID().String(), fmt.Sprintf("%s: secp256r1", priv.Type().String()), key)
		if err != nil {
			utils.Log("panic", err.Error(), "p2p")
			panic(fmt.Sprintf("%v", err))
		}
	} else {
		// Update node
		err = tfnode.UpdateNode(hst.ID().String(), strings.Join(multiaddrs, ","), true)
		if err != nil {
			utils.Log("panic", err.Error(), "p2p")
			panic(err)
		}
	}

	// Setup a stream handler.
	// This gets called every time a peer connects and opens a stream to this node.
	h.SetStreamHandler(protocolID, func(s network.Stream) {
		message := fmt.Sprintf("Stream [protocol: %s] %s has been openned on node %s from node %s", protocolID, s.ID(), hst.ID(), s.Conn().RemotePeer().String())
		utils.Log("info", message, "p2p")
		go streamProposalResponse(s)
	})

	go discoverPeers(cntx, hst)

	ps, err := pubsub.NewGossipSub(cntx, hst)
	if err != nil {
		utils.Log("panic", err.Error(), "p2p")
		panic(fmt.Sprintf("%v", err))
	}

	for _, topicNameFlag := range topicNameFlags {
		err = joinSubscribeTopic(cntx, ps, topicNameFlag)
		if err != nil {
			utils.Log("panic", err.Error(), "p2p")
			panic(fmt.Sprintf("%v", err))
		}
	}

	if !daemon {
		// Print interactive menu
		runMenu()
	} else {
		// Running as a daemon never ends
		<-ctx.Done()
	}
}

func Stop(pid int) error {
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

func joinSubscribeTopic(cntx context.Context, ps *pubsub.PubSub, topicNameFlag *string) error {
	topic, err := ps.Join(*topicNameFlag)
	if err != nil {
		utils.Log("error", err.Error(), "p2p")
		return err
	}

	sub, err := topic.Subscribe()
	if err != nil {
		utils.Log("error", err.Error(), "p2p")
		return err
	}

	topicsSubscribed[*topicNameFlag] = topic

	go receivedMessage(cntx, sub)

	return nil
}

// Interactive menu loop
func runMenu() {
	for {
		prompt := promptui.Select{
			Label: "Choose an option",
			Items: []string{"Find remote services", "Find local services", "Exit"},
		}

		_, result, err := prompt.Run()
		if err != nil {
			msg := fmt.Sprintf("Prompt failed: %s", err.Error())
			fmt.Println(msg)
			utils.Log("error", msg, "p2p")
			return
		}

		switch result {
		case "Find remote services":
			frsPrompt := promptui.Prompt{
				Label:       "Service name",
				Default:     "New service",
				AllowEdit:   true,
				HideEntered: false,
				IsConfirm:   false,
				IsVimMode:   false,
			}
			snResult, err := frsPrompt.Run()
			if err != nil {
				msg := fmt.Sprintf("Entering service name failed: %s", err.Error())
				fmt.Println(msg)
				utils.Log("error", msg, "p2p")
				return
			}
			serviceManager := NewServiceManager()
			serviceManager.LookupRemoteService(snResult, "", "", "", "")
		case "Find local services":
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
				utils.Log("error", err.Error(), "p2p")
				continue
			}

			serviceCatalogue, err := serviceLookup(data, true)
			if err != nil {
				utils.Log("error", err.Error(), "p2p")
				continue
			}

			fmt.Printf("%v\n", serviceCatalogue)
		case "Exit":
			msg := "Exiting interactive mode..."
			fmt.Println(msg)
			utils.Log("info", msg, "p2p")
			return
		}
	}
}

func initDHT(cntx context.Context, hst host.Host) *dht.IpfsDHT {
	// Start a DHT, for use in peer discovery. We can't just make a new DHT
	// client because we want each peer to maintain its own local copy of the
	// DHT, so that the bootstrapping node of the DHT can go down without
	// inhibiting future peer discovery.
	kademliaDHT, err := dht.New(cntx, hst)
	if err != nil {
		utils.Log("panic", err.Error(), "p2p")
		panic(fmt.Sprintf("%v", err))
	}
	if err = kademliaDHT.Bootstrap(cntx); err != nil {
		utils.Log("panic", err.Error(), "p2p")
		panic(fmt.Sprintf("%v", err))
	}
	var wg sync.WaitGroup
	for _, peerAddr := range dht.DefaultBootstrapPeers {
		peerinfo, _ := peer.AddrInfoFromP2pAddr(peerAddr)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := hst.Connect(cntx, *peerinfo); err != nil {
				utils.Log("warn", err.Error(), "p2p")
			}
		}()
	}
	wg.Wait()

	return kademliaDHT
}

func discoverPeers(cntx context.Context, hst host.Host) {
	kademliaDHT := initDHT(cntx, hst)
	routingDiscovery := drouting.NewRoutingDiscovery(kademliaDHT)
	for _, topicNameFlag := range topicNameFlags {
		dutil.Advertise(cntx, routingDiscovery, *topicNameFlag)
	}

	// Look for others who have announced and attempt to connect to them
	anyConnected := false
	for !anyConnected {
		utils.Log("debug", "Searching for peers...", "p2p")
		for _, topicNameFlag := range topicNameFlags {
			peerChan, err := routingDiscovery.FindPeers(cntx, *topicNameFlag)
			if err != nil {
				utils.Log("panic", err.Error(), "p2p")
				panic(fmt.Sprintf("%v", err))
			}
			for peer := range peerChan {
				skip, err := ConnectNode(peer)
				if err != nil {
					msg := err.Error()
					utils.Log("error", msg, "p2p")
					panic(err)
				}
				if skip {
					msg := "Skipping node"
					utils.Log("info", msg, "p2p")
					continue
				}

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
					utils.Log("error", err.Error(), "p2p")
					continue
				}

				serviceCatalogue, err := serviceLookup(data, true)
				if err != nil {
					utils.Log("error", err.Error(), "p2p")
					continue
				}

				err = StreamData(peer, &serviceCatalogue)
				if err != nil {
					msg := err.Error()
					utils.Log("error", msg, "p2p")
					continue
				}
			}
		}
	}
	utils.Log("debug", "Peer discovery complete", "p2p")
}

func IsNodeConnected(peer peer.AddrInfo) (bool, error) {
	running := IsHostRunning()
	if !running {
		msg := "host is not running"
		utils.Log("error", msg, "p2p")
		err := errors.New(msg)
		return false, err
	}
	_, hst := GetHostContext()

	connected := false

	// Check for connected peers
	connectedPeers := hst.Network().Peers()
	for _, peerID := range connectedPeers {
		if peerID == peer.ID {
			connected = true
			break
		}
	}

	return connected, nil
}

func ConnectNode(peer peer.AddrInfo) (bool, error) {
	connected, err := IsNodeConnected(peer)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "p2p")
		return false, err
	}
	if connected {
		msg := fmt.Sprintf("Node %s is already connected", peer.ID.String())
		utils.Log("warn", msg, "p2p")
		return true, nil // skip node but do not panic
	}

	cntx, hst := GetHostContext()

	if peer.ID == hst.ID() {
		msg := fmt.Sprintf("No self connection allowed. Trying to connect peer %s from host %s.", peer.ID.String(), hst.ID().String())
		utils.Log("warn", msg, "p2p")
		return true, nil // skip node but do not panic
	}
	err, blacklisted := blacklist_node.NodeBlacklisted(peer.ID.String())
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "p2p")
		return false, err
	}
	if blacklisted {
		msg := fmt.Sprintf("Node %s is blacklisted", peer.ID.String())
		utils.Log("warn", msg, "p2p")
		return true, nil // skip node but do not panic
	}

	err = hst.Connect(cntx, peer)
	if err != nil {
		utils.Log("debug", fmt.Sprintf("Failed connecting to %s, error: %s", peer.ID, err), "p2p")
		utils.Log("debug", fmt.Sprintf("Delete node %s from the DB, error: %s", peer.ID, err), "p2p")
		err = tfnode.DeleteNode(peer.ID.String())
		if err != nil {
			utils.Log("warn", fmt.Sprintf("Failed deleting node %s, error: %s", peer.ID, err), "p2p")
		}
		hst.Network().ClosePeer(peer.ID)
		hst.Network().Peerstore().RemovePeer(peer.ID)
		hst.Peerstore().ClearAddrs(peer.ID)
		hst.Peerstore().RemovePeer(peer.ID)
		utils.Log("debug", fmt.Sprintf("Removed peer %s from a peer store", peer.ID), "p2p")
	} else {
		utils.Log("debug", fmt.Sprintf("Connected to: %s", peer.ID.String()), "p2p")
		// Determine multiaddrs
		var multiaddrs []string
		for _, ma := range peer.Addrs {
			utils.Log("debug", fmt.Sprintf("Connected peer's multiaddr is %s", ma.String()), "p2p")
			multiaddrs = append(multiaddrs, ma.String())
		}
		// Check if node is already existing in the DB
		_, err := tfnode.FindNode(peer.ID.String())
		if err != nil {
			// Add new nodes to DB
			utils.Log("debug", fmt.Sprintf("add node %s with multiaddrs %s", peer.ID.String(), strings.Join(multiaddrs, ",")), "p2p")
			err = tfnode.AddNode(peer.ID.String(), strings.Join(multiaddrs, ","), false)
			if err != nil {
				utils.Log("error", err.Error(), "p2p")
				return true, nil // skip node but do not panic
			}
		} else {
			// Update node
			utils.Log("debug", fmt.Sprintf("update node %s with multiaddrs %s", peer.ID.String(), strings.Join(multiaddrs, ",")), "p2p")
			err = tfnode.UpdateNode(peer.ID.String(), strings.Join(multiaddrs, ","), false)
			if err != nil {
				utils.Log("error", err.Error(), "p2p")
				return true, nil // skip node but do not panic
			}
		}
	}

	return false, nil
}

func RequestData(peer peer.AddrInfo, jobId int32) error {
	_, err := ConnectNode(peer)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "p2p")
		return err
	}

	cntx, hst := GetHostContext()

	s, err := hst.NewStream(cntx, peer.ID, protocolID)
	if err != nil {
		utils.Log("error", err.Error(), "p2p")
		s.Reset()
		return err
	} else {
		var str []byte = []byte(hst.ID().String())
		var str255 [255]byte
		copy(str255[:], str)
		go streamProposal(s, str255, 1, jobId)
	}

	return nil
}

func StreamData[T any](peer peer.AddrInfo, data T) error {
	_, err := ConnectNode(peer)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "p2p")
		return err
	}

	cntx, hst := GetHostContext()

	s, err := hst.NewStream(cntx, peer.ID, protocolID)
	if err != nil {
		utils.Log("error", err.Error(), "p2p")
		return err
	} else {
		t := uint16(0)
		id := int32(0)
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
			utils.Log("error", msg, "p2p")
			s.Reset()
			return errors.New(msg)
		}

		var str []byte = []byte(hst.ID().String())
		var str255 [255]byte
		copy(str255[:], str)
		go streamProposal(s, str255, t, id)
		go sendStream(s, data)
	}

	return nil
}

func streamProposal(s network.Stream, p [255]byte, t uint16, id int32) {
	// Create an instance of StreamData to write
	streamData := node_types.StreamData{
		Type:   t,
		Id:     id,
		PeerId: p,
	}

	// Send stream data
	if err := binary.Write(s, binary.BigEndian, streamData); err != nil {
		utils.Log("error", err.Error(), "p2p")
		s.Reset()
	}
	message := fmt.Sprintf("Sending stream proposal %d (%d) has ended in stream %s", streamData.Type, streamData.Id, s.ID())
	utils.Log("debug", message, "p2p")
}

func streamProposalResponse(s network.Stream) {
	// Prepare to read the stream data
	var streamData node_types.StreamData
	err := binary.Read(s, binary.BigEndian, &streamData)
	if err != nil {
		utils.Log("error", err.Error(), "p2p")
		s.Reset()
	}

	message := fmt.Sprintf("Received stream data type %d, id %d from %s in stream %s",
		streamData.Type, streamData.Id, string(bytes.Trim(streamData.PeerId[:], "\x00")), s.ID())
	utils.Log("debug", message, "p2p")

	// Check what the stream proposal is
	switch streamData.Type {
	case 0, 2, 3:
		// Request to receive a Service Catalogue from the remote peer
		// Check settings, do we want to accept receiving service catalogues and updates
		accepted := streamProposalAssessment(streamData.Type)
		if accepted {
			streamAccepted(s)
			go receivedStream(s, streamData)
		} else {
			s.Reset()
		}
	case 1:
		// Request to send data to the remote peer
		// Check settings, do we want to accept sending data
		accepted := streamProposalAssessment(streamData.Type)
		if accepted {
			jobManager := NewJobManager()
			go jobManager.RunJob(streamData.Id)
			s.Reset()
		} else {
			s.Reset()
		}
	default:
		message := fmt.Sprintf("Unknown stream type %d is poposed", streamData.Type)
		utils.Log("debug", message, "p2p")
		s.Reset()
	}
}

func streamProposalAssessment(streamDataType uint16) bool {
	var accepted bool
	// Check what the stream proposal is about
	switch streamDataType {
	case 0:
		// Request to receive a Service Catalogue from the remote peer
		// Check settings
		accepted = readBoolSetting("accept_service_catalogue")
	case 1:
		// Request to send data to the remote peer
		// Check settings
		accepted = readBoolSetting("accept_sending_data")
	case 2:
		// Request to receive a binary stream from the remote peer
		// Check settings
		accepted = readBoolSetting("accept_binary_stream")
	case 3:
		// Request to receive a file from the remote peer
		// Check settings
		accepted = readBoolSetting("accept_file")
	default:
		message := fmt.Sprintf("Unknown stream type %d is proposed", streamDataType)
		utils.Log("debug", message, "p2p")
		return false
	}
	if accepted {
		message := fmt.Sprintf("As per local settings stream data type %d is accepted.", streamDataType)
		utils.Log("debug", message, "p2p")
	} else {
		message := fmt.Sprintf("As per local settings stream data type %d is not accepted", streamDataType)
		utils.Log("debug", message, "p2p")
	}

	return accepted
}

func readBoolSetting(key string) bool {
	b, err := settings.Read(key)
	if err != nil {
		utils.Log("error", err.Error(), "p2p")
		return false
	}
	bval, ok := b.(bool)
	if ok {
		return bval
	}
	return false
}

func streamAccepted(s network.Stream) {
	data := [7]byte{'T', 'F', 'R', 'E', 'A', 'D', 'Y'}
	err := binary.Write(s, binary.BigEndian, data)
	if err != nil {
		utils.Log("error", err.Error(), "p2p")
		s.Reset()
	}

	message := fmt.Sprintf("Sending TFREADY ended %s", s.ID())
	utils.Log("debug", message, "p2p")
}

func sendStream[T any](s network.Stream, data T) {
	var ready [7]byte
	var expected [7]byte = [7]byte{'T', 'F', 'R', 'E', 'A', 'D', 'Y'}

	err := binary.Read(s, binary.BigEndian, &ready)
	if err != nil {
		utils.Log("error", err.Error(), "p2p")
		s.Reset()
		return
	}

	// Check if received data matches TFREADY signal
	if !bytes.Equal(expected[:], ready[:]) {
		err := errors.New("did not get expected TFREADY signal")
		utils.Log("error", err.Error(), "p2p")
		s.Reset()
		return
	}

	message := fmt.Sprintf("Received %s from %s", string(ready[:]), s.ID())
	utils.Log("debug", message, "p2p")

	var chunkSize uint64 = 4096
	var pointer uint64 = 0

	// Load configs
	config, err := utils.ReadConfigs(configsPath)
	if err != nil {
		message := fmt.Sprintf("Can not read configs file. (%s)", err.Error())
		utils.Log("error", message, "p2p")
		return
	}

	// Read chunk size form configs
	cs := config["chunk_size"]
	chunkSize, err = strconv.ParseUint(cs, 10, 64)
	if err != nil {
		message := fmt.Sprintf("Invalid chunk size in configs file. Will set to the default chunk size (%s)", err.Error())
		utils.Log("warn", message, "p2p")
	}

	switch v := any(data).(type) {
	case *[]node_types.ServiceOffer:
		b, err := json.Marshal(data)
		if err != nil {
			utils.Log("error", err.Error(), "p2p")
			s.Reset()
			return
		}

		err = sendStreamChunks(b, pointer, chunkSize, s)
		if err != nil {
			utils.Log("error", err.Error(), "p2p")
			s.Reset()
			return
		}
	case *[]byte:
		err = sendStreamChunks(*v, pointer, chunkSize, s)
		if err != nil {
			utils.Log("error", err.Error(), "p2p")
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
				utils.Log("error", err.Error(), "p2p")
				s.Reset()
				return
			}

			if n-int(pointer) < int(chunkSize) {
				chunkSize = uint64(n - int(pointer))
			}

			err = binary.Write(s, binary.BigEndian, chunkSize)
			if err != nil {
				utils.Log("error", err.Error(), "p2p")
				s.Reset()
				return
			}
			message := fmt.Sprintf("Sending chunk size %d ended %s", chunkSize, s.ID())
			utils.Log("debug", message, "p2p")

			if chunkSize == 0 {
				break
			}

			err = binary.Write(s, binary.BigEndian, buffer[:n])
			if err != nil {
				utils.Log("error", err.Error(), "p2p")
				s.Reset()
				return
			}
			message = fmt.Sprintf("Sending chunk %v ended %s", buffer[:n], s.ID())
			utils.Log("debug", message, "p2p")

			pointer += chunkSize
		}
	default:
		msg := fmt.Sprintf("Data type %v is not allowed in this context (streaming data)", v)
		utils.Log("error", msg, "p2p")
		s.Reset()
		return
	}

	message = fmt.Sprintf("Sending ended %s", s.ID())
	utils.Log("debug", message, "p2p")
}

func sendStreamChunks(b []byte, pointer uint64, chunkSize uint64, s network.Stream) error {
	for {
		if len(b)-int(pointer) < int(chunkSize) {
			chunkSize = uint64(len(b) - int(pointer))
		}

		err := binary.Write(s, binary.BigEndian, chunkSize)
		if err != nil {
			utils.Log("error", err.Error(), "p2p")
			return err
		}
		message := fmt.Sprintf("Sending chunk size %d ended %s", chunkSize, s.ID())
		utils.Log("debug", message, "p2p")

		if chunkSize == 0 {
			break
		}

		err = binary.Write(s, binary.BigEndian, (b)[pointer:pointer+chunkSize])
		if err != nil {
			utils.Log("error", err.Error(), "p2p")
			return err
		}
		message = fmt.Sprintf("Sending chunk %v ended %s", (b)[pointer:pointer+chunkSize], s.ID())
		utils.Log("debug", message, "p2p")

		pointer += chunkSize
	}
	return nil
}

func receivedStream(s network.Stream, streamData node_types.StreamData) {
	// Prepare to read back the data
	var data []byte
	for {
		var chunkSize uint64
		err := binary.Read(s, binary.BigEndian, &chunkSize)
		if err != nil {
			utils.Log("error", err.Error(), "p2p")
			s.Reset()
		}

		message := fmt.Sprintf("Received chunk size %d from %s", chunkSize, s.ID())
		utils.Log("debug", message, "p2p")

		if chunkSize == 0 {
			break
		}

		chunk := make([]byte, chunkSize)

		err = binary.Read(s, binary.BigEndian, &chunk)
		if err != nil {
			utils.Log("error", err.Error(), "p2p")
			s.Reset()
		}

		message = fmt.Sprintf("Received %v of type %d id %d from %s", chunk, streamData.Type, streamData.Id, s.ID())
		utils.Log("debug", message, "p2p")

		// Concatenate receiving chunk
		data = append(data, chunk...)
	}

	message := fmt.Sprintf("Received data %v type from %s is %d, id %d, node %s", data, s.ID(),
		streamData.Type, streamData.Id, string(bytes.Trim(streamData.PeerId[:], "\x00")))
	utils.Log("debug", message, "p2p")

	// Determine data type
	switch streamData.Type {
	case 0:
		// Received a Service Catalogue from the remote peer
		var serviceCatalogue []node_types.ServiceOffer
		err := json.Unmarshal(data, &serviceCatalogue)
		if err != nil {
			msg := fmt.Sprintf("Could not load received binary stream into a Service Catalogue struct.\n\n%s", err.Error())
			utils.Log("error", msg, "p2p")
		}
	case 1:
		// Sent data to the remote peer
	case 2:
		// Received a binary stream from the remote peer
	case 3:
		// Received a file from the remote peer
	default:
		message := fmt.Sprintf("Unknown stream type %d is received", streamData.Type)
		utils.Log("warn", message, "p2p")
	}

	message = fmt.Sprintf("Receiving ended %s", s.ID())
	utils.Log("debug", message, "p2p")

	s.Reset()
}

func BroadcastMessage[T any](message T) error {
	cntx, _ := GetHostContext()

	var m []byte
	var err error
	var topic *pubsub.Topic

	config, err := utils.ReadConfigs(configsPath)
	if err != nil {
		message := fmt.Sprintf("Can not read configs file. (%s)", err.Error())
		utils.Log("error", message, "p2p")
		return err
	}

	switch v := any(message).(type) {
	case node_types.ServiceLookup:
		m, err = json.Marshal(message)
		if err != nil {
			utils.Log("error", err.Error(), "p2p")
			return err
		}

		topicKey := config["topic_name_prefix"] + "lookup.service"
		topic = topicsSubscribed[topicKey]
	default:
		msg := fmt.Sprintf("Message type %v is not allowed in this context (broadcasting message)", v)
		utils.Log("error", msg, "p2p")
		err := errors.New(msg)
		return err
	}

	if err := topic.Publish(cntx, m); err != nil {
		utils.Log("error", err.Error(), "p2p")
		return err
	}

	return nil
}

func receivedMessage(ctx context.Context, sub *pubsub.Subscription) {
	config, err := utils.ReadConfigs(configsPath)
	if err != nil {
		message := fmt.Sprintf("Can not read configs file. (%s)", err.Error())
		utils.Log("error", message, "p2p")
		return
	}

	for {
		m, err := sub.Next(ctx)
		if err != nil {
			utils.Log("error", err.Error(), "p2p")
		}

		err, blacklisted := blacklist_node.NodeBlacklisted(m.ReceivedFrom.String())
		if err != nil {
			msg := err.Error()
			utils.Log("warn", msg, "p2p")
			continue
		}
		if blacklisted {
			msg := fmt.Sprintf("Node %s is blacklisted", m.ReceivedFrom.String())
			utils.Log("warn", msg, "p2p")
			continue // Do not comminicate with blacklisted nodes
		}

		message := string(m.Message.Data)
		fmt.Println(m.ReceivedFrom, " Data: ", message)
		utils.Log("debug", fmt.Sprintf("Message %s received from %s", message, m.ReceivedFrom), "p2p")

		topic := string(*m.Topic)
		fmt.Println(m.ReceivedFrom, " Topic: ", topic)
		utils.Log("debug", fmt.Sprintf("Message topic is %s", topic), "p2p")

		peerId := m.ReceivedFrom
		p := peerId.String()
		fmt.Println(m.ReceivedFrom, " Peer: ", p)
		utils.Log("debug", fmt.Sprintf("From peer %s", p), "p2p")

		switch topic {
		case config["topic_name_prefix"] + "lookup.service":
			services, err := serviceLookup(m.Message.Data, true)
			if err != nil {
				utils.Log("error", err.Error(), "p2p")
				continue
			}

			// Retrieve known multiaddresses from the peerstore
			peerAddrInfo, err := GeneratePeerFromId(p)
			if err != nil {
				msg := err.Error()
				utils.Log("error", msg, "p2p")
				continue
			}

			// Stream back offered services with prices
			err = StreamData(peerAddrInfo, &services)
			if err != nil {
				msg := err.Error()
				utils.Log("error", msg, "p2p")
				continue
			}
		default:
			msg := fmt.Sprintf("Unknown topic %s", topic)
			utils.Log("error", msg, "p2p")
			continue
		}
	}
}

func serviceLookup(data []byte, active bool) ([]node_types.ServiceOffer, error) {
	config, err := utils.ReadConfigs(configsPath)
	if err != nil {
		message := fmt.Sprintf("Can not read configs file. (%s)", err.Error())
		utils.Log("error", message, "p2p")
		return nil, err
	}

	var services []node_types.ServiceOffer
	var lookup node_types.ServiceLookup

	if len(data) == 0 {
		return services, nil
	}

	err = json.Unmarshal(data, &lookup)
	if err != nil {
		utils.Log("error", err.Error(), "p2p")
		return nil, err
	}

	var searchService node_types.SearchService = node_types.SearchService{
		Name:        lookup.Name,
		Description: lookup.Description,
		NodeId:      lookup.NodeId,
		Type:        lookup.Type,
		Repo:        lookup.Repo,
		Active:      active,
	}

	// Search services
	var offset uint32 = 0
	var limit uint32 = 1
	l := config["search_services_limit"]
	l64, err := strconv.ParseUint(l, 10, 32)
	if err != nil {
		limit = 10
	} else {
		limit = uint32(l64)
	}
	serviceManager := NewServiceManager()

	for {
		servicesBatch, err := serviceManager.SearchServices(searchService, offset, limit)
		if err != nil {
			utils.Log("error", err.Error(), "p2p")
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

func GeneratePeerFromId(peerId string) (peer.AddrInfo, error) {
	var p peer.AddrInfo
	// Convert the string to a peer.ID
	pID, err := peer.Decode(peerId)
	if err != nil {
		msg := err.Error()
		utils.Log("warn", msg, "p2p")
		return p, err
	}

	// Get existing multiaddrs from DB
	node, err := tfnode.FindNode(peerId)
	if err != nil {
		utils.Log("error", err.Error(), "p2p")
		return p, err
	}

	multiaddrsList := strings.Split(node.Multiaddrs, ",")

	// Convert []string to []multiaddr.Multiaddr
	multiaddrs := []multiaddr.Multiaddr{}
	for _, addrStr := range multiaddrsList {
		addr, err := multiaddr.NewMultiaddr(addrStr)
		if err != nil {
			utils.Log("warn", err.Error(), "p2p")
			continue
		}
		multiaddrs = append(multiaddrs, addr)
	}

	// Create peer.AddrInfo from peer.ID
	p = peer.AddrInfo{
		ID: pID,
		// Add any multiaddresses if known (leave blank here if unknown)
		Addrs: multiaddrs,
	}

	return p, nil
}
