# Usage

## Realm keys

Generate a new 32-byte key once and distribute it through a trusted out-of-band channel:

```go
realmKey, err := realm.GenerateKey()
```

Possession proves realm membership only. Do not put the key in presence metadata, application
payloads, logs, or command-line arguments. Key storage, rotation, and member-level authorization
belong to the application.

## Node lifecycle

Construct a Node, register subscriptions, start it with a non-nil context, and always close it:

```go
subscription, err := node.Subscribe("build.completed", handler)
if err != nil {
    return err
}
defer subscription.Close()

if err := node.Start(ctx); err != nil {
    return err
}
defer node.Close()
```

`Start` may be called once. Cancelling its context stops Node background work, but callers should
still call `Close` to close the transport and subscriptions deterministically.

## Discovery and authentication

`DiscoveredPeers` contains advisory candidates. `Peers` contains only identities that completed
transport and realm authentication. `Send` accepts either an authenticated peer or a discovered
candidate; sending to a candidate lets the adapter establish and authenticate the first session.

```go
candidates := node.DiscoveredPeers()
if len(candidates) != 0 {
    err := node.Send(ctx, candidates[0].ID, []byte("hello"))
    // The peer enters node.Peers() only after authentication succeeds.
}
```

Applications should use `PeerID` as the peer handle. RNS destination strings are adapter-private.

## Pub/sub

Subscribe to a subject pattern and publish opaque bytes:

```go
_, err := node.Subscribe("build.completed", func(ctx context.Context, event meshbus.ReceivedEvent) error {
    log.Printf("event from %s: %s", event.Sender, event.Payload)
    return nil
})

result, err := node.Publish(ctx, "build.completed", payload, meshbus.PublishOptions{
    TTL:         30 * time.Second,
    ContentType: "application/json",
})
```

By default a Node publication is also delivered to matching local subscriptions. Set `RemoteOnly`
to suppress local delivery. `PublishResult` distinguishes attempted and successful remote sends,
per-peer failures, local delivery, and a local backpressure error. A partial failure is returned as
a joined error after every accepted peer has been attempted.

Published subjects are concrete and case-sensitive. Subscription patterns accept NATS-style
wildcards: `*` matches one segment, terminal `>` matches any non-empty suffix, and `>` by itself
matches every subject. For example, `git.*` matches `git.push` but not `git.ref.updated`, while
`git.>` matches both.

Each node advertises only its own current patterns to authenticated peers. Interests are ephemeral
soft state with a 60-second lease, renewed every 20 seconds by default. Subscribe announces a new
interest immediately; closing the last matching subscription stops renewal and lets the remote
entry expire. A newly authenticated peer is queried for its current interests. Publications are
sent only to peers with a non-expired matching interest. Send failures do not remove peers or
interests.

## Default bounds

| Resource | Default |
| --- | ---: |
| Event TTL | 1 minute |
| Maximum event TTL | 1 hour |
| Event payload | 64 MiB |
| Deduplication entries | 4096 |
| Queue entries per subscription | 32 |
| Subscriptions | No global cap; each has a bounded queue |
| Interest lease TTL | 60 seconds |
| Interest renewal interval | 20 seconds |
| Fan-out peers | Authenticated peers with a matching non-expired interest |
| Concurrent sends | 8 |
| Discovery candidates | 1024, oldest evicted at capacity |
| Authenticated peers | No hard count limit |
| Metadata per peer | 4096 bytes |
| Candidate stale TTL | 15 minutes |
| Peer sweep interval | 1 minute |

Zero-valued configuration fields select defaults. Negative directory bounds and contradictory Bus
or Node limits are rejected. Authenticated peers do not expire on the discovery TTL; transport
lifecycle and explicit shutdown govern their sessions.

## RNS modes

`rns.StackSharedClient` attaches to the platform-default Reticulum shared instance and never falls
back to owning its listener. `rns.StackStandalone` requires an explicit Reticulum-Go configuration
with at least one enabled interface.

Choose exactly one identity source:

- `IdentitySource` loads or creates a persistent identity file, or accepts encoded private RNS
  identity material;
- `EphemeralIdentity` creates a fresh in-memory identity for the process lifetime.

Set `Passive` to suppress local announces while retaining discovery and outbound messaging.

## Error handling

Public sentinel errors support `errors.Is`. In particular, callers can distinguish invalid input,
unknown peers, lifecycle misuse, resource bounds, backpressure, closed buses,
invalid RNS destinations, missing shared instances, and realm-authentication failure.

Subscription-handler and peer-observer errors are reported asynchronously through
`BusConfig.OnHandlerError` and `NodeConfig.OnPeerError`. Direct-handler error propagation is part of
the selected transport adapter's delivery contract.
