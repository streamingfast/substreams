package store

func (b *baseStore) Length() uint64 {
	return uint64(b.kvImpl.KeyCount())
}

func (b *baseStore) Iter(f func(key string, value []byte) error) error {
	return b.kvImpl.Iter(f)
}

func (b *baseStore) SizeBytes() uint64 {
	return b.totalSizeBytes
}
