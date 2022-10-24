package codegen

import (
	"os"
	"testing"

	"github.com/streamingfast/substreams/manifest"
	"github.com/stretchr/testify/require"
)

func TestGenerator_Generate(t *testing.T) {
	manifestPath := "./substreams.yaml"
	manifestReader := manifest.NewReader(manifestPath, manifest.SkipSourceCodeReader())

	pkg, err := manifestReader.Read()
	require.NoError(t, err)

	g := NewGenerator(pkg, os.Stdout)
	err = g.Generate()
	require.NoError(t, err)
}
