package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func TestWithRollingWindowMode(t *testing.T) {
	s := &Tier1Service{}
	assert.False(t, s.runtimeConfig.RollingWindowMode)

	WithRollingWindowMode()(s)
	assert.True(t, s.runtimeConfig.RollingWindowMode)

	// Tier2 never runs in this mode, the option must be a no-op there.
	require.NotPanics(t, func() { WithRollingWindowMode()(&Tier2Service{}) })
}

func TestStoreModulesUnsupportedError(t *testing.T) {
	usedModules := []*pbsubstreams.Module{
		{Name: "map_a", Kind: &pbsubstreams.Module_KindMap_{KindMap: &pbsubstreams.Module_KindMap{}}},
		{Name: "store_b", Kind: &pbsubstreams.Module_KindStore_{KindStore: &pbsubstreams.Module_KindStore{}}},
		{Name: "store_c", Kind: &pbsubstreams.Module_KindStore_{KindStore: &pbsubstreams.Module_KindStore{}}},
	}

	err := storeModulesUnsupportedError(usedModules)
	assert.EqualError(t, err, "store modules are not supported on this endpoint: store_b, store_c")
}
