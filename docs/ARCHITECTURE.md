# Architecture and authority

## Repository layout

```text
core/                       transport-independent public API and composition root
security/realm/             standard-library-only realm primitive
transport/rns/              public Reticulum adapter and package-private mechanics
internal/protocol/subject/  shared topic grammar and wildcard matching
internal/protocol/wire/     versioned event and interest binary codecs
docs/                       architecture, usage, and wire contracts
examples/                   buildable application examples
integration/                black-box tests that consume only exported APIs
.github/workflows/          standalone continuous verification
```

The module root contains repository metadata and project documentation only. Runtime code is grouped
by dependency direction: applications use `core`, security primitives live under `security`, and
transport adapters live under `transport`. Protocol details shared by those packages stay internal.

Within the public core, files follow responsibilities rather than types: `node.go` is the composition
root, while lifecycle, peer observation, interest exchange, and the public forwarding API live in
separate files. Bus publication, inbound delivery, and subscription workers follow the same split.
The `internal` packages contain pure policy/codecs and cannot depend back on the public core.

## Layers

```text
application
    ↓ direct handler / subscriptions / application authorization
core.Node
    ↓ authenticated PeerID messages and bounded one-hop events
transport adapter
    ↓ discovery, routes, sessions, encryption, authenticated identity
network
```

The core module has no Reticulum types. A transport implements `NodeTransport`, supplies its local
identity, reports discovery separately from successful authentication, and delivers
`ReceivedMessage` values. The `rns` adapter is one implementation of that contract.

## Authority boundary

Authority moves upward in explicit stages:

1. Discovery yields an advisory candidate identity and metadata. It does not prove realm-key
   possession or grant application permissions.
2. The transport authenticates the remote identity and the RNS adapter completes the mutual realm
   proof.
3. `Node` promotes the identity into its authenticated peer directory.
4. Direct messages and events expose only the sender established by that authenticated session.
5. The application decides what that realm member may do. Realm membership is not application
   authorization.

An identity serialized inside an opaque payload or presence metadata is never authoritative.

## State ownership

- The transport adapter owns routes, active sessions, reconnects, and transport-specific limits.
- `Node` bounds discovery candidates with oldest-first eviction and expires only stale candidates.
- Authenticated peers have no hard count limit and do not expire merely because announces stop.
- `Bus` owns in-memory subject-pattern subscriptions, bounded queues, event IDs, TTL handling, and
  bounded duplicate suppression.
- `Node` owns non-durable remote interest leases learned directly from each authenticated peer.
- Applications own durable state, retries beyond one send, authorization, and idempotency.

No network disconnect is treated as an application-lifetime signal.

## Delivery model

Direct messages are opaque authenticated bytes. Pub/sub wraps a bounded event frame in a direct
message and sends it once to authenticated peers whose non-expired leased patterns match the
concrete subject. Local dispatch uses the same wildcard matcher. Each peer advertises only its own
subscriptions: immediately on subscribe, in response to a current-interest query, and through
periodic renewal. Closing a subscription sends no withdrawal; the old route ages out at lease
expiry. There is no forwarding, durable subscription state, revisions, replay, acknowledgement
journal, consumer group, offset, or exactly-once guarantee. Lost and reordered renewals converge
through the next renewal and lease expiry.

Transport send failure is not a liveness signal. Peer discovery/session liveness and subscription
liveness remain separate; only an expired interest lease stops routing for that pattern. Higher-level
request/reply helpers, including RPC, are expected to use ordinary subjects and subscriptions rather
than define a parallel transport protocol.

## Resource model

Discovery candidates, metadata, event payloads, TTL, queues, deduplication memory, and fan-out
concurrency are explicitly bounded. Subscription and authenticated-peer counts have no artificial
global cap. The RNS adapter uses Channel for messages within the negotiated MDU and transparently
switches to Resource transfer for larger messages, so the core Bus payload budget remains reachable
over RNS. Reticulum-Go provides Channel sequencing, duplicate suppression, buffering,
retransmission, and its own Channel/Resource send synchronization; meshbus adds no generic ordering
layer or session-wide send mutex. Ordering across Channel and Resource payloads is not an
application-level contract.
