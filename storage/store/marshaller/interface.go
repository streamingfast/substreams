package marshaller

import (
	"io"
	"iter"
)

type StoreData struct {
	Kv             map[string][]byte
	DeletePrefixes []string
}

type StoreDataEntry struct {
	kv  KeyValue
	sdt *StoreDataTrailer
}

type KeyValue struct {
	Key   string
	Value []byte
}
type StoreDataTrailer struct {
	DeletePrefixes []string
	TotalSizeBytes uint64
}

func (e StoreDataEntry) KV() KeyValue               { return e.kv }
func (e StoreDataEntry) Trailer() *StoreDataTrailer { return e.sdt }

// NewKVEntry creates a StoreDataEntry holding a key-value pair.
func NewKVEntry(key string, value []byte) StoreDataEntry {
	return StoreDataEntry{kv: KeyValue{Key: key, Value: value}}
}

// NewTrailerEntry creates a StoreDataEntry holding trailer metadata.
func NewTrailerEntry(trailer *StoreDataTrailer) StoreDataEntry {
	return StoreDataEntry{sdt: trailer}
}

type Marshaller interface {
	Unmarshal(in []byte) (*StoreData, uint64, error)
	Marshal(data *StoreData) ([]byte, error)
}

type StreamMarshaller interface {
	Marshaller
	UnmarshalStream(reader io.Reader, estimatedSize int64) (*StoreData, uint64, error)
	UnmarshalIter(reader io.Reader, estimatedSize int64) iter.Seq2[StoreDataEntry, error]
	MarshalStream(data *StoreData, estimatedSize int64) io.ReadCloser
	MarshalStreamIter(iter func(fn func(key string, value []byte) error) error, deletePrefixes []string, estimatedSize int64) io.ReadCloser
}

func Default() StreamMarshaller {
	return &VTproto{}
}
