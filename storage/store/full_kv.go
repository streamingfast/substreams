package store

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

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

	// quickLoadReader holds the quicksave object opened by QuickLoadOpen until
	// QuickLoadFinish streams it (or QuickLoadClose discards it). Set/read only
	// from the request goroutine, so no synchronization is needed.
	quickLoadReader   io.ReadCloser
	quickLoadFilename string
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

// QuickLoad opens and streams the quicksave file in one call. It is equivalent
// to QuickLoadOpen followed by QuickLoadFinish and exists for callers that don't
// need to interleave work between the (cheap) open and the (slow) streaming decode.
func (s *FullKV) QuickLoad(ctx context.Context, atBlock bstream.BlockRef) error {
	if err := s.QuickLoadOpen(ctx, atBlock); err != nil {
		return err
	}
	return s.QuickLoadFinish(ctx, atBlock)
}

// QuickLoadOpen opens the quicksave object for atBlock, confirming it exists and
// is readable, but does not stream it yet. Pairing this with a later QuickLoadFinish
// lets the caller do work (e.g. send the client its session/trace-id and keepalives)
// during the potentially slow streaming decode instead of before it.
func (s *FullKV) QuickLoadOpen(ctx context.Context, atBlock bstream.BlockRef) error {
	if s.quickSaveStore == nil {
		return ErrNoQuickSaveStore
	}

	filename := atBlock.ID() + ".quicksave"
	s.logger.Debug("loading full store state from temporary file", zap.String("fileName", filename), zap.String("module_hash", s.moduleHash), zap.Uint64("block_num", atBlock.Num()), zap.String("block_id", atBlock.ID()))

	r, err := s.quickSaveStore.OpenObject(ctx, filename)
	if err != nil {
		return fmt.Errorf("opening file %q (in module %q): %w", filename, s.moduleHash, err)
	}

	s.quickLoadReader = r
	s.quickLoadFilename = filename
	return nil
}

// QuickLoadFinish streams the object opened by QuickLoadOpen into the store and
// closes it. It must be called after a successful QuickLoadOpen.
func (s *FullKV) QuickLoadFinish(ctx context.Context, atBlock bstream.BlockRef) error {
	if s.quickLoadReader == nil {
		return fmt.Errorf("quickload finish called without a prior successful open for store %q", s.name)
	}

	r := s.quickLoadReader
	filename := s.quickLoadFilename
	s.quickLoadReader = nil
	s.quickLoadFilename = ""
	defer r.Close()

	start := time.Now()
	var err error
	s.totalSizeBytes, err = unmarshalIterInto(ctx, s.kvImpl, s.marshaller, r, nil)
	if err != nil {
		return fmt.Errorf("unmarshal store (streaming): %w", err)
	}

	s.logger.Info("quickload: full store loaded",
		zap.String("fileName", filename),
		zap.Int("key_count", s.kvImpl.KeyCount()),
		zap.Uint64("data_size", s.totalSizeBytes),
		zap.Uint64("block_num", atBlock.Num()),
		zap.String("block_id", atBlock.ID()),
		zap.Duration("load_duration", time.Since(start)),
	)
	return nil
}

// QuickLoadClose discards a reader opened by QuickLoadOpen without streaming it,
// used to release resources when a sibling store's open failed and the whole
// quickload is being abandoned.
func (s *FullKV) QuickLoadClose() {
	if s.quickLoadReader != nil {
		_ = s.quickLoadReader.Close()
		s.quickLoadReader = nil
		s.quickLoadFilename = ""
	}
}

func (s *FullKV) QuickSave(ctx context.Context, atBlockHash string) error {
	if s.quickSaveStore == nil {
		return ErrNoQuickSaveStore
	}
	start := time.Now()
	s.logger.Info("quicksave: writing temporary store state", zap.Object("store", s))

	store := s.quickSaveStore
	filename := atBlockHash + ".quicksave"

	snap, err := s.kvImpl.Snapshot()
	if err != nil {
		return fmt.Errorf("snapshotting store %q: %w", s.name, err)
	}
	reader := s.marshaller.MarshalStreamSnapshot(snap, nil)

	fw := &fileWriter{
		store:    store,
		filename: filename,
		reader:   reader,
	}

	if err := fw.Write(ctx); err != nil {
		return err
	}

	s.logger.Info("quicksave: temporary store state written", zap.String("fileName", filename), zap.Int("key_count", s.kvImpl.KeyCount()), zap.Uint64("data_size", s.totalSizeBytes), zap.Duration("save_duration", time.Since(start)))
	return nil
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

	s.totalSizeBytes, err = unmarshalIterInto(ctx, s.kvImpl, s.marshaller, reader, nil)
	if err != nil {
		// A canceled/expired context aborts the streaming read and would
		// otherwise be reported as file corruption, tricking callers into
		// deleting a perfectly valid store file. Surface the cancellation
		// instead so it never gets misclassified.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
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
//
// Locking / ops note: Save opens a Snapshot and returns a fileWriter that the
// caller uploads to object storage LATER; the snapshot stays open (and is only
// released by fileWriter.Write -> reader.Close -> snap.Close) for the whole
// upload. On the mmap backend the snapshot holds the exclusive snapMu the whole
// time, so every write to this store AND Close() block until the upload
// finishes. This is intentional (the store must be frozen while it is
// serialized), and is bounded by ctx cancellation of the Write. But be aware: a
// multi-GB save over slow/stuck object storage wedges this store until the
// write's deadline. The memory backend copies its snapshot up front (no
// write-gate) but is otherwise equivalent in externally observable behaviour.
func (s *FullKV) Save(endBoundaryBlock uint64) (*FileInfo, *fileWriter, error) {
	s.logger.Debug("writing full store state", zap.Object("store", s))

	file := NewCompleteFileInfo(s.name, s.moduleInitialBlock, endBoundaryBlock)

	snap, err := s.kvImpl.Snapshot()
	if err != nil {
		return nil, nil, fmt.Errorf("snapshotting store %q: %w", s.name, err)
	}
	reader := s.marshaller.MarshalStreamSnapshot(snap, nil)

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

// setMetadataTimeout bounds the fire-and-forget metadata write so it can never
// hang indefinitely.
const setMetadataTimeout = 30 * time.Second

// SetMetadataDetached writes store metadata in the background WITHOUT retaining
// the FullKV or riding the caller's request context.
//
// Callers previously spawned `go func(){ fullKV.Store().SetMetadata(reqCtx, ...) }()`,
// which (1) captured the whole multi-GB fullKV until the write returned, pinning
// it long after the request could otherwise release it, and (2) ran on the
// request ctx, so a canceled/finished request killed the write. This helper
// takes only the store, filename and name, and runs on a bounded background
// context, so the write survives request cancellation and pins nothing.
func SetMetadataDetached(metaStore dstore.Store, filename, storeName string, metadata map[string]string, logger *zap.Logger) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), setMetadataTimeout)
		defer cancel()
		if err := metaStore.SetMetadata(ctx, filename, metadata); err != nil {
			logger.Warn("failed to set metadata on store",
				zap.String("store_name", storeName),
				zap.String("filename", filename),
				zap.Error(err))
		}
	}()
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
