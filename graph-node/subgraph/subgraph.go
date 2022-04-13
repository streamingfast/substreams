package subgraph

import (
	graphnode "github.com/streamingfast/substreams/graph-node"
	"go.uber.org/zap"
)

var MainSubgraphDef *Definition

type Subgraph interface {
	Init() error

	LoadDynamicDataSources(blockNum uint64) error
	// FIXME: this should be a `bstream.Block`, and generated `HandleBlock`
	// casts it to an ETH-specific block, or a `ToNative()` first thing.

	LogStatus()
}

type DDL interface {
	InitiateSchema(handleStatement func(statement string) error) error
	CreateTables(handleStatement func(table string, statement string) error) error
	CreateIndexes(handleStatement func(table string, statement string) error) error
	DropIndexes(handleStatement func(table string, statement string) error) error
}

type Definition struct {
	PackageName string

	HighestParallelStep int

	StartBlock    uint64
	IncludeFilter string
	Entities      *graphnode.Registry
	DDL           DDL
	Manifest      string
	GraphQLSchema string
	Abis          map[string]string

	New       func(Base) Subgraph
	MergeFunc func(step int, current, next graphnode.Entity) graphnode.Entity
}

// Base contains initialized values for a Subgraph instance, wrapped in a struct for future-proofness.
type Base struct {
	Intrinsics
	*Definition

	ID  string // QmHELLOWORLD as fetched when necessary
	Log *zap.Logger
}
