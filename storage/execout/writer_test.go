package execout

import (
	"strings"
	"testing"

	"github.com/test-go/testify/assert"
	"github.com/test-go/testify/require"
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
	res := NewExecOutputWriter(11, 15, mkmap("A"), testConfigs, false)
	require.NotNil(t, res)
	assert.Equal(t, 20, int(res.files["A"].ExclusiveEndBlock))
}

func TestNewExecOutputWriterIsSubRequest(t *testing.T) {
	res := NewExecOutputWriter(11, 15, mkmap("A"), testConfigs, true)
	require.NotNil(t, res)
	assert.Equal(t, 15, int(res.files["A"].ExclusiveEndBlock))
}

func mkmap(in string) map[string]bool {
	out := map[string]bool{}
	for _, el := range strings.Split(in, ",") {
		out[el] = true
	}
	return out
}
