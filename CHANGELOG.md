# Changelog

## Unreleased

## v0.4.0 — 2026-10-03

- Fixed a startup ordering race in the required shared-instance client: the transport is now
  marked connected to the shared instance before the live local interface starts, so the
  shared-instance link is an egress interface from the very first inbound packet. Previously a
  packet arriving between interface registration and the connectivity flag could be filtered,
  dropping outbound path requests and stranding client transports on a busy shared daemon
  until the daemon link flapped.
- Added a shared-instance correctness regression test.

## v0.3.0 — 2026-10-02

- Added NATS-style `*` and terminal `>` subscription patterns with wildcard local dispatch.
- Added direct, leased interest query/renewal and routed publications only to matching peers.
- Removed the redundant RNS session-wide send mutex and covered concurrent Channel/Resource sends.
- Expanded standalone documentation and continuous verification.
- Reorganized the public API into `core`, `security/realm`, and `transport/rns`; the old root,
  `realm`, and `rns` import paths were intentionally removed.
- Grouped topic grammar and binary codecs under `internal/protocol`.

## v0.1.0 — 2026-09-28

- Published the independent `github.com/mytecor/meshbus` module.
- Added realm membership, authenticated direct messages, bounded peer discovery, one-hop pub/sub,
  the cohesive Node API, and the Reticulum adapter.
- Documented and pinned the Realm v1, RNS Channel, `meshbus.v1` presence, and `MBE` v1 event wire
  contracts.
