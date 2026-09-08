package store

import (
	"context"
	"errors"

	"github.com/abourget/llerrgroup"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/substreams/reqctx"
	"go.uber.org/zap/zapcore"
)

var ErrNotFound = errors.New("store not found")

type Getter interface {
	Get(name string) (Store, bool)
	All() map[string]Store
}

type Setter interface {
	Set(name string, s Store)
}

type Map map[string]Store

func (m Map) Names() []string {
	out := make([]string, len(m))
	i := 0
	for _, s := range m {
		out[i] = s.Name()
		i++
	}
	return out
}

// quickSaveParallelism bounds how many stores are quicksaved/quickloaded
// concurrently. Each in-flight store streams its state one entry at a time, so
// peak extra memory is roughly this many single-entry buffers.
const quickSaveParallelism = 8

func (m *Map) QuickSave(ctx context.Context, atBlockHash string) error {
	eg := llerrgroup.New(quickSaveParallelism)
	for _, s := range m.All() {
		writable, ok := s.(QuickSave)
		if !ok {
			continue
		}
		if eg.Stop() {
			break
		}
		eg.Go(func() error {
			return writable.QuickSave(ctx, atBlockHash)
		})
	}
	return eg.Wait()
}

func (m *Map) QuickLoad(ctx context.Context, atBlock bstream.BlockRef) error {
	if err := m.QuickLoadOpen(ctx, atBlock); err != nil {
		return err
	}
	return m.QuickLoadFinish(ctx, atBlock)
}

// QuickLoadOpen opens every store's quicksave object (confirming they all exist
// and are readable) without streaming them. If any open fails, every reader opened
// so far is released before returning, so a failed open leaves no dangling handles.
// On success the caller must eventually call QuickLoadFinish (or QuickLoadClose).
func (m *Map) QuickLoadOpen(ctx context.Context, atBlock bstream.BlockRef) error {
	eg := llerrgroup.New(quickSaveParallelism)
	for _, s := range m.All() {
		loadable, ok := s.(QuickLoad)
		if !ok {
			continue
		}
		if eg.Stop() {
			break
		}
		eg.Go(func() error {
			return loadable.QuickLoadOpen(ctx, atBlock)
		})
	}
	if err := eg.Wait(); err != nil {
		m.QuickLoadClose()
		return err
	}
	return nil
}

// QuickLoadClose releases any readers opened by QuickLoadOpen that were not yet
// streamed, used to abandon a quickload after a partial open.
func (m *Map) QuickLoadClose() {
	for _, s := range m.All() {
		if loadable, ok := s.(QuickLoad); ok {
			loadable.QuickLoadClose()
		}
	}
}

// QuickLoadFinish streams every store's previously-opened quicksave object into
// the store, then does memory accounting. Must follow a successful QuickLoadOpen.
func (m *Map) QuickLoadFinish(ctx context.Context, atBlock bstream.BlockRef) error {
	eg := llerrgroup.New(quickSaveParallelism)
	for _, s := range m.All() {
		loadable, ok := s.(QuickLoad)
		if !ok {
			continue
		}
		if eg.Stop() {
			break
		}
		eg.Go(func() error {
			return loadable.QuickLoadFinish(ctx, atBlock)
		})
	}
	if err := eg.Wait(); err != nil {
		// A store's Finish failed (or was skipped after eg.Stop): release any
		// readers opened by QuickLoadOpen that were never streamed.
		m.QuickLoadClose()
		return err
	}

	// Memory accounting is done after all stores are loaded: the handler is not
	// concurrency-safe and may force-cancel the request, so it runs serially.
	if reqHandler := reqctx.ActiveRequestsHandler(ctx); reqHandler != nil {
		for _, s := range m.All() {
			reqHandler.AllocateFullKVSizeOrForceCancelRequest(s.SizeBytes())
			if ctx.Err() != nil {
				return ctx.Err()
			}
		}
	}
	return nil
}

func NewMap() Map {
	return map[string]Store{}
}

func (m Map) Set(s Store) {
	m[s.Name()] = s
}

func (m Map) Get(name string) (Store, bool) {
	s, found := m[name]
	return s, found
}

func (m Map) All() map[string]Store {
	return m
}

func (m Map) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddInt("count", len(m))
	return nil
}
