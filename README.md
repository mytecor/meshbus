# meshbus

[![CI](https://github.com/mytecor/meshbus/actions/workflows/ci.yml/badge.svg)](https://github.com/mytecor/meshbus/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/mytecor/meshbus.svg)](https://pkg.go.dev/github.com/mytecor/meshbus)

`meshbus` is a small brokerless messaging layer for authenticated peers. It provides shared-secret
realm membership, bounded peer discovery, opaque direct messages, and best-effort one-hop pub/sub.

The core package and [`realm`](./realm) use only the Go standard library. [`rns`](./rns) is the
Reticulum adapter and is the only package that depends on Reticulum-Go.

## Guarantees

- The sender exposed to an application always comes from the authenticated transport session.
- Serialized payload identities are never authoritative.
- Discovery metadata is advisory and bounded.
- Pub/sub is in-memory, TTL-bounded, deduplicated, and best-effort; it has no replay, forwarding,
  offsets, consumer groups, or exactly-once guarantee.
- Transport routes remain adapter-private; applications address `PeerID` values.

## Install

```sh
go get github.com/mytecor/meshbus@v0.1.0
```

## Quick start

Applications normally construct the cohesive `Node` API through a transport adapter. With the RNS
adapter, both peers must receive the same 32-byte realm key through an out-of-band trusted channel
and must have access to an already-running Reticulum shared instance:

```go
node, err := rns.NewNode(rns.NodeConfig{
    Endpoint: rns.Config{
        StackMode:      rns.StackSharedClient,
        IdentitySource: "./meshbus-identity",
        RealmKey:       realmKey,
        PresenceMetadata: map[string]string{
            "service": "example",
        },
    },
    DirectHandler: func(ctx context.Context, message meshbus.ReceivedMessage) error {
        log.Printf("direct message from %s: %s", message.Sender(), message.Payload())
        return nil
    },
})
if err != nil {
    return err
}
defer node.Close()

if err := node.Start(ctx); err != nil {
    return err
}
```

An announce creates an advisory candidate in `DiscoveredPeers`. A successful realm-authenticated
session promotes that identity into `Peers`; only promoted peers receive pub/sub fan-out. A direct
`Send` to a discovered candidate can establish that first session.

See [Usage](./docs/USAGE.md) for direct messaging, pub/sub, lifecycle, defaults, and RNS modes.
A buildable shared-instance program lives in
[`examples/rns-shared`](./examples/rns-shared).

## Packages

- `github.com/mytecor/meshbus` — transport-independent identities, peer directory, Node, direct
  messaging, and bounded pub/sub.
- `github.com/mytecor/meshbus/realm` — standard-library-only realm IDs and mutual proofs.
- `github.com/mytecor/meshbus/rns` — Reticulum discovery, Links, realm authentication, Channels,
  sessions, and identity-to-destination routing.

## Documentation

- [Architecture and authority](./docs/ARCHITECTURE.md)
- [Usage](./docs/USAGE.md)
- [Wire contracts](./docs/WIRE.md)
- [Security policy](./SECURITY.md)
- [Contributing](./CONTRIBUTING.md)
- [Changelog](./CHANGELOG.md)

## Verification

```sh
make check
```

## Status

The module is pre-v1 while its public API is exercised by external consumers. Published wire
version markers and domain separators are changed only by introducing a new version.
