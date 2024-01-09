package fs

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"math/big"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/streamingfast/substreams/wasm"
)

type Virtual struct {
	ctx context.Context

	resultOnce sync.Once
	result     []byte
}

func (v *Virtual) Result() []byte {
	return v.result
}

func NewVirtualFs(ctx context.Context) *Virtual {
	return &Virtual{
		ctx: ctx,
	}
}

func (v *Virtual) Open(name string) (fs.File, error) {
	return newVirtualFile(v.ctx, name, func(b []byte) {
		v.resultOnce.Do(func() {
			v.result = b
		})
	})
}

type VirtualFile struct {
	ctx       context.Context
	name      string
	nameParts []string
	Remaining []byte
	Loaded    bool

	outputSetter func([]byte)
}

func newVirtualFile(ctx context.Context, name string, outputSetter func([]byte)) (*VirtualFile, error) {
	return &VirtualFile{
		ctx:          ctx,
		name:         name,
		nameParts:    strings.Split(name, "/"),
		outputSetter: outputSetter,
	}, nil
}

func (v *VirtualFile) Stat() (fs.FileInfo, error) {
	size := int64(1)
	return NewVirtualFileInfo(v.name, size), nil
}

func (v *VirtualFile) Write(bytes []byte) (sent int, err error) {
	if strings.HasSuffix(v.name, "sys/substreams/output") { //special function
		v.outputSetter(bytes)
		return len(bytes), nil
	}

	err = dataToFile(v.ctx, v.nameParts[1:], bytes) //skip /sys
	if err != nil {
		return 0, fmt.Errorf("writing data for file %q: %w", v.name, err)
	}

	return len(bytes), nil
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

func dataToFile(ctx context.Context, parts []string, data []byte) error {
	switch parts[0] {
	case "stores":
		return stateDataToFile(ctx, parts[1:], data) //skip store
	}

	return nil
}

func stateDataToFile(ctx context.Context, parts []string, data []byte) error {
	verb := parts[1]
	call := wasm.FromContext(ctx)

	switch verb {
	case "write":
		ord := parts[3]
		ordinal, err := strconv.Atoi(ord)
		if err != nil {
			return fmt.Errorf("parsing ordinal %q: %w", ord, err)
		}
		key := parts[4]
		call.DoSet(uint64(ordinal), key, data)
	case "conditionalwrite":
		ord := parts[3]
		ordinal, err := strconv.Atoi(ord)
		if err != nil {
			return fmt.Errorf("parsing ordinal %q: %w", ord, err)
		}
		key := parts[4]
		call.DoSetIfNotExists(uint64(ordinal), key, data)
	case "delete":
		ord := parts[3]
		ordinal, err := strconv.Atoi(ord)
		if err != nil {
			return fmt.Errorf("parsing ordinal %q: %w", ord, err)
		}
		key := parts[4]
		call.DoDeletePrefix(uint64(ordinal), key)
	case "add":
		ord := parts[3]
		ordinal, err := strconv.Atoi(ord)
		if err != nil {
			return fmt.Errorf("parsing ordinal %q: %w", ord, err)
		}
		key := parts[4]
		dataType := parts[2]
		switch dataType {
		case "int64":
			i, err := int64FromByteString(data)
			if err != nil {
				return fmt.Errorf("parsing int64 from byte string: %w", err)
			}
			call.DoAddInt64(uint64(ordinal), key, i)
		case "float64":
			i, err := float64FromByteString(data)
			if err != nil {
				return fmt.Errorf("parsing float64 from byte string: %w", err)
			}
			call.DoAddFloat64(uint64(ordinal), key, i)
		case "bigint":
			i, err := bigIntStringFromByteString(data)
			if err != nil {
				return fmt.Errorf("parsing big int from byte string: %w", err)
			}
			call.DoAddBigInt(uint64(ordinal), key, i)
		case "bigfloat":
			i, err := bigFloatStringFromByteString(data)
			if err != nil {
				return fmt.Errorf("parsing big float from byte string: %w", err)
			}
			call.DoAddBigDecimal(uint64(ordinal), key, i)
		}
	case "max":
		ord := parts[3]
		ordinal, err := strconv.Atoi(ord)
		if err != nil {
			return fmt.Errorf("parsing ordinal %q: %w", ord, err)
		}
		key := parts[4]
		dataType := parts[2]
		switch dataType {
		case "int64":
			i, err := int64FromByteString(data)
			if err != nil {
				return fmt.Errorf("parsing int64 from byte string: %w", err)
			}
			call.DoSetMaxInt64(uint64(ordinal), key, i)
		case "float64":
			i, err := float64FromByteString(data)
			if err != nil {
				return fmt.Errorf("parsing float64 from byte string: %w", err)
			}
			call.DoSetMaxFloat64(uint64(ordinal), key, i)
		case "bigint":
			i, err := bigIntStringFromByteString(data)
			if err != nil {
				return fmt.Errorf("parsing big int from byte string: %w", err)
			}
			call.DoSetMaxBigInt(uint64(ordinal), key, i)
		case "bigfloat":
			i, err := bigFloatStringFromByteString(data)
			if err != nil {
				return fmt.Errorf("parsing big float from byte string: %w", err)
			}
			call.DoSetMaxBigDecimal(uint64(ordinal), key, i)
		}
	case "min":
		ord := parts[3]
		ordinal, err := strconv.Atoi(ord)
		if err != nil {
			return fmt.Errorf("parsing ordinal %q: %w", ord, err)
		}
		key := parts[4]
		dataType := parts[2]
		switch dataType {
		case "int64":
			i, err := int64FromByteString(data)
			if err != nil {
				return fmt.Errorf("parsing int64 from byte string: %w", err)
			}
			call.DoSetMinInt64(uint64(ordinal), key, i)
		case "float64":
			i, err := float64FromByteString(data)
			if err != nil {
				return fmt.Errorf("parsing float64 from byte string: %w", err)
			}
			call.DoSetMinFloat64(uint64(ordinal), key, i)
		case "bigint":
			i, err := bigIntStringFromByteString(data)
			if err != nil {
				return fmt.Errorf("parsing big int from byte string: %w", err)
			}
			call.DoSetMinBigInt(uint64(ordinal), key, i)
		case "bigfloat":
			i, err := bigFloatStringFromByteString(data)
			if err != nil {
				return fmt.Errorf("parsing big float from byte string: %w", err)
			}
			call.DoSetMinBigDecimal(uint64(ordinal), key, i)
		}
	default:
		return fmt.Errorf("unknown verb %q", verb)
	}
	return nil
}

func int64FromByteString(data []byte) (int64, error) {
	var i int64
	_, err := fmt.Sscanf(string(data), "%d", &i)
	if err != nil {
		return 0, fmt.Errorf("parsing int64 from string: %w", err)
	}
	return i, nil
}

func float64FromByteString(data []byte) (float64, error) {
	var i float64
	_, err := fmt.Sscanf(string(data), "%f", &i)
	if err != nil {
		return 0, fmt.Errorf("parsing float64 from string: %w", err)
	}
	return i, nil
}

func bigIntStringFromByteString(data []byte) (string, error) {
	return new(big.Int).SetBytes(data).String(), nil
}

func bigFloatStringFromByteString(data []byte) (string, error) {
	return string(data), nil
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
	case "check":
		fla := parts[2]
		switch fla {
		case "last":
			key := parts[3]
			found := call.DoHasLast(index, key)
			if found {
				return []byte{1}, nil
			} else {
				return []byte{0}, nil
			}
		case "first":
			key := parts[3]
			found := call.DoHasFirst(index, key)
			if found {
				return []byte{1}, nil
			} else {
				return []byte{0}, nil
			}
		case "at":
			ordinalString := parts[3]
			ordinal, err := strconv.Atoi(ordinalString)
			if err != nil {
				return nil, fmt.Errorf("parsing ordinal %q: %w", ordinalString, err)
			}
			key := parts[4]
			found := call.DoHasAt(index, uint64(ordinal), key)
			if found {
				return []byte{1}, nil
			} else {
				return []byte{0}, nil
			}
		}
	default:
		return nil, fmt.Errorf("unknown verb %q", verb)
	}
	return nil, nil
}
