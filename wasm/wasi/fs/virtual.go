package fs

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"strconv"
	"strings"
	"time"

	"github.com/streamingfast/substreams/wasm"
)

type Virtual struct {
	ctx context.Context
}

func NewVirtualFs(ctx context.Context) *Virtual {
	return &Virtual{
		ctx: ctx,
	}
}

func (v *Virtual) Open(name string) (fs.File, error) {
	return NewVirtualFile(v.ctx, name)
}

type VirtualFile struct {
	ctx       context.Context
	name      string
	nameParts []string
	Remaining []byte
	Loaded    bool
}

func NewVirtualFile(ctx context.Context, name string) (*VirtualFile, error) {
	//if !strings.HasPrefix(name, "/sys/") {
	//	fmt.Printf("invalid file name %q should start with /sys/\n", name)
	//	return NewVirtualFile(ctx, name)
	//}
	return &VirtualFile{
		ctx:       ctx,
		name:      name,
		nameParts: strings.Split(name, "/"),
	}, nil
}

func (v *VirtualFile) Stat() (fs.FileInfo, error) {
	size := int64(1)
	return NewVirtualFileInfo(v.name, size), nil
}

func (v *VirtualFile) Read(bytes []byte) (sent int, err error) {
	data := v.Remaining
	if !v.Loaded {
		data, err = dataForFile(v.ctx, v.nameParts[1:]) //skip /sys
		if err != nil {
			//sysfs.DirFS()
			return 0, fmt.Errorf("getting data for file %q: %w", v.name, err)
		}
		v.Loaded = true
	}

	if len(data) == 0 {
		return 0, io.EOF
	}

	toSend := len(bytes)
	if len(data) <= toSend {
		copy(bytes, data)
	} else {
		copy(bytes, data[:toSend])
		v.Remaining = data[toSend:]
	}

	return len(data), nil
}

func (v *VirtualFile) Close() error {
	return nil
}

type VirtualFileInfo struct {
	name string
	size int64
}

func NewVirtualFileInfo(name string, size int64) *VirtualFileInfo {
	return &VirtualFileInfo{
		name: name,
		size: size,
	}
}

func (v VirtualFileInfo) Name() string {
	return v.name
}

func (v VirtualFileInfo) Size() int64 {
	return v.size
}

func (v VirtualFileInfo) Mode() fs.FileMode {
	return fs.ModeDir
}

func (v VirtualFileInfo) ModTime() time.Time {
	return time.Now()
}

func (v VirtualFileInfo) IsDir() bool {
	//TODO implement me
	panic("implement me")
}

func (v VirtualFileInfo) Sys() any {
	return nil
}

func dataForFile(ctx context.Context, parts []string) ([]byte, error) {
	switch parts[0] {
	case "stores":
		return stateDataForFile(ctx, parts[1:]) //skip store
	}

	return nil, nil
}

func stateDataForFile(ctx context.Context, parts []string) ([]byte, error) {
	indexString := parts[0]
	index, err := strconv.Atoi(indexString)
	if err != nil {
		return nil, fmt.Errorf("parsing index %q: %w", indexString, err)
	}
	verb := parts[1]

	call := wasm.FromContext(ctx)

	switch verb {
	case "read":
		fla := parts[2]
		switch fla {
		case "last":
			key := parts[3]
			value, _ := call.DoGetLast(index, key)
			return value, nil
		case "first":
			key := parts[3]
			value, _ := call.DoGetFirst(index, key)
			return value, nil
		case "at":
			ordinalString := parts[3]
			ordinal, err := strconv.Atoi(ordinalString)
			if err != nil {
				return nil, fmt.Errorf("parsing ordinal %q: %w", ordinalString, err)
			}
			key := parts[4]
			value, _ := call.DoGetAt(index, uint64(ordinal), key)
			return value, nil
		}
	default:
		return nil, fmt.Errorf("unknown verb %q", verb)
	}
	return nil, nil
}
