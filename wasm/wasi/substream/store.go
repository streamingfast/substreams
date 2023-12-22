package substream

import "github.com/streamingfast/substreams/wasm/wasi/fs"

type StoreGet[T any] interface {
	GetAt(ord uint64, key string) (T, error)
	GetLast(key string) (T, error)
	GetFirst(key string) (T, error)
	HasAt(ord uint64, key string) bool
	HasLast(key string) bool
	HasFirst(key string) bool
}

func NewStringStore(idx uint32) StoreGet[string] {
	return &StringStore{
		idx: idx,
	}
}

// StringStore is a concrete implementation of StoreGet for strings.
type StringStore struct {
	idx uint32

	vfs fs.Virtual
}

// GetAt retrieves a string value at the given ordinal and key.
func (s *StringStore) GetAt(ord uint64, key string) (string, error) {
	return "", nil
}

// GetLast retrieves the last string value for the given key.
func (s *StringStore) GetLast(key string) (string, error) {
	// Implement your logic to retrieve a string
	// For now, return a dummy string and nil error
	return "dummy string from GetLast", nil
}

// GetFirst retrieves the first string value for the given key.
func (s *StringStore) GetFirst(key string) (string, error) {
	// Implement your logic to retrieve a string
	// For now, return a dummy string and nil error
	return "dummy string from GetFirst", nil
}

// HasAt checks if a key exists at the given ordinal.
func (s *StringStore) HasAt(ord uint64, key string) bool {
	// Implement your logic to check existence
	// For now, return a dummy value
	return true
}

// HasLast checks if the last key exists.
func (s *StringStore) HasLast(key string) bool {
	// Implement your logic to check existence
	// For now, return a dummy value
	return true
}

// HasFirst checks if the first key exists.
func (s *StringStore) HasFirst(key string) bool {
	// Implement your logic to check existence
	// For now, return a dummy value
	return true
}

// all the store funcs!

// generic interfaces for adders, appenders, etc etc etc
