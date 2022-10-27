package marshaller

type StoreData struct {
	Kv             map[string][]byte
	DeletePrefixes []string
}

type Marshaller interface {
	Unmarshal(in []byte) (*StoreData, error)
	Marshal(data *StoreData) ([]byte, error)
}
