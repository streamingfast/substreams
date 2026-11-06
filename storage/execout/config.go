package execout

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/streamingfast/derr"
	"github.com/streamingfast/dstore"
	"go.uber.org/zap"

	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

// MetadataDataSize is the object-store metadata key under which the uncompressed size of
// an execution output file's payloads is recorded. Same key as the one used for store
// snapshots.
const MetadataDataSize = "datasize"

// MetadataItemCount is the object-store metadata key under which the number of items an
// execution output file holds is recorded. One item is one block the module produced output
// for, and one `BlockScopedData` message on the wire — which is not the same as one block of
// the range: a module gated by a block index runs, and is sent, only on matching blocks.
const MetadataItemCount = "itemcount"

// setMetadataTimeout bounds the fire-and-forget metadata write so it can never hang
// indefinitely.
const setMetadataTimeout = 30 * time.Second

// metadataRewriteSchemes lists the dstore backends whose SetMetadata rewrites the object
// instead of updating it in place. S3 exposes no metadata-update API, so dstore implements
// it as a CopyObject onto itself: a full server-side copy of every execution output file,
// which also resets LastModified and adds a version when versioning is on. Too much to pay
// on every segment for a size hint the reader can recompute, so those backends are skipped.
var metadataRewriteSchemes = map[string]bool{"s3": true}

type Config struct {
	name               string
	moduleHash         string
	extendedModuleHash string // used for storing errors
	objStore           dstore.Store
	errStore           dstore.Store

	modKind            pbsubstreams.ModuleKind
	moduleInitialBlock uint64

	logger *zap.Logger
}

func NewConfig(name string, moduleInitialBlock uint64, modKind pbsubstreams.ModuleKind, moduleHash string, extendedModuleHash string, baseStore dstore.Store, logger *zap.Logger) (*Config, error) {
	subName := fmt.Sprintf("%s/outputs", moduleHash)
	if modKind == pbsubstreams.ModuleKindBlockIndex {
		subName = fmt.Sprintf("%s/index", moduleHash)
	}

	subStore, err := baseStore.SubStore(subName)
	if err != nil {
		return nil, fmt.Errorf("creating sub store: %w", err)
	}

	errStore, err := baseStore.SubStore(moduleHash)
	if err != nil {
		return nil, fmt.Errorf("creating err store: %w", err)
	}

	return &Config{
		name:               name,
		objStore:           subStore,
		errStore:           errStore,
		modKind:            modKind,
		moduleInitialBlock: moduleInitialBlock,
		moduleHash:         moduleHash,
		extendedModuleHash: extendedModuleHash,
		logger:             logger.With(zap.String("module", name)),
	}, nil
}

func (c *Config) WriteDeterministicError(ctx context.Context, atBlock uint64, err error) error {
	// Never write deterministic errors if context is cancelled, it cannot be deterministic in this case,
	// safeguard against wrong error handling over time.
	if ctx.Err() != nil {
		return nil
	}

	r := strings.NewReader(err.Error())
	// The trailing unix timestamp lets the reader expire errors older than a configured
	// duration; errors without a timestamp are treated as expired and deleted on read.
	return c.errStore.WriteObject(ctx, fmt.Sprintf("errors.%010d.%s.%d", atBlock, c.extendedModuleHash, time.Now().Unix()), r)
}

func (c *Config) NewFile(targetRange *block.Range) *File {
	return &File{
		moduleName: c.name,
		store:      c.objStore,
		Range:      targetRange,
		logger:     c.logger,
	}
}

func (c *Config) OpenFileReader(ctx context.Context, targetRange *block.Range) (FileReader, error) {
	return OpenFileReader(ctx, c.objStore, c.logger, targetRange, c.name)
}

func (c *Config) NewFileWriter(ctx context.Context, targetRange *block.Range) FileWriter {
	return NewFileWriter(ctx, c.objStore, c.logger, targetRange, c.name)
}

// OutputStats returns the total uncompressed size of the module payloads held in the output
// file for the given range, and how many items it holds.
func (c *Config) OutputStats(ctx context.Context, targetRange *block.Range) (size, items uint64, fromMetadata bool, err error) {
	return OutputStats(ctx, c.objStore, c.logger, targetRange, c.name)
}

// OutputStats returns the total uncompressed size of the module payloads held in the output
// file for the given range, and how many items it holds. The item count is what a consumer
// of that segment receives as `BlockScopedData` messages, so it is what the per-message
// overhead of a real request has to be multiplied by.
//
// It prefers the figures recorded as object metadata when the file was written, which cost a
// single attributes lookup; `fromMetadata` then reports true. When the attributes carry
// neither — a file written before that metadata existed, a backend that does not support
// metadata at all, or one where recording it is too expensive (see metadataRewriteSchemes) —
// the file is read and its items added up, which is much more expensive.
//
// A failing attributes lookup is not one of those fallbacks: it is reported as
// [dstore.ErrNotFound], because backends disagree on how they signal a missing object (only
// GCS maps it to that error) and there is nothing to measure either way, so the caller gets
// a single condition to handle.
func OutputStats(ctx context.Context, objStore dstore.Store, logger *zap.Logger, targetRange *block.Range, moduleName string) (size, items uint64, fromMetadata bool, err error) {
	filename := computeDBinFilename(targetRange.StartBlock, targetRange.ExclusiveEndBlock)

	attrs, err := objStore.ObjectAttributes(ctx, filename)
	if err != nil {
		return 0, 0, false, fmt.Errorf("%w: getting object attributes of %q: %w", dstore.ErrNotFound, filename, err)
	}
	// Both keys are written together, so a file carrying only one of them predates the other
	// and has to be read anyway; there is no point in reporting half the answer.
	if size, sizeFound := parseMetadataCount(logger, filename, attrs.Metadata, MetadataDataSize); sizeFound {
		if items, itemsFound := parseMetadataCount(logger, filename, attrs.Metadata, MetadataItemCount); itemsFound {
			return size, items, true, nil
		}
	}

	reader, err := OpenFileReader(ctx, objStore, logger, targetRange, moduleName)
	if err != nil {
		return 0, 0, false, fmt.Errorf("opening %q: %w", filename, err)
	}
	defer reader.Close()

	for item, err := range reader.Iter() {
		if err != nil {
			return 0, 0, false, fmt.Errorf("reading %q: %w", filename, err)
		}
		size += uint64(len(item.Payload))
		items++
	}

	return size, items, false, nil
}

func parseMetadataCount(logger *zap.Logger, filename string, metadata map[string]string, key string) (uint64, bool) {
	raw, found := metadata[key]
	if !found {
		return 0, false
	}

	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		logger.Info("cannot parse execution output metadata, falling back to reading the file",
			zap.String("filename", filename), zap.String("key", key), zap.String("value", raw), zap.Error(err))
		return 0, false
	}
	return value, true
}

func (c *Config) Name() string                        { return c.name }
func (c *Config) ModuleKind() pbsubstreams.ModuleKind { return c.modKind }
func (c *Config) ModuleInitialBlock() uint64          { return c.moduleInitialBlock }

func (c *Config) ListSnapshotFiles(ctx context.Context, from uint64, to uint64) (files FileInfos, err error) {
	err = derr.RetryContext(ctx, 3, func(ctx context.Context) error {
		// We must reset accumulated files between each retry
		files = nil

		fromFilename := computeDBinFilename(from, 0)
		toFilename := computeDBinFilename(to, 0)

		err := c.objStore.WalkFromTo(ctx, "", fromFilename, toFilename, func(filename string) (err error) {
			var fileInfo *FileInfo

			switch c.modKind {
			case pbsubstreams.ModuleKindBlockIndex:
				fileInfo, err = parseIndexFileName(filename)
			case pbsubstreams.ModuleKindMap:
				fileInfo, err = parseExecoutFileName(filename)
			default:
				return fmt.Errorf("wrong module kind: %v", c.modKind)
			}
			if err != nil {
				c.logger.Warn("seen exec output file that we don't know how to parse", zap.String("filename", filename), zap.Error(err))
				return nil
			}
			files = append(files, fileInfo)
			return nil
		})
		if err == dstore.StopIteration {
			return nil
		}
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("walking files: %w", err)
	}

	return files, nil
}
