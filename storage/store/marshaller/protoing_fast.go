package marshaller

import (
	"encoding/binary"
	"fmt"

	pbsubstreams "github.com/streamingfast/substreams/storage/store/marshaller/pb"
	"google.golang.org/protobuf/proto"
)

const KVEntryProtoTag = 0x0a
const KVEntryKeyProtoTag = 0x0a
const KVEntryValueProtoTag = 0x12
const DeletePrefixEntryProtoTag = 0x12

// ProtoingFast is a custom proto marshaller, that will marshal and unmarshall the storeData into a predefined
// proto struct (see below). The motivation here is that we want to write a proto message, making it readable by
// other tool with the appropriate message, but want to gain the marshal performance of a custom binary library
//
//	message StoreData {
//		map<string, bytes> kv = 1;
//		repeated string delete_prefixes = 2;
//	}
type ProtoingFast struct{}

func (p *ProtoingFast) Unmarshal(in []byte) (*StoreData, uint64, error) {
	stateData := &pbsubstreams.StoreData{}
	if err := proto.Unmarshal(in, stateData); err != nil {
		return nil, 0, fmt.Errorf("unmarshal store: %w", err)
	}
	return &StoreData{
		Kv:             stateData.GetKv(),
		DeletePrefixes: stateData.GetDeletePrefixes(),
	}, 0, nil
}

func (p *ProtoingFast) Marshal(data *StoreData) ([]byte, error) {
	sizeInBytes := p.kvByteSize(data.Kv)
	sizeInBytes += p.listByteSize(data.DeletePrefixes)
	buffer := make([]byte, sizeInBytes)
	cursor := buffer
	cursor = p.writeKV(cursor, data.Kv)
	p.writeDeletePrefix(cursor, data.DeletePrefixes)
	return buffer, nil

}

func (p *ProtoingFast) kvByteSize(entries map[string][]byte) int {
	size := 0
	for k, v := range entries {
		entrySize := kvEntryByteSize(k, v)
		size += 1                                   // Map Key/Value proto tag  0x0A  (field number 1 [the KV field],  type LEN [string])
		size += uvarintByteCount(uint64(entrySize)) // Number of bytes to represent both key and value
		size += entrySize
	}
	return size
}

func kvEntryByteSize(key string, value []byte) int {
	size := 1                                    // Key proto tag 0x0a (field number 1 [tke key],  type LEN [string])
	size += uvarintByteCount(uint64(len(key)))   // Number of bytes (characters) in the key
	size += len(key)                             // key
	size += 1                                    // Value proto tag 0x12 (field number  [the value], type LEN [byte array])
	size += uvarintByteCount(uint64(len(value))) // Number of bytes in the array
	size += len(value)                           // value
	return size
}

func (p *ProtoingFast) listByteSize(list []string) int {
	size := 0
	for _, l := range list {
		size += 1                                // List element proto tag 0x12 (field number 2 [the DeletePrefix field], type LEN [string])
		size += uvarintByteCount(uint64(len(l))) // Number of bytes (characters) to write
		size += len(l)                           // string
	}
	return size
}

func (p *ProtoingFast) writeKV(cursor []byte, entries map[string][]byte) []byte {
	for key, value := range entries {
		copy(cursor, []byte{KVEntryProtoTag})
		cursor = cursor[1:]

		written := binary.PutUvarint(cursor, uint64(kvEntryByteSize(key, value)))
		cursor = cursor[written:]

		copy(cursor, []byte{KVEntryKeyProtoTag})
		cursor = cursor[1:]

		written = binary.PutUvarint(cursor, uint64(len(key)))
		cursor = cursor[written:]

		copy(cursor, unsafeGetBytes(key))
		cursor = cursor[len(key):]

		copy(cursor, []byte{KVEntryValueProtoTag})
		cursor = cursor[1:]

		written = binary.PutUvarint(cursor, uint64(len(value)))
		cursor = cursor[written:]

		copy(cursor, value)
		cursor = cursor[len(value):]
	}
	return cursor
}

func (p *ProtoingFast) writeDeletePrefix(cursor []byte, entries []string) []byte {
	for _, value := range entries {
		copy(cursor, []byte{DeletePrefixEntryProtoTag})
		cursor = cursor[1:]

		written := binary.PutUvarint(cursor, uint64(len(value)))
		cursor = cursor[written:]

		copy(cursor, unsafeGetBytes(value))
		cursor = cursor[len(value):]
	}
	return cursor
}
