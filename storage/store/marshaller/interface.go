package marshaller

type StoreData struct {
	Kv             map[string][]byte
	DeletePrefixes []string
}

type Marshaller interface {
	Unmarshal(in []byte) (*StoreData, uint64, error)
	Marshal(data *StoreData) ([]byte, error)
}

func Default() Marshaller {
	return &VTproto{}
}
