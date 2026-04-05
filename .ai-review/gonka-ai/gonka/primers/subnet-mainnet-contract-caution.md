---
id: subnet-mainnet-contract-caution
type: caution
path_filters:
  - "./subnet/bridge/**"
---
**Mainnet integration is a stability contract.** The `MainnetBridge` interface and its implementations are the subnet’s contract with the inference chain. Changes here are **strongly undesirable** unless the PR proves they are necessary.

Before accepting or suggesting edits, ask:

- **Necessity:** Could the same outcome be achieved **only inside** subnet (state machine, gossip, user/host logic) without new mainnet messages, new query semantics, or new settlement/dispute behavior?
- **Compatibility:** Are old subnets, old hosts, or old chain versions still safe? Is there a migration, feature flag, or dual-read/dual-write story?
- **Semantics:** Do new callbacks, parameters, or return values change **who can move funds**, **when settlement finalizes**, or **how disputes resolve**? Those need explicit security and operations review.

Surface area to treat as contractual: notifications (`OnEscrowCreated`, `OnSettlementProposed`, `OnSettlementFinalized`), queries (`GetEscrow`, `GetHostInfo`, `VerifyWarmKey`), and actions (`SubmitDisputeState`). Prefer **internal** refactors over **widening** this API.

Report findings when a change **materially risks** broken escrow/settlement alignment, ambiguous failure handling against chain state, or unnecessary coupling that will force future chain upgrades.
