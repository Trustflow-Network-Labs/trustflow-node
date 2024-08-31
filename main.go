package main

import (
	"bufio"
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"sync"

	"github.com/adgsm/trustflow-node/keystore"
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

	// Add node
	err = tfnode.AddNode(h.ID().String(), multiaddrs, true)
	if err != nil {
		panic(fmt.Sprintf("%v", err))
	}

	// Add key
	key, err := crypto.MarshalPrivateKey(priv)
	err = keystore.AddKey(h.ID().String(), fmt.Sprintf("%s: secp256r1", priv.Type().String()), key)
	if err != nil {
		panic(fmt.Sprintf("%v", err))
	}

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
			} else {
				fmt.Println("Connected to:", peer.ID)
				anyConnected = true
			}
		}
	}
	fmt.Println("Peer discovery complete")
}

func sendStream(s network.Stream) {
	var buff string = "TODO"
	err := binary.Write(s, binary.BigEndian, buff)
	if err != nil {
		panic(err)
	}
}

func receivedStream(s network.Stream) {
	var buff string

	err := binary.Read(s, binary.BigEndian, &buff)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Received %s from %s\n", buff, s.ID())
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
