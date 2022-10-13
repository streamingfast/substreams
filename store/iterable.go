package store

func (s *BaseStore) Length() uint64 {
	return uint64(len(s.kv))
}

func (s *BaseStore) Iter(f func(key string, value []byte) error) error {
	for k, v := range s.kv {
		if err := f(k, v); err != nil {
			return err
		}
	}
	return nil
}
