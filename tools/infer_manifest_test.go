package tools

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

func TestInferManifestFile(t *testing.T) {
	tests := []struct {
		name         string
		createsFile  bool
		createsDir   bool
		inputString  string
		expectedPath string
		expectErr    bool
	}{
		{"input, valid manifest in pwd", true, false, "manifest.yaml", "manifest.yaml", false},
		{"input, invalid manifest in pwd", false, false, "substream.yaml", "", true},
		{"input, valid dir with manifest", true, true, "manifests", "manifest/substreams.yaml", false},
		{"input, invalid dir", false, false, "manifests", "", true},
		{"input, valid dir w/o manifest", false, true, "manifests", "", true},
		{"no input, has manifest", true, false, "", "substreams.yaml", false},
		{"no input, no manifest", false, false, "", "", true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var file *os.File
			var dir string
			var err error

			if test.createsDir {
				dir, _ = os.MkdirTemp("", test.inputString)

				if test.createsFile {
					file, _ = os.CreateTemp(dir, "substreams.yaml")
				}

				if file != nil {
					_, err = os.ReadFile(file.Name())
					if test.expectErr {
						assert.Error(t, err)
					} else {
						fmt.Printf("filepath; %s\n", file.Name())
						assert.NoError(t, err)
					}
				}
			} else if test.createsFile {

				if test.inputString == "" {
					file, _ = os.CreateTemp("", "substreams.yaml")
				} else {
					file, _ = os.CreateTemp("", test.inputString)
				}

				_, err = os.ReadFile(file.Name())
				if test.expectErr {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			} else {
				_, err := os.ReadFile("substreams.yaml")
				if test.expectErr {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			}
		})
	}
}
