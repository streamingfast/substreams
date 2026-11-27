package cdc

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"slices"
	"strings"
	"sync"

	"go.uber.org/zap"
)

// Batch is an ordered group of events delivered together: all the events of one block, or all the
// compensation events of one undo signal.
type Batch struct {
	// Block is the block the events belong to. For a compensation batch it is the last valid block.
	Block Block
	// Cursor is the stream cursor that follows this batch, emitters that persist can checkpoint it.
	Cursor string
	Events []Event
}

// Emitter delivers change events to a destination.
//
// The [Handler] calls it from a single goroutine, in stream order. A call must return only once the events
// are delivered (or durably queued by the emitter), since the [Handler] advances its cursor right after.
// Delivery is at-least-once: after a crash a batch can be emitted again, event ids are deterministic so
// receivers can deduplicate.
type Emitter interface {
	// EmitBatch delivers the events of one block, in order.
	EmitBatch(ctx context.Context, batch *Batch) error
	// EmitCompensation delivers the compensation events produced for an undo signal, in order.
	EmitCompensation(ctx context.Context, batch *Batch) error
	// Flush delivers anything the emitter still buffers.
	Flush(ctx context.Context) error
	// Close flushes and releases the emitter's resources.
	Close() error
}

// EmitterOptions is what [NewEmitter] hands to an [EmitterFactory] besides the URL.
type EmitterOptions struct {
	Logger *zap.Logger
	// StateFile is the sink state (cursor) file, emitters that persist data next to it derive their paths from it.
	StateFile string
	// Stdout overrides where the `stdout://` emitter writes, defaults to [os.Stdout].
	Stdout io.Writer
}

// EmitterFactory builds an [Emitter] for a destination URL.
type EmitterFactory func(ctx context.Context, u *url.URL, opts EmitterOptions) (Emitter, error)

var (
	emittersMu sync.RWMutex
	emitters   = map[string]EmitterFactory{}
)

// RegisterEmitter registers the factory handling URLs of the given scheme. It panics when the scheme is
// empty or already registered, it is meant to be called from `init`.
func RegisterEmitter(scheme string, factory EmitterFactory) {
	scheme = strings.ToLower(scheme)
	if scheme == "" || factory == nil {
		panic("cdc: RegisterEmitter needs a scheme and a factory")
	}

	emittersMu.Lock()
	defer emittersMu.Unlock()

	if _, found := emitters[scheme]; found {
		panic(fmt.Sprintf("cdc: emitter scheme %q already registered", scheme))
	}
	emitters[scheme] = factory
}

// EmitterSchemes returns the registered schemes, sorted.
func EmitterSchemes() []string {
	emittersMu.RLock()
	defer emittersMu.RUnlock()

	schemes := make([]string, 0, len(emitters))
	for scheme := range emitters {
		schemes = append(schemes, scheme)
	}
	slices.Sort(schemes)
	return schemes
}

// NewEmitter builds the emitter for rawURL, picking the factory by URL scheme. A lone `-` is the same as `stdout://`.
func NewEmitter(ctx context.Context, rawURL string, opts EmitterOptions) (Emitter, error) {
	if rawURL == "-" {
		rawURL = "stdout://"
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse emitter url %q: %w", rawURL, err)
	}

	scheme := strings.ToLower(u.Scheme)
	emittersMu.RLock()
	factory, found := emitters[scheme]
	emittersMu.RUnlock()

	if !found {
		return nil, fmt.Errorf("no emitter for scheme %q in %q, available schemes: %s", scheme, rawURL, strings.Join(EmitterSchemes(), ", "))
	}

	if opts.Logger == nil {
		opts.Logger = zap.NewNop()
	}

	return factory(ctx, u, opts)
}
