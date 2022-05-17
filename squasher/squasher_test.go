package squasher

import (
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/state"
	"testing"
)

func TestSquash(t *testing.T) {
	s := Squasher{map[string]*state.Builder{}}
	s.Squash("test", &block.Range{
		StartBlock:        0,
		ExclusiveEndBlock: 0,
	})
}

func testBuilder(name string, store dstore.Store) *state.Builder {
	return &state.Builder{
		Name:             name,
		Store:            store,
		ModuleStartBlock: 0,
		BlockRange:       nil,
		ModuleHash:       "",
		KV:               nil,
		Deltas:           nil,
		DeletedPrefixes:  nil,
	}
}
