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

// KVSnapshotIter is a pull-based iterator over a consistent snapshot of a KV
// store, yielding entries in lexicographic key order. Next returns ok=false
// once exhausted. Close MUST be called to release the snapshot's underlying
// resources (locks, transactions); it is safe to call after exhaustion or
// mid-iteration.
type KVSnapshotIter interface {
	Next() (key string, value []byte, ok bool, err error)
	Close() error
}

type StreamMarshaller interface {
	Marshaller
	UnmarshalStream(reader io.Reader, estimatedSize int64) (*StoreData, uint64, error)
	UnmarshalIter(reader io.Reader, estimatedSize int64) iter.Seq2[StoreDataEntry, error]
	MarshalStream(data *StoreData, estimatedSize int64) io.ReadCloser
	MarshalStreamSnapshot(snap KVSnapshotIter, deletePrefixes []string) io.ReadCloser
}

// UnsortedStreamMarshaller streams the store without sorting keys, trading
// deterministic entry order for skipping the O(n log n) key sort and the O(n)
// key-slice allocation. Suitable for order-independent consumers like quicksave.
type UnsortedStreamMarshaller interface {
	MarshalStreamUnsorted(data *StoreData) io.ReadCloser
}

func Default() StreamMarshaller {
	return &VTproto{}
}
