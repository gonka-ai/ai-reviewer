---
id: governance-parameters
type: implementation
path_filters: ["./inference-chain/**/params.proto", "./inference-chain/**/params.go"] 
---

Parameters are all controlled via governance votes on the blockchain. They must be proposed with a deposit, and then voted on by a quorum and a pass threshold, usually 50%.

Upgrades are passed in the same way and with the same process, so parameters are simply a safer, more focused way to modify the behavior of the chain without requiring a full upgrade.

In addition, there are vetos, where only 1/3rd of the voting power can veto a proposal. This protects against a bare majority from malicious governance proposals.
