package main

import (
	"bufio"
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/adgsm/trustflow-node/keystore"
	"github.com/adgsm/trustflow-node/node_types"
	"github.com/adgsm/trustflow-node/tfnode"
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
		panic(fmt.Sprintf("%v", err))
	}

	var multiaddrs []string
	fmt.Printf("Hello World, my hosts ID is %s\n", h.ID())
	for _, ma := range h.Addrs() {
		fmt.Printf("Hello World, my multiaddr is %s\n", ma.String())
		multiaddrs = append(multiaddrs, ma.String())
	}

	// Check if node is already existing in the DB
	_, err = tfnode.FindNode(h.ID().String())
	if err != nil {
		// Add node
		err = tfnode.AddNode(h.ID().String(), strings.Join(multiaddrs, ","), true)
		if err != nil {
			panic(fmt.Sprintf("%v", err))
		}

		// Add key
		key, err := crypto.MarshalPrivateKey(priv)
		if err != nil {
			panic(fmt.Sprintf("%v", err))
		}
		err = keystore.AddKey(h.ID().String(), fmt.Sprintf("%s: secp256r1", priv.Type().String()), key)
		if err != nil {
			panic(fmt.Sprintf("%v", err))
		}
	}

	// Setup a stream handler.
	//
	// This gets called every time a peer connects and opens a stream to this node.
	h.SetStreamHandler(protocolID, func(s network.Stream) {
		fmt.Println("Stream to this node has been just openned...")
		go streamProposalResponse(s)
	})

	go discoverPeers(ctx, h)

	ps, err := pubsub.NewGossipSub(ctx, h)
	if err != nil {
		panic(fmt.Sprintf("%v", err))
	}
	topic, err := ps.Join(*topicNameFlag)
	if err != nil {
		panic(fmt.Sprintf("%v", err))
	}
	go broadcastMessage(ctx, topic)

	sub, err := topic.Subscribe()
	if err != nil {
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
		panic(fmt.Sprintf("%v", err))
	}
	if err = kademliaDHT.Bootstrap(ctx); err != nil {
		panic(fmt.Sprintf("%v", err))
	}
	var wg sync.WaitGroup
	for _, peerAddr := range dht.DefaultBootstrapPeers {
		peerinfo, _ := peer.AddrInfoFromP2pAddr(peerAddr)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := h.Connect(ctx, *peerinfo); err != nil {
				fmt.Println("Bootstrap warning:", err)
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
		fmt.Println("Searching for peers...")
		peerChan, err := routingDiscovery.FindPeers(ctx, *topicNameFlag)
		if err != nil {
			panic(fmt.Sprintf("%v", err))
		}
		for peer := range peerChan {
			if peer.ID == h.ID() {
				continue // No self connection
			}
			err := h.Connect(ctx, peer)
			if err != nil {
				fmt.Printf("Failed connecting to %s, error: %s\n", peer.ID, err)
				h.Network().ClosePeer(peer.ID)
				h.Network().Peerstore().RemovePeer(peer.ID)
				kademliaDHT.RoutingTable().RemovePeer(peer.ID)
				h.Peerstore().ClearAddrs(peer.ID)
				h.Peerstore().RemovePeer(peer.ID)
				fmt.Printf("Removed peer %s from a peer store\n", peer.ID)
			} else {
				fmt.Println("Connected to:", peer.ID)
				anyConnected = true
				// Determine multiaddrs
				var multiaddrs []string
				for _, ma := range peer.Addrs {
					fmt.Printf("Connected peer's multiaddr is %s\n", ma.String())
					multiaddrs = append(multiaddrs, ma.String())
				}
				// Check if node is already existing in the DB
				_, err := tfnode.FindNode(peer.ID.String())
				if err != nil {
					// Add new nodes to DB
					err = tfnode.AddNode(peer.ID.String(), strings.Join(multiaddrs, ","), false)
					if err != nil {
						panic(err)
					}
				}
				s, err := h.NewStream(ctx, peer.ID, protocolID)
				if err != nil {
					fmt.Printf("%v\n", err)
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
	fmt.Println("Peer discovery complete")
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
		log.Fatalf("Failed to write stream data: %v", err)
	}
	fmt.Printf("Sending stream proposal %v has ended in stream %s\n", streamData, s.ID())
}

func streamProposalResponse(s network.Stream) {
	// Prepare to read the stream data
	var streamData node_types.StreamData
	err := binary.Read(s, binary.BigEndian, &streamData)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Received %v from %s\n", streamData, s.ID())
	fmt.Printf("%s\n", string(streamData.PeerId[:]))

	// Check what stream is being proposed
	switch streamData.Type {
	case 0:
		// Service Catalogue
		// TODO, Check settings, do we have set to accept
		// service catalogues and updates
		accepted := true
		if accepted {
			streamAccepted(s)
			go receivedStream(s, streamData)
		} else {
			s.Reset()
		}
	default:
		fmt.Printf("Unknown stream type %d is poposed\n", streamData.Type)
		s.Reset()
	}
}

func streamAccepted(s network.Stream) {
	data := []byte{'R', 'E', 'A', 'D', 'Y'}
	err := binary.Write(s, binary.BigEndian, data)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Sending READY ended %s\n", s.ID())
}

func sendStream(s network.Stream, data *[]byte) {
	var ready [5]byte

	err := binary.Read(s, binary.BigEndian, &ready)
	if err != nil {
		panic(err)
	}

	// TODO, chech if received data matches READY signal

	fmt.Printf("Received %v from %s\n", ready, s.ID())

	// TODO, read chunk size form settings
	var chunkSize uint64 = 8
	var pointer uint64 = 0

	for {
		if len(*data)-int(pointer) < int(chunkSize) {
			chunkSize = uint64(len(*data) - int(pointer))
		}

		err = binary.Write(s, binary.BigEndian, chunkSize)
		if err != nil {
			panic(err)
		}
		fmt.Printf("Sending chunk size %d ended %s\n", chunkSize, s.ID())

		if chunkSize == 0 {
			break
		}

		err = binary.Write(s, binary.BigEndian, (*data)[pointer:pointer+chunkSize])
		if err != nil {
			panic(err)
		}
		fmt.Printf("Sending chunk %v ended %s\n", (*data)[pointer:pointer+chunkSize], s.ID())

		pointer += chunkSize
	}

	fmt.Printf("Sending ended %s\n", s.ID())
}

func receivedStream(s network.Stream, streamData node_types.StreamData) {
	// Prepare to read back the data
	for {
		var chunkSize uint64
		err := binary.Read(s, binary.BigEndian, &chunkSize)
		if err != nil {
			panic(err)
		}

		fmt.Printf("Received chunk size %d from %s\n", chunkSize, s.ID())

		if chunkSize == 0 {
			break
		}

		data := make([]byte, chunkSize)

		err = binary.Read(s, binary.BigEndian, &data)
		if err != nil {
			panic(err)
		}

		fmt.Printf("Received %v of type %v from %s\n", data, streamData, s.ID())
	}

	// TODO, concat the data and understand what was received
	fmt.Printf("Received data type from %s is %d, version %d, node %s\n", s.ID(), streamData.Type, streamData.Version, streamData.PeerId)

	fmt.Printf("Receiving ended %s\n", s.ID())

	s.Reset()
}

func broadcastMessage(ctx context.Context, topic *pubsub.Topic) {
	reader := bufio.NewReader(os.Stdin)
	for {
		s, err := reader.ReadString('\n')
		if err != nil {
			panic(fmt.Sprintf("%v", err))
		}
		if err := topic.Publish(ctx, []byte(s)); err != nil {
			fmt.Println("### Publish error:", err)
		}
	}
}

func receivedMessage(ctx context.Context, sub *pubsub.Subscription) {
	for {
		m, err := sub.Next(ctx)
		if err != nil {
			panic(fmt.Sprintf("%v", err))
		}
		message := string(m.Message.Data)
		fmt.Println(m.ReceivedFrom, ": ", string(message))

		switch message {
		case "":
			// TODO
		}
	}
}
