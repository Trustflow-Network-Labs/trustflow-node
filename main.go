package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"

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
	"github.com/libp2p/go-libp2p/core/routing"
	drouting "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	dutil "github.com/libp2p/go-libp2p/p2p/discovery/util"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	libp2ptls "github.com/libp2p/go-libp2p/p2p/security/tls"
)

var (
	topicNameFlag = flag.String("topicName", "trustflow.network", "name of topic to join")
	idht          *dht.IpfsDHT
)

const protocolID = "/trustflow-network/1.0.0"

func main() {
	flag.Parse()
	ctx := context.Background()

	// Create or get previously created node key
	priv, _, err := tfnode.GetNodeKey()
	if err != nil {
		utils.Log("panic", err.Error(), "main")
		panic(fmt.Sprintf("%v", err))
	}

	h, err := libp2p.New(
		// Use the keypair we generated
		libp2p.Identity(priv),
		// Multiple listen addresses
		libp2p.ListenAddrStrings(
			"/ip4/0.0.0.0/tcp/30609",      // regular tcp connections
			"/ip4/0.0.0.0/udp/30609/quic", // a UDP endpoint for the QUIC transport
			"/ip6/::1/tcp/30609",
			"/ip6/::1/udp/30609/quic",
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
		utils.Log("panic", err.Error(), "main")
		panic(fmt.Sprintf("%v", err))
	}

	var multiaddrs []string
	message := fmt.Sprintf("Node ID is %s", h.ID())
	utils.Log("info", message, "main")
	for _, ma := range h.Addrs() {
		message := fmt.Sprintf("Multiaddr is %s", ma.String())
		utils.Log("info", message, "main")
		multiaddrs = append(multiaddrs, ma.String())
	}

	// Check if node is already existing in the DB
	_, err = tfnode.FindNode(h.ID().String())
	if err != nil {
		// Add node
		err = tfnode.AddNode(h.ID().String(), strings.Join(multiaddrs, ","), true)
		if err != nil {
			utils.Log("panic", err.Error(), "main")
			panic(fmt.Sprintf("%v", err))
		}

		// Add key
		key, err := crypto.MarshalPrivateKey(priv)
		if err != nil {
			utils.Log("panic", err.Error(), "main")
			panic(fmt.Sprintf("%v", err))
		}
		err = keystore.AddKey(h.ID().String(), fmt.Sprintf("%s: secp256r1", priv.Type().String()), key)
		if err != nil {
			utils.Log("panic", err.Error(), "main")
			panic(fmt.Sprintf("%v", err))
		}
	}

	// Setup a stream handler.
	// This gets called every time a peer connects and opens a stream to this node.
	h.SetStreamHandler(protocolID, func(s network.Stream) {
		message := fmt.Sprintf("Stream [protocol: %s] %s has been openned on node %s from node %s", protocolID, s.ID(), h.ID(), s.Conn().RemotePeer().String())
		utils.Log("info", message, "main")
		go streamProposalResponse(s)
	})

	go discoverPeers(ctx, h)

	ps, err := pubsub.NewGossipSub(ctx, h)
	if err != nil {
		utils.Log("panic", err.Error(), "main")
		panic(fmt.Sprintf("%v", err))
	}
	topic, err := ps.Join(*topicNameFlag)
	if err != nil {
		utils.Log("panic", err.Error(), "main")
		panic(fmt.Sprintf("%v", err))
	}
	go broadcastMessage(ctx, topic)

	sub, err := topic.Subscribe()
	if err != nil {
		utils.Log("panic", err.Error(), "main")
		panic(fmt.Sprintf("%v", err))
	}
	receivedMessage(ctx, sub)
}

func initDHT(ctx context.Context, h host.Host) *dht.IpfsDHT {
	// Start a DHT, for use in peer discovery. We can't just make a new DHT
	// client because we want each peer to maintain its own local copy of the
	// DHT, so that the bootstrapping node of the DHT can go down without
	// inhibiting future peer discovery.
	kademliaDHT, err := dht.New(ctx, h)
	if err != nil {
		utils.Log("panic", err.Error(), "main")
		panic(fmt.Sprintf("%v", err))
	}
	if err = kademliaDHT.Bootstrap(ctx); err != nil {
		utils.Log("panic", err.Error(), "main")
		panic(fmt.Sprintf("%v", err))
	}
	var wg sync.WaitGroup
	for _, peerAddr := range dht.DefaultBootstrapPeers {
		peerinfo, _ := peer.AddrInfoFromP2pAddr(peerAddr)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := h.Connect(ctx, *peerinfo); err != nil {
				utils.Log("warn", err.Error(), "main")
			}
		}()
	}
	wg.Wait()

	return kademliaDHT
}

func discoverPeers(ctx context.Context, h host.Host) {
	kademliaDHT := initDHT(ctx, h)
	routingDiscovery := drouting.NewRoutingDiscovery(kademliaDHT)
	dutil.Advertise(ctx, routingDiscovery, *topicNameFlag)

	// Look for others who have announced and attempt to connect to them
	anyConnected := false
	for !anyConnected {
		utils.Log("debug", "Searching for peers...", "main")
		peerChan, err := routingDiscovery.FindPeers(ctx, *topicNameFlag)
		if err != nil {
			utils.Log("panic", err.Error(), "main")
			panic(fmt.Sprintf("%v", err))
		}
		for peer := range peerChan {
			if peer.ID == h.ID() {
				continue // No self connection
			}
			err := h.Connect(ctx, peer)
			if err != nil {
				utils.Log("debug", fmt.Sprintf("Failed connecting to %s, error: %s", peer.ID, err), "main")
				h.Network().ClosePeer(peer.ID)
				h.Network().Peerstore().RemovePeer(peer.ID)
				kademliaDHT.RoutingTable().RemovePeer(peer.ID)
				h.Peerstore().ClearAddrs(peer.ID)
				h.Peerstore().RemovePeer(peer.ID)
				utils.Log("debug", fmt.Sprintf("Removed peer %s from a peer store", peer.ID), "main")
			} else {
				utils.Log("debug", fmt.Sprintf("Connected to: %s", peer.ID.String()), "main")
				anyConnected = true
				// Determine multiaddrs
				var multiaddrs []string
				for _, ma := range peer.Addrs {
					utils.Log("debug", fmt.Sprintf("Connected peer's multiaddr is %s", ma.String()), "main")
					multiaddrs = append(multiaddrs, ma.String())
				}
				// Check if node is already existing in the DB
				_, err := tfnode.FindNode(peer.ID.String())
				if err != nil {
					// Add new nodes to DB
					err = tfnode.AddNode(peer.ID.String(), strings.Join(multiaddrs, ","), false)
					if err != nil {
						utils.Log("panic", err.Error(), "main")
						panic(err)
					}
				}
				s, err := h.NewStream(ctx, peer.ID, protocolID)
				if err != nil {
					utils.Log("warn", err.Error(), "main")
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
	utils.Log("debug", "Peer discovery complete", "main")
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
		utils.Log("error", err.Error(), "main")
		s.Reset()
	}
	message := fmt.Sprintf("Sending stream proposal %d (%d) has ended in stream %s", streamData.Type, streamData.Version, s.ID())
	utils.Log("debug", message, "main")
}

func streamProposalResponse(s network.Stream) {
	// Prepare to read the stream data
	var streamData node_types.StreamData
	err := binary.Read(s, binary.BigEndian, &streamData)
	if err != nil {
		utils.Log("error", err.Error(), "main")
		s.Reset()
	}

	message := fmt.Sprintf("Received stream data type %d, version %d from %s in stream %s",
		streamData.Type, streamData.Version, string(bytes.Trim(streamData.PeerId[:], "\x00")), s.ID())
	utils.Log("debug", message, "main")

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
			utils.Log("debug", message, "main")

			streamAccepted(s)
			go receivedStream(s, streamData)
		} else {
			message := fmt.Sprintf("Stream data type %d, version %d in stream %s are not accepted",
				streamData.Type, streamData.Version, s.ID())
			utils.Log("debug", message, "main")

			s.Reset()
		}
	default:
		message := fmt.Sprintf("Unknown stream type %d is poposed", streamData.Type)
		utils.Log("debug", message, "main")
		s.Reset()
	}
}

func streamAccepted(s network.Stream) {
	data := []byte{'R', 'E', 'A', 'D', 'Y'}
	err := binary.Write(s, binary.BigEndian, data)
	if err != nil {
		utils.Log("error", err.Error(), "main")
		s.Reset()
	}

	message := fmt.Sprintf("Sending READY ended %s", s.ID())
	utils.Log("debug", message, "main")
}

func sendStream(s network.Stream, data *[]byte) {
	var ready [5]byte

	err := binary.Read(s, binary.BigEndian, &ready)
	if err != nil {
		utils.Log("error", err.Error(), "main")
		s.Reset()
	}

	// TODO, chech if received data matches READY signal

	message := fmt.Sprintf("Received %s from %s", string(ready[:]), s.ID())
	utils.Log("debug", message, "main")

	// TODO, read chunk size form settings
	var chunkSize uint64 = 8
	var pointer uint64 = 0

	for {
		if len(*data)-int(pointer) < int(chunkSize) {
			chunkSize = uint64(len(*data) - int(pointer))
		}

		err = binary.Write(s, binary.BigEndian, chunkSize)
		if err != nil {
			utils.Log("error", err.Error(), "main")
			s.Reset()
		}
		message := fmt.Sprintf("Sending chunk size %d ended %s", chunkSize, s.ID())
		utils.Log("debug", message, "main")

		if chunkSize == 0 {
			break
		}

		err = binary.Write(s, binary.BigEndian, (*data)[pointer:pointer+chunkSize])
		if err != nil {
			utils.Log("error", err.Error(), "main")
			s.Reset()
		}
		message = fmt.Sprintf("Sending chunk %v ended %s", (*data)[pointer:pointer+chunkSize], s.ID())
		utils.Log("debug", message, "main")

		pointer += chunkSize
	}

	message = fmt.Sprintf("Sending ended %s", s.ID())
	utils.Log("debug", message, "main")
}

func receivedStream(s network.Stream, streamData node_types.StreamData) {
	// Prepare to read back the data
	for {
		var chunkSize uint64
		err := binary.Read(s, binary.BigEndian, &chunkSize)
		if err != nil {
			utils.Log("error", err.Error(), "main")
			s.Reset()
		}

		message := fmt.Sprintf("Received chunk size %d from %s", chunkSize, s.ID())
		utils.Log("debug", message, "main")

		if chunkSize == 0 {
			break
		}

		data := make([]byte, chunkSize)

		err = binary.Read(s, binary.BigEndian, &data)
		if err != nil {
			utils.Log("error", err.Error(), "main")
			s.Reset()
		}

		message = fmt.Sprintf("Received %v of type %d version %d from %s", data, streamData.Type, streamData.Version, s.ID())
		utils.Log("debug", message, "main")
	}

	// TODO, concat the data and understand what was received
	message := fmt.Sprintf("Received data type from %s is %d, version %d, node %s", s.ID(),
		streamData.Type, streamData.Version, string(bytes.Trim(streamData.PeerId[:], "\x00")))
	utils.Log("debug", message, "main")

	message = fmt.Sprintf("Receiving ended %s", s.ID())
	utils.Log("debug", message, "main")

	s.Reset()
}

func broadcastMessage(ctx context.Context, topic *pubsub.Topic) {
	reader := bufio.NewReader(os.Stdin)
	for {
		s, err := reader.ReadString('\n')
		if err != nil {
			utils.Log("error", err.Error(), "main")
		}
		if err := topic.Publish(ctx, []byte(s)); err != nil {
			utils.Log("error", err.Error(), "main")
		}
	}
}

func receivedMessage(ctx context.Context, sub *pubsub.Subscription) {
	for {
		m, err := sub.Next(ctx)
		if err != nil {
			utils.Log("error", err.Error(), "main")
		}
		message := string(m.Message.Data)
		fmt.Println(m.ReceivedFrom, ": ", string(message))

		switch message {
		case "":
			// TODO
		}
	}
}
