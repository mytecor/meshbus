# Architecture and authority

## Repository layout

```text
./                 transport-independent public meshbus package
realm/             standard-library-only realm primitive
rns/               public Reticulum adapter and its package-private mechanics
docs/              architecture, usage, and wire contracts
examples/          buildable application examples
integration/       black-box tests that consume only exported APIs
.github/workflows/ standalone continuous verification
```

The public core remains at the module root so consumers import `github.com/mytecor/meshbus` rather
than an artificial `pkg/meshbus` or `core` suffix. Files move into a new package only when there is
a real dependency boundary, not merely to reduce the number of files shown at the root.

## Layers

```text
application
    ↓ direct handler / subscriptions / application authorization
meshbus.Node
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
- `Bus` owns in-memory subscriptions, bounded queues, event IDs, TTL handling, and bounded duplicate
  suppression.
- Applications own durable state, retries beyond one send, authorization, and idempotency.

No network disconnect is treated as an application-lifetime signal.

## Delivery model

Direct messages are opaque authenticated bytes. Pub/sub wraps a bounded event frame in a direct
message and fans it out once to the current authenticated peer snapshot. There is no forwarding,
subscription advertisement, persistence, replay, acknowledgement journal, consumer group, offset,
or exactly-once guarantee. Handlers must tolerate duplicate delivery, especially across process
restarts where the in-memory deduplication cache is lost.

## Resource model

Discovery candidates, metadata, event payloads, TTL, queues, deduplication memory, and fan-out
concurrency are explicitly bounded. Subscription and authenticated-peer counts have no artificial
global cap. The RNS adapter uses Channel for messages within the negotiated MDU and transparently
switches to Resource transfer for larger messages, so the core Bus payload budget remains reachable
over RNS.
