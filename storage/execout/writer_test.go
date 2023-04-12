package execout

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testConfigs = &Configs{
	execOutputSaveInterval: 10,
	ConfigMap: map[string]*Config{
		"A": &Config{
			moduleInitialBlock: 5,
		},
		"B": &Config{
			moduleInitialBlock: 10,
		},
		"C": &Config{
			moduleInitialBlock: 15,
		},
	},
}

func TestNewExecOutputWriterNotSubrequest(t *testing.T) {
	res := NewWriter(11, 15, "A", testConfigs, false)
	require.NotNil(t, res)
	assert.Equal(t, 20, int(res.files["A"].ExclusiveEndBlock))
}

func TestNewExecOutputWriterIsSubRequest(t *testing.T) {
	res := NewWriter(11, 15, "A", testConfigs, true)
	require.NotNil(t, res)
	assert.Equal(t, 15, int(res.files["A"].ExclusiveEndBlock))
}
