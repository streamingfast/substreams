package tests_e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These build real Substreams projects: `buf` fetches the buffa plugin from the BSR
// and cargo compiles for wasm32. They cover `substreams build` writing `src/pb/mod.rs`,
// which no protoc plugin can do for us.

const modTreeManifest = `specVersion: v0.1.0
package:
  name: %NAME%
  version: v0.1.0
network: mainnet
protobuf:
  files:
%FILES%
  importPaths:
    - ./proto
  excludePaths:
    - google
    - sf/substreams
binaries:
  default:
    type: wasm/rust-v1
    file: ./target/wasm32-unknown-unknown/release/%NAME%.wasm
modules:
  - name: map_out
    kind: map
    initialBlock: 22000000
    inputs:
      - source: sf.ethereum.type.v2.Block
    output:
      type: proto:%OUTPUT%
`

const modTreeCargo = `[package]
name = "%NAME%"
version = "0.0.0"
edition = "2021"

[lib]
crate-type = ["cdylib"]

[dependencies]
substreams = "0.8.0-beta"
substreams-ethereum = "0.12.0-beta.1"
buffa = { version = "0.9", default-features = false, features = ["std", "fast-utf8"] }
buffa-types = { version = "0.9", default-features = false, features = ["std"] }

[profile.release]
lto = true
opt-level = 's'
strip = "debuginfo"
`

// These drive `buf` against the BSR and compile for wasm32, so they need the real
// toolchain. On a developer machine a missing tool skips, since that is an environment
// gap rather than a defect in the code under test. Where the environment is supposed to
// provide the toolchain, `SUBSTREAMS_E2E_REQUIRE_TOOLS=true` turns the same gap into a
// failure: a skip is invisible in a CI summary, so without it this suite can go green
// having run none of its tests.
func requireCodegenE2E(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping codegen end-to-end test in short mode")
	}
	requireTool(t, "buf")
	requireTool(t, "cargo")
	requireWasmTarget(t)
}

// toolsAreRequired reports whether a missing tool fails instead of skipping.
func toolsAreRequired() bool {
	return os.Getenv("SUBSTREAMS_E2E_REQUIRE_TOOLS") == "true"
}

// missingTool skips, or fails when the environment promised the toolchain.
func missingTool(t *testing.T, format string, args ...any) {
	t.Helper()
	if toolsAreRequired() {
		t.Fatalf(format+" (SUBSTREAMS_E2E_REQUIRE_TOOLS=true)", args...)
	}
	t.Skipf(format, args...)
}

func requireWasmTarget(t *testing.T) {
	t.Helper()
	out, err := exec.Command("rustup", "target", "list", "--installed").Output()
	if err != nil {
		missingTool(t, "could not list rustup targets: %v", err)
		return
	}
	if !strings.Contains(string(out), "wasm32-unknown-unknown") {
		missingTool(t, "wasm32-unknown-unknown target not installed")
	}
}

func requireTool(t *testing.T, name string) {
	t.Helper()
	if _, err := exec.LookPath(name); err != nil {
		missingTool(t, "%s not found in PATH", name)
	}
}

// buildSubstreamsCLI compiles the CLI from this checkout, so the test exercises the
// local code rather than whatever happens to be installed.
func buildSubstreamsCLI(t *testing.T) string {
	t.Helper()
	requireTool(t, "go")

	bin := filepath.Join(t.TempDir(), "substreams")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/substreams")
	cmd.Dir = ".."
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "building the substreams CLI: %s", out)

	return bin
}

type modTreeFile struct {
	path    string
	content string
}

func scaffoldProject(t *testing.T, name, output string, protoFiles []string, files ...modTreeFile) string {
	t.Helper()
	dir := t.TempDir()

	var fileLines strings.Builder
	for _, f := range protoFiles {
		fileLines.WriteString("    - " + f + "\n")
	}

	manifest := strings.NewReplacer(
		"%NAME%", name,
		"%FILES%", strings.TrimRight(fileLines.String(), "\n"),
		"%OUTPUT%", output,
	).Replace(modTreeManifest)

	all := append([]modTreeFile{
		{"substreams.yaml", manifest},
		{"Cargo.toml", strings.ReplaceAll(modTreeCargo, "%NAME%", name)},
	}, files...)

	for _, f := range all {
		full := filepath.Join(dir, f.path)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0755))
		require.NoError(t, os.WriteFile(full, []byte(f.content), 0644))
	}

	return dir
}

func runSubstreamsBuild(t *testing.T, bin, dir string) (string, error) {
	t.Helper()
	cmd := exec.Command(bin, "build")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestCodegenBuildGeneratesModuleTree(t *testing.T) {
	requireCodegenE2E(t)
	bin := buildSubstreamsCLI(t)

	dir := scaffoldProject(t, "modtree1", "mydata.v1.Events",
		[]string{"mydata/v1/events.proto"},
		modTreeFile{"proto/mydata/v1/events.proto", `syntax = "proto3";
package mydata.v1;
message Events { repeated Event events = 1; }
message Event { string id = 1; uint64 block = 2; }
`},
		modTreeFile{"src/lib.rs", `mod pb;
use pb::mydata::v1::{Event, Events};
use substreams_ethereum::pb::eth::v2 as eth;

#[substreams::handlers::map]
fn map_out(blk: eth::Block) -> Result<Events, substreams::errors::Error> {
    Ok(Events {
        events: vec![Event { id: format!("{}", blk.number), block: blk.number, ..Default::default() }],
        ..Default::default()
    })
}
`},
	)

	out, err := runSubstreamsBuild(t, bin, dir)
	require.NoError(t, err, "substreams build: %s", out)

	bufGen, err := os.ReadFile(filepath.Join(dir, "buf.gen.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(bufGen), "buf.build/anthropics/buffa", "the default buf.gen.yaml must select buffa")
	assert.NotContains(t, string(bufGen), "prost", "the default buf.gen.yaml must not reference prost")

	modRS, err := os.ReadFile(filepath.Join(dir, "src", "pb", "mod.rs"))
	require.NoError(t, err)
	assert.Contains(t, string(modRS), `include!("mydata.v1.mod.rs")`)

	assert.FileExists(t, filepath.Join(dir, "modtree1-v0.1.0.spkg"))
	assert.FileExists(t, filepath.Join(dir, "target", "wasm32-unknown-unknown", "release", "modtree1.wasm"))
}

// Guards the 71 chain-modules and the skills examples, which all commit a
// hand-maintained mod.rs that regeneration must not touch.
func TestCodegenBuildPreservesHandWrittenModuleTree(t *testing.T) {
	requireCodegenE2E(t)
	bin := buildSubstreamsCLI(t)

	handWritten := `#[allow(clippy::all)]
// deliberately hand-maintained
pub mod mydata {
    pub mod v1 {
        include!("mydata.v1.mod.rs");
    }
}
`

	dir := scaffoldProject(t, "modtree2", "mydata.v1.Events",
		[]string{"mydata/v1/events.proto"},
		modTreeFile{"proto/mydata/v1/events.proto", `syntax = "proto3";
package mydata.v1;
message Events { repeated Event events = 1; }
message Event { string id = 1; }
`},
		modTreeFile{"src/pb/mod.rs", handWritten},
		modTreeFile{"src/lib.rs", `mod pb;
use pb::mydata::v1::{Event, Events};
use substreams_ethereum::pb::eth::v2 as eth;

#[substreams::handlers::map]
fn map_out(blk: eth::Block) -> Result<Events, substreams::errors::Error> {
    Ok(Events {
        events: vec![Event { id: format!("{}", blk.number), ..Default::default() }],
        ..Default::default()
    })
}
`},
	)

	out, err := runSubstreamsBuild(t, bin, dir)
	require.NoError(t, err, "substreams build: %s", out)

	got, err := os.ReadFile(filepath.Join(dir, "src", "pb", "mod.rs"))
	require.NoError(t, err)
	assert.Equal(t, handWritten, string(got), "a hand-written mod.rs must survive regeneration")
}

// With the protos unchanged the hash cache short-circuits generation, but a missing
// module tree still has to come back or the Rust compile fails.
func TestCodegenBuildRestoresModuleTreeOnCacheHit(t *testing.T) {
	requireCodegenE2E(t)
	bin := buildSubstreamsCLI(t)

	dir := scaffoldProject(t, "modtree3", "mydata.v1.Events",
		[]string{"mydata/v1/events.proto"},
		modTreeFile{"proto/mydata/v1/events.proto", `syntax = "proto3";
package mydata.v1;
message Events { repeated Event events = 1; }
message Event { string id = 1; }
`},
		modTreeFile{"src/lib.rs", `mod pb;
use pb::mydata::v1::{Event, Events};
use substreams_ethereum::pb::eth::v2 as eth;

#[substreams::handlers::map]
fn map_out(blk: eth::Block) -> Result<Events, substreams::errors::Error> {
    Ok(Events {
        events: vec![Event { id: format!("{}", blk.number), ..Default::default() }],
        ..Default::default()
    })
}
`},
	)

	out, err := runSubstreamsBuild(t, bin, dir)
	require.NoError(t, err, "first substreams build: %s", out)

	modPath := filepath.Join(dir, "src", "pb", "mod.rs")
	require.NoError(t, os.Remove(modPath))

	out, err = runSubstreamsBuild(t, bin, dir)
	require.NoError(t, err, "second substreams build: %s", out)
	assert.Contains(t, out, "skipped (no changes detected)", "expected the hash cache to short-circuit generation")
	assert.FileExists(t, modPath, "the module tree must be restored even when generation is skipped")
}

// Packages sharing a prefix are where the nesting logic is easiest to get wrong.
func TestCodegenBuildNestsSiblingPackages(t *testing.T) {
	requireCodegenE2E(t)
	bin := buildSubstreamsCLI(t)

	dir := scaffoldProject(t, "modtree4", "solo.Out",
		[]string{"a/b/c.proto", "a/b/d.proto", "solo/solo.proto"},
		modTreeFile{"proto/a/b/c.proto", "syntax = \"proto3\";\npackage a.b.c;\nmessage Cc { string x = 1; }\n"},
		modTreeFile{"proto/a/b/d.proto", "syntax = \"proto3\";\npackage a.b.d;\nmessage Dd { string x = 1; }\n"},
		modTreeFile{"proto/solo/solo.proto", "syntax = \"proto3\";\npackage solo;\nmessage Out { string x = 1; }\n"},
		modTreeFile{"src/lib.rs", `mod pb;
use pb::a::b::c::Cc;
use pb::a::b::d::Dd;
use pb::solo::Out;
use substreams_ethereum::pb::eth::v2 as eth;

#[substreams::handlers::map]
fn map_out(blk: eth::Block) -> Result<Out, substreams::errors::Error> {
    let _c = Cc { x: "c".into(), ..Default::default() };
    let _d = Dd { x: "d".into(), ..Default::default() };
    Ok(Out { x: format!("{}", blk.number), ..Default::default() })
}
`},
	)

	out, err := runSubstreamsBuild(t, bin, dir)
	require.NoError(t, err, "substreams build: %s", out)

	got, err := os.ReadFile(filepath.Join(dir, "src", "pb", "mod.rs"))
	require.NoError(t, err)

	// Siblings share their common ancestor, and a single-segment package sits at the
	// root. Packages buf pulled in transitively are present too, so this asserts the
	// nesting rather than the exact set.
	assert.Contains(t, string(got), `pub mod a {
    pub mod b {
        pub mod c {
            include!("a.b.c.mod.rs");
        }
        pub mod d {
            include!("a.b.d.mod.rs");
        }
    }
}
`)
	assert.Contains(t, string(got), `pub mod solo {
    include!("solo.mod.rs");
}
`)
}

// A proto that references a type from another package is the case the module tree
// exists for: buffa emits a relative `super::…::other_pkg` path, so every emitted
// package has to be wired up or the crate does not compile.
func TestCodegenBuildWiresUpImportedPackages(t *testing.T) {
	requireCodegenE2E(t)
	bin := buildSubstreamsCLI(t)

	dir := scaffoldProject(t, "modtree5", "mydata.v1.Events",
		[]string{"mydata/v1/events.proto"},
		modTreeFile{"proto/mydata/v1/events.proto", `syntax = "proto3";
package mydata.v1;
import "google/protobuf/timestamp.proto";
message Events {
  google.protobuf.Timestamp at = 1;
  repeated Event events = 2;
}
message Event { string id = 1; }
`},
		modTreeFile{"src/lib.rs", `mod pb;
use pb::mydata::v1::{Event, Events};
use substreams_ethereum::pb::eth::v2 as eth;

#[substreams::handlers::map]
fn map_out(blk: eth::Block) -> Result<Events, substreams::errors::Error> {
    Ok(Events {
        events: vec![Event { id: format!("{}", blk.number), ..Default::default() }],
        ..Default::default()
    })
}
`},
	)

	// google is deliberately not excluded, so buffa generates it locally and refers
	// to it relatively rather than through ::buffa_types.
	manifestPath := filepath.Join(dir, "substreams.yaml")
	manifest, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(manifestPath,
		[]byte(strings.Replace(string(manifest), "    - google\n", "", 1)), 0644))

	out, err := runSubstreamsBuild(t, bin, dir)
	require.NoError(t, err, "substreams build: %s", out)

	modRS, err := os.ReadFile(filepath.Join(dir, "src", "pb", "mod.rs"))
	require.NoError(t, err)
	assert.Contains(t, string(modRS), "google.protobuf.mod.rs", "an imported package must be wired up")

	assert.FileExists(t, filepath.Join(dir, "modtree5-v0.1.0.spkg"))
}

// A prost-crate generated mod.rs carries a bare `// @generated`, not our header, so
// it is treated as hand-maintained and left alone.
func TestCodegenBuildLeavesProstCrateModuleTreeAlone(t *testing.T) {
	requireCodegenE2E(t)
	bin := buildSubstreamsCLI(t)

	prostCrate := `// @generated
// @@protoc_insertion_point(attribute:mydata.v1)
pub mod mydata {
    pub mod v1 {
        include!("mydata.v1.mod.rs");
    }
}
`

	dir := scaffoldProject(t, "modtree6", "mydata.v1.Events",
		[]string{"mydata/v1/events.proto"},
		modTreeFile{"proto/mydata/v1/events.proto", `syntax = "proto3";
package mydata.v1;
message Events { repeated Event events = 1; }
message Event { string id = 1; }
`},
		modTreeFile{"src/pb/mod.rs", prostCrate},
		modTreeFile{"src/lib.rs", `mod pb;
use pb::mydata::v1::{Event, Events};
use substreams_ethereum::pb::eth::v2 as eth;

#[substreams::handlers::map]
fn map_out(blk: eth::Block) -> Result<Events, substreams::errors::Error> {
    Ok(Events {
        events: vec![Event { id: format!("{}", blk.number), ..Default::default() }],
        ..Default::default()
    })
}
`},
	)

	out, err := runSubstreamsBuild(t, bin, dir)
	require.NoError(t, err, "substreams build: %s", out)

	got, err := os.ReadFile(filepath.Join(dir, "src", "pb", "mod.rs"))
	require.NoError(t, err)
	assert.Equal(t, prostCrate, string(got), "a prost-crate mod.rs must survive regeneration")
}
