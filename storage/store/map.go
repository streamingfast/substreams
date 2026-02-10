package store

import (
	"context"
	"errors"

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

func (m *Map) QuickSave(ctx context.Context, atBlockHash string) error {
	for _, s := range m.All() {
		if writable, ok := s.(QuickSave); ok {
			if err := writable.QuickSave(ctx, atBlockHash); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *Map) QuickLoad(ctx context.Context, atBlock bstream.BlockRef) error {

	for _, s := range m.All() {
		if writable, ok := s.(QuickLoad); ok {
			if err := writable.QuickLoad(ctx, atBlock); err != nil {
				return err
			}
		}

		if reqHandler := reqctx.ActiveRequestsHandler(ctx); reqHandler != nil {
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
