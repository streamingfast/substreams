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
// which spans approximatively 5m so that retry is perform indefinitely without
// never exiting the process.
func WithInfiniteRetry() Option {
	return func(s *Sinker) {
		s.infiniteRetry = true
	}
}

// WithFinalBlocksOnly configures the [Sinker] to only stream Substreams output that
// is considered final by the Substreams backend server.
//
// This means that `WithBlockDataBuffer` if used is discarded and [BlockUndoSignalHandler]
// will never be called.
func WithFinalBlocksOnly() Option {
	return func(s *Sinker) {
		s.buffer = nil
		s.finalBlocksOnly = true
	}
}

// WithRetryBackoff configures the [Sinker] to which itself configurs the Substreams
// gRPC stream to only send [pbsubstreamsrpc.BlockScopedData] once the block is final, this
// means that `WithBlockDataBuffer` if used has is discarded and [BlockUndoSignalHandler]
// will never be called.
func WithRetryBackOff(backOff backoff.BackOff) Option {
	return func(s *Sinker) {
		s.backOff = backOff
	}
}

// WithLivenessChecker configures a [LivnessCheck] on the [Sinker] instance.
//
// By configuring a liveness checker, the [MessageContext] received by [BlockScopedDataHandler]
// and [BlockUndoSignalHandler] will have the field [MessageContext.IsLive] properly populated.
func WithLivenessChecker(livenessChecker LivenessChecker) Option {
	return func(s *Sinker) {
		s.livenessChecker = livenessChecker
	}
}

// WithBlockRange configures the [Sinker] instance to only stream for the range specified. If
// there is no range specified on the [Sinker], the [Sinker] is going to sink automatically
// from module's start block to live never ending.
func WithBlockRange(blockRange *bstream.Range) Option {
	return func(s *Sinker) {
		s.blockRange = blockRange
	}
}

// WithExtraHeaders configures the [Sinker] instance to send extra headers to the Substreams
// backend server.
func WithExtraHeaders(headers []string) Option {
	return func(s *Sinker) {
		s.extraHeaders = headers
	}
}
