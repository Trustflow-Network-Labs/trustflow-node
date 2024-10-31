package p2p

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	blacklist_node "github.com/adgsm/trustflow-node/cmd/blacklist-node"
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
)

var (
	topicNameFlag *string
	protocolID    protocol.ID
	idht          *dht.IpfsDHT
)

// provide configs file path
var configsPath string = "p2p/configs"

var h host.Host

// IsHostRunning checks if the provided host is actively running
func IsHostRunning() (bool, host.Host) {
	// Check if the host is listening on any network addresses
	return len(h.Network().ListenAddresses()) > 0, h
}

func Start(port uint16) {
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

	// Read topic name
	topicNameFlag = flag.String("topicName", config["topic_name"], "name of topic to join")

	// Read streaming protocol
	protocolID = protocol.ID(config["protocol_id"])

	flag.Parse()
	ctx := context.Background()

	// Create or get previously created node key
	priv, _, err := tfnode.GetNodeKey()
	if err != nil {
		utils.Log("panic", err.Error(), "p2p")
		panic(fmt.Sprintf("%v", err))
	}

	h, err = libp2p.New(
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
		libp2p.Routing(func(h host.Host) (routing.PeerRouting, error) {
			idht, err = dht.New(ctx, h)
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

	var multiaddrs []string
	message := fmt.Sprintf("Node ID is %s", h.ID())
	utils.Log("info", message, "p2p")
	for _, ma := range h.Addrs() {
		message := fmt.Sprintf("Multiaddr is %s", ma.String())
		utils.Log("info", message, "p2p")
		multiaddrs = append(multiaddrs, ma.String())
	}

	// Check if node is already existing in the DB
	_, err = tfnode.FindNode(h.ID().String())
	if err != nil {
		// Add node
		err = tfnode.AddNode(h.ID().String(), strings.Join(multiaddrs, ","), true)
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
		err = keystore.AddKey(h.ID().String(), fmt.Sprintf("%s: secp256r1", priv.Type().String()), key)
		if err != nil {
			utils.Log("panic", err.Error(), "p2p")
			panic(fmt.Sprintf("%v", err))
		}
	} else {
		// Update node
		err = tfnode.UpdateNode(h.ID().String(), strings.Join(multiaddrs, ","), true)
		if err != nil {
			utils.Log("panic", err.Error(), "p2p")
			panic(err)
		}
	}

	// Setup a stream handler.
	// This gets called every time a peer connects and opens a stream to this node.
	h.SetStreamHandler(protocolID, func(s network.Stream) {
		message := fmt.Sprintf("Stream [protocol: %s] %s has been openned on node %s from node %s", protocolID, s.ID(), h.ID(), s.Conn().RemotePeer().String())
		utils.Log("info", message, "p2p")
		go streamProposalResponse(s)
	})

	go discoverPeers(ctx, h)

	ps, err := pubsub.NewGossipSub(ctx, h)
	if err != nil {
		utils.Log("panic", err.Error(), "p2p")
		panic(fmt.Sprintf("%v", err))
	}
	topic, err := ps.Join(*topicNameFlag)
	if err != nil {
		utils.Log("panic", err.Error(), "p2p")
		panic(fmt.Sprintf("%v", err))
	}
	go broadcastMessage(ctx, topic)

	sub, err := topic.Subscribe()
	if err != nil {
		utils.Log("panic", err.Error(), "p2p")
		panic(fmt.Sprintf("%v", err))
	}
	receivedMessage(ctx, sub)
}

func Stop() error {
	running, h := IsHostRunning()
	if !running {
		message := "host is not running"
		err := errors.New(message)
		utils.Log("warn", message, "p2p")
		return err
	}

	err := h.Close()
	if err != nil {
		utils.Log("warn", err.Error(), "p2p")
		return err
	}

	return nil
}

func initDHT(ctx context.Context, hst host.Host) *dht.IpfsDHT {
	// Start a DHT, for use in peer discovery. We can't just make a new DHT
	// client because we want each peer to maintain its own local copy of the
	// DHT, so that the bootstrapping node of the DHT can go down without
	// inhibiting future peer discovery.
	kademliaDHT, err := dht.New(ctx, hst)
	if err != nil {
		utils.Log("panic", err.Error(), "p2p")
		panic(fmt.Sprintf("%v", err))
	}
	if err = kademliaDHT.Bootstrap(ctx); err != nil {
		utils.Log("panic", err.Error(), "p2p")
		panic(fmt.Sprintf("%v", err))
	}
	var wg sync.WaitGroup
	for _, peerAddr := range dht.DefaultBootstrapPeers {
		peerinfo, _ := peer.AddrInfoFromP2pAddr(peerAddr)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := hst.Connect(ctx, *peerinfo); err != nil {
				utils.Log("warn", err.Error(), "p2p")
			}
		}()
	}
	wg.Wait()

	return kademliaDHT
}

func discoverPeers(ctx context.Context, hst host.Host) {
	kademliaDHT := initDHT(ctx, h)
	routingDiscovery := drouting.NewRoutingDiscovery(kademliaDHT)
	dutil.Advertise(ctx, routingDiscovery, *topicNameFlag)

	// Look for others who have announced and attempt to connect to them
	anyConnected := false
	for !anyConnected {
		utils.Log("debug", "Searching for peers...", "p2p")
		peerChan, err := routingDiscovery.FindPeers(ctx, *topicNameFlag)
		if err != nil {
			utils.Log("panic", err.Error(), "p2p")
			panic(fmt.Sprintf("%v", err))
		}
		for peer := range peerChan {
			if peer.ID == hst.ID() {
				continue // No self connection
			}
			err, blacklisted := blacklist_node.NodeBlacklisted(peer.ID.String())
			if err != nil {
				msg := err.Error()
				utils.Log("error", msg, "p2p")
				panic(err)
			}
			if blacklisted {
				msg := fmt.Sprintf("Node %s is blacklisted", peer.ID.String())
				utils.Log("warn", msg, "p2p")
				continue // Do not connect blacklisted nodes
			}

			err = hst.Connect(ctx, peer)
			if err != nil {
				utils.Log("debug", fmt.Sprintf("Failed connecting to %s, error: %s", peer.ID, err), "p2p")
				utils.Log("debug", fmt.Sprintf("Delete node %s from the DB, error: %s", peer.ID, err), "p2p")
				err = tfnode.DeleteNode(peer.ID.String())
				if err != nil {
					utils.Log("warn", fmt.Sprintf("Failed deleting node %s, error: %s", peer.ID, err), "p2p")
				}
				hst.Network().ClosePeer(peer.ID)
				hst.Network().Peerstore().RemovePeer(peer.ID)
				kademliaDHT.RoutingTable().RemovePeer(peer.ID)
				hst.Peerstore().ClearAddrs(peer.ID)
				hst.Peerstore().RemovePeer(peer.ID)
				utils.Log("debug", fmt.Sprintf("Removed peer %s from a peer store", peer.ID), "p2p")
			} else {
				utils.Log("debug", fmt.Sprintf("Connected to: %s", peer.ID.String()), "p2p")
				anyConnected = true
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
						utils.Log("panic", err.Error(), "p2p")
						panic(err)
					}
				} else {
					// Update node
					utils.Log("debug", fmt.Sprintf("update node %s with multiaddrs %s", peer.ID.String(), strings.Join(multiaddrs, ",")), "p2p")
					err = tfnode.UpdateNode(peer.ID.String(), strings.Join(multiaddrs, ","), false)
					if err != nil {
						utils.Log("panic", err.Error(), "p2p")
						panic(err)
					}
				}
				s, err := hst.NewStream(ctx, peer.ID, protocolID)
				if err != nil {
					utils.Log("warn", err.Error(), "p2p")
					s.Reset()
				} else {
					var str []byte = []byte(peer.ID.String())
					var str255 [255]byte
					copy(str255[:], str)
					go streamProposal(s, str255, 0, 0)
					// TODO, read data from appropriate source
					data := []byte{'H', 'E', 'L', 'L', 'O', ' ', 'W', 'O', 'R', 'L', 'D'}
					go sendStream(s, &data)
				}
			}
		}
	}
	utils.Log("debug", "Peer discovery complete", "p2p")
}

func streamProposal(s network.Stream, p [255]byte, t uint16, v uint16) {
	// Create an instance of StreamData to write
	streamData := node_types.StreamData{
		Type:    t,
		Version: v,
		PeerId:  p,
	}

	// Send stream data
	if err := binary.Write(s, binary.BigEndian, streamData); err != nil {
		utils.Log("error", err.Error(), "p2p")
		s.Reset()
	}
	message := fmt.Sprintf("Sending stream proposal %d (%d) has ended in stream %s", streamData.Type, streamData.Version, s.ID())
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

	message := fmt.Sprintf("Received stream data type %d, version %d from %s in stream %s",
		streamData.Type, streamData.Version, string(bytes.Trim(streamData.PeerId[:], "\x00")), s.ID())
	utils.Log("debug", message, "p2p")

	// Check what stream is being proposed
	switch streamData.Type {
	case 0:
		// Service Catalogue
		// TODO, Check settings, do we have set to accept
		// service catalogues and updates
		accepted := true
		if accepted {
			message := fmt.Sprintf("Stream data type %d, version %d in stream %s are accepted",
				streamData.Type, streamData.Version, s.ID())
			utils.Log("debug", message, "p2p")

			streamAccepted(s)
			go receivedStream(s, streamData)
		} else {
			message := fmt.Sprintf("Stream data type %d, version %d in stream %s are not accepted",
				streamData.Type, streamData.Version, s.ID())
			utils.Log("debug", message, "p2p")

			s.Reset()
		}
	default:
		message := fmt.Sprintf("Unknown stream type %d is poposed", streamData.Type)
		utils.Log("debug", message, "p2p")
		s.Reset()
	}
}

func streamAccepted(s network.Stream) {
	data := []byte{'R', 'E', 'A', 'D', 'Y'}
	err := binary.Write(s, binary.BigEndian, data)
	if err != nil {
		utils.Log("error", err.Error(), "p2p")
		s.Reset()
	}

	message := fmt.Sprintf("Sending READY ended %s", s.ID())
	utils.Log("debug", message, "p2p")
}

func sendStream(s network.Stream, data *[]byte) {
	var ready [5]byte

	err := binary.Read(s, binary.BigEndian, &ready)
	if err != nil {
		utils.Log("error", err.Error(), "p2p")
		s.Reset()
	}

	// TODO, chech if received data matches READY signal

	message := fmt.Sprintf("Received %s from %s", string(ready[:]), s.ID())
	utils.Log("debug", message, "p2p")

	// TODO, read chunk size form settings
	var chunkSize uint64 = 8
	var pointer uint64 = 0

	for {
		if len(*data)-int(pointer) < int(chunkSize) {
			chunkSize = uint64(len(*data) - int(pointer))
		}

		err = binary.Write(s, binary.BigEndian, chunkSize)
		if err != nil {
			utils.Log("error", err.Error(), "p2p")
			s.Reset()
		}
		message := fmt.Sprintf("Sending chunk size %d ended %s", chunkSize, s.ID())
		utils.Log("debug", message, "p2p")

		if chunkSize == 0 {
			break
		}

		err = binary.Write(s, binary.BigEndian, (*data)[pointer:pointer+chunkSize])
		if err != nil {
			utils.Log("error", err.Error(), "p2p")
			s.Reset()
		}
		message = fmt.Sprintf("Sending chunk %v ended %s", (*data)[pointer:pointer+chunkSize], s.ID())
		utils.Log("debug", message, "p2p")

		pointer += chunkSize
	}

	message = fmt.Sprintf("Sending ended %s", s.ID())
	utils.Log("debug", message, "p2p")
}

func receivedStream(s network.Stream, streamData node_types.StreamData) {
	// Prepare to read back the data
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

		data := make([]byte, chunkSize)

		err = binary.Read(s, binary.BigEndian, &data)
		if err != nil {
			utils.Log("error", err.Error(), "p2p")
			s.Reset()
		}

		message = fmt.Sprintf("Received %v of type %d version %d from %s", data, streamData.Type, streamData.Version, s.ID())
		utils.Log("debug", message, "p2p")
	}

	// TODO, concat the data and understand what was received
	message := fmt.Sprintf("Received data type from %s is %d, version %d, node %s", s.ID(),
		streamData.Type, streamData.Version, string(bytes.Trim(streamData.PeerId[:], "\x00")))
	utils.Log("debug", message, "p2p")

	message = fmt.Sprintf("Receiving ended %s", s.ID())
	utils.Log("debug", message, "p2p")

	s.Reset()
}

func broadcastMessage(ctx context.Context, topic *pubsub.Topic) {
	reader := bufio.NewReader(os.Stdin)
	for {
		s, err := reader.ReadString('\n')
		if err != nil {
			utils.Log("error", err.Error(), "p2p")
		}
		if err := topic.Publish(ctx, []byte(s)); err != nil {
			utils.Log("error", err.Error(), "p2p")
		}
	}
}

func receivedMessage(ctx context.Context, sub *pubsub.Subscription) {
	for {
		m, err := sub.Next(ctx)
		if err != nil {
			utils.Log("error", err.Error(), "p2p")
		}

		err, blacklisted := blacklist_node.NodeBlacklisted(m.ReceivedFrom.String())
		if err != nil {
			msg := err.Error()
			utils.Log("error", msg, "p2p")
			return
		}
		if blacklisted {
			msg := fmt.Sprintf("Node %s is blacklisted", m.ReceivedFrom.String())
			utils.Log("warn", msg, "p2p")
			return // Do not comminicate with blacklisted nodes
		}

		message := string(m.Message.Data)
		fmt.Println(m.ReceivedFrom, ": ", string(message))

		switch message {
		case "":
			// TODO
		}
	}
}
