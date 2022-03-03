package manifest

import (
	"fmt"
	"io/ioutil"
	"strings"

	"gopkg.in/yaml.v3"
)

func DecodeYamlManifestFromFile(yamlFilePath string) (string, *Manifest, error) {
	yamlContent, err := ioutil.ReadFile(yamlFilePath)
	if err != nil {
		return "", nil, fmt.Errorf("reading substreams file %q: %w", yamlFilePath, err)
	}

	substreamsManifest, err := DecodeYamlManifest(string(yamlContent))
	if err != nil {
		return "", nil, fmt.Errorf("decoding substreams file %q: %w", yamlFilePath, err)
	}

	return string(yamlContent), substreamsManifest, nil
}

func DecodeYamlManifest(manifestContent string) (*Manifest, error) {
	var substreamsManifest *Manifest
	if err := yaml.NewDecoder(strings.NewReader(manifestContent)).Decode(&substreamsManifest); err != nil {
		return nil, fmt.Errorf("decoding manifest content %q: %w", manifestContent, err)
	}

	return substreamsManifest, nil
}
