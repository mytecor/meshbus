# Security policy

## Reporting a vulnerability

Do not open a public issue for a suspected vulnerability. Use GitHub's private vulnerability
reporting for this repository so the report, reproducer, affected versions, and proposed mitigation
can be discussed before disclosure.

Include the affected package and version, required attacker capabilities, expected and observed
authority boundaries, and whether the issue permits impersonation, cross-realm delivery, resource
exhaustion, secret disclosure, or wire-format confusion.

## Security model

- Transport authentication establishes peer identity.
- Mutual realm proof establishes possession of the shared realm key.
- Realm membership does not grant application permissions.
- Presence metadata and payload contents are untrusted and never supply sender authority.
- Applications remain responsible for key distribution, storage, rotation, authorization, durable
  replay protection, and handler idempotency.

The supported wire contracts and their compatibility rules are documented in
[WIRE.md](./docs/WIRE.md).
