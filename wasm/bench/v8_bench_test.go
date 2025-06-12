package main

import (
	"os"
	"testing"

	"rogchap.com/v8go"
)

func mustRead(tb testing.TB, path string) []byte {
	tb.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		tb.Fatalf("ReadFile %q: %v", path, err)
	}
	return data
}

func newV8Instance(b *testing.B) *v8go.Context {
	iso := v8go.NewIsolate()
	ctx := v8go.NewContext(iso)
	return ctx
}

func BenchmarkV8_RunJS_Separate(b *testing.B) {

	poly := mustRead(b, "../v8/runtime/polyfill.bundle.js")
	bund := mustRead(b, "testdata/bundle.js")

	ctx := newV8Instance(b)
	defer ctx.Isolate().Dispose()

	if _, err := ctx.RunScript(string(poly), "polyfill.js"); err != nil {
		b.Fatalf("polyfill init error: %v", err)
	}

	for b.Loop() {
		if _, err := ctx.RunScript(string(bund), "bundle.js"); err != nil {
			b.Fatalf("bundle error: %v", err)
		}
	}
}
func BenchmarkV8_RunJS_Concat(b *testing.B) {
	poly := mustRead(b, "../v8/runtime/polyfill.bundle.js")
	bund := mustRead(b, "testdata/bundle.js")

	combined := append(poly, bund...)

	ctx := newV8Instance(b)
	defer ctx.Isolate().Dispose()

	for b.Loop() {
		if _, err := ctx.RunScript(string(combined), "polyfill_and_bundle.js"); err != nil {
			b.Fatalf("concat error: %v", err)
		}
	}
}
