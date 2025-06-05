package integration

import (
	"testing"

	"github.com/stretchr/testify/require"
	"rogchap.com/v8go"
)

func Test_V8_BasicExecution(t *testing.T) {
	iso := v8go.NewIsolate()
	ctx := v8go.NewContext(iso)

	val, err := ctx.RunScript(`1 + 2`, "basic.js")
	require.NoError(t, err)

	require.True(t, val.IsNumber())
	require.Equal(t, 3.0, val.Number())
}
