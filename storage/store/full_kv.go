package store

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/streamingfast/dstore"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	"github.com/streamingfast/substreams/storage/store/marshaller"
	"go.uber.org/zap"
)

var _ Store = (*FullKV)(nil)

type FullKV struct {
	*baseStore

	loadedFrom string
}

func (s *FullKV) Store() dstore.Store {
	return s.objStore
}

func (s *FullKV) Marshaller() marshaller.Marshaller {
	return s.marshaller
}

func (s *FullKV) DerivePartialStore(initialBlock uint64) *PartialKV {
	b := &baseStore{
		Config:     s.Config,
		kvOps:      &pbssinternal.Operations{},
		kv:         make(map[string][]byte),
		logger:     s.logger,
		marshaller: marshaller.Default(),
	}
	return &PartialKV{
		baseStore:    b,
		initialBlock: initialBlock,
		seen:         make(map[string]bool),
	}
}

var ErrNoQuickSaveStore = fmt.Errorf("no quick save store")

func (s *FullKV) QuickLoad(ctx context.Context, atBlockHash string) error {
	if s.quickSaveStore == nil {
		return ErrNoQuickSaveStore
	}

	filename := atBlockHash + ".quicksave"
	s.logger.Debug("loading full store state from temporary file", zap.String("fileName", filename), zap.String("module_hash", s.moduleHash))

	r, err := s.quickSaveStore.OpenObject(ctx, filename)
	if err != nil {
		return fmt.Errorf("opening file %q (in module %q): %w", filename, s.moduleHash, err)
	}

	defer r.Close()

	var storeData *marshaller.StoreData
	var size uint64

	if unmarshaller, ok := s.marshaller.(marshaller.StreamMarshaller); ok {
		storeData, size, err = unmarshaller.UnmarshalStream(r, 10*1024*1024) // TODO: bubble up approximation of store size here
		if err != nil {
			return fmt.Errorf("unmarshal store (streaming): %w", err)
		}
	} else {
		data, err := io.ReadAll(r)
		if err != nil {
			return fmt.Errorf("reading data: %w", err)
		}

		storeData, size, err = s.marshaller.Unmarshal(data)
		if err != nil {
			return fmt.Errorf("unmarshal store: %w", err)
		}
	}

	s.kv = storeData.Kv
	s.totalSizeBytes = size
	if s.kv == nil {
		s.kv = make(map[string][]byte)
	}

	s.logger.Debug("full store loaded", zap.String("fileName", filename), zap.Int("key_count", len(s.kv)), zap.Uint64("data_size", size))
	return nil
}

func (s *FullKV) QuickSave(ctx context.Context, atBlockHash string) error {
	if s.quickSaveStore == nil {
		return ErrNoQuickSaveStore
	}
	s.logger.Debug("writing temporary store state", zap.Object("store", s))

	stateData := &marshaller.StoreData{
		Kv: s.kv,
	}

	store := s.quickSaveStore
	filename := atBlockHash + ".quicksave"

	var fw *fileWriter

	// New streaming marshaller support
	if marshaller, ok := s.marshaller.(marshaller.StreamMarshaller); ok && s.totalSizeBytes > 524288 { // we don't use the streaming approach for payloads below 512kiB, it is slower
		reader := marshaller.MarshalStream(stateData, int64(s.totalSizeBytes))

		fw = &fileWriter{
			store:    store,
			filename: filename,
			reader:   reader,
		}
	} else {
		content, err := s.marshaller.Marshal(stateData)
		if err != nil {
			return fmt.Errorf("marshal kv state: %w", err)
		}

		fw = &fileWriter{
			store:    store,
			filename: filename,
			reader:   io.NopCloser(bytes.NewReader(content)),
		}
	}

	return fw.Write(ctx)
}

func (s *FullKV) Load(ctx context.Context, file *FileInfo) error {
	s.loadedFrom = file.Filename
	s.logger.Debug("loading full store state from file", zap.String("fileName", file.Filename))

	var storeData *marshaller.StoreData
	var size uint64

	if unmarshaller, ok := s.marshaller.(marshaller.StreamMarshaller); ok {
		reader, err := loadStoreStream(ctx, s.objStore, file.Filename)
		if err != nil {
			return fmt.Errorf("load store stream: %w", err)
		}
		defer reader.Close()
		storeData, size, err = unmarshaller.UnmarshalStream(reader, 10*1024*1024) // TODO: bubble up approximation of store size here
		if err != nil {
			return fmt.Errorf("unmarshal store (streaming): %w", err)
		}
	} else {
		data, err := loadStore(ctx, s.objStore, file.Filename)
		if err != nil {
			return fmt.Errorf("load full store %s at %s: %w", s.name, file.Filename, err)
		}
		storeData, size, err = s.marshaller.Unmarshal(data)
		if err != nil {
			return fmt.Errorf("unmarshal store: %w", err)
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	//if reqHandler := reqctx.ActiveRequestsHandler(ctx); reqHandler != nil {
	//	reqHandler.AdjustFullKVSize(size)
	//}
	s.kv = storeData.Kv
	s.totalSizeBytes = size
	if s.kv == nil {
		s.kv = make(map[string][]byte)
	}

	s.logger.Debug("full store loaded", zap.String("fileName", file.Filename), zap.Int("key_count", len(s.kv)), zap.Uint64("data_size", size))
	return nil
}

// Save is to be called ONLY when we just passed the
// `nextExpectedBoundary` and processed nothing more after that
// boundary.
func (s *FullKV) Save(endBoundaryBlock uint64) (*FileInfo, *fileWriter, error) {
	s.logger.Debug("writing full store state", zap.Object("store", s))

	stateData := &marshaller.StoreData{
		Kv: s.kv,
	}

	file := NewCompleteFileInfo(s.name, s.moduleInitialBlock, endBoundaryBlock)
	var fw *fileWriter

	var streaming bool
	// New streaming marshaller support
	if marshaller, ok := s.marshaller.(marshaller.StreamMarshaller); ok && s.totalSizeBytes > 524288 { // we don't use the streaming approach for payloads below 512kiB, it is slower
		reader := marshaller.MarshalStream(stateData, int64(s.totalSizeBytes))

		fw = &fileWriter{
			store:    s.objStore,
			filename: file.Filename,
			reader:   reader,
		}
		streaming = true
	} else {
		content, err := s.marshaller.Marshal(stateData)
		if err != nil {
			return nil, nil, fmt.Errorf("marshal kv state: %w", err)
		}

		fw = &fileWriter{
			store:    s.objStore,
			filename: file.Filename,
			reader:   io.NopCloser(bytes.NewReader(content)),
		}
	}

	s.logger.Debug("saving store",
		zap.String("file_name", file.Filename),
		zap.Object("block_range", file.Range),
		zap.Bool("streaming", streaming),
	)

	return file, fw, nil
}

func (s *FullKV) Filename() string {
	return s.loadedFrom
}

func (s *FullKV) String() string {
	return fmt.Sprintf("fullKV name %s moduleInitialBlock %d keyCount %d loadedFrom %s deltasCount %d", s.Name(), s.moduleInitialBlock, len(s.kv), s.loadedFrom, len(s.deltas))
}
