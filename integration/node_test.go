package integration_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/mytecor/meshbus/core"
)

type externalNetwork struct {
	mu    sync.RWMutex
	nodes map[core.PeerID]*externalTransport
}

type externalTransport struct {
	mu       sync.RWMutex
	network  *externalNetwork
	id       core.PeerID
	handler  core.Handler
	observer core.PeerObserver
	started  bool
}

func newExternalNetwork() *externalNetwork {
	return &externalNetwork{nodes: make(map[core.PeerID]*externalTransport)}
}

func (n *externalNetwork) factory(identity byte, captured **externalTransport) core.TransportFactory {
	return func(handler core.Handler) (core.NodeTransport, error) {
		id, err := core.NewPeerID([]byte{identity})
		if err != nil {
			return nil, err
		}
		transport := &externalTransport{network: n, id: id, handler: handler}
		n.mu.Lock()
		n.nodes[id] = transport
		n.mu.Unlock()
		*captured = transport
		return transport, nil
	}
}

func (t *externalTransport) Identity() core.PeerID { return t.id }

func (t *externalTransport) SetPeerObserver(observer core.PeerObserver) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.observer = observer
	return nil
}

func (t *externalTransport) Start(context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.started = true
	return nil
}

func (t *externalTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.started = false
	return nil
}

func (t *externalTransport) SendMessage(ctx context.Context, peer core.PeerID, payload []byte) error {
	t.network.mu.RLock()
	target := t.network.nodes[peer]
	t.network.mu.RUnlock()
	if target == nil {
		return errors.New("peer unavailable")
	}
	t.mu.RLock()
	started, observer := t.started, t.observer
	t.mu.RUnlock()
	target.mu.RLock()
	targetStarted, targetObserver, targetHandler := target.started, target.observer, target.handler
	target.mu.RUnlock()
	if !started || !targetStarted {
		return errors.New("peer unavailable")
	}
	if err := observer.Authenticated(target.id); err != nil {
		return err
	}
	if err := targetObserver.Authenticated(t.id); err != nil {
		return err
	}
	message, err := core.NewReceivedMessage(t.id.Bytes(), payload)
	if err != nil {
		return err
	}
	return targetHandler(ctx, message)
}

func (t *externalTransport) discover(peer *externalTransport) error {
	t.mu.RLock()
	observer := t.observer
	t.mu.RUnlock()
	return observer.Discovered(core.Peer{ID: peer.id})
}

func TestPublicNodeAPIFromExternalPackage(t *testing.T) {
	network := newExternalNetwork()
	var transportA, transportB *externalTransport
	nodeA, err := core.NewNode(core.NodeConfig{Transport: network.factory(1, &transportA)})
	if err != nil {
		t.Fatal(err)
	}
	defer nodeA.Close()
	nodeB, err := core.NewNode(core.NodeConfig{Transport: network.factory(2, &transportB)})
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
	if err := transportA.discover(transportB); err != nil {
		t.Fatal(err)
	}
	if err := transportB.discover(transportA); err != nil {
		t.Fatal(err)
	}
	events := make(chan core.ReceivedEvent, 1)
	if _, err := nodeB.Subscribe("example.event", func(_ context.Context, event core.ReceivedEvent) error {
		events <- event
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := nodeA.Send(ctx, nodeB.Identity(), []byte("authenticate")); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		result, publishErr := nodeA.Publish(ctx, "example.event", []byte("hello"), core.PublishOptions{})
		if publishErr != nil {
			t.Fatal(publishErr)
		}
		if result.Attempted == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("remote interest was not reconciled")
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case event := <-events:
		if event.Sender != nodeA.Identity() || string(event.Payload) != "hello" {
			t.Fatalf("event sender=%s payload=%q", event.Sender, event.Payload)
		}
	case <-time.After(time.Second):
		t.Fatal("external consumer did not receive event")
	}
}

func ExampleNode() {
	network := newExternalNetwork()
	var transportA, transportB *externalTransport
	nodeA, _ := core.NewNode(core.NodeConfig{Transport: network.factory(1, &transportA)})
	nodeB, _ := core.NewNode(core.NodeConfig{Transport: network.factory(2, &transportB)})
	defer nodeA.Close()
	defer nodeB.Close()
	ctx := context.Background()
	_ = nodeA.Start(ctx)
	_ = nodeB.Start(ctx)
	_ = transportA.discover(transportB)
	_ = transportB.discover(transportA)
	_ = nodeA.Send(ctx, nodeB.Identity(), []byte("authenticate"))

	received := make(chan core.ReceivedEvent, 1)
	_, _ = nodeB.Subscribe("example.event", func(_ context.Context, event core.ReceivedEvent) error {
		received <- event
		return nil
	})
	for {
		result, _ := nodeA.Publish(ctx, "example.event", []byte("hello"), core.PublishOptions{})
		if result.Attempted != 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	event := <-received
	fmt.Printf("%s: %s\n", event.Sender, event.Payload)
	// Output: 01: hello
}
