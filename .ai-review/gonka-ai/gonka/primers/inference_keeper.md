---
id: inference_keeper
type: implementation
path_filters: ["./inference-chain/**/keeper/keeper.go"]
---
Gonka uses the collections that were introduced in Cosmos SDK 0.50.x+

This is in contrast to earlier versions that would use the KVStore directly. There is some legacy code in Gonka that still uses this older approach, as Gonka converted to (mostly) using collections shortly before going live:

Still using the KVStore:
- The x/bls module
- The x/bookkeeper module
- The x/genesistransfer module
- The x/streamvesting module
- The x/restrictions module
- In x/inference, ActiveParticipants use the KVStore directly to support validation of data via merkle proofs.
- In x/inference, these remaining data structures:
  - Developer Stats 
  - Params
  - MLNodeVersion
  - TokenomicsData

When using collections, the best practice for data structures is:
- Map: to work with typed arbitrary KV pairings.
- KeySet: to work with just typed keys
- Item: to work with just one typed value
- Sequence: which is a monotonically increasing number.
- IndexedMap: which combines Map and KeySet to provide a Map with indexing capabilities.

Keys can be composite (more than one field and value).