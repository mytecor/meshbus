package rns

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/mytecor/meshbus"
)

func TestAuthMessageRoundTrip(t *testing.T) {
	want := &authMessage{kind: authKindResponse, nonce: bytes.Repeat([]byte{2}, authNonceSize), proof: bytes.Repeat([]byte{3}, authProofSize)}
	data, err := want.Pack()
	if err != nil {
		t.Fatal(err)
	}
	var got authMessage
	if err := got.Unpack(data); err != nil {
		t.Fatal(err)
	}
	if got.kind != want.kind || !bytes.Equal(got.nonce, want.nonce) || !bytes.Equal(got.proof, want.proof) {
		t.Fatalf("unpacked = %+v", got)
	}
}

func TestApplicationDataBeforeAuthenticationIsDropped(t *testing.T) {
	received := make(chan meshbus.ReceivedMessage, 1)
	endpoint := &Endpoint{handler: func(_ context.Context, message meshbus.ReceivedMessage) error {
		received <- message
		return nil
	}}
	active := &session{sender: []byte{1}}
	endpoint.deliver(active, []byte("before-auth"))
	select {
	case message := <-received:
		t.Fatalf("pre-auth message was delivered: %q", message.Payload())
	case <-time.After(10 * time.Millisecond):
	}

	active.authenticated = true
	endpoint.deliver(active, []byte("after-auth"))
	select {
	case message := <-received:
		if string(message.Payload()) != "after-auth" {
			t.Fatalf("payload = %q", message.Payload())
		}
	case <-time.After(time.Second):
		t.Fatal("authenticated message was not delivered")
	}
}

func TestEarlyAuthenticationMessagesRemainBounded(t *testing.T) {
	endpoint := &Endpoint{}
	active := &session{}
	for index := range 1000 {
		endpoint.handleAuthentication(active, &authMessage{
			kind:  authKindChallenge,
			nonce: bytes.Repeat([]byte{byte(index)}, authNonceSize),
		})
	}
	active.mu.RLock()
	defer active.mu.RUnlock()
	if len(active.pendingAuth) != 1 {
		t.Fatalf("pending auth messages = %d, want 1 per message kind", len(active.pendingAuth))
	}
	want := bytes.Repeat([]byte{byte(999 % 256)}, authNonceSize)
	if got := active.pendingAuth[authKindChallenge].nonce; !bytes.Equal(got, want) {
		t.Fatalf("pending challenge nonce = %x, want latest %x", got, want)
	}
}

func TestAuthReadyMessageRoundTrip(t *testing.T) {
	want := &authMessage{kind: authKindReady}
	data, err := want.Pack()
	if err != nil {
		t.Fatal(err)
	}
	var got authMessage
	if err := got.Unpack(data); err != nil {
		t.Fatal(err)
	}
	if got.kind != authKindReady || len(got.nonce) != 0 || len(got.proof) != 0 {
		t.Fatalf("unpacked = %+v", got)
	}
}
