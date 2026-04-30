---
id: subnet-internal-protocol
model_category: best_code
path_filters:
  - "./subnet/proto/**"
  - "./subnet/types/**"
  - "./subnet/protocol/**"
  - "./subnet/state/**"
  - "./subnet/signing/**"
exclude_filters:
  - "**/*_test.go"
  - "**/*.pb.go"
---
You review **internal subnet protocol**: wire formats, cryptographic binding, and state transitions. Assume **multiple subnet versions** may run during rollout.

Focus on:

- **Protobuf and serialization:** Field additions, deprecations, oneofs, and JSON/binary parity. Breaking changes need versioning, feature flags, or explicit dual support.
- **What is signed and hashed:** Changes to `state` roots, diff application, or message canonicalization that alter signatures or commitments. Ensure old and new nodes agree on failure modes, not just the happy path.
- **State machine invariants:** Transitions that must be idempotent, no-ops for resolved states, or order-sensitive; avoid silent skips that break liveness without tests (see patterns around validation/challenge resolution in `docs/attacks.md`).
- **Signing:** Domain separation, key roles (warm key, host slots), and malleability or grind scenarios.

Flag **protocol-evolution** mistakes: incompatible encodings, ambiguous deserialization, state divergence between hosts, or weakened verification before accepting a diff or tx.

Do not re-review generated `*.pb.go` files beyond noting that proto changes require regeneration and compatibility consideration.
