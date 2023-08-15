package manifest

import "path/filepath"

type ManifestOverrideConfiguration struct {
	Package PackageMetaOverride `yaml:"package"`

	Network       string            `yaml:"network"`
	InitialBlocks map[string]uint64 `yaml:"initialBlocks"`
	Params        map[string]string `yaml:"params"`

	Workdir string `yaml:"-"`
}

func (m *ManifestOverrideConfiguration) resolvePath(path string) string {
	if m.Workdir == "" || filepath.IsAbs(path) || httpSchemePrefixRegex.MatchString(path) {
		return path
	}

	return filepath.Join(m.Workdir, path)
}

func (o *ManifestOverrideConfiguration) Apply(m *Manifest) error {
	//recaluclate initial block graph?

	return nil
}

type PackageMetaOverride struct {
	Name string `yaml:"name"`
	//Version string `yaml:"version"` // Semver for package authors
	//URL     string `yaml:"url"`
	//Doc     string `yaml:"doc"`
}
