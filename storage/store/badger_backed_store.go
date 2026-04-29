package store

import (
	"context"
	"fmt"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dstore"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	pbmodel "github.com/streamingfast/substreams-foundational-store/pb/sf/substreams/foundational-store/model/v2"
	pbservice "github.com/streamingfast/substreams-foundational-store/pb/sf/substreams/foundational-store/service/v2"
	"github.com/streamingfast/substreams/storage/store/marshaller"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/anypb"
)

var _ Store = (*BadgerBackedStore)(nil)

// BadgerBackedStore implements the Store interface backed by a Badger database via the foundational store service
// It keeps all existing delta/ops/ordinal machinery from baseStore but delegates persistence to Badger
type BadgerBackedStore struct {
	*baseStore
	
	forkAwareClient pbservice.StoreClient // gRPC client to foundational store server
	blockNum        uint64                // current block number being processed
	readCache       map[string][]byte     // local read cache for intra-block reads
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
		kv:         make(map[string][]byte), // Used as read cache only
		logger:     logger,
		marshaller: marshaller.Default(),
	}

	return &BadgerBackedStore{
		baseStore:       b,
		forkAwareClient: forkAwareClient,
		readCache:       make(map[string][]byte),
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

// Flush processes buffered operations, produces deltas, and sends entries to the foundational store
func (s *BadgerBackedStore) Flush() error {
	// First, call baseStore.Flush() to produce deltas from kvOps
	if err := s.baseStore.Flush(); err != nil {
		return fmt.Errorf("base store flush failed: %w", err)
	}

	// Convert kvOps to foundational store entries with policy and value type annotations
	entries, err := s.convertOpsToEntries()
	if err != nil {
		return fmt.Errorf("failed to convert ops to entries: %w", err)
	}

	// Send entries to the foundational store via gRPC
	if len(entries) > 0 {
		sinkEntries := &pbmodel.SinkEntries{
			Entries: entries,
		}
		
		req := &pbservice.SetRequest{
			SinkEntries: sinkEntries,
			BlockNumber: s.blockNum,
		}
		
		_, err = s.forkAwareClient.SetAll(context.Background(), req)
		if err != nil {
			return fmt.Errorf("failed to send entries to foundational store: %w", err)
		}
		
		s.logger.Debug("flushed entries to foundational store",
			zap.Uint64("block_num", s.blockNum),
			zap.Int("entry_count", len(entries)),
		)
	}

	return nil
}

// convertOpsToEntries converts kvOps to foundational store Entry format with update policy
func (s *BadgerBackedStore) convertOpsToEntries() ([]*pbmodel.Entry, error) {
	var entries []*pbmodel.Entry

	for _, op := range s.kvOps.Operations {
		policy, err := s.operationToUpdatePolicy(op.Type)
		if err != nil {
			return nil, err
		}

		// Create a simple bytes wrapper for the value
		// The Any type URL will be empty, and the value will be raw bytes
		anyValue := &anypb.Any{
			Value: op.Value,
		}

		entry := &pbmodel.Entry{
			Key: &pbmodel.Key{
				Bytes: []byte(op.Key),
			},
			Value:        anyValue,
			UpdatePolicy: policy,
			ValueType:    s.valueType,
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

// operationToUpdatePolicy maps operation types to UpdatePolicy enum
func (s *BadgerBackedStore) operationToUpdatePolicy(opType pbssinternal.Operation_Type) (pbmodel.UpdatePolicy, error) {
	switch opType {
	case pbssinternal.Operation_SET, pbssinternal.Operation_SET_BYTES:
		return pbmodel.UpdatePolicy_UPDATE_POLICY_SET, nil
	case pbssinternal.Operation_SET_IF_NOT_EXISTS, pbssinternal.Operation_SET_BYTES_IF_NOT_EXISTS:
		return pbmodel.UpdatePolicy_UPDATE_POLICY_SET_IF_NOT_EXISTS, nil
	case pbssinternal.Operation_APPEND:
		return pbmodel.UpdatePolicy_UPDATE_POLICY_APPEND, nil
	case pbssinternal.Operation_SUM_BIG_INT, pbssinternal.Operation_SUM_INT64, 
		 pbssinternal.Operation_SUM_FLOAT64, pbssinternal.Operation_SUM_BIG_DECIMAL:
		return pbmodel.UpdatePolicy_UPDATE_POLICY_ADD, nil
	case pbssinternal.Operation_SET_MIN_BIG_INT, pbssinternal.Operation_SET_MIN_INT64,
		 pbssinternal.Operation_SET_MIN_FLOAT64, pbssinternal.Operation_SET_MIN_BIG_DECIMAL:
		return pbmodel.UpdatePolicy_UPDATE_POLICY_MIN, nil
	case pbssinternal.Operation_SET_MAX_BIG_INT, pbssinternal.Operation_SET_MAX_INT64,
		 pbssinternal.Operation_SET_MAX_FLOAT64, pbssinternal.Operation_SET_MAX_BIG_DECIMAL:
		return pbmodel.UpdatePolicy_UPDATE_POLICY_MAX, nil
	case pbssinternal.Operation_SET_SUM_INT64, pbssinternal.Operation_SET_SUM_FLOAT64,
		 pbssinternal.Operation_SET_SUM_BIG_INT, pbssinternal.Operation_SET_SUM_BIG_DECIMAL:
		return pbmodel.UpdatePolicy_UPDATE_POLICY_SET_SUM, nil
	default:
		return 0, fmt.Errorf("unsupported operation type: %v", opType)
	}
}

// Reset clears the kvOps, deltas, and read cache
func (s *BadgerBackedStore) Reset() {
	s.baseStore.Reset()
	s.readCache = make(map[string][]byte)
}

// GetFirst retrieves the first value with a given prefix from Badger or read cache
func (s *BadgerBackedStore) GetFirst(prefix string) ([]byte, bool) {
	// Check read cache first for intra-block reads
	for k, v := range s.readCache {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			return v, true
		}
	}

	// Query foundational store via gRPC
	req := &pbservice.GetRequest{
		Keys: []*pbmodel.Key{{Bytes: []byte(prefix)}},
		BlockNumber: s.blockNum,
	}

	resp, err := s.forkAwareClient.GetFirst(context.Background(), req)
	if err != nil {
		s.logger.Warn("failed to get first from foundational store", zap.Error(err))
		return nil, false
	}

	if len(resp.Entries.Entries) == 0 || resp.Entries.Entries[0].Code != pbmodel.ResponseCode_RESPONSE_CODE_FOUND {
		return nil, false
	}

	entry := resp.Entries.Entries[0].Entry
	key := string(entry.Key.Bytes)
	value := entry.Value.Value

	// Cache the result
	s.readCache[key] = value

	return value, true
}

// GetLast retrieves the last value with a given prefix from Badger or read cache
func (s *BadgerBackedStore) GetLast(prefix string) ([]byte, bool) {
	// For Badger-backed stores, GetLast would need special handling
	// For now, delegate to GetFirst (can be optimized later)
	return s.GetFirst(prefix)
}

// GetAt retrieves a value at a specific ordinal (intra-block reads from local cache)
func (s *BadgerBackedStore) GetAt(ord uint64, key string) ([]byte, bool) {
	// Intra-block reads must be served from local kv cache (baseStore handles this via deltas)
	return s.baseStore.GetAt(ord, key)
}

// HasFirst checks if a key with the given prefix exists
func (s *BadgerBackedStore) HasFirst(prefix string) bool {
	_, found := s.GetFirst(prefix)
	return found
}

// HasLast checks if a key with the given prefix exists
func (s *BadgerBackedStore) HasLast(prefix string) bool {
	_, found := s.GetLast(prefix)
	return found
}

// HasAt checks if a key exists at a specific ordinal
func (s *BadgerBackedStore) HasAt(ord uint64, key string) bool {
	_, found := s.GetAt(ord, key)
	return found
}

// FlushToBadger signals the foundational store to flush entries up to LIB to Badger
func (s *BadgerBackedStore) FlushToBadger(lib uint64) error {
	req := &pbservice.FlushRequest{
		BlockNumber: lib,
		IfNotExist:  false,
	}
	
	_, err := s.forkAwareClient.FlushUpToBlock(context.Background(), req)
	if err != nil {
		return fmt.Errorf("failed to flush foundational store to block %d: %w", lib, err)
	}
	
	s.logger.Debug("flushed foundational store up to block", zap.Uint64("lib", lib))
	return nil
}

// EvictFromBadger signals the foundational store to evict entries from the specified block onwards (fork rollback)
func (s *BadgerBackedStore) EvictFromBadger(reorgBlock uint64) error {
	req := &pbservice.EvictRequest{
		BlockNumber: reorgBlock,
	}
	
	_, err := s.forkAwareClient.EvictUpToBlock(context.Background(), req)
	if err != nil {
		return fmt.Errorf("failed to evict from foundational store at block %d: %w", reorgBlock, err)
	}
	
	s.logger.Debug("evicted foundational store from block", zap.Uint64("reorg_block", reorgBlock))
	return nil
}

// DerivePartialStore creates a partial store (not supported for Badger-backed stores)
func (s *BadgerBackedStore) DerivePartialStore(initialBlock uint64) *PartialKV {
	panic("DerivePartialStore not supported for BadgerBackedStore")
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

// QuickLoad is not supported for Badger-backed stores
func (s *BadgerBackedStore) QuickLoad(ctx context.Context, atBlock bstream.BlockRef) error {
	return fmt.Errorf("QuickLoad not supported for BadgerBackedStore")
}

// QuickSave is not supported for Badger-backed stores
func (s *BadgerBackedStore) QuickSave(ctx context.Context, atBlockHash string) error {
	return fmt.Errorf("QuickSave not supported for BadgerBackedStore")
}

// Length returns the number of keys in the read cache
func (s *BadgerBackedStore) Length() uint64 {
	return uint64(len(s.readCache))
}

// Iter iterates over all keys in the read cache
func (s *BadgerBackedStore) Iter(f func(key string, value []byte) error) error {
	for k, v := range s.readCache {
		if err := f(k, v); err != nil {
			return err
		}
	}
	return nil
}
