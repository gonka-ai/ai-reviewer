---
id: subnet-attack-surface-index
type: implementation
path_filters:
  - "./subnet/**"
---
Use this as a **map from component to threats** when reading subnet diffs. Detailed narratives live in `subnet/docs/attacks.md`.

| Area | Main threats to keep in mind |
|------|------------------------------|
| **User / sequencer** (`subnet/user`) | Withholding or corrupting prompts; excluding `MsgFinishInference` from diffs; manipulating `started_at` or deadlines; abordering relative to honest hosts |
| **Host** (`subnet/host`) | Mempool staleness, timeouts, signature withholding, interaction between user requests and internal state |
| **State machine** (`subnet/state`) | Timeout bases (`started_at` vs executor-confirmed time), validation/settlement transitions, redundant operations that stall liveness, economic/token cost overflow |
| **Gossip** (`subnet/gossip`) | Equivocation handling, **unverified** nonces or hashes entering trusted maps, restricted senders vs open endpoints, amplification of bad txs into mempool DoS |
| **Transport / auth** (`subnet/transport`) | Authentication, rate limits, trusting client-supplied fields |
| **Signing** (`subnet/signing`) | Signature domains, malleability, seed/warm-key assumptions |
| **Bridge** (`subnet/bridge`) | Any change that alters mainnet alignment (see primer `subnet-mainnet-contract-caution`) |
| **Protos / types** (`subnet/proto`, `subnet/types`, `subnet/protocol`) | Wire compatibility, what is hashed and signed, cross-version rollout |

If a PR touches one row’s code, explicitly check whether it strengthens or weakens the corresponding row’s invariants.
