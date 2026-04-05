---
id: subnet-l2-architecture
type: overview
path_filters:
  - "./subnet/**"
---
# Gonka subnet (L2-style execution)

The `subnet` module implements a **user-as-sequencer** execution layer that should stay **largely autonomous** from the primary inference chain. Hosts verify work, co-sign state, and gossip among themselves; the **user** sequences transactions and diffs—assume the user can **reorder, delay, censor, or withhold** information unless the code cryptographically prevents it.

**Mainnet (inference chain) boundary:** integration goes through `subnet/bridge` (`MainnetBridge`): escrow lifecycle, settlement proposals and finalization, disputes, warm-key checks, and queries. Treat any expansion or semantic change to that surface as **high scrutiny**; prefer fixes and features that stay inside subnet unless there is a clear, documented need to touch mainnet behavior.

**Internal protocol:** protobuf definitions under `subnet/proto`, diffs, tx shapes, signing domains, and the state machine under `subnet/state` define the **subnet wire and consensus** among hosts. Changes there affect **versioning, compatibility, and rollout** across user and host software.

**Reference docs:** `subnet/docs/attacks.md` catalogs concrete attack paths (sequencer manipulation, gossip/mempool liveness, timeouts, signatures). `subnet/gossip/doc.go` summarizes host-to-host flows.

When reviewing a PR, classify changes as **subnet-internal**, **mainnet-integration**, or **operational surface** (transport, CLI, testenv) and apply the appropriate severity bar.
