package core

import (
	"errors"
	"testing"
	"time"
)

func TestInterestWireQueryAndRenewRoundTrip(t *testing.T) {
	query, err := decodeInterest(encodeInterestQuery())
	if err != nil || query.kind != interestKindQuery {
		t.Fatalf("query=%+v error=%v", query, err)
	}
	wire, err := encodeInterestRenew([]string{"git.*", "jobs.>", ">"}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	renew, err := decodeInterest(wire)
	if err != nil || renew.kind != interestKindRenew || renew.lease != time.Minute || len(renew.patterns) != 3 {
		t.Fatalf("renew=%+v error=%v", renew, err)
	}

	malformed := append([]byte(nil), wire...)
	malformed = append(malformed, 0)
	if _, err := decodeInterest(malformed); !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("trailing data error=%v", err)
	}
}

func TestInterestTableUsesLeaseExpiryAndWildcardMatching(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	table := newInterestTable(func() time.Time { return now }, time.Minute)
	peer := busTestPeer("peer")
	table.renew(peer, []string{"foo.*"}, time.Hour)
	if got := table.InterestedPeers("foo.bar"); len(got) != 1 || got[0] != peer {
		t.Fatalf("matching peers=%v", got)
	}
	if got := table.InterestedPeers("foo.bar.baz"); len(got) != 0 {
		t.Fatalf("unexpected matching peers=%v", got)
	}
	now = now.Add(time.Minute + time.Millisecond)
	if got := table.InterestedPeers("foo.bar"); len(got) != 0 {
		t.Fatalf("expired matching peers=%v", got)
	}
}
