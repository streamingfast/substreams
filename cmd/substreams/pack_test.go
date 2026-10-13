package main

import (
	"fmt"
	"testing"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/require"
)

func TestMarshalPackageIsDeterministic(t *testing.T) {
	networks := map[string]*pbsubstreams.NetworkParams{}
	for i := 0; i < 16; i++ {
		networks[fmt.Sprintf("network-%02d", i)] = &pbsubstreams.NetworkParams{
			InitialBlocks: map[string]uint64{"map_a": uint64(i), "map_b": uint64(i + 1)},
			Params:        map[string]string{"map_a": fmt.Sprintf("a=%d", i), "map_b": fmt.Sprintf("b=%d", i)},
		}
	}
	pkg := &pbsubstreams.Package{Networks: networks}

	first, err := marshalPackage(pkg)
	require.NoError(t, err)

	// Go randomizes map iteration, so plain proto.Marshal orders these 16 entries differently from one call
	// to the next.
	for i := 0; i < 20; i++ {
		again, err := marshalPackage(pkg)
		require.NoError(t, err)
		require.Equal(t, first, again, "marshal %d differs from the first", i+1)
	}
}
