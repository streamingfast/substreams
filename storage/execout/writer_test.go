package execout

import (
	"context"
	"testing"

	"github.com/streamingfast/dstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

var testConfigs = &Configs{
	execOutputSaveInterval: 10,
	logger:                 zap.NewNop(),
	ConfigMap: map[string]*Config{
		"A": {
			moduleInitialBlock: 5,
			objStore:           dstore.NewMockStore(nil),
			logger:             zap.NewNop(),
		},
		"B": {
			moduleInitialBlock: 10,
			objStore:           dstore.NewMockStore(nil),
			logger:             zap.NewNop(),
		},
		"C": {
			moduleInitialBlock: 15,
			objStore:           dstore.NewMockStore(nil),
			logger:             zap.NewNop(),
		},
	},
}

func TestNewExecOutputWriterIsSubRequest(t *testing.T) {
	res := NewWriter(context.Background(), 11, 15, "A", testConfigs, false)
	require.NotNil(t, res)
	assert.Equal(t, 15, int(res.CurrentFile.ExclusiveEndBlock))
}
