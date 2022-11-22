package codegen

import (
	"io"
	"os"
	"path/filepath"
	"strings"
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
	g := InitTestGenerator(t)
	err := g.Generate()
	require.NoError(t, err)
}

func TestGenerate_GenerateMod(t *testing.T) {
	t.Skip()

	g := InitTestGenerator(t)

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
	g.writer = w

	protoPackages := map[string]string{}
	for _, definition := range g.protoDefinitions {
		p := definition.GetPackage()
		protoPackages[p] = strings.ReplaceAll(p, ".", "_")
	}

	err = generate("", tplMod, protoPackages, "", WithTestWriter(w))

	require.NoError(t, err)
	err = w.Close()
	require.NoError(t, err)

	<-done

	expectedMod, err := os.ReadFile(filepath.Join("./test_substreams/expected_test_outputs/generated/mod.rs"))
	require.NoError(t, err)
	require.Equal(t, string(expectedMod), string(out))
}

func TestGenerate_GeneratePbMod(t *testing.T) {
	t.Skip()

	g := InitTestGenerator(t)

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
	err = generate("", tplPbMod, protoPackages(g.protoDefinitions), "use std.out", WithTestWriter(w))
	require.NoError(t, err)
	err = w.Close()
	require.NoError(t, err)

	<-done

	expectedMod, err := os.ReadFile(filepath.Join("./test_substreams/expected_test_outputs/pb/mod.rs"))
	require.NoError(t, err)
	require.Equal(t, string(expectedMod), string(out))
}

func TestGenerate_GenerateExterns(t *testing.T) {
	t.Skip()

	g := InitTestGenerator(t)

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

	err = generate("GenerateExterns", tplExterns, g.engine, "use std.out", WithTestWriter(w))

	require.NoError(t, err)
	err = w.Close()
	require.NoError(t, err)

	<-done

	expectedMod, err := os.ReadFile(filepath.Join("./test_substreams/expected_test_outputs/generated/externs.rs"))
	require.NoError(t, err)
	require.Equal(t, string(expectedMod), string(out))
}

func TestGenerate_GenerateLib(t *testing.T) {
	t.Skip()

	g := InitTestGenerator(t)

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

	err = generate("Lib", tplLibRs, g.engine, "use std.out", WithTestWriter(w))

	require.NoError(t, err)
	err = w.Close()
	require.NoError(t, err)

	<-done

	expectedMod, err := os.ReadFile(filepath.Join("./test_substreams/expected_test_outputs/lib.rs"))
	require.NoError(t, err)
	require.Equal(t, string(expectedMod), string(out))
}

func TestGenerate_GenerateSubstreams(t *testing.T) {
	t.Skip()

	g := InitTestGenerator(t)

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

	err = generate("Substreams", tplSubstreams, g.engine, "use std.out", WithTestWriter(w))

	require.NoError(t, err)
	err = w.Close()
	require.NoError(t, err)

	<-done

	expectedMod, err := os.ReadFile(filepath.Join("./test_substreams/expected_test_outputs/generated/substreams.rs"))
	require.NoError(t, err)
	require.Equal(t, string(expectedMod), string(out))
}
