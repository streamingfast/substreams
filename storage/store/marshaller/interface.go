package marshaller

import "io"

type StoreData struct {
	Kv             map[string][]byte
	DeletePrefixes []string
}

type Marshaller interface {
	Unmarshal(in []byte) (*StoreData, uint64, error)
	Marshal(data *StoreData) ([]byte, error)
}

type StreamMarshaller interface {
	Marshaller
	UnmarshalStream(reader io.Reader, estimatedSize int64) (*StoreData, uint64, error)
	MarshalStream(data *StoreData, estimatedSize int64) io.ReadCloser
}

// UnsortedStreamMarshaller streams the store without sorting keys, trading
// deterministic entry order for skipping the O(n log n) key sort and the O(n)
// key-slice allocation. Suitable for order-independent consumers like quicksave.
type UnsortedStreamMarshaller interface {
	MarshalStreamUnsorted(data *StoreData) io.ReadCloser
}

func Default() Marshaller {
	return &VTproto{}
}
