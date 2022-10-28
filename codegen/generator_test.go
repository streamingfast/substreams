package codegen

import (
	"io"
	"os"
	"path/filepath"
	"testing"

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

//todo: add expected rs tests
// 1- mod.rs
// 2.

func TestGenerator_Generate(t *testing.T) {
	t.Skip()
	g := Init()
	err := g.Generate()
	require.NoError(t, err)
}

func TestGenerate_GenerateMod(t *testing.T) {
	g := Init()

	r, w := io.Pipe()
	var out []byte
	var err error

	done := make(chan bool)

	go func() {
		out, err = io.ReadAll(r)
		err = r.Close()
		require.NoError(t, err)
		close(done)
	}()

	err = g.GenerateMod(w)
	require.NoError(t, err)
	err = w.Close()
	require.NoError(t, err)

	<-done

	expectedMod, err := os.ReadFile(filepath.Join("./test_substreams/generated_rust_files/expected_mod.rs"))
	require.NoError(t, err)
	require.Equal(t, string(expectedMod), string(out))
}

func TestGenerate_GeneratePbMod(t *testing.T) {
	g := Init()

	r, w := io.Pipe()
	var out []byte
	var err error

	done := make(chan bool)

	go func() {
		out, err = io.ReadAll(r)
		err = r.Close()
		require.NoError(t, err)
		close(done)
	}()

	err = g.GeneratePbMod(w)
	require.NoError(t, err)
	err = w.Close()
	require.NoError(t, err)

	<-done

	expectedMod, err := os.ReadFile(filepath.Join("./test_substreams/generated_rust_files/expected_pb_mod.rs"))
	require.NoError(t, err)
	require.Equal(t, string(expectedMod), string(out))
}

func TestGenerate_GenerateExterns(t *testing.T) {
	g := Init()

	r, w := io.Pipe()
	var out []byte
	var err error

	done := make(chan bool)

	go func() {
		out, err = io.ReadAll(r)
		err = r.Close()
		require.NoError(t, err)
		close(done)
	}()

	err = g.GenerateExterns(w)
	require.NoError(t, err)
	err = w.Close()
	require.NoError(t, err)

	<-done

	expectedMod, err := os.ReadFile(filepath.Join("./test_substreams/generated_rust_files/expected_pb_mod.rs"))
	require.NoError(t, err)
	require.Equal(t, string(expectedMod), string(out))
}

func TestGenerate_GenerateLib(t *testing.T) {
	g := Init()

	r, w := io.Pipe()
	var out []byte
	var err error

	done := make(chan bool)

	go func() {
		out, err = io.ReadAll(r)
		err = r.Close()
		require.NoError(t, err)
		close(done)
	}()

	err = g.GenerateLib(w)
	require.NoError(t, err)
	err = w.Close()
	require.NoError(t, err)

	<-done

	expectedMod, err := os.ReadFile(filepath.Join("./test_substreams/generated_rust_files/expected_pb_mod.rs"))
	require.NoError(t, err)
	require.Equal(t, string(expectedMod), string(out))
}

func TestGenerate_GenerateSubstreams(t *testing.T) {
	g := Init()

	r, w := io.Pipe()
	var out []byte
	var err error

	done := make(chan bool)

	go func() {
		out, err = io.ReadAll(r)
		err = r.Close()
		require.NoError(t, err)
		close(done)
	}()

	err = g.GenerateSubstreams(w)
	require.NoError(t, err)
	err = w.Close()
	require.NoError(t, err)

	<-done

	expectedMod, err := os.ReadFile(filepath.Join("./test_substreams/generated_rust_files/expected_pb_mod.rs"))
	require.NoError(t, err)
	require.Equal(t, string(expectedMod), string(out))
}
