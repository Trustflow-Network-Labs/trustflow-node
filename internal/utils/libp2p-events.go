package utils

import (
	"fmt"
	"slices"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

type TopicAwareNotifiee struct {
	ps                *pubsub.PubSub
	topic             *pubsub.Topic
	completeTopicName string
	peerChannel       chan []peer.AddrInfo
}

func NewTopicAwareNotifiee(ps *pubsub.PubSub, topic *pubsub.Topic, completeTopicName string, peerChannel chan []peer.AddrInfo) *TopicAwareNotifiee {
	return &TopicAwareNotifiee{
		ps:                ps,
		topic:             topic,
		completeTopicName: completeTopicName,
		peerChannel:       peerChannel,
	}
}

func (n *TopicAwareNotifiee) Connected(net network.Network, conn network.Conn) {
	logsManager := NewLogsManager()
	remotePeer := conn.RemotePeer()

	// Check if the remote peer is subscribed to the same topic
	if p, b := n.isPeerInTopic(remotePeer); b {
		msg := fmt.Sprintf("✅ Connected to node subscribed to topic '%s': %s (%s)\n", n.completeTopicName, p.String(), remotePeer.String())
		logsManager.Log("debug", msg, "libp2p-events")
	} else {
		//		msg := fmt.Sprintf("❌ Connected to node NOT subscribed to topic '%s': %s\n", n.completeTopicName, remotePeer.String())
		//		logsManager.Log("debug", msg, "libp2p-events")
	}
}

func (n *TopicAwareNotifiee) Disconnected(net network.Network, conn network.Conn) {
	logsManager := NewLogsManager()
	remotePeer := conn.RemotePeer()

	if p, b := n.isPeerInTopic(remotePeer); b {
		msg := fmt.Sprintf("🔴 Disconnected from node subscribed to topic '%s': %s (%s)\n", n.completeTopicName, p.String(), remotePeer.String())
		logsManager.Log("debug", msg, "libp2p-events")
	}
}

// Lightweight subscription check
func (n *TopicAwareNotifiee) isPeerInTopic(p peer.ID) (peer.ID, bool) {
	/*
		select {
		case <-n.peerChannel:
			for _, peer := range <-n.peerChannel {
				fmt.Printf("Found topic peer: %s (%s)\n", peer.ID, p)
				if peer.ID == p {
					return p, true
				}
			}
		default:

		}
		return p, false
	*/
	buffer := NewFIFOBuffer(10)
	buffer.Add(p)
	peers := n.ps.ListPeers(n.completeTopicName)
	for _, b := range buffer.Entries() {
		if slices.Contains(peers, b.(peer.ID)) {
			buffer.Remove(b)
			return b.(peer.ID), true
		}
	}
	return p, false

	//fmt.Printf("%v <-> %v\n", p, peers)
	//return p, slices.Contains(peers, p)
}

func (n *TopicAwareNotifiee) Listen(net network.Network, addr multiaddr.Multiaddr)      {}
func (n *TopicAwareNotifiee) ListenClose(net network.Network, addr multiaddr.Multiaddr) {}
func (n *TopicAwareNotifiee) OpenedStream(net network.Network, stream network.Stream)   {}
func (n *TopicAwareNotifiee) ClosedStream(net network.Network, stream network.Stream)   {}
