package codegen

import (
	"testing"

	"github.com/streamingfast/substreams/manifest"
	"github.com/stretchr/testify/require"
)

func TestGenerator_Generate(t *testing.T) {
	manifestPath := "../../substreams-uniswap-v3/substreams.yaml"
	manifestReader := manifest.NewReader(manifestPath, manifest.SkipSourceCodeReader())

	pkg, err := manifestReader.Read()
	require.NoError(t, err)

	g := NewGenerator(pkg)
	err = g.Generate()
	require.NoError(t, err)

}
