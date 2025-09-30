package sink

import (
	"github.com/cenkalti/backoff/v4"
	"github.com/streamingfast/bstream"
)

type Option func(s *Sinker)

// WithBlockDataBuffer creates a buffer of block data which is used to handle undo fork steps.
//
// Ensure that this buffer is large enough to capture all block reorganizations.
// If the buffer is too small, the sinker will not be able to handle the reorganization and will error if an undo is received for a block which has already been returned to the sink.
// If the buffer is too large, the sinker will take more time than necessary to write data to the sink.
//
// If the sink is configured to handle irreversible blocks, the default buffer size is 12. If
// you pass 0, block data buffer will be disabled completely.
//
// Deprecated: Do not rely on [New] and [Option], instead use [NewFromConfig] and
// configure the [SinkerConfig] directly, the counterpart of this option is to set
// [SinkerConfig.UndoBufferSize] to a value > 0, which is effective only if
// [SinkerConfig.FinalBlocksOnly] is false, meaning that undo signals are expected.
func WithBlockDataBuffer(bufferSize int) Option {
	return func(s *Sinker) {
		if bufferSize == 0 {
			s.buffer = nil
		} else {
			s.buffer = newBlockDataBuffer(bufferSize)
		}
	}
}

// WithInfiniteRetry remove the maximum retry limit of 15 (hard-coded right now)
// which spans approximately 5m so that retry is perform indefinitely without
// never exiting the process.
//
// Deprecated: Do not rely on [New] and [Option], instead use [NewFromConfig] and
// configure the [SinkerConfig] directly.
func WithInfiniteRetry() Option {
	return func(s *Sinker) {
		s.SinkerConfig.MaxRetries = -1
	}
}

// WithFinalBlocksOnly configures the [Sinker] to only stream Substreams output that
// is considered final by the Substreams backend server.
//
// This means that `WithBlockDataBuffer` if used is discarded and [BlockUndoSignalHandler]
// will never be called.
//
// Deprecated: Do not rely on [New] and [Option], instead use [NewFromConfig] and
// configure the [SinkerConfig] directly.
func WithFinalBlocksOnly() Option {
	return func(s *Sinker) {
		s.buffer = nil
		s.SinkerConfig.FinalBlocksOnly = true
	}
}

// WithRetryBackoff configures the [Sinker] to which itself configurs the Substreams
// gRPC stream to only send [pbsubstreamsrpc.BlockScopedData] once the block is final, this
// means that `WithBlockDataBuffer` if used has is discarded and [BlockUndoSignalHandler]
// will never be called.
//
// Deprecated: Do not rely on [New] and [Option], instead use [NewFromConfig] and
// configure the [SinkerConfig] directly.
func WithRetryBackOff(backOff backoff.BackOff) Option {
	return func(s *Sinker) {
		s.SinkerConfig.BackOff = backOff
	}
}

// WithLivenessChecker configures a [LivnessCheck] on the [Sinker] instance.
//
// By configuring a liveness checker, the [MessageContext] received by [BlockScopedDataHandler]
// and [BlockUndoSignalHandler] will have the field [MessageContext.IsLive] properly populated.
//
// Deprecated: Do not rely on [New] and options, instead use [NewFromConfig] and
// configure the [SinkerConfig] directly.
func WithLivenessChecker(livenessChecker LivenessChecker) Option {
	return func(s *Sinker) {
		s.SinkerConfig.LivenessChecker = livenessChecker
	}
}

// WithBlockRange configures the [Sinker] instance to only stream for the range specified. If
// there is no range specified on the [Sinker], the [Sinker] is going to sink automatically
// from module's start block to live never ending.
//
// Deprecated: Do not rely on [New] and [Option], instead use [NewFromConfig] and
// configure the [SinkerConfig] directly.
func WithBlockRange(blockRange *bstream.Range) Option {
	return func(s *Sinker) {
		s.SinkerConfig.StartBlock = int64(blockRange.StartBlock())
		if blockRange.EndBlock() == nil {
			s.SinkerConfig.StopBlock = 0
		} else {
			s.SinkerConfig.StopBlock = *blockRange.EndBlock()
		}
	}
}

// WithExtraHeaders configures the [Sinker] instance to send extra headers to the Substreams
// backend server.
//
// Deprecated: Do not rely on [New] and [Option], instead use [NewFromConfig] and
// configure the [SinkerConfig] directly.
func WithExtraHeaders(headers []string) Option {
	return func(s *Sinker) {
		s.SinkerConfig.ExtraHeaders = headers
	}
}

// WithAgent configures the [Sinker] instance to use a custom agent string when
// connecting to the Substreams backend server. Otherwise, the default agent
// string is used.
//
// Deprecated: Do not rely on [New] and [Option], instead use [NewFromConfig] and
// configure the [SinkerConfig] directly.
func WithAgent(agent string) Option {
	return func(s *Sinker) {
		s.SinkerConfig.ClientConfig.SetAgent(agent)
	}
}
