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

func Default() Marshaller {
	return &VTproto{}
}
