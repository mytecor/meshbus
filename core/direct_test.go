package core

import (
	"bytes"
	"errors"
	"testing"
)

func TestReceivedMessageIsolatesAuthenticatedSenderAndPayload(t *testing.T) {
	sender := []byte{0x01, 0x02}
	payload := []byte("payload")
	message, err := NewReceivedMessage(sender, payload)
	if err != nil {
		t.Fatal(err)
	}
	sender[0] = 0xff
	payload[0] = 'X'

	if got := message.Sender().Bytes(); !bytes.Equal(got, []byte{0x01, 0x02}) {
		t.Fatalf("sender = %x", got)
	}
	if message.Sender().String() != "0102" {
		t.Fatalf("sender text = %q", message.Sender().String())
	}
	gotPayload := message.Payload()
	if string(gotPayload) != "payload" {
		t.Fatalf("payload = %q", gotPayload)
	}
	gotPayload[0] = 'Y'
	if string(message.Payload()) != "payload" {
		t.Fatal("returned payload aliases message storage")
	}
}

func TestReceivedMessageRejectsMissingAuthorityAndAllowsEmptyPayload(t *testing.T) {
	if _, err := NewReceivedMessage(nil, []byte("payload")); !errors.Is(err, ErrInvalidPeerID) {
		t.Fatalf("missing sender error = %v", err)
	}
	message, err := NewReceivedMessage([]byte{1}, nil)
	if err != nil {
		t.Fatalf("empty payload error = %v", err)
	}
	if len(message.Payload()) != 0 {
		t.Fatalf("empty payload became %x", message.Payload())
	}
}

func TestPeerIDRejectsOversizedIdentity(t *testing.T) {
	if _, err := NewPeerID(make([]byte, MaxPeerIDBytes+1)); !errors.Is(err, ErrInvalidPeerID) {
		t.Fatalf("oversized identity error = %v, want ErrInvalidPeerID", err)
	}
}
