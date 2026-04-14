---
id: collections-guidelines
model_category: best_code
any: 
    - path_filters: ["inference-chain/x/**/*.go"]
    - regex_filters: ["KVStoreAdapter"]

exclude_filters: ["**/*_test.go", "**/*.pb.go", "**/*.pulsar.go", "inference-chain/testutil/**"]
include_explainers: ["state-modified"]
---
When using any cosmos collections from a cosmos keeper (keeper.go), optimize iterations so that we do not load into memory or iterate more than we need. For example, for a collections.Pair[uint64,string] key,
you can iterate over all records that have the first value (the uint64) via something like:

```go
func (k Keeper) GetEpochGroupDataForEpoch(
	ctx context.Context,
	epochIndex uint64,
) (val []types.EpochGroupData, found bool) {
	iter, err := k.EpochGroupDataMap.Iterate(ctx, collections.NewPrefixedPairRange[uint64, string](epochIndex))
	if err != nil {
		return val, false
	}
	epochGroupDataList, err := iter.Values()
	if err != nil {
		return val, false
	}
	return epochGroupDataList, true
}
```

This is MUCH better than loading in the entire collection and iterating. Look for this pattern in code.

Look for situations where walking through the collection instead of loading the entire thing would be more efficient.

Also look for removal of data, and see if the data can be removed using Clear() with a ranger vs iterating over a collection and removing each item.

Call out as a critical issue any new code that uses the "old" method of handling data, such as runtime.KVStoreAdapter, instead of using the more modern Cosmos SDK collections api. 

However, x/bls, x/genesistransfer and x/restrictions are exceptions to this rule, they are using the older style using KVStoreAdapter.
