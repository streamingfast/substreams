package execout

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testConfigs = &Configs{
	execOutputSaveInterval: 10,
	ConfigMap: map[string]*Config{
		"A": {
			moduleInitialBlock: 5,
		},
		"B": {
			moduleInitialBlock: 10,
		},
		"C": {
			moduleInitialBlock: 15,
		},
	},
}

func TestNewExecOutputWriterIsSubRequest(t *testing.T) {
	res := NewWriter(11, 15, "A", testConfigs)
	require.NotNil(t, res)
	assert.Equal(t, 15, int(res.currentFile.ExclusiveEndBlock))
}
