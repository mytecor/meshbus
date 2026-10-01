# Contributing

Run the complete local verification before submitting a change:

```sh
make check
```

Keep `core` and `security/realm` limited to the Go standard library. Transport-specific
dependencies belong in adapter packages such as `transport/rns`.

Changes to realm domains, RNS Channel message types, presence versions, or event framing must add a
new wire version and update [WIRE.md](./docs/WIRE.md) and its golden vectors. Never reinterpret an
existing version marker.

The sender authenticated by the transport is authoritative. Never trust sender identity copied
from an application payload or advisory discovery metadata.
