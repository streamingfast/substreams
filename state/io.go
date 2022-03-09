package state

import (
	"bytes"
	"context"
	"fmt"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/merger/bundle"
	"io/ioutil"
	"strconv"
	"strings"
)

type IOFactory interface {
	New(name string) StateIO
}

//type DiskStateIOFactory struct {
//	dataFolder string
//}
//
//func NewDiskStateIOFactory(folder string) IOFactory {
//	return &DiskStateIOFactory{dataFolder: folder}
//}
//
//func (f *DiskStateIOFactory) New(name string) StateIO {
//	return &DiskStateIO{
//		name:       name,
//		dataFolder: f.dataFolder,
//	}
//}

type StoreStateIOFactory struct {
	store dstore.Store
}

func NewStoreStateIOFactory(store dstore.Store) IOFactory {
	return &StoreStateIOFactory{store: store}
}

func (f *StoreStateIOFactory) New(name string) StateIO {
	return &StoreStateIO{
		name:  name,
		store: f.store,
	}
}

type StateIO interface {
	WriteState(ctx context.Context, content []byte, block *bstream.Block) error
	ReadState(ctx context.Context, blockNum uint64) ([]byte, error)
}

type StoreStateIO struct {
	name  string
	store dstore.Store
}

func (s *StoreStateIO) WriteState(ctx context.Context, content []byte, block *bstream.Block) error {
	return s.store.WriteObject(ctx, GetStateFileName(s.name, block), bytes.NewBuffer(content))
}

func (s *StoreStateIO) ReadState(ctx context.Context, blockNum uint64) ([]byte, error) {
	relativeStartBlock := (blockNum / 100) * 100
	block := &bstream.Block{Number: relativeStartBlock}

	objectName := GetStateFileName(s.name, block)
	obj, err := s.store.OpenObject(ctx, objectName)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", objectName, err)
	}

	data, err := ioutil.ReadAll(obj)
	return data, err
}

func GetStateFileName(name string, block *bstream.Block) string {
	blockNum := (block.Num() / 100) * 100
	return fmt.Sprintf("%d-%s.kv", blockNum, name)
}

func mustParseFileToOneBlockFile(path string) *bundle.OneBlockFile {
	trimmedPath := strings.TrimSuffix(path, ".delta")
	parts := strings.Split(trimmedPath, "-")
	if len(parts) != 5 {
		panic("invalid path")
	}

	uint64ToPtr := func(num uint64) *uint64 {
		var p *uint64
		p = new(uint64)
		*p = num
		return p
	}

	blockId := parts[2]
	blockPrevId := parts[3]
	blockNum, err := strconv.Atoi(parts[0])
	if err != nil {
		panic("invalid block num")
	}
	blockLibNum, err := strconv.Atoi(parts[1])
	if err != nil {
		panic("invalid prev block num")
	}

	return &bundle.OneBlockFile{
		CanonicalName: path,
		ID:            blockId,
		Num:           uint64(blockNum),
		InnerLibNum:   uint64ToPtr(uint64(blockLibNum)),
		PreviousID:    blockPrevId,
		Filenames:     map[string]struct{}{},
	}
}

func mustBlockToOneBlockFile(name string, block *bstream.Block) *bundle.OneBlockFile {
	getUint64Pointer := func(n uint64) *uint64 {
		var ptr *uint64
		ptr = new(uint64)
		*ptr = n
		return ptr
	}

	filename := GetDeltaFileName(name, block)

	return &bundle.OneBlockFile{
		CanonicalName: filename,
		Filenames: map[string]struct{}{
			filename: {},
		},
		ID:          block.ID(),
		PreviousID:  block.PreviousID(),
		BlockTime:   block.Time(),
		Num:         block.Num(),
		InnerLibNum: getUint64Pointer(block.LibNum),
	}
}

func mustOneBlockFileToBlock(obf *bundle.OneBlockFile) *bstream.Block {
	return &bstream.Block{
		Id:         obf.ID,
		Number:     obf.Num,
		PreviousId: obf.PreviousID,
		Timestamp:  obf.BlockTime,
		LibNum:     obf.LibNum(),
	}
}
