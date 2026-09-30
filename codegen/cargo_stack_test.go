package codegen

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectProtobufStack(t *testing.T) {
	for _, tt := range []struct {
		name     string
		manifest string
		want     protobufStack
	}{
		{
			name:     "buffa",
			manifest: "[dependencies]\nbuffa = \"0.9.2\"\nsubstreams = \"0.8.0-beta\"\n",
			want:     stackBuffa,
		},
		{
			name:     "prost",
			manifest: "[dependencies]\nprost = \"0.13\"\nsubstreams = \"0.7\"\n",
			want:     stackProst,
		},
		{
			// Real projects write this; the version is inherited but the name is still here.
			name:     "buffa inherited from the workspace",
			manifest: "[dependencies]\nbuffa = { workspace = true }\n",
			want:     stackBuffa,
		},
		{
			name:     "prost inherited from the workspace",
			manifest: "[dependencies]\nprost = { workspace = true }\n",
			want:     stackProst,
		},
		{
			name:     "prost as a build dependency",
			manifest: "[build-dependencies]\nprost = \"0.13\"\n",
			want:     stackProst,
		},
		{
			name:     "buffa as a dev dependency",
			manifest: "[dev-dependencies]\nbuffa = \"0.9.2\"\n",
			want:     stackBuffa,
		},
		{
			// Mid-migration manifests name both; buffa is what the new bindings must match.
			name:     "both, buffa wins",
			manifest: "[dependencies]\nbuffa = \"0.9.2\"\nprost = \"0.13\"\n",
			want:     stackBuffa,
		},
		{
			name:     "neither",
			manifest: "[dependencies]\nsubstreams = \"0.7\"\n",
			want:     stackUnknown,
		},
		{
			// prost-types alone does not decide it; prost itself is the marker.
			name:     "prost-types only",
			manifest: "[dependencies]\nprost-types = \"0.13\"\n",
			want:     stackUnknown,
		},
		{
			name:     "not valid toml",
			manifest: "[dependencies\nprost =",
			want:     stackUnknown,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "Cargo.toml")
			if err := os.WriteFile(path, []byte(tt.manifest), 0644); err != nil {
				t.Fatalf("writing manifest: %v", err)
			}

			if got := detectProtobufStack(path); got != tt.want {
				t.Errorf("detectProtobufStack = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDetectProtobufStackWithoutAManifest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Cargo.toml")

	if got := detectProtobufStack(path); got != stackUnknown {
		t.Errorf("detectProtobufStack = %v, want stackUnknown", got)
	}
}

// The manifest and the buf configuration are read from the project directory, which is not
// necessarily the working directory: `substreams protogen path/to/project/substreams.yaml`
// runs from anywhere.
func TestProjectPathResolvesAgainstTheProjectNotTheWorkingDirectory(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "Cargo.toml"), []byte("[dependencies]\nprost = \"0.13\"\n"), 0644); err != nil {
		t.Fatalf("writing the project manifest: %v", err)
	}

	// A different manifest in the working directory must not be the one consulted.
	elsewhere := t.TempDir()
	if err := os.WriteFile(filepath.Join(elsewhere, "Cargo.toml"), []byte("[dependencies]\nbuffa = \"0.9.2\"\n"), 0644); err != nil {
		t.Fatalf("writing the decoy manifest: %v", err)
	}
	t.Chdir(elsewhere)

	generator := NewProtoGenerator(filepath.Join(project, "src", "pb"), nil, true)
	generator.SetProjectPath(project)

	if got := detectProtobufStack(generator.projectFile("Cargo.toml")); got != stackProst {
		t.Errorf("detectProtobufStack = %v, want stackProst (read the project manifest, not the one in cwd)", got)
	}
}

// With no project path set, the working directory is the project.
func TestProjectPathDefaultsToTheWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("[dependencies]\nbuffa = \"0.9.2\"\n"), 0644); err != nil {
		t.Fatalf("writing manifest: %v", err)
	}
	t.Chdir(dir)

	generator := NewProtoGenerator("src/pb", nil, true)

	if got := detectProtobufStack(generator.projectFile("Cargo.toml")); got != stackBuffa {
		t.Errorf("detectProtobufStack = %v, want stackBuffa", got)
	}
}

// `buf` resolves `buf.gen.yaml` and the `out:` inside it against its own working directory, so
// the configuration, the output and `buf` itself all have to share the project as their base.
// Running `substreams build` from a subdirectory, which findManifest supports, otherwise wrote a
// configuration that pointed somewhere else.
func TestOutputPathIsRelativeToTheProject(t *testing.T) {
	project := t.TempDir()

	for _, tt := range []struct{ name, given, want string }{
		{"relative stays as given", "src/pb", "src/pb"},
		{"absolute is rebased onto the project", filepath.Join(project, "src", "pb"), filepath.Join("src", "pb")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// A working directory that is neither the project nor its parent.
			t.Chdir(t.TempDir())

			generator := NewProtoGenerator(tt.given, nil, true)
			generator.SetProjectPath(project)

			if generator.outputPath != tt.want {
				t.Errorf("outputPath = %q, want %q", generator.outputPath, tt.want)
			}
			// What this process opens must resolve inside the project.
			if want := filepath.Join(project, tt.want); generator.outputDir() != want {
				t.Errorf("outputDir() = %q, want %q", generator.outputDir(), want)
			}
		})
	}
}

// A workspace keeps its dependency list at the root, so the manifest that decides the stack is
// often several directories above the project.
func TestFindCargoManifestWalksUp(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "crates", "my-project")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	rootManifest := filepath.Join(root, "Cargo.toml")
	if err := os.WriteFile(rootManifest, []byte("[dependencies]\nbuffa = \"0.9\"\n"), 0644); err != nil {
		t.Fatalf("writing manifest: %v", err)
	}

	if got := findCargoManifest(nested); got != rootManifest {
		t.Errorf("findCargoManifest = %q, want %q", got, rootManifest)
	}
}

// The nearest manifest wins: a crate declaring its own dependencies is not governed by the
// workspace root above it.
func TestFindCargoManifestPrefersTheNearest(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "crates", "my-project")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	for _, dir := range []string{root, nested} {
		if err := os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("[dependencies]\n"), 0644); err != nil {
			t.Fatalf("writing manifest: %v", err)
		}
	}

	want := filepath.Join(nested, "Cargo.toml")
	if got := findCargoManifest(nested); got != want {
		t.Errorf("findCargoManifest = %q, want %q", got, want)
	}
}

// With no manifest anywhere above, the path returned is the one beside the project, so the
// caller reads a missing file and falls back to prost rather than searching the filesystem root.
func TestFindCargoManifestWithoutAnyManifest(t *testing.T) {
	dir := t.TempDir()

	want := filepath.Join(dir, "Cargo.toml")
	if got := findCargoManifest(dir); got != want {
		t.Errorf("findCargoManifest = %q, want %q", got, want)
	}
	if detectProtobufStack(findCargoManifest(dir)) != stackUnknown {
		t.Error("a missing manifest should not name a stack")
	}
}
