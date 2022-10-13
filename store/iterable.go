package store

func (s *KVStore) Length() uint64 {
	return uint64(len(s.kv))
}

func (s *KVStore) Iter(f func(key string, value []byte) error) error {
	for k, v := range s.kv {
		if err := f(k, v); err != nil {
			return err
		}
	}
	return nil
}
