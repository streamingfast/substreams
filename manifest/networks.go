package manifest

import (
	"fmt"
	"strings"

	"github.com/schollz/closestmatch"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func toNetworkParams(params *pbsubstreams.NetworkParams) *NetworkParams {
	return &NetworkParams{
		Params:        params.Params,
		InitialBlocks: params.InitialBlocks,
	}
}

func ApplyNetwork(network string, pkg *pbsubstreams.Package) error {
	if len(pkg.Networks) == 0 {
		return nil
	}
	var netParams *NetworkParams
	for k, v := range pkg.Networks {
		if k != network {
			continue
		}
		netParams = toNetworkParams(v)
	}
	if netParams == nil {
		return fmt.Errorf("cannot apply network %q: not found in manifest", network)
	}

	if err := ApplyParams(netParams.Params, pkg); err != nil {
		return fmt.Errorf("cannot apply params for network %s: %w", network, err)
	}

	for mod, block := range netParams.InitialBlocks {
		var found bool
		var closest []string
		for _, module := range pkg.Modules.Modules {
			closest = append(closest, module.Name)
			if module.Name == mod {
				module.InitialBlock = block
			}
			found = true
		}
		if !found {
			closeEnough := closestmatch.New(closest, []int{2}).Closest(mod)
			return fmt.Errorf("param for module %q: module not found, did you mean %q ?", mod, closeEnough)
		}

	}

	return nil
}

// validateNetworks checks that network overloads have the same keys for initialBlocks and params for modules that are owned by the package
func validateNetworks(pkg *pbsubstreams.Package, includeImportedModules map[string]bool, overrideNetwork string) error {
	if pkg.Networks == nil {
		return nil
	}

	network := pkg.Network
	if overrideNetwork != "" {
		network = overrideNetwork
	}
	seenPackagesInitialBlocks := make(map[string]bool)
	seenPackagesParams := make(map[string]bool)

	networksContainingLocalModules := make(map[string]*pbsubstreams.NetworkParams)
networkLoop:
	for name, nw := range pkg.Networks {
		if name == network { // always consider the current network as containing local modules
			networksContainingLocalModules[name] = nw
			continue networkLoop
		}
		for k := range nw.InitialBlocks {
			if !strings.Contains(k, ":") {
				networksContainingLocalModules[name] = nw
				continue networkLoop
			}
		}
		for k := range nw.InitialBlocks {
			if !strings.Contains(k, ":") {
				networksContainingLocalModules[name] = nw
				continue networkLoop
			}
			seenPackagesInitialBlocks[k] = true
		}
	}
	if network != "" && networksContainingLocalModules[network] == nil {
		networksContainingLocalModules[network] = &pbsubstreams.NetworkParams{}
	}

	var firstNetwork string
	for name, nw := range networksContainingLocalModules {
		if firstNetwork == "" {
			for k := range nw.InitialBlocks {
				if strings.Contains(k, ":") && !includeImportedModules[k] {
					continue // skip modules that are not owned by the package
				}
				seenPackagesInitialBlocks[k] = true
			}
			for k := range nw.Params {
				if strings.Contains(k, ":") && !includeImportedModules[k] {
					continue // skip modules that are not owned by the package
				}
				seenPackagesParams[k] = true
			}
			firstNetwork = name
			continue
		}

		for k := range nw.InitialBlocks {
			if strings.Contains(k, ":") && !includeImportedModules[k] {
				continue // skip modules that are not owned by the package
			}
			if !seenPackagesInitialBlocks[k] {
				return fmt.Errorf("missing 'initialBlock' value for module %q in network %s", k, firstNetwork)
			}
		}
		for k := range nw.Params {
			if strings.Contains(k, ":") && !includeImportedModules[k] {
				continue // skip modules that are not owned by the package
			}
			if !seenPackagesParams[k] {
				return fmt.Errorf("missing 'params' value for module %q in network %s", k, firstNetwork)
			}
		}

		for k := range seenPackagesInitialBlocks {
			if _, ok := nw.InitialBlocks[k]; !ok {
				return fmt.Errorf("missing 'initialBlock' value for module %q in network %s", k, name)
			}
		}
		for k := range seenPackagesParams {
			if _, ok := nw.Params[k]; !ok {
				return fmt.Errorf("missing 'params' value for module %q in network %s", k, name)
			}
		}

	}

	return nil

}

func mergeNetwork(src, dest *pbsubstreams.NetworkParams, srcPrefix string) {
	if dest == nil {
		panic("mergeNetwork should never be called with nil dest")
	}
	if src == nil {
		return
	}

	if src.InitialBlocks != nil {
		if dest.InitialBlocks == nil {
			dest.InitialBlocks = make(map[string]uint64)
		}
		for kk, vv := range src.InitialBlocks {
			newKey := withPrefix(kk, srcPrefix)
			if _, ok := dest.InitialBlocks[newKey]; !ok {
				dest.InitialBlocks[newKey] = vv
			}
		}
	}

	if src.Params != nil {
		if dest.Params == nil {
			dest.Params = make(map[string]string)
		}
		for kk, vv := range src.Params {
			newKey := withPrefix(kk, srcPrefix)
			if _, ok := dest.Params[newKey]; !ok {
				dest.Params[newKey] = vv
			}
		}
	}
}

func mergeNetworks(src, dest *pbsubstreams.Package, srcPrefix string) {
	if src.Networks == nil {
		return
	}

	if dest.Networks == nil {
		dest.Networks = make(map[string]*pbsubstreams.NetworkParams)
		for k, srcNet := range src.Networks {
			destNet := &pbsubstreams.NetworkParams{}
			mergeNetwork(srcNet, destNet, srcPrefix)
			dest.Networks[k] = destNet
		}
		return
	}

	allKeys := make(map[string]bool)

	for k := range dest.Networks {
		allKeys[k] = true
	}
	for k := range src.Networks {
		allKeys[k] = true
	}

	for k := range allKeys {
		destNet := dest.Networks[k]
		if destNet == nil {
			destNet = &pbsubstreams.NetworkParams{}
			dest.Networks[k] = destNet
		}
		mergeNetwork(src.Networks[k], destNet, srcPrefix)
	}
}
