package sink

import (
	"context"
	"fmt"

	"github.com/streamingfast/bstream"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"go.uber.org/zap/zapcore"
)

type sinkerHandlers struct {
	handleBlockScopedData func(ctx context.Context, data *pbsubstreamsrpc.BlockScopedData, isLive *bool, cursor *Cursor) error
	handleBlockUndoSignal func(ctx context.Context, undoSignal *pbsubstreamsrpc.BlockUndoSignal, cursor *Cursor) error
}

func (h sinkerHandlers) HandleBlockScopedData(ctx context.Context, data *pbsubstreamsrpc.BlockScopedData, isLive *bool, cursor *Cursor) error {
	return h.handleBlockScopedData(ctx, data, isLive, cursor)
}

func (h sinkerHandlers) HandleBlockUndoSignal(ctx context.Context, undoSignal *pbsubstreamsrpc.BlockUndoSignal, cursor *Cursor) error {
	return h.handleBlockUndoSignal(ctx, undoSignal, cursor)
}

func NewSinkerHandlers(
	handleBlockScopedData func(ctx context.Context, data *pbsubstreamsrpc.BlockScopedData, isLive *bool, cursor *Cursor) error,
	handleBlockUndoSignal func(ctx context.Context, undoSignal *pbsubstreamsrpc.BlockUndoSignal, cursor *Cursor) error,
) SinkerHandler {
	return sinkerHandlers{handleBlockScopedData, handleBlockUndoSignal}
}

type SinkerHandler interface {
	// HandleBlockScopedData defines the callback that will handle Substreams `BlockScopedData` messages.
	//
	// The handler receives the following arguments:
	// - `ctx` is the context runtime, your handler should be minimal, so normally you shouldn't use this.
	// - `data` contains the block scoped data that was received from the Substreams API, refer to it's definition for proper usage.
	// - `isLive` will be non-nil if a [LivenessChecker] has been configured on the [Sinker] instance that call the handler.
	// - `cursor` is the cursor at the given block, this cursor should be saved regularly as a checkpoint in case the process is interrupted.
	//
	// The [HandleBlockScopedData] must be non-nil, the [Sinker] enforces this.
	//
	// Your handler must return an error value that can be nil or non-nil. If non-nil, the error is assumed to be a fatal
	// error and the [Sinker] will not retry it. If the error is retryable, wrap it in `derr.NewRetryableError(err)` to notify
	// the [Sinker] that it should retry from last valid cursor. It's your responsibility to ensure no data was persisted prior the
	// the error.
	/**/ HandleBlockScopedData(ctx context.Context, data *pbsubstreamsrpc.BlockScopedData, isLive *bool, cursor *Cursor) error

	// HandleBlockUndoSignal defines the callback that will handle Substreams `BlockUndoSignal` messages.
	//
	// The handler receives the following arguments:
	// - `ctx` is the context runtime, your handler should be minimal, so normally you shouldn't use this.
	// - `undoSignal` contains the last valid block that is still valid, any data saved after this last saved block should be discarded.
	// - `cursor` is the cursor at the given block, this cursor should be saved regularly as a checkpoint in case the process is interrupted.
	//
	// The [HandleBlockUndoSignal] can be nil if the sinker is configured to stream final blocks only, otherwise it must be set,
	// the [Sinker] enforces this.
	//
	// Your handler must return an error value that can be nil or non-nil. If non-nil, the error is assumed to be a fatal
	// error and the [Sinker] will not retry it. If the error is retryable, wrap it in `derr.NewRetryableError(err)` to notify
	// the [Sinker] that it should retry from last valid cursor. It's your responsibility to ensure no data was persisted prior the
	// the error.
	HandleBlockUndoSignal(ctx context.Context, undoSignal *pbsubstreamsrpc.BlockUndoSignal, cursor *Cursor) error
}

// SinkerCompletionHandler defines an extra interface that can be implemented on top of `SinkerHandler` where the
// callback will be invoked when the sinker is done processing the requested range. This is useful to implement
// a checkpointing mechanism where when the range has correctly fully processed, you can do something meaningful.
type SinkerCompletionHandler interface {
	// HandleBlockRangeCompletion is called when the sinker is done processing the requested range, only when
	// the stream has correctly reached its end block. If the sinker is configured to stream live, this callback
	// will never be called.
	//
	// If the sinker terminates with an error, this callback will not be called.
	//
	// The handler receives the following arguments:
	// - `ctx` is the context runtime, your handler should be minimal, so normally you shouldn't use this.
	// - `cursor` is the cursor at the given block, this cursor should be saved regularly as a checkpoint in case the process is interrupted.
	HandleBlockRangeCompletion(ctx context.Context, cursor *Cursor) error
}

type Cursor struct {
	*bstream.Cursor
}

func NewCursor(cursor string) (*Cursor, error) {
	if cursor == "" {
		return blankCursor, nil
	}

	decoded, err := bstream.CursorFromOpaque(cursor)
	if err != nil {
		return nil, fmt.Errorf("decode %q: %w", cursor, err)
	}

	return &Cursor{decoded}, nil
}

func MustNewCursor(cursor string) *Cursor {
	decoded, err := NewCursor(cursor)
	if err != nil {
		panic(err)
	}

	return decoded
}

var blankCursor = (*Cursor)(nil)

func NewBlankCursor() *Cursor {
	return blankCursor
}

func (c *Cursor) Block() bstream.BlockRef {
	if c.IsBlank() {
		return unsetBlockRef{}
	}

	return c.Cursor.Block
}

func (c *Cursor) IsBlank() bool {
	return c == nil || c == blankCursor
}

func (c *Cursor) IsEqualTo(other *Cursor) bool {
	if c.IsBlank() && other.IsBlank() {
		return true
	}

	// We know both are not equal, so if either side is `nil`, we are sure the other is not, so not equal
	if !c.IsBlank() || !other.IsBlank() {
		return false
	}

	// Both side are non-nil here
	actual := c.Cursor
	candidate := other.Cursor

	return actual.Equals(candidate)
}

func (c *Cursor) MarshalLogObject(encoder zapcore.ObjectEncoder) error {
	cursor := "<Blank>"
	if !c.IsBlank() {
		cursor = c.String()
	}

	encoder.AddString("cursor", cursor)
	return nil
}

// String returns a string representation suitable for handling a Firehose request
// meaning a blank cursor returns "".
func (c *Cursor) String() string {
	if c.IsBlank() {
		return ""
	}

	return c.Cursor.ToOpaque()
}

//go:generate go-enum -f=$GOFILE --marshal --names

// ENUM(
//
//	Development
//	Production
//
// )
type SubstreamsMode uint
