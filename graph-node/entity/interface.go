package entity

import "time"

// An entity which is finalizeable is one that you guarantee will not be read
// nor written to anymore. In this case, you could purge it from a cache or write
// them early to a store
type Finalizable interface {
	IsFinal(blockNum uint64, blockTime time.Time) bool
}

// An entity that is mergeable is able to take an Entity from a previous _parallel step_, and merge it with the one from the current step. It needs to understand the state of the data in each field, at which step each field is "ready" (or valid), and know how to merge them.
//
// The Entity that is kept is the _receiver_ of the method, the previous is not kept around.  You are guaranteed that the type of `previous` will be the same as the _receiver_.
//
// For example, a field like `TransactionCount` that is computed on step 2, will want to be sum'ed up on step 3. You do not want to merge anything on step 1 because the data for TransactionCount is not "ready" on that step.
//
// Another example would be a field that computes an average over time. Merging an average would mean adding the two and dividing by two, but for the step at which we know the data was properly computed: if the update to the value comes from step 3, you will want to only apply the merge at step 4.
//
// NOTE: You should use `previous.MutatedOnStep` to validate the entity was effectively changed at the step you expect it to have changed.  In some situations, this might not be the case.
type Mergeable interface {
	Merge(step int, previous Interface)
}

type Sanitizable interface {
	Sanitize()
}

type CSVProcessing interface {
	Process(previous Interface)
}

type Cacheable interface {
	SkipDBLookup() bool
}

type Interface interface {
	GetID() string
	SetID(id string)
	GetVID() uint64
	SetVID(uint64)
	SetBlockRange(br *BlockRange)
	GetBlockRange() *BlockRange
	SetUpdatedBlockNum(blockNum uint64)

	Exists() bool
	SetExists(exists bool)
	SetMutated(step int)
}

type NamedEntity interface {
	TableName() string
}
