package store

func (b *baseStore) Length() uint64 {
	return uint64(len(b.kv))
}

func (b *baseStore) Iter(f func(key string, value []byte) error) error {
	for k, v := range b.kv {
		if err := f(k, v); err != nil {
			return err
		}
	}
	return nil
}
