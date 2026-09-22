package stage

import (
	"context"
	"testing"

	"github.com/streamingfast/substreams/orchestrator/plan"
	"github.com/streamingfast/substreams/pipeline/exec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAllStoresCompletedSentOnce(t *testing.T) {
	reqPlan, err := plan.BuildTier1RequestPlan(true, 10, 0, 0, 0, 100, 100, false)
	require.NoError(t, err)
	stages := NewStages(context.Background(), exec.TestGraphMapper(0), reqPlan, nil, nil)
	require.True(t, stages.AllStoresCompleted())

	cmd := stages.CmdAllStoresCompletedOnce()
	require.NotNil(t, cmd)
	assert.IsType(t, MsgAllStoresCompleted{}, cmd())
	assert.Nil(t, stages.CmdAllStoresCompletedOnce())

	for range 3 {
		assert.Nil(t, stages.CmdTryMerge(0), "a mapper stage has nothing to merge")
	}
}
