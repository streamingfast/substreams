package sink

import (
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/streamingfast/logging"
	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap"
)

// SinkerConfig contains all configuration needed to create and run a Sinker.
type SinkerConfig struct {
	// Substreams package configuration
	Pkg                              *pbsubstreams.Package
	OutputModule                     *pbsubstreams.Module
	OutputModuleHash                 manifest.ModuleHash
	SupportIndexOutputProductionMode bool

	// Client configuration
	ClientConfig *client.SubstreamsClientConfig

	// Operational configuration
	Mode                 SubstreamsMode
	NoopMode             bool
	LimitProcessedBlocks uint64

	// Block processing configuration
	StartBlock      int64
	StopBlock       uint64
	UndoBufferSize  int
	FinalBlocksOnly bool

	// Dev-mode extras
	DevOutputSnapshots []string
	DevOutputModules   []string // if this is empty, the request will contain the output module in here

	// Retry and reliability configuration
	MaxRetries int // 0 = no retries, -1 = infinite retries, >0 = specific number of retries
	BackOff    backoff.BackOff

	// Liveness configuration
	LiveBlockTimeDelta time.Duration
	LivenessChecker    LivenessChecker

	// Additional configuration
	ExtraHeaders []string

	// Logging and tracing
	Logger *zap.Logger
	Tracer logging.Tracer

	// Expose metrics
	PrometheusAddr string // if non-null, will listen to this address

	// Legacy fields for backward compatibility
	Params                []string
	Network               string
	SkipPackageValidation bool
}

// ExtractDefaultParams extracts default parameter values from the package modules
// that are not already specified in the existing parameters. This is useful for
// GUI applications that want to show default values from the package.
func (c *SinkerConfig) ExtractDefaultParams() []string {
	// Create a map of existing parameters to avoid duplicates
	existingParamsMap := make(map[string]struct{})
	for _, param := range c.Params {
		if len(param) > 0 {
			moduleName := param
			if idx := strings.Index(param, "="); idx != -1 {
				moduleName = param[:idx]
			}
			existingParamsMap[moduleName] = struct{}{}
		}
	}

	var defaultParams []string
	if c.Pkg != nil && c.Pkg.Modules != nil {
		for _, module := range c.Pkg.Modules.Modules {
			moduleName := module.Name
			for _, input := range module.Inputs {
				param := input.GetParams()
				if param != nil {
					if _, found := existingParamsMap[moduleName]; !found {
						defaultParams = append(defaultParams, moduleName+"="+param.Value)
					}
				}
			}
		}
	}

	return defaultParams
}
