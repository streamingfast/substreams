package codegen

import (
	"testing"

	"github.com/streamingfast/substreams/manifest"
	"github.com/stretchr/testify/require"
)

//func TestGenerator_ModRs(t *testing.T) {
//	manifestPath := "./substreams.yaml"
//	manifestReader := manifest.NewReader(manifestPath, manifest.SkipSourceCodeReader())
//
//	pkg, err := manifestReader.Read()
//	require.NoError(t, err)
//
//	expectedValue := "pub mod generated;\n"
//	buf := new(bytes.Buffer)
//
//	g := NewGenerator(pkg, buf)
//	err = g.GenerateModRs()
//	require.NoError(t, err)
//
//	require.Equal(t, expectedValue, buf.String())
//}

func TestGenerator_Generate(t *testing.T) {
	manifestPath := "./test_substreams/substreams.yaml"
	manifestReader := manifest.NewReader(manifestPath, manifest.SkipSourceCodeReader())

	pkg, err := manifestReader.Read()
	require.NoError(t, err)

	g := NewGenerator(pkg, "")
	err = g.Generate()
	require.NoError(t, err)
}
