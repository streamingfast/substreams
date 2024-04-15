package index

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"google.golang.org/protobuf/proto"

	pbindexes "github.com/streamingfast/substreams/storage/index/pb"

	"github.com/RoaringBitmap/roaring/roaring64"
	"github.com/streamingfast/derr"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/block"
	"go.uber.org/zap"
)

type File struct {
	blockRange *block.Range
	store      dstore.Store
	moduleName string
	indexes    map[string]*roaring64.Bitmap
	logger     *zap.Logger
}

func NewFile(baseStore dstore.Store, moduleHash string, moduleName string, logger *zap.Logger, blockRange *block.Range) (*File, error) {
	subStore, err := baseStore.SubStore(fmt.Sprintf("%s/index", moduleHash))
	if err != nil {
		return nil, fmt.Errorf("creating sub store: %w", err)
	}
	return &File{
		blockRange: blockRange,
		store:      subStore,
		moduleName: moduleName,
		logger:     logger,
	}, nil
}

func (f *File) Set(indexes map[string]*roaring64.Bitmap) {
	f.indexes = indexes
}

func convertIndexesMapToBytes(indexes map[string]*roaring64.Bitmap) (map[string][]byte, error) {
	out := make(map[string][]byte, len(indexes))
	for key, value := range indexes {
		valueToBytes, err := value.ToBytes()
		if err != nil {
			return nil, fmt.Errorf("converting bitmap to bytes: %w", err)
		}
		out[key] = valueToBytes
	}
	return out, nil
}

func (f *File) Save(ctx context.Context) error {
	filename := f.Filename()
	convertedIndexes, err := convertIndexesMapToBytes(f.indexes)
	if err != nil {
		return fmt.Errorf("converting indexes to bytes: %w", err)
	}
	pbIndexesMap := pbindexes.Map{Indexes: convertedIndexes}
	cnt, err := proto.Marshal(&pbIndexesMap)
	if err != nil {
		return fmt.Errorf("marshalling indexes: %w", err)
	}

	f.logger.Info("writing indexes file", zap.String("filename", filename))
	return derr.RetryContext(ctx, 5, func(ctx context.Context) error {
		reader := bytes.NewReader(cnt)
		err := f.store.WriteObject(ctx, filename, reader)
		return err
	})
}

func (f *File) Load(ctx context.Context) error {
	pbIndexesMap := pbindexes.Map{}

	filename := f.Filename()
	file, err := f.store.OpenObject(ctx, filename)
	if err != nil {
		return err
	}
	content, err := io.ReadAll(file)
	if err != nil {
		return err
	}

	err = proto.Unmarshal(content, &pbIndexesMap)
	if err != nil {
		return err
	}

	f.indexes = make(map[string]*roaring64.Bitmap)

	for k, v := range pbIndexesMap.Indexes {
		f.indexes[k] = roaring64.New()
		_, err := f.indexes[k].FromUnsafeBytes(v)
		if err != nil {
			return err
		}
	}

	return nil
}

func (f *File) Print() {
	for k, v := range f.indexes {
		fmt.Printf("%s: %v\n", k, v.ToArray())
	}
}

func (f *File) Filename() string {
	return computeDBinFilename(f.blockRange.StartBlock, f.blockRange.ExclusiveEndBlock)
}

func computeDBinFilename(startBlock, stopBlock uint64) string {
	return fmt.Sprintf("%010d-%010d.index", startBlock, stopBlock)
}