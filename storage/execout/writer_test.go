package execout

import (
	"testing"

	"github.com/streamingfast/dstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testConfigs = &Configs{
	execOutputSaveInterval: 10,
	ConfigMap: map[string]*Config{
		"A": {
			moduleInitialBlock: 5,
			objStore:           dstore.NewMockStore(nil),
		},
		"B": {
			moduleInitialBlock: 10,
			objStore:           dstore.NewMockStore(nil),
		},
		"C": {
			moduleInitialBlock: 15,
			objStore:           dstore.NewMockStore(nil),
		},
	},
}

func TestNewExecOutputWriterIsSubRequest(t *testing.T) {
	res := NewWriter(11, 15, "A", testConfigs, false)
	require.NotNil(t, res)
	assert.Equal(t, 15, int(res.CurrentFile.ExclusiveEndBlock))
}
