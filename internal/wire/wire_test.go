package wire

import (
	"bytes"
	"errors"
	"testing"
	"time"
)

func TestEventRoundTrip(t *testing.T) {
	want := Event{
		ID: [16]byte{1}, Topic: "jobs.completed", PublishedAt: time.UnixMilli(1_700_000_000_123).UTC(),
		TTL: time.Minute, ContentType: "text/plain", Payload: []byte("done"),
	}
	encoded, err := EncodeEvent(want, 1024, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeEvent(encoded, 1024, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != want.ID || got.Topic != want.Topic || got.PublishedAt != want.PublishedAt ||
		got.TTL != want.TTL || got.ContentType != want.ContentType || !bytes.Equal(got.Payload, want.Payload) {
		t.Fatalf("decoded event = %+v", got)
	}
	encoded[len(encoded)-1] ^= 0xff
	if bytes.Equal(got.Payload, encoded[len(encoded)-len(got.Payload):]) {
		t.Fatal("decoded payload aliases input")
	}
}

func TestInterestRoundTripAndRejectsTrailingData(t *testing.T) {
	encoded, err := EncodeInterestRenew([]string{"jobs.*", "audit.>"}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeInterest(encoded)
	if err != nil || got.Kind != InterestRenew || got.Lease != time.Minute || len(got.Patterns) != 2 {
		t.Fatalf("decoded interest = %+v, error = %v", got, err)
	}
	if _, err := DecodeInterest(append(encoded, 0)); !errors.Is(err, ErrInvalidFrame) {
		t.Fatalf("trailing data error = %v", err)
	}
}
