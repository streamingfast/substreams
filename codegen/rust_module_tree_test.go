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

// The escaping has to agree with buffa's `escape_mod_ident`
// (buffa-codegen/src/idents.rs): buffa refers to a type in another package through a
// relative `super::...::other_pkg` path, so a segment the two escape differently
// resolves to a module that does not exist. This pins the table against the language
// rather than against buffa's source, which is not available here.
func TestRustKeywordsCoversEveryReservedWord(t *testing.T) {
	// Every strict, reserved and edition-specific keyword, from the Rust reference.
	// `gen` is reserved in edition 2024 and `try` in 2018, so both belong here.
	reserved := []string{
		"as", "break", "const", "continue", "crate", "dyn", "else", "enum", "extern",
		"false", "fn", "for", "if", "impl", "in", "let", "loop", "match", "mod",
		"move", "mut", "pub", "ref", "return", "self", "Self", "static", "struct",
		"super", "trait", "true", "type", "unsafe", "use", "where", "while",
		"async", "await", "abstract", "become", "box", "do", "final", "macro",
		"override", "priv", "typeof", "unsized", "virtual", "yield", "try", "gen",
	}

	for _, word := range reserved {
		if !rustKeywords[word] {
			t.Errorf("rustKeywords is missing the reserved word %q", word)
		}
	}

	if len(rustKeywords) != len(reserved) {
		for word := range rustKeywords {
			var found bool
			for _, r := range reserved {
				if r == word {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("rustKeywords contains %q, which is not a Rust keyword", word)
			}
		}
	}
}

// Rust rejects these four as `r#name`, so they take a trailing underscore instead.
func TestCannotBeRawIdentMatchesRust(t *testing.T) {
	want := map[string]bool{"crate": true, "self": true, "Self": true, "super": true}

	for word := range want {
		if !cannotBeRawIdent[word] {
			t.Errorf("cannotBeRawIdent is missing %q", word)
		}
		if !rustKeywords[word] {
			t.Errorf("%q is in cannotBeRawIdent but not in rustKeywords", word)
		}
	}
	for word := range cannotBeRawIdent {
		if !want[word] {
			t.Errorf("cannotBeRawIdent contains %q, which Rust accepts as a raw identifier", word)
		}
	}
}
