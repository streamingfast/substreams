package streamproto

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	pb "github.com/streamingfast/substreams/storage/execout/pb"
)

func WriteItem(writer io.Writer, item *pb.Item) (size int, err error) {
	data, err := item.MarshalVT()
	if err != nil {
		return 0, fmt.Errorf("failed to marshal item: %w", err)
	}

	prefixedSizeEncoding := varintEncodeWithArrayPrefix(uint64(len(data)))

	if _, err := writer.Write(append(prefixedSizeEncoding, data...)); err != nil {
		return 0, fmt.Errorf("failed to write array item: %w", err)
	}
	return len(prefixedSizeEncoding) + len(data), nil
}

func WriteOrderedBool(writer io.Writer) (size int, err error) {
	return writer.Write(boolField2True)
}

func ReadOrderedBool(reader io.Reader) (isOrdered bool, readBytes []byte, err error) {
	readBytes = make([]byte, 2)
	if _, err := reader.Read(readBytes); err != nil {
		if err == io.EOF {
			return true, nil, nil // an empty array can be considered "ordered"
		}
		return false, nil, fmt.Errorf("failed to read ordered bool: %w", err)
	}
	return bytes.Equal(readBytes, boolField2True), readBytes, nil
}

// When you marshal an Array object, the first bytes will encode field 1 (items) with wire type 2:
// `(1 << 3) | 2 = 8 | 2 = 10 = 0x0a`
const arrayPrefix = byte(10)

// When you marshal a bool field with field number 2 and value true:
// `(2 << 3) | 0 = 16 | 0 = 16 = 0x10` (field 2, wire type 0 for varint)
// followed by `0x01` for true value
var boolField2True = []byte{0x10, 0x01}

func varintEncodeWithArrayPrefix(i uint64) []byte {
	var buf [11]byte // Max varint size for uint64 + 1 byte for prefix
	buf[0] = arrayPrefix
	n := binary.PutUvarint(buf[1:], i)
	return buf[:n+1]
}

func ReadNextItem(reader io.Reader) (item *pb.Item, err error) {
	// Read the array prefix byte
	prefixBuf := make([]byte, 1)
	if _, err := reader.Read(prefixBuf); err != nil {
		if err == io.EOF {
			return nil, io.EOF
		}
		return nil, fmt.Errorf("failed to read array prefix: %w", err)
	}

	if prefixBuf[0] != arrayPrefix {
		return nil, fmt.Errorf("invalid array prefix: expected %d, got %d", arrayPrefix, prefixBuf[0])
	}

	size, err := readVarint(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read item size: %w", err)
	}

	itemData := make([]byte, size)
	if _, err := io.ReadFull(reader, itemData); err != nil {
		return nil, fmt.Errorf("failed to read item data: %w", err)
	}

	// itemData is a fresh allocation nothing else reads, so the item aliases it
	// instead of copying the payload out.
	item = &pb.Item{}
	if err := item.UnmarshalVTUnsafe(itemData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal item: %w", err)
	}

	return item, nil
}

func readVarint(reader io.Reader) (uint64, error) {
	var value uint64
	var shift uint
	var bytesRead int

	for {
		buf := make([]byte, 1)
		n, err := reader.Read(buf)
		if err != nil {
			return 0, fmt.Errorf("varint read error after %d bytes: %w", bytesRead, err)
		}
		if n != 1 {
			return 0, fmt.Errorf("varint incomplete read: expected 1 byte, got %d after %d bytes", n, bytesRead)
		}

		bytesRead++
		b := buf[0]

		// Check for potential overflow before applying the shift
		if shift >= 64 {
			return 0, fmt.Errorf("varint overflow: too many bytes (%d)", bytesRead)
		}

		// Add the 7 bits from this byte to our value
		value |= uint64(b&0x7F) << shift

		// If MSB is 0, this is the last byte
		if b&0x80 == 0 {
			break
		}

		shift += 7

		// Protect against infinite loops with malformed data
		if bytesRead > 10 {
			return 0, fmt.Errorf("varint too long: read %d bytes without termination", bytesRead)
		}
	}

	return value, nil
}
