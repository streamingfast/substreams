package sink

import (
	"fmt"
	"strings"

	"github.com/bobg/go-generics/v2/slices"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap"
)

// ReadManifestAndModule reads the manifest and returns the package, the output module and its hash.
//
// If outputModuleName is set to InferOutputModuleFromPackage, the sink will try to infer the output module from the
// package's sink_module field, if present.
//
// If expectedOutputModuleType is set to IgnoreOutputModuleType, the sink will not validate the output module type.
//
// If skipPackageValidation is set to true, the sink will not validate the package, you will have to do it yourself.
func ReadManifestAndModule(
	manifestPath string,
	network string,
	paramsStrings []string,
	outputModuleName string,
	expectedOutputModuleType string,
	skipPackageValidation bool,
	zlog *zap.Logger,
) (
	pkg *pbsubstreams.Package,
	module *pbsubstreams.Module,
	outputModuleHash manifest.ModuleHash,
	err error,
) {
	zlog.Info("reading substreams manifest", zap.String("manifest_path", manifestPath))

	var opts []manifest.Option
	if skipPackageValidation {
		opts = append(opts, manifest.SkipPackageValidationReader())
	}
	opts = append(opts, manifest.WithOverrideNetwork(network))
	params, err := manifest.ParseParams(paramsStrings)
	if err != nil {
		return nil, nil, nil, err
	}
	opts = append(opts, manifest.WithParams(params))

	reader, err := manifest.NewReader(manifestPath, opts...)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("manifest reader %q: %w", manifestPath, err)
	}

	pkgBundle, err := reader.Read()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read manifest %q: %w", manifestPath, err)
	}

	pkg = pkgBundle.Package

	resolvedOutputModuleName := outputModuleName
	if resolvedOutputModuleName == InferOutputModuleFromPackage {
		zlog.Debug("inferring module output name from package directly")

		if pkg.SinkModule == "" {
			return nil, nil, nil, fmt.Errorf("sink module is required in sink config")
		}

		resolvedOutputModuleName = pkg.SinkModule
	}

	zlog.Info("finding output module", zap.String("module_name", resolvedOutputModuleName))
	module, err = pkgBundle.Graph.Module(resolvedOutputModuleName)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("get output module %q: %w", resolvedOutputModuleName, err)
	}
	if module.GetKindStore() != nil {
		return nil, nil, nil, fmt.Errorf("ouput module %q is of type 'Store'", resolvedOutputModuleName)
	}

	zlog.Info("validating output module type", zap.String("module_name", module.Name), zap.String("module_type", module.Output.Type))

	if expectedOutputModuleType != IgnoreOutputModuleType && expectedOutputModuleType != "" {
		unprefixedExpectedTypes, prefixedExpectedTypes := sanitizeModuleTypes(expectedOutputModuleType)
		unprefixedActualType, prefixedActualType := sanitizeModuleType(module.Output.Type)

		if !slices.Contains(prefixedExpectedTypes, prefixedActualType) {
			return nil, nil, nil, fmt.Errorf("sink only supports map module with output type %q but selected module %q output type is %q", strings.Join(unprefixedExpectedTypes, ", "), module.Name, unprefixedActualType)
		}
	}

	hashes := manifest.NewModuleHashes()
	outputModuleHash, err = hashes.HashModule(pkg.Modules, module, pkgBundle.Graph)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("hash module %q: %w", module.Name, err)
	}

	return pkg, module, outputModuleHash, nil
}

// ReadManifestAndModuleAndBlockRange acts exactly like ReadManifestAndModule but also reads the block range.
func ReadManifestAndModuleAndBlockRange(
	manifestPath string,
	network string,
	params []string,
	outputModuleName string,
	expectedOutputModuleType string,
	skipPackageValidation bool,
	blockRange string,
	zlog *zap.Logger,
) (
	pkg *pbsubstreams.Package,
	module *pbsubstreams.Module,
	outputModuleHash manifest.ModuleHash,
	resolvedBlockRange *bstream.Range,
	err error,
) {
	pkg, module, outputModuleHash, err = ReadManifestAndModule(manifestPath, network, params, outputModuleName, expectedOutputModuleType, skipPackageValidation, zlog)
	if err != nil {
		err = fmt.Errorf("read manifest and module: %w", err)
		return
	}

	resolvedBlockRange, err = ReadBlockRange(module, blockRange)
	if err != nil {
		err = fmt.Errorf("resolve block range: %w", err)
		return
	}

	return
}

// sanitizeModuleTypes has the same behavior as sanitizeModuleType but explodes
// the inpput string on comma and returns a slice of unprefixed and prefixed
// types for each of the input types.
func sanitizeModuleTypes(in string) (unprefixed, prefixed []string) {
	slices.Each(strings.Split(in, ","), func(in string) {
		unprefixedType, prefixedType := sanitizeModuleType(strings.TrimSpace(in))
		unprefixed = append(unprefixed, unprefixedType)
		prefixed = append(prefixed, prefixedType)
	})

	return
}

// sanitizeModuleType give back both prefixed (so with `proto:`) and unprefixed
// version of the input string:
//
// - `sanitizeModuleType("com.acme") == (com.acme, proto:com.acme)`
// - `sanitizeModuleType("proto:com.acme") == (com.acme, proto:com.acme)`
func sanitizeModuleType(in string) (unprefixed, prefixed string) {
	if strings.HasPrefix(in, "proto:") {
		return strings.TrimPrefix(in, "proto:"), in
	}

	return in, "proto:" + in
}

type expectedModuleType string

func (e expectedModuleType) String() string {
	if e == expectedModuleType(IgnoreOutputModuleType) {
		return "<Ignored>"
	}

	return string(e)
}
