package sink

import (
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
	Pkg              *pbsubstreams.Package
	OutputModule     *pbsubstreams.Module
	OutputModuleHash manifest.ModuleHash

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
	InfiniteRetry bool
	BackOff       backoff.BackOff

	// Liveness configuration
	LiveBlockTimeDelta time.Duration
	LivenessChecker    LivenessChecker

	// Additional configuration
	ExtraHeaders []string

	// Logging and tracing
	Logger *zap.Logger
	Tracer logging.Tracer

	// Legacy fields for backward compatibility
	Params                []string
	Network               string
	SkipPackageValidation bool
}
