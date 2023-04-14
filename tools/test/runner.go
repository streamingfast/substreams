package test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/charmbracelet/lipgloss"
	"github.com/jhump/protoreflect/dynamic"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap"
)

type Runner struct {
	tests          map[uint64]map[string][]*Test
	descs          map[string]*manifest.ModuleDescriptor
	messageFactory *dynamic.MessageFactory

	logger *zap.Logger

	configured uint64
	passed     uint64
	failed     uint64
	notfound   uint64
	verbose    bool
	results    []*Result
}

var successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
var failedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)

func NewRunner(path string, descs map[string]*manifest.ModuleDescriptor, verbose bool, logger *zap.Logger) (*Runner, error) {
	spec, err := readSpecFromFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading spec: %w", err)
	}

	r := &Runner{
		tests:          map[uint64]map[string][]*Test{},
		descs:          descs,
		messageFactory: dynamic.NewMessageFactoryWithDefaults(),
		logger:         logger.Named("substreams_test"),
		verbose:        verbose,
		configured:     uint64(len(spec.Tests)),
	}

	for idx, testConfig := range spec.Tests {
		if _, found := r.tests[testConfig.Block]; !found {
			r.tests[testConfig.Block] = map[string][]*Test{}
		}

		blockTests := r.tests[testConfig.Block]

		test, err := testConfig.Test(idx)
		if err != nil {
			return nil, fmt.Errorf("failed to setup test number %d: %w", idx, err)
		}
		blockTests[testConfig.Module] = append(blockTests[testConfig.Module], test)
	}

	return r, nil
}

func (r *Runner) Test(
	ctx context.Context,
	output *pbsubstreamsrpc.MapModuleOutput,
	debugMapOutputs []*pbsubstreamsrpc.MapModuleOutput,
	debugStoreOutputs []*pbsubstreamsrpc.StoreModuleOutput,
	clock *pbsubstreams.Clock,
) (results []*Result) {
	logger := r.logger.With(zap.Uint64("block_num", clock.Number))

	blockTests, found := r.tests[clock.Number]
	if !found {
		logger.Debug("skip block test not test found")
		return nil
	}

	for _, out := range append([]*pbsubstreamsrpc.MapModuleOutput{output}, debugMapOutputs...) {
		moduleName := out.Name
		logger = logger.With(zap.String("module", moduleName))
		moduleTests, found := blockTests[moduleName]
		if !found {
			logger.Debug("skipping module test no test found")
			continue
		}

		results = append(results, r.testMapModule(ctx, out, moduleTests, logger)...)
	}

	//for _, out := range debugStoreOutputs {
	//}

	return results
}

func (r *Runner) testMapModule(ctx context.Context, module *pbsubstreamsrpc.MapModuleOutput, tests []*Test, logger *zap.Logger) (out []*Result) {
	moduleName := module.Name

	msgDesc, ok := r.descs[moduleName]
	if !ok {
		logger.Debug("skipping module test cannot decode message without proto descriptor")
		return nil
	}

	in := module.MapOutput.GetValue()
	dynMsg := r.messageFactory.NewDynamicMessage(msgDesc.MessageDescriptor)
	if err := dynMsg.Unmarshal(in); err != nil {
		logger.Debug("skipping module cannot failed to decode message", zap.Error(err))
		return nil
	}

	cnt, err := dynMsg.MarshalJSONIndent()
	if err != nil {
		logger.Debug("skipping module cannot failed to JSON marshal payload", zap.Error(err))
		return nil
	}

	// Idiosyncrasy of the JQ library
	//
	// You cannot use custom type values as the query input.
	// The type should be []interface{} for an array and map[string]interface{} for a map
	// (just like decoded to an interface{} using the encoding/json package).
	// You can't use []int or map[string]string, for example.
	// If you want to query your custom struct, marshal to JSON, unmarshal to interface{}
	// and use it as the query input.
	var input interface{}
	if err := json.Unmarshal(cnt, &input); err != nil {
		logger.Debug("json unmarshalling ", zap.Error(err))
		return nil
	}

	for _, test := range tests {
		logger.Debug("running test", zap.String("path", test.path))
		iter := test.code.RunWithContext(ctx, input) // or query.RunWithContext
		// we will asume there should be only 1 result
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, ok := v.(error); ok {
			logger.Debug("failed get path ", zap.Error(err))
			continue
		}

		actual, ok := v.(string)
		if !ok {
			r.notfound++
			continue
		}

		valid, msg, err := test.comparable.Cmp(actual)
		if err != nil {
			logger.Warn("failed to run test", zap.Error(err))
			continue
		}

		result := &Result{
			test:  test,
			Valid: valid,
			Msg:   msg,
		}
		out = append(out, result)
		r.results = append(r.results, result)
		if result.Valid {
			r.passed++
		} else {
			r.failed++
		}
	}
	return out
}

func (r *Runner) LogResults() {
	if r.verbose {
		fmt.Println()
		for _, result := range r.results {
			status := successStyle.Render("ok")
			if !result.Valid {
				status = fmt.Sprintf("%s > %s", failedStyle.Render("failed"), result.Msg)
			}
			fmt.Printf("test %s:%d:%d ... %s\n", result.test.moduleName, result.test.block, result.test.fileIndex, status)
		}
	}

	fmt.Println()
	fmt.Printf("test result: ok. %d configured; %d passed; %d failed; %d not matched\n", r.configured, r.passed, r.failed, int(r.configured)-len(r.results))
	fmt.Println()
}
