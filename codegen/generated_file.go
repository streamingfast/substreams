package codegen

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// bufPlugin is one entry of a buf configuration's plugin list.
type bufPlugin struct {
	Plugin string `yaml:"plugin"`
	Remote string `yaml:"remote"`
	Local  any    `yaml:"local"`
	Name   string `yaml:"name"`
	Out    string `yaml:"out"`
}

// isBuffa reports whether this entry names the buffa plugin. Only the plugin reference
// decides it, so a comment mentioning buffa, or a path in some unrelated field, does not count.
func (p bufPlugin) isBuffa() bool {
	for _, ref := range []string{p.Plugin, p.Remote, p.Name, fmt.Sprint(p.Local)} {
		if strings.Contains(ref, "buffa") {
			return true
		}
	}
	return false
}

// readBufPlugins parses the plugin list out of the buf configuration at path. A configuration
// that cannot be read or parsed has no plugins, which callers read as "not ours to touch".
func readBufPlugins(path string) []bufPlugin {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var config struct {
		Plugins []bufPlugin `yaml:"plugins"`
	}
	if err := yaml.Unmarshal(content, &config); err != nil {
		return nil
	}
	return config.Plugins
}

// generatesBuffaOutput reports whether the buf configuration at path runs the buffa plugin.
//
// Only buffa's output needs a `mod.rs` written for it. prost's `prost-crate` plugin writes its
// own, so a project configured for prost gets none from us, and neither does one whose
// configuration cannot be read.
func generatesBuffaOutput(path string) bool {
	for _, plugin := range readBufPlugins(path) {
		if plugin.isBuffa() {
			return true
		}
	}
	return false
}

// buffaOutputPath returns the directory buffa is configured to generate into, empty if the
// configuration does not run buffa or does not say where its output goes.
//
// `buf` writes where the configuration tells it to, so that is where `mod.rs` belongs. Reading
// it keeps the two in step for a project whose generated code does not sit in `src/pb`.
func buffaOutputPath(path string) string {
	for _, plugin := range readBufPlugins(path) {
		if plugin.isBuffa() {
			return plugin.Out
		}
	}
	return ""
}

// generatedFile is a file this tool owns and may rewrite, identified by a marker on
// its first line.
//
// Ownership is explicit rather than assumed because the path is shared with the user:
// `src/pb/mod.rs` was hand-written in projects that predate its generation, and buf
// itself writes the sibling files. Replacing a file we did not write would discard
// someone's work, so a file without our marker is left alone.
type generatedFile struct {
	path   string
	marker string
}

func newGeneratedFile(path, marker string) *generatedFile {
	return &generatedFile{path: path, marker: marker}
}

// ownership describes what the tool is allowed to do with the path.
type ownership int

const (
	// ownershipAbsent means nothing is there, so writing it is safe.
	ownershipAbsent ownership = iota
	// ownershipOurs means the file carries our marker and may be replaced.
	ownershipOurs
	// ownershipForeign means the file is hand-written or produced by something else.
	ownershipForeign
)

// ownership reports whether the file may be replaced. An unreadable file is an error
// rather than a guess: silently treating it as foreign would skip a write the caller
// asked for, and treating it as ours would risk clobbering it.
func (f *generatedFile) ownership() (ownership, error) {
	content, err := os.ReadFile(f.path)
	if err != nil {
		if os.IsNotExist(err) {
			return ownershipAbsent, nil
		}
		return ownershipForeign, fmt.Errorf("reading %q: %w", f.path, err)
	}

	if f.hasMarker(content) {
		return ownershipOurs, nil
	}
	return ownershipForeign, nil
}

// hasMarker compares the first line, ignoring its line ending.
func (f *generatedFile) hasMarker(content []byte) bool {
	firstLine, _, _ := strings.Cut(string(content), "\n")
	return strings.TrimRight(firstLine, "\r") == f.marker
}

// write replaces the file with content, unless it belongs to someone else. It reports
// whether the file on disk changed, so callers can avoid touching the mtime and
// forcing a needless recompile.
func (f *generatedFile) write(content string) (changed bool, err error) {
	owner, err := f.ownership()
	if err != nil {
		return false, err
	}
	if owner == ownershipForeign {
		return false, nil
	}

	if existing, err := os.ReadFile(f.path); err == nil && string(existing) == content {
		return false, nil
	}

	if err := os.WriteFile(f.path, []byte(content), 0644); err != nil {
		return false, fmt.Errorf("writing %q: %w", f.path, err)
	}
	return true, nil
}
