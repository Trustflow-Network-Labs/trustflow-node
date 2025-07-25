package node

import (
	"fmt"
	"slices"

	"github.com/adgsm/trustflow-node/internal/utils"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

type TopicAwareNotifiee struct {
	ps                *pubsub.PubSub
	topic             *pubsub.Topic
	completeTopicName string
	bootstrapPeers    []peer.AddrInfo
	topicAwareConnMgr *TopicAwareConnectionManager
	lm                *utils.LogsManager
}

func NewTopicAwareNotifiee(
	ps *pubsub.PubSub,
	topic *pubsub.Topic,
	completeTopicName string,
	bootstrapPeers []peer.AddrInfo,
	tcm *TopicAwareConnectionManager,
	lm *utils.LogsManager,
) *TopicAwareNotifiee {
	return &TopicAwareNotifiee{
		lm:                lm,
		ps:                ps,
		topic:             topic,
		completeTopicName: completeTopicName,
		bootstrapPeers:    bootstrapPeers,
		topicAwareConnMgr: tcm,
	}
}

func (n *TopicAwareNotifiee) isPeerInBootstrap(peerID peer.ID, bootstrapPeers []peer.AddrInfo) bool {
	return slices.ContainsFunc(bootstrapPeers, func(addrInfo peer.AddrInfo) bool {
		return addrInfo.ID == peerID
	})
}

func (n *TopicAwareNotifiee) Connected(net network.Network, conn network.Conn) {
	remotePeer := conn.RemotePeer()

	// Notify connection manager about new peer
	if n.topicAwareConnMgr != nil {
		n.topicAwareConnMgr.OnPeerConnected(remotePeer)
	}

	// Check if the remote peer is subscribed to the same topic
	if p, b := n.isPeerInTopic(remotePeer); b {
		msg := fmt.Sprintf("✅ Node %s subscribed to topic '%s' is connected to our node. Check was done for %s\n", p.String(), n.completeTopicName, remotePeer.String())
		n.lm.Log("debug", msg, "libp2p-events")

		// Update connection quality for topic peers (they get high priority)
		if n.topicAwareConnMgr != nil {
			n.topicAwareConnMgr.UpdatePeerStats(remotePeer, true, 0.9)
			// use previously completed peers evaluation
			//	connStats := n.topicAwareConnMgr.GetConnectionStats()
			//	msg := fmt.Sprintf("Connection stats:\nTotal connections: %d\nTopic peers connected: %d\nRouting peers connected: %d\nOther peers connected: %d\n",
			//		connStats["total"], connStats["topic_peers"], connStats["routing_peers"], connStats["other_peers"])
			//	n.lm.Log("debug", msg, "libp2p-events")
		}

	} else {
		//		msg := fmt.Sprintf("🔗 Connected to potential routing peer: %s", remotePeer.String())
		//		logsManager.Log("debug", msg, "libp2p-events")
	}
}

func (n *TopicAwareNotifiee) Disconnected(net network.Network, conn network.Conn) {
	remotePeer := conn.RemotePeer()

	// Notify connection manager about disconnection
	if n.topicAwareConnMgr != nil {
		n.topicAwareConnMgr.OnPeerDisconnected(remotePeer)
		// use previously completed peers evaluation
		//		connStats := n.topicAwareConnMgr.GetConnectionStats()
		//		msg := fmt.Sprintf("Connection stats:\nTotal connections: %d\nTopic peers connected: %d\nRouting peers connected: %d\nOther peers connected: %d\n",
		//			connStats["total"], connStats["topic_peers"], connStats["routing_peers"], connStats["other_peers"])
		//		n.lm.Log("debug", msg, "libp2p-events")
	}

	if p, b := n.isPeerInTopic(remotePeer); b {
		msg := fmt.Sprintf("🔴 Disconnected from node subscribed to topic '%s': %s (check was done for: %s)\n", n.completeTopicName, p.String(), remotePeer.String())
		n.lm.Log("debug", msg, "libp2p-events")
	}
}

// Lightweight subscription check
func (n *TopicAwareNotifiee) isPeerInTopic(p peer.ID) (peer.ID, bool) {
	buffer := utils.NewFIFOBuffer(600)
	buffer.Add(p)
	peers := n.ps.ListPeers(n.completeTopicName)
	for _, b := range buffer.Entries() {
		if slices.Contains(peers, b.(peer.ID)) {
			buffer.Remove(b)
			return b.(peer.ID), true
		}
	}
	return p, false
}

func (n *TopicAwareNotifiee) Listen(net network.Network, addr multiaddr.Multiaddr)      {}
func (n *TopicAwareNotifiee) ListenClose(net network.Network, addr multiaddr.Multiaddr) {}
func (n *TopicAwareNotifiee) OpenedStream(net network.Network, stream network.Stream)   {}
func (n *TopicAwareNotifiee) ClosedStream(net network.Network, stream network.Stream)   {}
