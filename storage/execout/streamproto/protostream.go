package streamproto

import (
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

// When you marshal an Array object, the first bytes will encode field 1 (items) with wire type 2:
// `(1 << 3) | 2 = 8 | 2 = 10 = 0x0a`
const arrayPrefix = byte(10)

func varintEncodeWithArrayPrefix(i uint64) []byte {
	var buf [11]byte // Max varint size for uint64 + 1 byte for prefix
	buf[0] = arrayPrefix
	n := binary.PutUvarint(buf[1:], i)
	return buf[:n+1]
}
