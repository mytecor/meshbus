# Changelog

## Unreleased

- Added NATS-style `*` and terminal `>` subscription patterns with wildcard local dispatch.
- Added direct, leased interest query/renewal and routed publications only to matching peers.
- Removed the redundant RNS session-wide send mutex and covered concurrent Channel/Resource sends.
- Expanded standalone documentation and continuous verification.
- Organized package documentation, examples, integration tests, and RNS configuration by role
  without changing public import paths.

## v0.1.0 — 2026-09-28

- Published the independent `github.com/mytecor/meshbus` module.
- Added realm membership, authenticated direct messages, bounded peer discovery, one-hop pub/sub,
  the cohesive Node API, and the Reticulum adapter.
- Documented and pinned the Realm v1, RNS Channel, `meshbus.v1` presence, and `MBE` v1 event wire
  contracts.
