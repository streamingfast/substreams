package store

import (
	"context"
	"fmt"

	"github.com/streamingfast/dstore"
	pbmodel "github.com/streamingfast/substreams-foundational-store/pb/sf/substreams/foundational-store/model/v2"
	pbservice "github.com/streamingfast/substreams-foundational-store/pb/sf/substreams/foundational-store/service/v2"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/storage/store/marshaller"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/anypb"
)

var _ Store = (*BadgerBackedStore)(nil)

// BadgerBackedStore implements the Store interface backed by a Badger database via the foundational store service.
// kv is the authoritative accumulated state — identical contract to FullKV.  It is NOT cleared on Reset().
// Persistence to Badger happens via FlushToBadger(lib) at LIB; ForkAware is a pure durability buffer.
type BadgerBackedStore struct {
	*baseStore

	forkAwareClient pbservice.StoreClient // gRPC client to foundational store server
	blockNum        uint64                // current block number being processed
	ctx             context.Context       // pipeline context, updated each block via SetBlockContext
}

// NewBadgerBackedKV creates a new BadgerBackedStore with the specified configuration and gRPC client
func NewBadgerBackedKV(
	logger *zap.Logger,
	objStore dstore.Store,
	config *Config,
	forkAwareClient pbservice.StoreClient,
) (*BadgerBackedStore, error) {
	if config.moduleHash == "" {
		return nil, fmt.Errorf("missing module hash")
	}

	b := &baseStore{
		Config:     config,
		kvOps:      &pbssinternal.Operations{},
		kv:         make(map[string][]byte), // authoritative accumulated state, NOT cleared on Reset
		logger:     logger,
		marshaller: marshaller.Default(),
	}

	return &BadgerBackedStore{
		baseStore:       b,
		forkAwareClient: forkAwareClient,
		ctx:             context.Background(),
	}, nil
}

// Store returns the underlying dstore.Store (not used for Badger-backed stores)
func (s *BadgerBackedStore) Store() dstore.Store {
	return s.objStore
}

// Marshaller returns the marshaller (kept for interface compatibility)
func (s *BadgerBackedStore) Marshaller() marshaller.Marshaller {
	return s.marshaller
}

// SetBlockNum sets the current block number being processed
func (s *BadgerBackedStore) SetBlockNum(blockNum uint64) {
	s.blockNum = blockNum
}

// SetBlockContext sets the pipeline context for the current block.
// All gRPC calls use this context so they respect pipeline cancellation.
func (s *BadgerBackedStore) SetBlockContext(ctx context.Context) {
	s.ctx = ctx
}

// Flush processes buffered operations, produces deltas, and sends the resolved
// kv values for touched keys to the foundational store.
//
// baseStore.Flush() applies kvOps → kv (with all arithmetic) and produces
// deltas.  We then read the resolved values straight from kv — not from the
// raw ops — and send them with UPDATE_POLICY_SET.  ForkAware is a pure
// durability buffer: it stores resolved values and flushes to Badger at LIB.
// No arithmetic happens in ForkAware; all arithmetic lives in baseStore.Flush().
func (s *BadgerBackedStore) Flush() error {
	// Snapshot the keys touched this block before Flush clears anything.
	// We collect touched keys from kvOps now, before baseStore.Flush() runs.
	touchedKeys := make(map[string]struct{}, len(s.kvOps.Operations))
	var deletePrefixes []string
	for _, op := range s.kvOps.Operations {
		if op.Type == pbssinternal.Operation_DELETE_PREFIX {
			deletePrefixes = append(deletePrefixes, op.Key)
		} else {
			touchedKeys[op.Key] = struct{}{}
		}
	}

	// Apply ops → kv, produce deltas.
	if err := s.baseStore.Flush(); err != nil {
		return fmt.Errorf("base store flush failed: %w", err)
	}

	// Build entries from the resolved kv values for touched keys.
	// Keys deleted by a DELETE_PREFIX won't be in kv — that's correct.
	var entries []*pbmodel.Entry
	for key := range touchedKeys {
		val, exists := s.kv[key]
		if !exists {
			// Key was deleted within this block (e.g. delete_prefix covered it).
			continue
		}
		entries = append(entries, &pbmodel.Entry{
			Key:          &pbmodel.Key{Bytes: []byte(key)},
			Value:        &anypb.Any{TypeUrl: s.valueType, Value: val},
			UpdatePolicy: pbmodel.UpdatePolicy_UPDATE_POLICY_SET,
			ValueType:    s.valueType,
		})
	}

	if len(entries) == 0 && len(deletePrefixes) == 0 {
		return nil
	}

	req := &pbservice.SetRequest{
		SinkEntries: &pbmodel.SinkEntries{
			Entries:        entries,
			DeletePrefixes: deletePrefixes,
		},
		BlockNumber: s.blockNum,
	}

	if _, err := s.forkAwareClient.SetAll(s.ctx, req); err != nil {
		return fmt.Errorf("failed to send entries to foundational store: %w", err)
	}

	s.logger.Debug("flushed entries to foundational store",
		zap.Uint64("block_num", s.blockNum),
		zap.Int("entry_count", len(entries)),
	)
	return nil
}

// DerivePartialStore creates a partial store (not supported for Badger-backed stores)
func (s *BadgerBackedStore) DerivePartialStore(initialBlock uint64) *PartialKV {
	panic("DerivePartialStore not supported for BadgerBackedStore")
}

// Reset clears kvOps, deltas, and lastOrdinal for the next block.
// kv is intentionally NOT cleared — it is the authoritative accumulated state,
// exactly as in FullKV. Cross-block correctness (delta OldValue, GetFirst/GetLast
// for downstream readers) depends on kv surviving across blocks.
func (s *BadgerBackedStore) Reset() {
	s.baseStore.Reset()
}

// GetFirst retrieves the value as of the start of the current block (before any
// intra-block mutations).  It walks deltas first, then falls back to kv, then
// to the foundational store gRPC on a cold-start cache miss.
func (s *BadgerBackedStore) GetFirst(key string) ([]byte, bool) {
	if val, found := s.baseStore.GetFirst(key); found {
		return val, true
	}
	// kv miss (key never written before) — query foundational store for cold start.
	rawVal, found := s.fetchFromBadger(key)
	if !found {
		return nil, false
	}
	s.kv[key] = rawVal
	return s.stripSetSumPrefix(rawVal), true
}

// GetLast retrieves the value as of the end of the current block.
func (s *BadgerBackedStore) GetLast(key string) ([]byte, bool) {
	if val, found := s.baseStore.GetLast(key); found {
		return val, true
	}
	rawVal, found := s.fetchFromBadger(key)
	if !found {
		return nil, false
	}
	s.kv[key] = rawVal
	return s.stripSetSumPrefix(rawVal), true
}

// GetAt retrieves the value at a specific intra-block ordinal.
func (s *BadgerBackedStore) GetAt(ord uint64, key string) ([]byte, bool) {
	// Ensure kv is populated so baseStore.getAt has a baseline.
	if _, ok := s.kv[key]; !ok {
		if rawVal, found := s.fetchFromBadger(key); found {
			s.kv[key] = rawVal
		}
	}
	return s.baseStore.GetAt(ord, key)
}

// HasFirst checks whether a key exists as of the start of the current block.
func (s *BadgerBackedStore) HasFirst(prefix string) bool {
	_, found := s.GetFirst(prefix)
	return found
}

// HasLast checks whether a key exists as of the end of the current block.
func (s *BadgerBackedStore) HasLast(prefix string) bool {
	_, found := s.GetLast(prefix)
	return found
}

// HasAt checks whether a key exists at the given intra-block ordinal.
func (s *BadgerBackedStore) HasAt(ord uint64, key string) bool {
	_, found := s.GetAt(ord, key)
	return found
}

// stripSetSumPrefix strips the 4-byte "sum:" or "set:" prefix for SET_SUM stores.
func (s *BadgerBackedStore) stripSetSumPrefix(val []byte) []byte {
	if s.UpdatePolicy() == pbsubstreams.Module_KindStore_UPDATE_POLICY_SET_SUM &&
		len(val) >= 4 && (string(val[:4]) == "sum:" || string(val[:4]) == "set:") {
		return val[4:]
	}
	return val
}

// fetchFromBadger performs a gRPC Get for a single exact key at the current block number.
// This is only called on a cold-start cache miss — normal operation serves from kv.
func (s *BadgerBackedStore) fetchFromBadger(key string) ([]byte, bool) {
	req := &pbservice.GetRequest{
		Keys:        []*pbmodel.Key{{Bytes: []byte(key)}},
		BlockNumber: s.blockNum,
	}
	resp, err := s.forkAwareClient.Get(s.ctx, req)
	if err != nil {
		s.logger.Warn("failed to get from foundational store", zap.String("key", key), zap.Error(err))
		return nil, false
	}
	if resp.Entries == nil || len(resp.Entries.Entries) == 0 {
		return nil, false
	}
	queried := resp.Entries.Entries[0]
	if queried.Code != pbmodel.ResponseCode_RESPONSE_CODE_FOUND {
		return nil, false
	}
	return queried.Entry.Value.Value, true
}

// FlushToBadger signals the foundational store to flush ForkAware entries up to lib to Badger.
func (s *BadgerBackedStore) FlushToBadger(lib uint64) error {
	_, err := s.forkAwareClient.FlushUpToBlock(s.ctx, &pbservice.FlushRequest{
		BlockNumber: lib,
		IfNotExist:  false,
	})
	if err != nil {
		return fmt.Errorf("failed to flush foundational store to block %d: %w", lib, err)
	}
	s.logger.Debug("flushed foundational store up to block", zap.Uint64("lib", lib))
	return nil
}

// EvictFromBadger signals the foundational store to evict speculative entries at reorgBlock (fork rollback).
func (s *BadgerBackedStore) EvictFromBadger(reorgBlock uint64) error {
	_, err := s.forkAwareClient.EvictUpToBlock(s.ctx, &pbservice.EvictRequest{
		BlockNumber: reorgBlock,
	})
	if err != nil {
		return fmt.Errorf("failed to evict from foundational store at block %d: %w", reorgBlock, err)
	}
	s.logger.Debug("evicted foundational store from block", zap.Uint64("reorg_block", reorgBlock))
	return nil
}

// Load is not supported for Badger-backed stores (state is in Badger, not object storage)
func (s *BadgerBackedStore) Load(ctx context.Context, file *FileInfo) error {
	return fmt.Errorf("Load not supported for BadgerBackedStore - state is in Badger")
}

// Save is not supported for Badger-backed stores (state is in Badger, not object storage)
func (s *BadgerBackedStore) Save(endBoundaryBlock uint64) (*FileInfo, *fileWriter, error) {
	return nil, nil, fmt.Errorf("Save not supported for BadgerBackedStore - state is in Badger")
}

// DeleteStore is not supported for Badger-backed stores
func (s *BadgerBackedStore) DeleteStore(ctx context.Context, filename string) error {
	return fmt.Errorf("DeleteStore not supported for BadgerBackedStore")
}

// SizeBytes returns an approximation of the store size (from read cache)
func (s *BadgerBackedStore) SizeBytes() uint64 {
	return s.totalSizeBytes
}

// Length returns the number of keys in kv (accumulated state loaded so far).
func (s *BadgerBackedStore) Length() uint64 {
	return uint64(len(s.kv))
}

// Iter iterates over keys in kv (the accumulated in-memory state).
func (s *BadgerBackedStore) Iter(f func(key string, value []byte) error) error {
	for k, v := range s.kv {
		if err := f(k, v); err != nil {
			return err
		}
	}
	return nil
}
