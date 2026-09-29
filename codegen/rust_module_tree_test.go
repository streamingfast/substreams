package codegen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEscapeModIdent(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{"type", "r#type"},
		{"mod", "r#mod"},
		{"async", "r#async"},
		{"self", "self_"},
		{"Self", "Self_"},
		{"crate", "crate_"},
		{"super", "super_"},
		{"v1", "v1"},
		{"solana", "solana"},
	} {
		if got := escapeModIdent(tt.in); got != tt.want {
			t.Errorf("escapeModIdent(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// `sf.solana.type.v1` is a real package: `type` is a Rust keyword and has to be
// escaped or the file does not parse.
func TestWriteModuleTreeEscapesRustKeywords(t *testing.T) {
	out := filepath.Join(t.TempDir(), "src", "pb")
	touchModFiles(t, out, "sf.solana.type.v1", "a.mod.v1", "b.self.v1", "c.crate.v1", "d.async.v1")
	pkg := pkgWith(
		[2]string{"sf/solana/type/v1/account.proto", "sf.solana.type.v1"},
		[2]string{"a/mod/v1/a.proto", "a.mod.v1"},
		[2]string{"b/self/v1/b.proto", "b.self.v1"},
		[2]string{"c/crate/v1/c.proto", "c.crate.v1"},
		[2]string{"d/async/v1/d.proto", "d.async.v1"},
	)

	if err := NewProtoGenerator(out, nil, true).writeModuleTree(pkg); err != nil {
		t.Fatalf("writeModuleTree: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(out, "mod.rs"))
	if err != nil {
		t.Fatalf("reading mod.rs: %v", err)
	}
	for _, want := range []string{
		"pub mod r#type {",
		"pub mod r#mod {",
		"pub mod r#async {",
		"pub mod self_ {",
		"pub mod crate_ {",
	} {
		if !strings.Contains(string(got), want) {
			t.Errorf("expected %q, got:\n%s", want, got)
		}
	}
	// The include! path keeps the real package name; only the module ident is escaped.
	if !strings.Contains(string(got), `include!("sf.solana.type.v1.mod.rs")`) {
		t.Errorf("include path should keep the unescaped package name, got:\n%s", got)
	}
}
