package core

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type memoryNetwork struct {
	mu        sync.RWMutex
	endpoints map[PeerID]*memoryNodeTransport
}

func newMemoryNetwork() *memoryNetwork {
	return &memoryNetwork{endpoints: make(map[PeerID]*memoryNodeTransport)}
}

type memoryNodeTransport struct {
	network  *memoryNetwork
	id       PeerID
	handler  Handler
	observer PeerObserver

	mu      sync.RWMutex
	started bool
	closed  bool
}

func (n *memoryNetwork) factory(identity byte, capture **memoryNodeTransport) TransportFactory {
	return func(handler Handler) (NodeTransport, error) {
		peer, err := NewPeerID([]byte{identity})
		if err != nil {
			return nil, err
		}
		transport := &memoryNodeTransport{network: n, id: peer, handler: handler}
		n.mu.Lock()
		n.endpoints[transport.id] = transport
		n.mu.Unlock()
		*capture = transport
		return transport, nil
	}
}

func (t *memoryNodeTransport) Start(context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return ErrNodeClosed
	}
	t.started = true
	return nil
}

func (t *memoryNodeTransport) Close() error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil
	}
	t.closed = true
	t.mu.Unlock()
	t.network.mu.Lock()
	delete(t.network.endpoints, t.id)
	t.network.mu.Unlock()
	return nil
}

func (t *memoryNodeTransport) Identity() PeerID { return t.id }

func (t *memoryNodeTransport) SetPeerObserver(observer PeerObserver) error {
	t.observer = observer
	return nil
}

func (t *memoryNodeTransport) SendMessage(ctx context.Context, peer PeerID, payload []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	t.mu.RLock()
	ready := t.started && !t.closed
	t.mu.RUnlock()
	if !ready {
		return errors.New("memory transport is not available")
	}
	t.network.mu.RLock()
	target := t.network.endpoints[peer]
	t.network.mu.RUnlock()
	if target == nil {
		return errors.New("memory peer is unavailable")
	}
	target.mu.RLock()
	targetReady := target.started && !target.closed
	handler := target.handler
	target.mu.RUnlock()
	if !targetReady {
		return errors.New("memory target is not available")
	}
	if err := t.observer.Authenticated(target.id); err != nil {
		return err
	}
	if err := target.observer.Authenticated(t.id); err != nil {
		return err
	}
	message, err := NewReceivedMessage(t.id.Bytes(), bytes.Clone(payload))
	if err != nil {
		return err
	}
	return handler(ctx, message)
}

func (t *memoryNodeTransport) discover(peer *memoryNodeTransport, metadata map[string]string) error {
	return t.observer.Discovered(Peer{ID: peer.id, Metadata: metadata})
}

func TestNodeComposesDiscoveryDirectMessagesAndPubSub(t *testing.T) {
	network := newMemoryNetwork()
	var transportA, transportB *memoryNodeTransport
	directB := make(chan ReceivedMessage, 1)
	nodeA, err := NewNode(NodeConfig{
		Transport: network.factory(0xa1, &transportA),
		Bus:       BusConfig{FanoutConcurrency: 1},
		Directory: DirectoryConfig{MaxPeers: 1, MaxMetadataBytes: 8},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer nodeA.Close()
	nodeB, err := NewNode(NodeConfig{
		Transport: network.factory(0xb2, &transportB),
		DirectHandler: func(_ context.Context, message ReceivedMessage) error {
			directB <- message
			return nil
		},
		Bus:       BusConfig{FanoutConcurrency: 1},
		Directory: DirectoryConfig{MaxPeers: 1, MaxMetadataBytes: 8},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer nodeB.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := nodeA.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := nodeB.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := transportA.discover(transportB, map[string]string{"os": "go"}); err != nil {
		t.Fatal(err)
	}
	if err := transportB.discover(transportA, nil); err != nil {
		t.Fatal(err)
	}
	if len(nodeA.DiscoveredPeers()) != 1 || len(nodeB.DiscoveredPeers()) != 1 || len(nodeA.Peers()) != 0 || len(nodeB.Peers()) != 0 {
		t.Fatalf("before auth discovered A=%+v B=%+v authenticated A=%+v B=%+v", nodeA.DiscoveredPeers(), nodeB.DiscoveredPeers(), nodeA.Peers(), nodeB.Peers())
	}

	if err := nodeA.Send(ctx, transportB.id, []byte("direct")); err != nil {
		t.Fatal(err)
	}
	select {
	case message := <-directB:
		if message.Sender() != transportA.id || string(message.Payload()) != "direct" {
			t.Fatalf("direct sender=%s payload=%q", message.Sender().String(), message.Payload())
		}
	case <-time.After(time.Second):
		t.Fatal("direct message was not delivered")
	}
	if len(nodeA.Peers()) != 1 || nodeA.Peers()[0].ID != transportB.id || len(nodeB.Peers()) != 1 {
		t.Fatalf("authenticated peer snapshots A=%+v B=%+v", nodeA.Peers(), nodeB.Peers())
	}

	events := make(chan ReceivedEvent, 1)
	localEvents := make(chan ReceivedEvent, 1)
	if _, err := nodeA.Subscribe("node.event", func(_ context.Context, event ReceivedEvent) error {
		localEvents <- event
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := nodeB.Subscribe("node.event", func(_ context.Context, event ReceivedEvent) error {
		events <- event
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	result, err := nodeA.Publish(ctx, "node.event", []byte("published"), PublishOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Attempted != 1 || result.Delivered != 1 || !result.LocalDelivered || len(result.Failed) != 0 {
		t.Fatalf("publish result = %+v", result)
	}
	select {
	case event := <-events:
		if event.Sender != transportA.id || string(event.Payload) != "published" {
			t.Fatalf("event sender=%s payload=%q", event.Sender.String(), event.Payload)
		}
	case <-time.After(time.Second):
		t.Fatal("published event was not delivered")
	}
	select {
	case event := <-localEvents:
		if event.Sender != transportA.id || string(event.Payload) != "published" {
			t.Fatalf("local event sender=%s payload=%q", event.Sender.String(), event.Payload)
		}
	case <-time.After(time.Second):
		t.Fatal("published event was not delivered locally")
	}

	extra, _ := NewPeerID([]byte{0xc3})
	if err := nodeA.Authenticated(extra); err != nil {
		t.Fatalf("authenticated peer above candidate bound: %v", err)
	}
	if len(nodeA.Peers()) != 2 {
		t.Fatalf("authenticated peers = %+v, want 2", nodeA.Peers())
	}
	if _, err := nodeB.Subscribe("second.event", func(context.Context, ReceivedEvent) error { return nil }); err != nil {
		t.Fatalf("second subscription error=%v", err)
	}
}

func TestNodeRoutesOnlyToLeasedWildcardInterests(t *testing.T) {
	network := newMemoryNetwork()
	var transportA, transportB *memoryNodeTransport
	var nowUnix atomic.Int64
	nowUnix.Store(1_700_000_000)
	clock := func() time.Time { return time.Unix(nowUnix.Load(), 0).UTC() }
	config := func(identity byte, capture **memoryNodeTransport) NodeConfig {
		return NodeConfig{
			Transport:        network.factory(identity, capture),
			Bus:              BusConfig{clock: clock},
			InterestLeaseTTL: time.Minute, InterestRenewInterval: time.Millisecond,
		}
	}
	nodeA, err := NewNode(config(0xa1, &transportA))
	if err != nil {
		t.Fatal(err)
	}
	defer nodeA.Close()
	nodeB, err := NewNode(config(0xb2, &transportB))
	if err != nil {
		t.Fatal(err)
	}
	defer nodeB.Close()
	ctx := context.Background()
	if err := nodeA.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := nodeB.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := transportA.discover(transportB, nil); err != nil {
		t.Fatal(err)
	}
	if err := nodeA.Send(ctx, transportB.id, []byte("authenticate")); err != nil {
		t.Fatal(err)
	}

	if result, err := nodeA.Publish(ctx, "git.ref", nil, PublishOptions{RemoteOnly: true}); err != nil || result.Attempted != 0 {
		t.Fatalf("uninterested publish result=%+v error=%v", result, err)
	}
	received := make(chan ReceivedEvent, 1)
	subscription, err := nodeB.Subscribe("git.*", func(_ context.Context, event ReceivedEvent) error {
		received <- event
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := nodeA.Publish(ctx, "git.ref", []byte("match"), PublishOptions{RemoteOnly: true})
	if err != nil || result.Attempted != 1 || result.Delivered != 1 {
		t.Fatalf("matching publish result=%+v error=%v", result, err)
	}
	select {
	case <-received:
	case <-time.After(time.Second):
		t.Fatal("matching event was not delivered")
	}
	if result, err := nodeA.Publish(ctx, "git.ref.updated", nil, PublishOptions{RemoteOnly: true}); err != nil || result.Attempted != 0 {
		t.Fatalf("single-segment wildcard routed suffix result=%+v error=%v", result, err)
	}
	nowUnix.Add(59)
	time.Sleep(10 * time.Millisecond)
	nowUnix.Add(2)
	if result, err := nodeA.Publish(ctx, "git.ref", []byte("renewed"), PublishOptions{RemoteOnly: true}); err != nil || result.Attempted != 1 {
		t.Fatalf("periodically renewed interest result=%+v error=%v", result, err)
	}
	select {
	case <-received:
	case <-time.After(time.Second):
		t.Fatal("event after periodic interest renewal was not delivered")
	}

	if err := subscription.Close(); err != nil {
		t.Fatal(err)
	}
	nowUnix.Add(61)
	if result, err := nodeA.Publish(ctx, "git.ref", nil, PublishOptions{RemoteOnly: true}); err != nil || result.Attempted != 0 {
		t.Fatalf("expired interest publish result=%+v error=%v", result, err)
	}
	if len(nodeA.Peers()) != 1 {
		t.Fatalf("interest expiry removed authenticated peer: %+v", nodeA.Peers())
	}
}

func TestNodeRejectsUnknownPeersAndClosesComposition(t *testing.T) {
	network := newMemoryNetwork()
	var transport *memoryNodeTransport
	node, err := NewNode(NodeConfig{Transport: network.factory(1, &transport)})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	unknown, _ := NewPeerID([]byte{2})
	if err := node.Send(ctx, unknown, []byte("payload")); !errors.Is(err, ErrNodeNotStarted) {
		t.Fatalf("Send() before start error=%v", err)
	}
	if err := node.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := node.Start(ctx); !errors.Is(err, ErrNodeAlreadyStarted) {
		t.Fatalf("second Start() error=%v", err)
	}
	if err := node.Send(ctx, unknown, []byte("payload")); !errors.Is(err, ErrUnknownPeer) {
		t.Fatalf("unknown peer error=%v", err)
	}
	if err := node.Close(); err != nil {
		t.Fatal(err)
	}
	if err := node.Close(); err != nil {
		t.Fatalf("second Close() error=%v", err)
	}
	if err := node.Start(ctx); !errors.Is(err, ErrNodeClosed) {
		t.Fatalf("Start() after close error=%v", err)
	}
	if err := node.SendMessage(ctx, unknown, []byte("payload")); !errors.Is(err, ErrNodeClosed) {
		t.Fatalf("SendMessage() after close error=%v", err)
	}
}

func TestNodeValidatesComposition(t *testing.T) {
	if _, err := NewNode(NodeConfig{}); !errors.Is(err, ErrInvalidNode) {
		t.Fatalf("missing transport error=%v", err)
	}
	network := newMemoryNetwork()
	var transport *memoryNodeTransport
	if _, err := NewNode(NodeConfig{
		Transport: network.factory(1, &transport),
		Bus:       BusConfig{Sender: &recordingSender{}},
	}); !errors.Is(err, ErrInvalidNode) {
		t.Fatalf("caller-supplied bus sender error=%v", err)
	}
}

func TestNodeExpiresCandidatesButKeepsAuthenticatedPeers(t *testing.T) {
	var nowUnix atomic.Int64
	nowUnix.Store(1_700_000_000)
	network := newMemoryNetwork()
	var transport *memoryNodeTransport
	node, err := NewNode(NodeConfig{
		Transport: network.factory(1, &transport),
		Directory: DirectoryConfig{now: func() time.Time { return time.Unix(nowUnix.Load(), 0) }},
		PeerTTL:   10 * time.Millisecond, SweepInterval: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer node.Close()
	if err := node.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	peer, _ := NewPeerID([]byte{2})
	if err := node.Discovered(Peer{ID: peer}); err != nil {
		t.Fatal(err)
	}
	if err := node.Authenticated(peer); err != nil {
		t.Fatal(err)
	}
	candidate, _ := NewPeerID([]byte{3})
	if err := node.Discovered(Peer{ID: candidate}); err != nil {
		t.Fatal(err)
	}
	if len(node.Peers()) != 1 {
		t.Fatal("authenticated peer was not recorded")
	}
	nowUnix.Add(1)
	deadline := time.Now().Add(time.Second)
	for len(node.DiscoveredPeers()) != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(node.DiscoveredPeers()) != 0 {
		t.Fatalf("stale candidates = %+v", node.DiscoveredPeers())
	}
	if len(node.Peers()) != 1 || node.Peers()[0].ID != peer {
		t.Fatalf("authenticated peer expired = %+v", node.Peers())
	}
}
