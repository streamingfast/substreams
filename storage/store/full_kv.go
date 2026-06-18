package store

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/derr"
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
		kvImpl:     newMemoryKVImpl(),
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

func (s *FullKV) QuickLoad(ctx context.Context, atBlock bstream.BlockRef) error {
	if s.quickSaveStore == nil {
		return ErrNoQuickSaveStore
	}

	filename := atBlock.ID() + ".quicksave"
	s.logger.Debug("loading full store state from temporary file", zap.String("fileName", filename), zap.String("module_hash", s.moduleHash), zap.Uint64("block_num", atBlock.Num()), zap.String("block_id", atBlock.ID()))

	r, err := s.quickSaveStore.OpenObject(ctx, filename)
	if err != nil {
		return fmt.Errorf("opening file %q (in module %q): %w", filename, s.moduleHash, err)
	}

	defer r.Close()

	s.totalSizeBytes, err = unmarshalIterInto(s.kvImpl, s.marshaller, r, nil)
	if err != nil {
		return fmt.Errorf("unmarshal store (streaming): %w", err)
	}

	s.logger.Info("quickload: full store loaded",
		zap.String("fileName", filename),
		zap.Int("key_count", s.kvImpl.KeyCount()),
		zap.Uint64("data_size", s.totalSizeBytes),
		zap.Uint64("block_num", atBlock.Num()),
		zap.String("block_id", atBlock.ID()),
	)
	return nil
}

func (s *FullKV) QuickSave(ctx context.Context, atBlockHash string) error {
	if s.quickSaveStore == nil {
		return ErrNoQuickSaveStore
	}
	s.logger.Info("quicksave: writing temporary store state", zap.Object("store", s))

	store := s.quickSaveStore
	filename := atBlockHash + ".quicksave"

	reader := s.marshaller.MarshalStreamIter(s.kvImpl.Iter, nil, int64(s.totalSizeBytes))

	fw := &fileWriter{
		store:    store,
		filename: filename,
		reader:   reader,
	}

	return fw.Write(ctx)
}

var ErrInvalidFullKVFile = errors.New("unmarshal store error") // this error will bubble up to the user

func (s *FullKV) Delete(ctx context.Context, file *FileInfo) error {
	s.Store().DeleteObject(ctx, file.Filename)
	return nil
}

func (s *FullKV) Load(ctx context.Context, file *FileInfo) error {
	s.loadedFrom = file.Filename
	s.logger.Debug("loading full store state from file", zap.String("fileName", file.Filename))

	reader, err := loadStoreStream(ctx, s.objStore, file.Filename)
	if err != nil {
		return fmt.Errorf("load store stream: %w", err)
	}
	defer reader.Close()

	s.totalSizeBytes, err = unmarshalIterInto(s.kvImpl, s.marshaller, reader, nil)
	if err != nil {
		return fmt.Errorf("%w (streaming): %s", ErrInvalidFullKVFile, err.Error())
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	s.logger.Debug("full store loaded",
		zap.String("fileName", file.Filename),
		zap.Int("key_count", s.kvImpl.KeyCount()),
		zap.Uint64("data_size", s.totalSizeBytes),
	)
	return nil
}

// Save is to be called ONLY when we just passed the
// `nextExpectedBoundary` and processed nothing more after that
// boundary.
func (s *FullKV) Save(endBoundaryBlock uint64) (*FileInfo, *fileWriter, error) {
	s.logger.Debug("writing full store state", zap.Object("store", s))

	file := NewCompleteFileInfo(s.name, s.moduleInitialBlock, endBoundaryBlock)

	reader := s.marshaller.MarshalStreamIter(s.kvImpl.Iter, nil, int64(s.totalSizeBytes))

	fw := &fileWriter{
		store:    s.objStore,
		filename: file.Filename,
		reader:   reader,
	}

	s.logger.Debug("saving store",
		zap.String("file_name", file.Filename),
		zap.Object("block_range", file.Range),
	)

	return file, fw, nil
}

func (s *FullKV) Filename() string {
	return s.loadedFrom
}

func (s *FullKV) String() string {
	return fmt.Sprintf("fullKV name %s moduleInitialBlock %d keyCount %d loadedFrom %s deltasCount %d", s.Name(), s.moduleInitialBlock, s.kvImpl.KeyCount(), s.loadedFrom, len(s.deltas))
}

func (s *FullKV) GetSize(ctx context.Context, filename string) (compressedSize uint64, uncompressedSize *uint64, metadata map[string]string, err error) {
	err = derr.RetryContext(ctx, 2, func(ctx context.Context) error {
		r, err := s.objStore.ObjectAttributes(ctx, filename)
		if err != nil {
			return fmt.Errorf("opening file: %w", err)
		}
		compressedSize = uint64(r.Size)
		metadata = r.Metadata

		if ds, ok := r.Metadata["datasize"]; ok {
			size, err := strconv.ParseUint(ds, 10, 64)
			if err != nil {
				s.logger.Info("failed to parse datasize from metadata", zap.Error(err), zap.String("datasize", ds))
				return nil
			}
			uncompressedSize = &size
		}

		return nil
	})
	return
}
