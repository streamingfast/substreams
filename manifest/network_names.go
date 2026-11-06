package manifest

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	networks "github.com/streamingfast/firehose-networks"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

// NetworkNameWarnings reports the network names of the package that the Firehose network registry
// cannot turn into a single chain, either because it knows nothing about them or because the name
// is an alias shared by several networks. A package published with such a name cannot be mapped
// onto a real chain by its consumers.
//
// These are warnings and never errors: private and unlisted chains are legitimate.
func NetworkNameWarnings(pkg *pbsubstreams.Package) []string {
	return networkNameWarnings(pkg, func(name string) []string {
		var ids []string
		for _, network := range networks.GetSubstreamsRegistry().FindAll(name) {
			ids = append(ids, network.ID)
		}

		return ids
	})
}

// networkNameWarnings takes the registry lookup as a parameter, resolving a name to the registry IDs
// it matches, so that it can be exercised without depending on the registry's live content.
func networkNameWarnings(pkg *pbsubstreams.Package, lookup func(name string) []string) []string {
	names := map[string]bool{}
	if pkg.Network != "" {
		names[pkg.Network] = true
	}
	for name := range pkg.Networks {
		names[name] = true
	}

	var warnings []string
	for _, name := range slices.Sorted(maps.Keys(names)) {
		switch ids := lookup(name); len(ids) {
		case 1:
			// Resolves to exactly one network, an alias is as good as the registry ID itself here.
		case 0:
			warnings = append(warnings, fmt.Sprintf("network %q is not a known Firehose network registry ID or alias", name))
		default:
			slices.Sort(ids)
			warnings = append(warnings, fmt.Sprintf("network %q is ambiguous, it resolves to: %s — use a specific registry ID", name, strings.Join(ids, ", ")))
		}
	}

	return warnings
}
