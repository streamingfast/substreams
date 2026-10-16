package codegen

import (
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

// protobufStack is the protobuf implementation a project's Rust code is written against.
type protobufStack int

const (
	// stackUnknown means the project does not name either implementation, so there is
	// nothing to infer from.
	stackUnknown protobufStack = iota
	stackBuffa
	stackProst
)

// cargoManifest is the part of a `Cargo.toml` that names the protobuf implementation. A
// dependency inherited from the workspace still appears here by name, which is what makes
// reading only the member manifest enough.
type cargoManifest struct {
	Dependencies      map[string]any `toml:"dependencies"`
	DevDependencies   map[string]any `toml:"dev-dependencies"`
	BuildDependencies map[string]any `toml:"build-dependencies"`
}

// findCargoManifest returns the nearest `Cargo.toml` at or above dir.
//
// A manifest does not always sit beside the substreams manifest: a workspace keeps the
// dependency list at its root, and a project can hold several manifests in subdirectories.
func findCargoManifest(dir string) string {
	current, err := filepath.Abs(dir)
	if err != nil {
		return filepath.Join(dir, "Cargo.toml")
	}

	for {
		candidate := filepath.Join(current, "Cargo.toml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}

		if isRepositoryRoot(current) {
			return filepath.Join(dir, "Cargo.toml")
		}

		parent := filepath.Dir(current)
		if parent == current {
			return filepath.Join(dir, "Cargo.toml")
		}
		current = parent
	}
}

// detectProtobufStack reports which protobuf implementation the manifest at path depends on.
//
// A project generating buffa bindings while its code is written against prost does not
// compile, so the manifest decides which plugin to configure when the project has no
// `buf.gen.yaml` of its own.
func detectProtobufStack(path string) protobufStack {
	content, err := os.ReadFile(path)
	if err != nil {
		return stackUnknown
	}

	var manifest cargoManifest
	if err := toml.Unmarshal(content, &manifest); err != nil {
		return stackUnknown
	}

	for _, deps := range []map[string]any{manifest.Dependencies, manifest.DevDependencies, manifest.BuildDependencies} {
		if _, ok := deps["buffa"]; ok {
			return stackBuffa
		}
	}
	for _, deps := range []map[string]any{manifest.Dependencies, manifest.DevDependencies, manifest.BuildDependencies} {
		if _, ok := deps["prost"]; ok {
			return stackProst
		}
	}
	return stackUnknown
}
