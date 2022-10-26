package store

import (
	"encoding/binary"
	"fmt"
	"reflect"
	"unsafe"
)

type Marshaller interface {
	Unmarshal(in []byte) (map[string][]byte, error)
	Marshal(kv map[string][]byte) ([]byte, error)
}

type BinaryMarshaller struct{}

func (k *BinaryMarshaller) Marshal(kv map[string][]byte) ([]byte, error) {
	content, err := writeMapStringBytes(kv)
	if err != nil {
		return nil, fmt.Errorf("marshalling map string bytes kv state: %w", err)
	}

	return content, nil
}

func (k *BinaryMarshaller) Unmarshal(in []byte) (map[string][]byte, error) {
	kv, err := readMapStringBytes(in)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling  map string bytes kv state: %w", err)
	}

	return kv, nil
}

func writeMapStringBytes(entries map[string][]byte) ([]byte, error) {
	sizeInBytes := uvarintByteCount(uint64(len(entries)))
	for key, value := range entries {
		sizeInBytes += uvarintByteCount(uint64(len(key))) + len(key) + uvarintByteCount(uint64(len(value))) + len(value)
	}

	buffer := make([]byte, sizeInBytes)
	cursor := buffer

	byteCountWritten := binary.PutUvarint(cursor, uint64(len(entries)))
	cursor = cursor[byteCountWritten:]

	for key, value := range entries {
		written := binary.PutUvarint(cursor, uint64(len(key)))
		cursor = cursor[written:]

		copy(cursor, unsafeGetBytes(key))
		cursor = cursor[len(key):]

		written = binary.PutUvarint(cursor, uint64(len(value)))
		cursor = cursor[written:]

		copy(cursor, value)
		cursor = cursor[len(value):]
	}

	return buffer, nil
}

func readMapStringBytes(in []byte) (map[string][]byte, error) {
	cursor := in

	entries, n := binary.Uvarint(cursor)
	if n == 0 {
		return nil, fmt.Errorf("no bytes to read from cursor")
	}
	cursor = cursor[n:]

	out := make(map[string][]byte, entries)

	for i := uint64(0); i < entries; i++ {
		keyLen, bytesCountRead := binary.Uvarint(cursor)
		if bytesCountRead == 0 {
			return nil, fmt.Errorf("no bytes to read from cursor for key")
		}
		cursor = cursor[bytesCountRead:]

		if uint64(len(cursor)) < keyLen {
			return nil, fmt.Errorf("accessing key out of bytes slice")
		}
		ks := unsafeGetString(cursor[:keyLen])
		cursor = cursor[keyLen:]

		valueLen, bytesCountRead := binary.Uvarint(cursor)
		if bytesCountRead == 0 {
			return nil, fmt.Errorf("no bytes to read from cursor for value")
		}
		cursor = cursor[bytesCountRead:]

		if uint64(len(cursor)) < valueLen {
			return nil, fmt.Errorf("accessing value out of bytes slice")
		}
		out[ks] = cursor[:valueLen]
		cursor = cursor[valueLen:]
	}
	return out, nil
}

// Get the string from a '[]byte' without any allocation
// See https://github.com/golang/go/issues/25484
func unsafeGetString(bs []byte) string {
	return *(*string)(unsafe.Pointer(&bs))
}

// Get the bytes of a `string` variable without doing any allocation, useful for writing to storage
// with high efficiency. This method exists because `[]byte(stringVar)` does an allocation, by using
// this method, you avoid this allocation.
//
// See https://stackoverflow.com/q/59209493/697930 for full discussion
func unsafeGetBytes(s string) []byte {
	return unsafe.Slice((*byte)(unsafe.Pointer((*reflect.StringHeader)(unsafe.Pointer(&s)).Data)), len(s))
}

// uvarintByteCount counts how many bytes are needed for a uvarint. It's a copy of `binary.PutUvarint`
// with the write part removed.
func uvarintByteCount(x uint64) int {
	i := 0
	for x >= 0x80 {
		x >>= 7
		i++
	}
	return i + 1
}
