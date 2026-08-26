package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/service/config"
)

func TestParseOperationMode(t *testing.T) {
	for _, in := range []string{"", "default", "DEFAULT"} {
		mode, err := config.ParseOperationMode(in)
		require.NoError(t, err, in)
		assert.Equal(t, config.OperationModeDefault, mode, in)
	}

	for _, in := range []string{"rolling-window", "rolling_window", "RollingWindow", "rollingwindow"} {
		mode, err := config.ParseOperationMode(in)
		require.NoError(t, err, in)
		assert.Equal(t, config.OperationModeRollingWindow, mode, in)
	}

	_, err := config.ParseOperationMode("sliding-window")
	require.Error(t, err)
}

func TestWithOperationMode(t *testing.T) {
	s := &Tier1Service{}
	assert.False(t, s.runtimeConfig.IsRollingWindow())

	WithOperationMode(config.OperationModeRollingWindow)(s)
	assert.True(t, s.runtimeConfig.IsRollingWindow())

	// Tier2 has a single mode, the option must be a no-op there.
	require.NotPanics(t, func() { WithOperationMode(config.OperationModeRollingWindow)(&Tier2Service{}) })
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
