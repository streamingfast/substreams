package marshaller

import (
	"bufio"
	"fmt"
	"io"
	"iter"
	"sync"

	pbstore "github.com/streamingfast/substreams/storage/store/marshaller/pb"
)

type VTproto struct{}

// Unmarshal takes the whole data bytes
func (p *VTproto) Unmarshal(in []byte) (*StoreData, uint64, error) {
	stateData := &pbstore.StoreData{}
	dataSize, err := unmarshalVT(stateData, in)
	if err != nil {
		return nil, 0, fmt.Errorf("unmarshal store: %w", err)
	}
	return &StoreData{
		Kv:             stateData.GetKv(),
		DeletePrefixes: stateData.GetDeletePrefixes(),
	}, dataSize, nil
}

// UnmarshalStreamFast is an optimized version for high-performance scenarios
// It pre-allocates larger buffers and uses optimized read patterns
func (p *VTproto) UnmarshalStreamFast(reader io.Reader, estimatedSize int64) (*StoreData, uint64, error) {
	stateData := &pbstore.StoreData{}
	dataSize, err := unmarshalVTStreamFast(stateData, reader, estimatedSize)
	if err != nil {
		return nil, 0, fmt.Errorf("unmarshal store stream fast: %w", err)
	}
	return &StoreData{
		Kv:             stateData.GetKv(),
		DeletePrefixes: stateData.GetDeletePrefixes(),
	}, dataSize, nil
}

// UnmarshalStream reads and unmarshals data from a stream.
// The caller is responsible for closing the reader if it implements io.Closer.
// All internal pooled resources are automatically cleaned up on error or success.
func (p *VTproto) UnmarshalStream(reader io.Reader, estimatedSize int64) (*StoreData, uint64, error) {
	stateData := &pbstore.StoreData{}
	var dataSize uint64
	var err error

	// Use the fast version if we have a good size estimate
	if estimatedSize > 0 {
		dataSize, err = unmarshalVTStreamFast(stateData, reader, estimatedSize)
	} else {
		dataSize, err = unmarshalVTStream(stateData, reader)
	}

	if err != nil {
		return nil, 0, fmt.Errorf("unmarshal store stream: %w", err)
	}
	return &StoreData{
		Kv:             stateData.GetKv(),
		DeletePrefixes: stateData.GetDeletePrefixes(),
	}, dataSize, nil
}

func (p *VTproto) Marshal(data *StoreData) ([]byte, error) {
	stateData := &pbstore.StoreData{
		Kv:             data.Kv,
		DeletePrefixes: data.DeletePrefixes,
	}

	return stateData.MarshalVT()
}

// MarshalStream returns an io.ReadCloser that streams the marshaled data.
// The data is assumed to not change until the returned ReadCloser is closed.
// estimatedSize is used for buffer optimization; use 0 for auto-sizing.
// IMPORTANT: The caller MUST call Close() on the returned ReadCloser to prevent resource leaks.
func (p *VTproto) MarshalStream(data *StoreData, estimatedSize int64) io.ReadCloser {
	return newFastStreamingMarshaler(data, estimatedSize)
}

// UnmarshalIter streams entries from reader one at a time, yielding each KV pair
// without materializing the full map. This is the production load path for mmap:
// bytes → UnmarshalIter → LoadFromStream → bbolt, with no intermediate map allocation.
func (p *VTproto) UnmarshalIter(reader io.Reader, estimatedSize int64) iter.Seq2[StoreDataEntry, error] {
	return func(yield func(StoreDataEntry, error) bool) {
		bufferSize := int64(32768)
		if estimatedSize > 0 {
			proposed := estimatedSize / 16
			if proposed > 1048576 {
				proposed = 1048576
			}
			if proposed < 32768 {
				proposed = 32768
			}
			bufferSize = proposed
		}

		br := bufio.NewReaderSize(reader, int(bufferSize))

		workBuf := make([]byte, max(65536, int(estimatedSize/8)))

		readVarint := func() (uint64, error) {
			var value uint64
			var shift uint
			for {
				if shift >= 64 {
					return 0, pbstore.ErrIntOverflow
				}
				b, err := br.ReadByte()
				if err != nil {
					return 0, err
				}
				value |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
				shift += 7
			}
			return value, nil
		}

		readExact := func(n int) ([]byte, error) {
			if n > len(workBuf) {
				workBuf = make([]byte, n*2)
			}
			_, err := io.ReadFull(br, workBuf[:n])
			return workBuf[:n], err
		}

		var totalSizeBytes uint64

		for {
			wire, err := readVarint()
			if err != nil {
				if err == io.EOF {
					break
				}
				yield(StoreDataEntry{}, err)
				return
			}

			fieldNum := int32(wire >> 3)
			wireType := int(wire & 0x7)

			if wireType == 4 {
				yield(StoreDataEntry{}, fmt.Errorf("proto: StoreData: wiretype end group for non-group"))
				return
			}
			if fieldNum <= 0 {
				yield(StoreDataEntry{}, fmt.Errorf("proto: StoreData: illegal tag %d (wire type %d)", fieldNum, wire))
				return
			}

			switch fieldNum {
			case 1: // kv entry
				if wireType != 2 {
					yield(StoreDataEntry{}, fmt.Errorf("proto: wrong wireType = %d for field Kv", wireType))
					return
				}
				msglen, err := readVarint()
				if err != nil {
					yield(StoreDataEntry{}, err)
					return
				}
				entryData, err := readExact(int(msglen))
				if err != nil {
					yield(StoreDataEntry{}, err)
					return
				}

				var mapkey string
				var mapvalue []byte
				iNdEx := 0
				l := int(msglen)
				for iNdEx < l {
					var w uint64
					for shift := uint(0); ; shift += 7 {
						if shift >= 64 {
							yield(StoreDataEntry{}, pbstore.ErrIntOverflow)
							return
						}
						if iNdEx >= l {
							yield(StoreDataEntry{}, io.ErrUnexpectedEOF)
							return
						}
						b := entryData[iNdEx]
						iNdEx++
						w |= uint64(b&0x7F) << shift
						if b < 0x80 {
							break
						}
					}
					fn := int32(w >> 3)
					if fn == 1 {
						var slen uint64
						for shift := uint(0); ; shift += 7 {
							if shift >= 64 {
								yield(StoreDataEntry{}, pbstore.ErrIntOverflow)
								return
							}
							if iNdEx >= l {
								yield(StoreDataEntry{}, io.ErrUnexpectedEOF)
								return
							}
							b := entryData[iNdEx]
							iNdEx++
							slen |= uint64(b&0x7F) << shift
							if b < 0x80 {
								break
							}
						}
						end := iNdEx + int(slen)
						if end > l {
							yield(StoreDataEntry{}, io.ErrUnexpectedEOF)
							return
						}
						mapkey = unsafeGetString(entryData[iNdEx:end])
						iNdEx = end
					} else if fn == 2 {
						var vlen uint64
						for shift := uint(0); ; shift += 7 {
							if shift >= 64 {
								yield(StoreDataEntry{}, pbstore.ErrIntOverflow)
								return
							}
							if iNdEx >= l {
								yield(StoreDataEntry{}, io.ErrUnexpectedEOF)
								return
							}
							b := entryData[iNdEx]
							iNdEx++
							vlen |= uint64(b&0x7F) << shift
							if b < 0x80 {
								break
							}
						}
						end := iNdEx + int(vlen)
						if end > l {
							yield(StoreDataEntry{}, io.ErrUnexpectedEOF)
							return
						}
						// copy value — entryData is a shared workBuf slice
						mapvalue = make([]byte, int(vlen))
						copy(mapvalue, entryData[iNdEx:end])
						iNdEx = end
					} else {
						skippy, err := skipInBuffer(entryData[iNdEx:])
						if err != nil {
							yield(StoreDataEntry{}, err)
							return
						}
						iNdEx += skippy
					}
				}

				totalSizeBytes += uint64(len(mapkey) + len(mapvalue))
				if !yield(StoreDataEntry{kv: KeyValue{Key: mapkey, Value: mapvalue}}, nil) {
					return
				}

			case 2: // delete_prefixes — emit as trailer at end, collect for now
				if wireType != 2 {
					yield(StoreDataEntry{}, fmt.Errorf("proto: wrong wireType = %d for field DeletePrefixes", wireType))
					return
				}
				slen, err := readVarint()
				if err != nil {
					yield(StoreDataEntry{}, err)
					return
				}
				data, err := readExact(int(slen))
				if err != nil {
					yield(StoreDataEntry{}, err)
					return
				}
				prefix := string(data)
				if !yield(StoreDataEntry{sdt: &StoreDataTrailer{DeletePrefixes: []string{prefix}}}, nil) {
					return
				}

			default:
				if err := skipFieldFromBufferedReader(br, wireType); err != nil {
					yield(StoreDataEntry{}, err)
					return
				}
			}
		}

		// Emit final trailer with total size
		yield(StoreDataEntry{sdt: &StoreDataTrailer{TotalSizeBytes: totalSizeBytes}}, nil)
	}
}

// MarshalStreamIter streams marshal directly from a key-value iterator, bypassing
// the intermediate map[string][]byte allocation. The iterator must yield keys in
// lexicographic order (bbolt Iter guarantees this).
// IMPORTANT: The caller MUST call Close() on the returned ReadCloser to prevent resource leaks.
func (p *VTproto) MarshalStreamIter(iter func(fn func(key string, value []byte) error) error, deletePrefixes []string, estimatedSize int64) io.ReadCloser {
	return newFastStreamingMarshalerFromIter(iter, deletePrefixes, estimatedSize)
}

// fastStreamingMarshaler is a high-performance streaming marshaler that pre-computes
// and batches operations to minimize per-Read() overhead
type fastStreamingMarshaler struct {
	chunks   [][]byte // Pre-computed chunks of data
	chunkIdx int      // Current chunk index
	chunkPos int      // Position within current chunk
	closed   bool
}

func newFastStreamingMarshaler(data *StoreData, estimatedSize int64) *fastStreamingMarshaler {
	marshaler := &fastStreamingMarshaler{}
	marshaler.precomputeChunks(data, estimatedSize)
	return marshaler
}

func (f *fastStreamingMarshaler) precomputeChunks(data *StoreData, estimatedSize int64) {
	// Calculate total size to minimize allocations
	totalEstimate := estimatedSize
	if totalEstimate <= 0 {
		// Quick estimate: 50 bytes per KV + 20 bytes per prefix
		totalEstimate = int64(len(data.Kv)*50 + len(data.DeletePrefixes)*20)
	}

	// Use single large buffer if data is small enough
	if totalEstimate < 1024*1024 { // < 1MB, use single buffer
		buffer := make([]byte, 0, totalEstimate+1024) // Add some padding
		buffer = f.marshalAllToBuffer(data, buffer)
		f.chunks = [][]byte{buffer}
		return
	}

	// For larger data, use chunking
	chunkSize := 128 * 1024           // 128KB chunks for better performance
	if totalEstimate > 50*1024*1024 { // > 50MB
		chunkSize = 512 * 1024 // 512KB chunks
	}

	// Pre-allocate chunks slice
	estimatedChunks := int(totalEstimate)/chunkSize + 1
	f.chunks = make([][]byte, 0, estimatedChunks)

	currentChunk := make([]byte, 0, chunkSize)
	varintBuf := [10]byte{} // Stack-allocated buffer

	// Pre-sort keys once
	sortedKeys := f.getSortedKeys(data.Kv)

	// Process KV entries
	for _, key := range sortedKeys {
		value := data.Kv[key]
		fieldSize := f.calculateKVFieldSize(key, value)

		if len(currentChunk)+fieldSize > chunkSize && len(currentChunk) > 0 {
			f.chunks = append(f.chunks, currentChunk)
			currentChunk = make([]byte, 0, chunkSize)
		}

		currentChunk = f.appendKVField(currentChunk, key, value, varintBuf[:])
	}

	// Process delete prefixes
	for _, prefix := range data.DeletePrefixes {
		fieldSize := varintSize(2<<3|2) + varintSize(uint64(len(prefix))) + len(prefix)

		if len(currentChunk)+fieldSize > chunkSize && len(currentChunk) > 0 {
			f.chunks = append(f.chunks, currentChunk)
			currentChunk = make([]byte, 0, chunkSize)
		}

		currentChunk = f.appendDeletePrefixField(currentChunk, prefix, varintBuf[:])
	}

	if len(currentChunk) > 0 {
		f.chunks = append(f.chunks, currentChunk)
	}
}

func newFastStreamingMarshalerFromIter(iter func(fn func(key string, value []byte) error) error, deletePrefixes []string, estimatedSize int64) *fastStreamingMarshaler {
	m := &fastStreamingMarshaler{}
	m.precomputeChunksFromIter(iter, deletePrefixes, estimatedSize)
	return m
}

// precomputeChunksFromIter is like precomputeChunks but reads from a KVImpl.Iter callback
// instead of a map. Keys arrive in lexicographic order so no sorting is needed.
func (f *fastStreamingMarshaler) precomputeChunksFromIter(iter func(fn func(key string, value []byte) error) error, deletePrefixes []string, estimatedSize int64) {
	chunkSize := 128 * 1024
	if estimatedSize > 50*1024*1024 {
		chunkSize = 512 * 1024
	}

	varintBuf := [10]byte{}
	currentChunk := make([]byte, 0, chunkSize)

	_ = iter(func(key string, value []byte) error {
		fieldSize := f.calculateKVFieldSize(key, value)
		if len(currentChunk)+fieldSize > chunkSize && len(currentChunk) > 0 {
			f.chunks = append(f.chunks, currentChunk)
			currentChunk = make([]byte, 0, chunkSize)
		}
		currentChunk = f.appendKVField(currentChunk, key, value, varintBuf[:])
		return nil
	})

	for _, prefix := range deletePrefixes {
		fieldSize := varintSize(2<<3|2) + varintSize(uint64(len(prefix))) + len(prefix)
		if len(currentChunk)+fieldSize > chunkSize && len(currentChunk) > 0 {
			f.chunks = append(f.chunks, currentChunk)
			currentChunk = make([]byte, 0, chunkSize)
		}
		currentChunk = f.appendDeletePrefixField(currentChunk, prefix, varintBuf[:])
	}

	if len(currentChunk) > 0 {
		f.chunks = append(f.chunks, currentChunk)
	}
}

// marshalAllToBuffer marshals everything into a single buffer for small datasets
func (f *fastStreamingMarshaler) marshalAllToBuffer(data *StoreData, buffer []byte) []byte {
	varintBuf := [10]byte{}
	sortedKeys := f.getSortedKeys(data.Kv)

	for _, key := range sortedKeys {
		value := data.Kv[key]
		buffer = f.appendKVField(buffer, key, value, varintBuf[:])
	}

	for _, prefix := range data.DeletePrefixes {
		buffer = f.appendDeletePrefixField(buffer, prefix, varintBuf[:])
	}

	return buffer
}

// getSortedKeys returns sorted keys with minimal allocations
func (f *fastStreamingMarshaler) getSortedKeys(kv map[string][]byte) []string {
	if len(kv) == 0 {
		return nil
	}

	keys := make([]string, 0, len(kv))
	for key := range kv {
		keys = append(keys, key)
	}

	// Use optimized sorting for small slices
	n := len(keys)
	if n <= 20 {
		// Insertion sort for small arrays
		for i := 1; i < n; i++ {
			key := keys[i]
			j := i - 1
			for j >= 0 && keys[j] > key {
				keys[j+1] = keys[j]
				j--
			}
			keys[j+1] = key
		}
	} else {
		// Quick sort for larger arrays
		f.quickSort(keys, 0, n-1)
	}

	return keys
}

// quickSort implements efficient in-place sorting
func (f *fastStreamingMarshaler) quickSort(keys []string, low, high int) {
	if low < high {
		pi := f.partition(keys, low, high)
		f.quickSort(keys, low, pi-1)
		f.quickSort(keys, pi+1, high)
	}
}

func (f *fastStreamingMarshaler) partition(keys []string, low, high int) int {
	pivot := keys[high]
	i := low - 1

	for j := low; j < high; j++ {
		if keys[j] <= pivot {
			i++
			keys[i], keys[j] = keys[j], keys[i]
		}
	}
	keys[i+1], keys[high] = keys[high], keys[i+1]
	return i + 1
}

// calculateKVFieldSize calculates the size of a KV field
func (f *fastStreamingMarshaler) calculateKVFieldSize(key string, value []byte) int {
	keyLen := len(key)
	valueLen := len(value)
	keyFieldSize := varintSize(1<<3|2) + varintSize(uint64(keyLen)) + keyLen
	valueFieldSize := varintSize(2<<3|2) + varintSize(uint64(valueLen)) + valueLen
	mapEntrySize := keyFieldSize + valueFieldSize
	return varintSize(1<<3|2) + varintSize(uint64(mapEntrySize)) + mapEntrySize
}

// appendKVField appends a complete KV field to the buffer
func (f *fastStreamingMarshaler) appendKVField(buf []byte, key string, value []byte, varintBuf []byte) []byte {
	keyLen := len(key)
	valueLen := len(value)
	keyFieldSize := varintSize(1<<3|2) + varintSize(uint64(keyLen)) + keyLen
	valueFieldSize := varintSize(2<<3|2) + varintSize(uint64(valueLen)) + valueLen
	mapEntrySize := keyFieldSize + valueFieldSize

	// Field header and length
	buf = appendVarint(buf, 1<<3|2, varintBuf)
	buf = appendVarint(buf, uint64(mapEntrySize), varintBuf)

	// Key field
	buf = appendVarint(buf, 1<<3|2, varintBuf)
	buf = appendVarint(buf, uint64(keyLen), varintBuf)
	buf = append(buf, key...)

	// Value field
	buf = appendVarint(buf, 2<<3|2, varintBuf)
	buf = appendVarint(buf, uint64(valueLen), varintBuf)
	buf = append(buf, value...)

	return buf
}

// appendDeletePrefixField appends a delete prefix field to the buffer
func (f *fastStreamingMarshaler) appendDeletePrefixField(buf []byte, prefix string, varintBuf []byte) []byte {
	buf = appendVarint(buf, 2<<3|2, varintBuf)
	buf = appendVarint(buf, uint64(len(prefix)), varintBuf)
	return append(buf, prefix...)
}

func (f *fastStreamingMarshaler) Read(p []byte) (n int, err error) {
	if f.closed {
		return 0, io.EOF
	}

	totalRead := 0

	for totalRead < len(p) && f.chunkIdx < len(f.chunks) {
		currentChunk := f.chunks[f.chunkIdx]

		// Copy data from current chunk
		available := len(currentChunk) - f.chunkPos
		needed := len(p) - totalRead
		toCopy := available
		if needed < available {
			toCopy = needed
		}

		copy(p[totalRead:], currentChunk[f.chunkPos:f.chunkPos+toCopy])
		totalRead += toCopy
		f.chunkPos += toCopy

		// Move to next chunk if current is exhausted
		if f.chunkPos >= len(currentChunk) {
			f.chunkIdx++
			f.chunkPos = 0
		}
	}

	if totalRead == 0 {
		return 0, io.EOF
	}

	return totalRead, nil
}

func (f *fastStreamingMarshaler) Close() error {
	f.closed = true
	f.chunks = nil // Help GC
	return nil
}

// Fast varint encoding that reuses a buffer
func appendVarint(dst []byte, x uint64, buf []byte) []byte {
	i := 0
	for x >= 0x80 {
		buf[i] = byte(x) | 0x80
		x >>= 7
		i++
	}
	buf[i] = byte(x)
	return append(dst, buf[:i+1]...)
}

// Fast varint size calculation
func varintSize(x uint64) int {
	switch {
	case x < 1<<7:
		return 1
	case x < 1<<14:
		return 2
	case x < 1<<21:
		return 3
	case x < 1<<28:
		return 4
	case x < 1<<35:
		return 5
	case x < 1<<42:
		return 6
	case x < 1<<49:
		return 7
	case x < 1<<56:
		return 8
	case x < 1<<63:
		return 9
	default:
		return 10
	}
}

// The function `func (m *StoreData) UnmarshalVT(dAtA []byte) error` that is generated
// by the vtprotobuf protobuf plugin is ok, but we can greatly improve the allocation and
// speed with a few optimizations. This function is a 98% copy of the function in
// ./pb/store_vtproto.pb.go
// we've added byte counter too
func unmarshalVT(m *pbstore.StoreData, dAtA []byte) (dataSize uint64, err error) {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return 0, pbstore.ErrIntOverflow
			}
			if iNdEx >= l {
				return 0, io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= uint64(b&0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return 0, fmt.Errorf("proto: StoreData: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return 0, fmt.Errorf("proto: StoreData: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return 0, fmt.Errorf("proto: wrong wireType = %d for field Kv", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, pbstore.ErrIntOverflow
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return 0, pbstore.ErrInvalidLength
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return 0, pbstore.ErrInvalidLength
			}
			if postIndex > l {
				return 0, io.ErrUnexpectedEOF
			}
			if m.Kv == nil {
				m.Kv = make(map[string][]byte)
			}
			var mapkey string
			var mapvalue []byte
			for iNdEx < postIndex {
				entryPreIndex := iNdEx
				var wire uint64
				for shift := uint(0); ; shift += 7 {
					if shift >= 64 {
						return 0, pbstore.ErrIntOverflow
					}
					if iNdEx >= l {
						return 0, io.ErrUnexpectedEOF
					}
					b := dAtA[iNdEx]
					iNdEx++
					wire |= uint64(b&0x7F) << shift
					if b < 0x80 {
						break
					}
				}
				fieldNum := int32(wire >> 3)
				if fieldNum == 1 {
					var stringLenmapkey uint64
					for shift := uint(0); ; shift += 7 {
						if shift >= 64 {
							return 0, pbstore.ErrIntOverflow
						}
						if iNdEx >= l {
							return 0, io.ErrUnexpectedEOF
						}
						b := dAtA[iNdEx]
						iNdEx++
						stringLenmapkey |= uint64(b&0x7F) << shift
						if b < 0x80 {
							break
						}
					}
					intStringLenmapkey := int(stringLenmapkey)
					if intStringLenmapkey < 0 {
						return 0, pbstore.ErrInvalidLength
					}
					postStringIndexmapkey := iNdEx + intStringLenmapkey
					if postStringIndexmapkey < 0 {
						return 0, pbstore.ErrInvalidLength
					}
					if postStringIndexmapkey > l {
						return 0, io.ErrUnexpectedEOF
					}

					// @julien do not waste time allocating here
					//mapkey = string(dAtA[iNdEx:postStringIndexmapkey])
					mapkey = unsafeGetString(dAtA[iNdEx:postStringIndexmapkey])

					iNdEx = postStringIndexmapkey
				} else if fieldNum == 2 {
					var mapbyteLen uint64
					for shift := uint(0); ; shift += 7 {
						if shift >= 64 {
							return 0, pbstore.ErrIntOverflow
						}
						if iNdEx >= l {
							return 0, io.ErrUnexpectedEOF
						}
						b := dAtA[iNdEx]
						iNdEx++
						mapbyteLen |= uint64(b&0x7F) << shift
						if b < 0x80 {
							break
						}
					}
					intMapbyteLen := int(mapbyteLen)
					if intMapbyteLen < 0 {
						return 0, pbstore.ErrInvalidLength
					}
					postbytesIndex := iNdEx + intMapbyteLen
					if postbytesIndex < 0 {
						return 0, pbstore.ErrInvalidLength
					}
					if postbytesIndex > l {
						return 0, io.ErrUnexpectedEOF
					}

					// @julien do not waste time allocating here
					//mapvalue = make([]byte, mapbyteLen)
					//copy(mapvalue, dAtA[iNdEx:postbytesIndex])
					mapvalue = dAtA[iNdEx:postbytesIndex]
					iNdEx = postbytesIndex

				} else {
					iNdEx = entryPreIndex
					skippy, err := skip(dAtA[iNdEx:])
					if err != nil {
						return 0, err
					}
					if (skippy < 0) || (iNdEx+skippy) < 0 {
						return 0, pbstore.ErrInvalidLength
					}
					if (iNdEx + skippy) > postIndex {
						return 0, io.ErrUnexpectedEOF
					}
					iNdEx += skippy
				}
			}
			m.Kv[mapkey] = mapvalue
			dataSize += uint64(len(mapkey) + len(mapvalue))
			iNdEx = postIndex
		case 2:
			if wireType != 2 {
				return 0, fmt.Errorf("proto: wrong wireType = %d for field DeletePrefixes", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, pbstore.ErrIntOverflow
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				stringLen |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			intStringLen := int(stringLen)
			if intStringLen < 0 {
				return 0, pbstore.ErrInvalidLength
			}
			postIndex := iNdEx + intStringLen
			if postIndex < 0 {
				return 0, pbstore.ErrInvalidLength
			}
			if postIndex > l {
				return 0, io.ErrUnexpectedEOF
			}

			// @julien do not waste time allocating here
			//m.DeletePrefixes = append(m.DeletePrefixes, string(dAtA[iNdEx:postIndex]))
			m.DeletePrefixes = append(m.DeletePrefixes, unsafeGetString(dAtA[iNdEx:postIndex]))
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skip(dAtA[iNdEx:])
			if err != nil {
				return 0, err
			}
			if (skippy < 0) || (iNdEx+skippy) < 0 {
				return 0, pbstore.ErrInvalidLength
			}
			if (iNdEx + skippy) > l {
				return 0, io.ErrUnexpectedEOF
			}
			//m.unknownFields = append(m.unknownFields, dAtA[iNdEx:iNdEx+skippy]...)
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return 0, io.ErrUnexpectedEOF
	}
	return
}

// Buffer pool for reusing byte slices to reduce allocations
var bufferPool = sync.Pool{
	New: func() any {
		buf := make([]byte, 8192) // 8KB initial buffer
		return &buf
	},
}

// Optimized buffered reader pool
var bufferedReaderPool = sync.Pool{
	New: func() any {
		return bufio.NewReaderSize(nil, 32768) // 32KB buffer
	},
}

// Large buffer pool for high-performance operations
var largeBufPool = sync.Pool{
	New: func() any {
		buf := make([]byte, 65536) // 64KB buffer
		return &buf
	},
}

// unmarshalVTStreamFast is an ultra-optimized version that pre-allocates larger buffers
// and uses bulk reading strategies for better performance on large datasets
func unmarshalVTStreamFast(m *pbstore.StoreData, reader io.Reader, estimatedSize int64) (dataSize uint64, err error) {
	// Use larger buffer for better performance
	bufPtr := largeBufPool.Get().(*[]byte)
	defer func() {
		// Ensure buffer is always returned to pool, even on panic
		largeBufPool.Put(bufPtr)
	}()

	// Create larger buffered reader based on estimated size
	bufferSize := int64(32768) // Default 32KB
	if estimatedSize > 0 {
		// Use 1/16 of estimated size for buffer, capped at 1MB
		proposed := estimatedSize / 16
		if proposed > 1048576 {
			proposed = 1048576
		}
		if proposed < 32768 {
			proposed = 32768
		}
		bufferSize = proposed
	}

	br := bufio.NewReaderSize(reader, int(bufferSize))
	workBuf := *bufPtr

	// Expand working buffer if needed for large entries
	if estimatedSize > 65536 {
		if int64(len(workBuf)) < estimatedSize/8 {
			workBuf = make([]byte, estimatedSize/8)
			*bufPtr = workBuf
		}
	}

	// Fast varint reading
	readVarint := func() (uint64, error) {
		var value uint64
		var shift uint
		for {
			if shift >= 64 {
				return 0, pbstore.ErrIntOverflow
			}
			b, err := br.ReadByte()
			if err != nil {
				return 0, err
			}
			value |= uint64(b&0x7F) << shift
			if b < 0x80 {
				break
			}
			shift += 7
		}
		return value, nil
	}

	// High-performance exact read with buffer growth, returns copy for persistent data
	readExactFast := func(n int) ([]byte, error) {
		if n > len(workBuf) {
			// Exponential growth for large allocations
			newSize := max(len(workBuf)*2, n)
			workBuf = make([]byte, newSize)
			*bufPtr = workBuf
		}
		_, err := io.ReadFull(br, workBuf[:n])
		if err != nil {
			return nil, err
		}
		// Return a copy since the working buffer will be reused
		result := make([]byte, n)
		copy(result, workBuf[:n])
		return result, nil
	}

	for {
		wire, err := readVarint()
		if err != nil {
			if err == io.EOF {
				break
			}
			return 0, err
		}

		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)

		if wireType == 4 {
			return 0, fmt.Errorf("proto: StoreData: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return 0, fmt.Errorf("proto: StoreData: illegal tag %d (wire type %d)", fieldNum, wire)
		}

		switch fieldNum {
		case 1: // kv field
			if wireType != 2 {
				return 0, fmt.Errorf("proto: wrong wireType = %d for field Kv", wireType)
			}

			msglen, err := readVarint()
			if err != nil {
				return 0, err
			}

			entryData, err := readExactFast(int(msglen))
			if err != nil {
				return 0, err
			}

			if m.Kv == nil {
				m.Kv = make(map[string][]byte)
			}

			var mapkey string
			var mapvalue []byte

			// Fast parse map entry
			iNdEx := 0
			l := int(msglen)
			for iNdEx < l {
				var wire uint64
				for shift := uint(0); ; shift += 7 {
					if shift >= 64 {
						return 0, pbstore.ErrIntOverflow
					}
					if iNdEx >= l {
						return 0, io.ErrUnexpectedEOF
					}
					b := entryData[iNdEx]
					iNdEx++
					wire |= uint64(b&0x7F) << shift
					if b < 0x80 {
						break
					}
				}
				fieldNum := int32(wire >> 3)

				if fieldNum == 1 { // key
					var stringLenmapkey uint64
					for shift := uint(0); ; shift += 7 {
						if shift >= 64 {
							return 0, pbstore.ErrIntOverflow
						}
						if iNdEx >= l {
							return 0, io.ErrUnexpectedEOF
						}
						b := entryData[iNdEx]
						iNdEx++
						stringLenmapkey |= uint64(b&0x7F) << shift
						if b < 0x80 {
							break
						}
					}
					intStringLenmapkey := int(stringLenmapkey)
					if intStringLenmapkey < 0 {
						return 0, pbstore.ErrInvalidLength
					}
					postStringIndexmapkey := iNdEx + intStringLenmapkey
					if postStringIndexmapkey < 0 {
						return 0, pbstore.ErrInvalidLength
					}
					if postStringIndexmapkey > l {
						return 0, io.ErrUnexpectedEOF
					}

					// @julien do not waste time allocating here
					mapkey = unsafeGetString(entryData[iNdEx:postStringIndexmapkey])
					iNdEx = postStringIndexmapkey

				} else if fieldNum == 2 { // value
					var mapbyteLen uint64
					for shift := uint(0); ; shift += 7 {
						if shift >= 64 {
							return 0, pbstore.ErrIntOverflow
						}
						if iNdEx >= l {
							return 0, io.ErrUnexpectedEOF
						}
						b := entryData[iNdEx]
						iNdEx++
						mapbyteLen |= uint64(b&0x7F) << shift
						if b < 0x80 {
							break
						}
					}
					intMapbyteLen := int(mapbyteLen)
					if intMapbyteLen < 0 {
						return 0, pbstore.ErrInvalidLength
					}
					postbytesIndex := iNdEx + intMapbyteLen
					if postbytesIndex < 0 {
						return 0, pbstore.ErrInvalidLength
					}
					if postbytesIndex > l {
						return 0, io.ErrUnexpectedEOF
					}

					// @julien do not waste time allocating here
					mapvalue = entryData[iNdEx:postbytesIndex]
					iNdEx = postbytesIndex
				} else {
					skippy, err := skipInBuffer(entryData[iNdEx:])
					if err != nil {
						return 0, err
					}
					if skippy < 0 || iNdEx+skippy < 0 {
						return 0, pbstore.ErrInvalidLength
					}
					if iNdEx+skippy > l {
						return 0, io.ErrUnexpectedEOF
					}
					iNdEx += skippy
				}
			}

			m.Kv[mapkey] = mapvalue
			dataSize += uint64(len(mapkey) + len(mapvalue))

		case 2: // delete_prefixes field
			if wireType != 2 {
				return 0, fmt.Errorf("proto: wrong wireType = %d for field DeletePrefixes", wireType)
			}

			stringLen, err := readVarint()
			if err != nil {
				return 0, err
			}

			stringData, err := readExactFast(int(stringLen))
			if err != nil {
				return 0, err
			}

			// Create a copy for persistent storage since stringData will be reused
			prefixCopy := make([]byte, len(stringData))
			copy(prefixCopy, stringData)
			// @julien do not waste time allocating here
			m.DeletePrefixes = append(m.DeletePrefixes, unsafeGetString(prefixCopy))

		default:
			if err := skipFieldFromBufferedReader(br, wireType); err != nil {
				return 0, err
			}
		}
	}

	return dataSize, nil
}

// unmarshalVTStream unmarshals data from a stream using standard buffer sizes
func unmarshalVTStream(m *pbstore.StoreData, reader io.Reader) (dataSize uint64, err error) {
	// Get pooled buffer and buffered reader
	bufPtr := bufferPool.Get().(*[]byte)
	defer func() {
		// Ensure buffer is always returned to pool, even on panic
		bufferPool.Put(bufPtr)
	}()

	br := bufferedReaderPool.Get().(*bufio.Reader)
	br.Reset(reader)
	defer func() {
		// Ensure buffered reader is always returned to pool, even on panic
		bufferedReaderPool.Put(br)
	}()

	workBuf := *bufPtr

	// Fast varint reading using buffered reader
	readVarint := func() (uint64, error) {
		var value uint64
		var shift uint
		for {
			if shift >= 64 {
				return 0, pbstore.ErrIntOverflow
			}
			b, err := br.ReadByte()
			if err != nil {
				return 0, err
			}
			value |= uint64(b&0x7F) << shift
			if b < 0x80 {
				break
			}
			shift += 7
		}
		return value, nil
	}

	// Optimized function to read exact bytes, returning a copy for persistent data
	readExactOptimized := func(n int) ([]byte, error) {
		if n > len(workBuf) {
			// Grow buffer if needed
			workBuf = make([]byte, n*2)
			*bufPtr = workBuf
		}
		_, err := io.ReadFull(br, workBuf[:n])
		if err != nil {
			return nil, err
		}
		// Return a copy since the working buffer will be reused
		result := make([]byte, n)
		copy(result, workBuf[:n])
		return result, nil
	}

	for {
		// Read wire type and field number
		wire, err := readVarint()
		if err != nil {
			if err == io.EOF {
				break
			}
			return 0, err
		}

		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)

		if wireType == 4 {
			return 0, fmt.Errorf("proto: StoreData: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return 0, fmt.Errorf("proto: StoreData: illegal tag %d (wire type %d)", fieldNum, wire)
		}

		switch fieldNum {
		case 1: // kv field
			if wireType != 2 {
				return 0, fmt.Errorf("proto: wrong wireType = %d for field Kv", wireType)
			}

			// Read message length
			msglen, err := readVarint()
			if err != nil {
				return 0, err
			}

			// Read the entire map entry using optimized buffer
			entryData, err := readExactOptimized(int(msglen))
			if err != nil {
				return 0, err
			}

			if m.Kv == nil {
				m.Kv = make(map[string][]byte)
			}

			var mapkey string
			var mapvalue []byte

			// Parse the map entry from the buffer
			iNdEx := 0
			l := int(msglen)
			for iNdEx < l {
				// Read field wire
				var wire uint64
				for shift := uint(0); ; shift += 7 {
					if shift >= 64 {
						return 0, pbstore.ErrIntOverflow
					}
					if iNdEx >= l {
						return 0, io.ErrUnexpectedEOF
					}
					b := entryData[iNdEx]
					iNdEx++
					wire |= uint64(b&0x7F) << shift
					if b < 0x80 {
						break
					}
				}
				fieldNum := int32(wire >> 3)

				if fieldNum == 1 { // key
					var stringLenmapkey uint64
					for shift := uint(0); ; shift += 7 {
						if shift >= 64 {
							return 0, pbstore.ErrIntOverflow
						}
						if iNdEx >= l {
							return 0, io.ErrUnexpectedEOF
						}
						b := entryData[iNdEx]
						iNdEx++
						stringLenmapkey |= uint64(b&0x7F) << shift
						if b < 0x80 {
							break
						}
					}
					intStringLenmapkey := int(stringLenmapkey)
					if intStringLenmapkey < 0 {
						return 0, pbstore.ErrInvalidLength
					}
					postStringIndexmapkey := iNdEx + intStringLenmapkey
					if postStringIndexmapkey < 0 {
						return 0, pbstore.ErrInvalidLength
					}
					if postStringIndexmapkey > l {
						return 0, io.ErrUnexpectedEOF
					}

					// @julien do not waste time allocating here
					mapkey = unsafeGetString(entryData[iNdEx:postStringIndexmapkey])
					iNdEx = postStringIndexmapkey

				} else if fieldNum == 2 { // value
					var mapbyteLen uint64
					for shift := uint(0); ; shift += 7 {
						if shift >= 64 {
							return 0, pbstore.ErrIntOverflow
						}
						if iNdEx >= l {
							return 0, io.ErrUnexpectedEOF
						}
						b := entryData[iNdEx]
						iNdEx++
						mapbyteLen |= uint64(b&0x7F) << shift
						if b < 0x80 {
							break
						}
					}
					intMapbyteLen := int(mapbyteLen)
					if intMapbyteLen < 0 {
						return 0, pbstore.ErrInvalidLength
					}
					postbytesIndex := iNdEx + intMapbyteLen
					if postbytesIndex < 0 {
						return 0, pbstore.ErrInvalidLength
					}
					if postbytesIndex > l {
						return 0, io.ErrUnexpectedEOF
					}

					// @julien do not waste time allocating here
					mapvalue = entryData[iNdEx:postbytesIndex]
					iNdEx = postbytesIndex
				} else {
					// Skip unknown field
					skippy, err := skipInBuffer(entryData[iNdEx:])
					if err != nil {
						return 0, err
					}
					if skippy < 0 || iNdEx+skippy < 0 {
						return 0, pbstore.ErrInvalidLength
					}
					if iNdEx+skippy > l {
						return 0, io.ErrUnexpectedEOF
					}
					iNdEx += skippy
				}
			}

			m.Kv[mapkey] = mapvalue
			dataSize += uint64(len(mapkey) + len(mapvalue))

		case 2: // delete_prefixes field
			if wireType != 2 {
				return 0, fmt.Errorf("proto: wrong wireType = %d for field DeletePrefixes", wireType)
			}

			// Read string length
			stringLen, err := readVarint()
			if err != nil {
				return 0, err
			}

			// Read string data using optimized buffer
			stringData, err := readExactOptimized(int(stringLen))
			if err != nil {
				return 0, err
			}

			// Create a copy for persistent storage since stringData will be reused
			prefixCopy := make([]byte, len(stringData))
			copy(prefixCopy, stringData)
			// @julien do not waste time allocating here
			m.DeletePrefixes = append(m.DeletePrefixes, unsafeGetString(prefixCopy))

		default:
			// Skip unknown field using buffered reader
			if err := skipFieldFromBufferedReader(br, wireType); err != nil {
				return 0, err
			}
		}
	}

	return dataSize, nil
}

// skipFieldFromBufferedReader skips a field by reading from the buffered stream
func skipFieldFromBufferedReader(br *bufio.Reader, wireType int) error {
	readVarint := func() (uint64, error) {
		var value uint64
		var shift uint
		for {
			if shift >= 64 {
				return 0, pbstore.ErrIntOverflow
			}
			b, err := br.ReadByte()
			if err != nil {
				return 0, err
			}
			value |= uint64(b&0x7F) << shift
			if b < 0x80 {
				break
			}
			shift += 7
		}
		return value, nil
	}

	switch wireType {
	case 0: // Varint
		_, err := readVarint()
		return err
	case 1: // Fixed64
		_, err := br.Discard(8)
		return err
	case 2: // Length-delimited
		length, err := readVarint()
		if err != nil {
			return err
		}
		// Skip the data efficiently
		_, err = br.Discard(int(length))
		return err
	case 5: // Fixed32
		_, err := br.Discard(4)
		return err
	default:
		return fmt.Errorf("proto: illegal wireType %d", wireType)
	}
}

// skipInBuffer is like skip but works on a buffer slice
func skipInBuffer(dAtA []byte) (n int, err error) {
	l := len(dAtA)
	iNdEx := 0
	if iNdEx < l {
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return 0, pbstore.ErrIntOverflow
			}
			if iNdEx >= l {
				return 0, io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		wireType := int(wire & 0x7)
		switch wireType {
		case 0:
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, pbstore.ErrIntOverflow
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				iNdEx++
				if dAtA[iNdEx-1] < 0x80 {
					break
				}
			}
		case 1:
			iNdEx += 8
		case 2:
			var length int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, pbstore.ErrIntOverflow
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				length |= (int(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if length < 0 {
				return 0, pbstore.ErrInvalidLength
			}
			iNdEx += length
		case 5:
			iNdEx += 4
		default:
			return 0, fmt.Errorf("proto: illegal wireType %d", wireType)
		}
		if iNdEx < 0 {
			return 0, pbstore.ErrInvalidLength
		}
		return iNdEx, nil
	}
	return 0, io.ErrUnexpectedEOF
}

func skip(dAtA []byte) (n int, err error) {
	l := len(dAtA)
	iNdEx := 0
	depth := 0
	for iNdEx < l {
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return 0, pbstore.ErrIntOverflow
			}
			if iNdEx >= l {
				return 0, io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		wireType := int(wire & 0x7)
		switch wireType {
		case 0:
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, pbstore.ErrIntOverflow
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				iNdEx++
				if dAtA[iNdEx-1] < 0x80 {
					break
				}
			}
		case 1:
			iNdEx += 8
		case 2:
			var length int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, pbstore.ErrIntOverflow
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				length |= (int(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if length < 0 {
				return 0, pbstore.ErrInvalidLength
			}
			iNdEx += length
		case 3:
			depth++
		case 4:
			if depth == 0 {
				return 0, pbstore.ErrUnexpectedEndOfGroup
			}
			depth--
		case 5:
			iNdEx += 4
		default:
			return 0, fmt.Errorf("proto: illegal wireType %d", wireType)
		}
		if iNdEx < 0 {
			return 0, pbstore.ErrInvalidLength
		}
		if depth == 0 {
			return iNdEx, nil
		}
	}
	return 0, io.ErrUnexpectedEOF
}
