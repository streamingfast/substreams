package streamproto

import (
	"bytes"
	"fmt"
	"io"
	"testing"
	"time"

	pb "github.com/streamingfast/substreams/storage/execout/pb"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func Benchmark_VarintEncode(b *testing.B) {
	testCases := []uint64{
		0,
		127,
		128,
		16383,
		16384,
		2097151,
		2097152,
		268435455,
		268435456,
		4294967295,
		4294967296,
		549755813887,
		549755813888,
		70368744177663,
		70368744177664,
		9223372036854775807,
		9223372036854775808,
		18446744073709551615,
	}

	for _, tc := range testCases {
		b.Run(fmt.Sprintf("encode_%d", tc), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = varintEncodeWithArrayPrefix(tc)
			}
		})
	}
}

func TestVarintEncodeWithArrayPrefix(t *testing.T) {
	testCases := []struct {
		input    uint64
		expected []byte
	}{
		{0, []byte{arrayPrefix, 0}},
		{1, []byte{arrayPrefix, 1}},
		{127, []byte{arrayPrefix, 127}},
		{128, []byte{arrayPrefix, 128, 1}},
		{300, []byte{arrayPrefix, 172, 2}},
		{16384, []byte{arrayPrefix, 128, 128, 1}},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("encode_%d", tc.input), func(t *testing.T) {
			result := varintEncodeWithArrayPrefix(tc.input)
			if len(result) != len(tc.expected) {
				t.Fatalf("Expected length %d, got %d for input %d",
					len(tc.expected), len(result), tc.input)
			}
			for i := range result {
				if result[i] != tc.expected[i] {
					t.Fatalf("Mismatch at position %d for input %d: expected %d, got %d",
						i, tc.input, tc.expected[i], result[i])
				}
			}
		})
	}
}

func TestVarintReadWrite(t *testing.T) {
	// Test specific sizes around the problematic threshold of 478805
	testSizes := []int{
		478800,  // Just below the reported threshold
		478805,  // The exact threshold reported as problematic
		478810,  // Just above the threshold
		500000,  // Further above
		1000000, // Much larger
	}

	for _, size := range testSizes {
		t.Run(fmt.Sprintf("size_%d", size), func(t *testing.T) {
			// Create an item with payload of specific size
			item := &pb.Item{
				BlockNum:  100,
				BlockId:   "test_block",
				Payload:   bytes.Repeat([]byte("x"), size),
				Timestamp: timestamppb.New(time.Now()),
				Cursor:    "test_cursor",
			}

			// Write the item
			var buf bytes.Buffer
			writtenSize, err := WriteItem(&buf, item)
			if err != nil {
				t.Fatalf("Failed to write item with size %d: %v", size, err)
			}

			t.Logf("Written size: %d bytes for payload size: %d", writtenSize, size)

			// Read the item back
			reader := bytes.NewReader(buf.Bytes())
			readItem, err := ReadNextItem(reader)
			if err != nil {
				t.Fatalf("Failed to read item with size %d: %v", size, err)
			}

			// Verify the payload size matches
			if len(readItem.Payload) != size {
				t.Fatalf("Payload size mismatch for size %d: expected %d, got %d",
					size, size, len(readItem.Payload))
			}

			// Verify the content matches
			expectedPayload := bytes.Repeat([]byte("x"), size)
			if !bytes.Equal(readItem.Payload, expectedPayload) {
				t.Fatalf("Payload content mismatch for size %d", size)
			}

			t.Logf("Successfully read/write test passed for size %d", size)
		})
	}
}

func TestVarintEncodingDetails(t *testing.T) {
	// Test the exact encoding/decoding of varints around the problem size
	testCases := []uint64{
		478800,
		478805,
		478810,
		478845, // This was the "getting from size" value we saw
		478850, // This was another "getting from size" value
	}

	for _, value := range testCases {
		t.Run(fmt.Sprintf("varint_%d", value), func(t *testing.T) {
			// Test our custom varint encoding
			encoded := varintEncodeWithArrayPrefix(value)
			t.Logf("Value %d encoded as: %v (hex: %x)", value, encoded, encoded)

			// Manually decode to verify
			if encoded[0] != arrayPrefix {
				t.Fatalf("Expected array prefix %d, got %d", arrayPrefix, encoded[0])
			}

			// Test our readVarint function
			reader := bytes.NewReader(encoded[1:]) // Skip the array prefix
			decoded, err := readVarint(reader)
			if err != nil {
				t.Fatalf("Failed to decode varint for value %d: %v", value, err)
			}

			if decoded != value {
				t.Fatalf("Varint mismatch: encoded %d, decoded %d", value, decoded)
			}

			t.Logf("Varint encoding/decoding successful for %d", value)
		})
	}
}

func TestDetailedItemAnalysis(t *testing.T) {
	// Create an item with exactly 478805 bytes payload
	payloadSize := 478805
	item := &pb.Item{
		BlockNum:  100,
		BlockId:   "test_block",
		Payload:   bytes.Repeat([]byte("x"), payloadSize),
		Timestamp: timestamppb.New(time.Now()),
		Cursor:    "test_cursor",
	}

	// Marshal the item directly to see its size
	marshaledData, err := item.MarshalVT()
	if err != nil {
		t.Fatalf("Failed to marshal item: %v", err)
	}

	t.Logf("Item marshaled size: %d bytes", len(marshaledData))
	t.Logf("Payload size: %d bytes", payloadSize)
	t.Logf("Overhead: %d bytes", len(marshaledData)-payloadSize)

	// Test the varint encoding of the marshaled size
	encodedSize := varintEncodeWithArrayPrefix(uint64(len(marshaledData)))
	t.Logf("Size varint encoding: %v (hex: %x)", encodedSize, encodedSize)
	t.Logf("Size varint length: %d bytes", len(encodedSize))

	// Write using our WriteItem function
	var buf bytes.Buffer
	writtenSize, err := WriteItem(&buf, item)
	if err != nil {
		t.Fatalf("Failed to write item: %v", err)
	}

	t.Logf("Total written size: %d bytes", writtenSize)
	t.Logf("Expected total: %d bytes", len(encodedSize)+len(marshaledData))

	// Examine the first few bytes of the buffer
	bufBytes := buf.Bytes()
	headerBytes := bufBytes[:min(20, len(bufBytes))]
	t.Logf("First 20 bytes of written data: %v (hex: %x)", headerBytes, headerBytes)

	// Now try to read it back step by step
	reader := bytes.NewReader(bufBytes)

	// Read array prefix
	prefixBuf := make([]byte, 1)
	if _, err := reader.Read(prefixBuf); err != nil {
		t.Fatalf("Failed to read array prefix: %v", err)
	}
	t.Logf("Read array prefix: %d", prefixBuf[0])

	// Read the size varint
	size, err := readVarint(reader)
	if err != nil {
		t.Fatalf("Failed to read size varint: %v", err)
	}
	t.Logf("Read size from varint: %d", size)
	t.Logf("Expected size: %d", len(marshaledData))

	if size != uint64(len(marshaledData)) {
		t.Fatalf("Size mismatch: expected %d, got %d", len(marshaledData), size)
	}
}

func TestEncode(t *testing.T) {
	// Create some sample items
	items := []*pb.Item{
		{
			BlockNum:  100,
			BlockId:   "abc123",
			Payload:   []byte("payload1"),
			Timestamp: timestamppb.New(time.Now()),
			Cursor:    "cursor1",
		},
		{
			BlockNum:  101,
			BlockId:   "def456",
			Payload:   []byte("payload2"),
			Timestamp: timestamppb.New(time.Now()),
			Cursor:    "cursor2",
		},

		{
			BlockNum:  102,
			BlockId:   "def789",
			Payload:   bytes.Repeat([]byte("hello"), 9999999),
			Timestamp: timestamppb.New(time.Now()),
			Cursor:    "cursor2",
		},
	}

	arr := pb.Array{
		Items: items,
	}

	encodedFull, err := proto.Marshal(&arr)
	if err != nil {
		t.Fatalf("Failed to encode data: %v", err)
	}

	writer := bytes.NewBuffer(nil)
	var encodedStream []byte

	for _, item := range items {
		_, err := WriteItem(writer, item)
		if err != nil {
			t.Fatalf("Failed to stream item: %v", err)
		}
	}

	encodedStream = writer.Bytes()

	// Compare lengths
	t.Logf("Full encoded length: %d, Streamed encoded length: %d",
		len(encodedFull), len(encodedStream))

	// For debugging, you can also compare the actual bytes
	if !bytes.Equal(encodedFull, encodedStream) {
		t.Logf("Encoding methods produced different results")
		// You might want to examine the first few bytes to see where they differ
		// Limit to first 20 bytes for readability
		compareLength := min(min(len(encodedFull), len(encodedStream)), 20)

		t.Logf("First %d bytes of full encoding: %v", compareLength, encodedFull[:compareLength])
		t.Logf("First %d bytes of streamed encoding: %v", compareLength, encodedStream[:compareLength])
	} else {
		t.Logf("Both encoding methods produced identical results")
	}

}

func TestVarintReaderDiagnostics(t *testing.T) {
	// Test different types of readers to diagnose varint reading issues
	problemSize := 478805
	item := &pb.Item{
		BlockNum:  100,
		BlockId:   "test_block",
		Payload:   bytes.Repeat([]byte("x"), problemSize),
		Timestamp: timestamppb.New(time.Now()),
		Cursor:    "test_cursor",
	}

	// Write the item
	var buf bytes.Buffer
	_, err := WriteItem(&buf, item)
	if err != nil {
		t.Fatalf("Failed to write item: %v", err)
	}

	data := buf.Bytes()
	t.Logf("Total written data size: %d bytes", len(data))

	// Test 1: bytes.Reader (should work)
	t.Run("bytes.Reader", func(t *testing.T) {
		reader := bytes.NewReader(data)
		item, err := ReadNextItem(reader)
		if err != nil {
			t.Fatalf("Failed with bytes.Reader: %v", err)
		}
		if len(item.Payload) != problemSize {
			t.Fatalf("Payload size mismatch: expected %d, got %d", problemSize, len(item.Payload))
		}
		t.Logf("bytes.Reader: SUCCESS")
	})

	// Test 2: Limited reader (simulates network/streaming conditions)
	t.Run("limited_reader", func(t *testing.T) {
		reader := io.LimitReader(bytes.NewReader(data), int64(len(data)))
		item, err := ReadNextItem(reader)
		if err != nil {
			t.Fatalf("Failed with LimitReader: %v", err)
		}
		if len(item.Payload) != problemSize {
			t.Fatalf("Payload size mismatch: expected %d, got %d", problemSize, len(item.Payload))
		}
		t.Logf("LimitReader: SUCCESS")
	})

	// Test 3: Slow reader (simulates partial reads)
	t.Run("slow_reader", func(t *testing.T) {
		slowReader := &slowReader{data: data, maxBytesPerRead: 1}
		item, err := ReadNextItem(slowReader)
		if err != nil {
			t.Fatalf("Failed with slowReader: %v", err)
		}
		if len(item.Payload) != problemSize {
			t.Fatalf("Payload size mismatch: expected %d, got %d", problemSize, len(item.Payload))
		}
		t.Logf("slowReader (1 byte/read): SUCCESS")
	})

	// Test 4: Chunk reader (simulates buffered reading)
	t.Run("chunk_reader", func(t *testing.T) {
		chunkReader := &slowReader{data: data, maxBytesPerRead: 1024}
		item, err := ReadNextItem(chunkReader)
		if err != nil {
			t.Fatalf("Failed with chunkReader: %v", err)
		}
		if len(item.Payload) != problemSize {
			t.Fatalf("Payload size mismatch: expected %d, got %d", problemSize, len(item.Payload))
		}
		t.Logf("chunkReader (1024 bytes/read): SUCCESS")
	})

	// Test 5: Varint-only test with different readers
	t.Run("varint_only", func(t *testing.T) {
		// Extract just the varint portion (skip array prefix)
		if data[0] != arrayPrefix {
			t.Fatalf("Expected array prefix, got %d", data[0])
		}

		// Find where varint ends
		varintEnd := 1
		for varintEnd < len(data) && data[varintEnd]&0x80 != 0 {
			varintEnd++
		}
		varintEnd++ // Include the last byte

		varintData := data[1:varintEnd]
		t.Logf("Varint data: %v (hex: %x)", varintData, varintData)

		// Test with different readers
		readers := map[string]io.Reader{
			"bytes.Reader": bytes.NewReader(varintData),
			"slowReader":   &slowReader{data: varintData, maxBytesPerRead: 1},
			"chunkReader":  &slowReader{data: varintData, maxBytesPerRead: 1024},
		}

		for name, reader := range readers {
			value, err := readVarint(reader)
			if err != nil {
				t.Fatalf("Varint failed with %s: %v", name, err)
			}
			t.Logf("Varint with %s: %d", name, value)
		}
	})
}

// slowReader simulates a reader that returns data slowly or in chunks
type slowReader struct {
	data            []byte
	pos             int
	maxBytesPerRead int
}

func (r *slowReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}

	remaining := len(r.data) - r.pos
	toRead := min(len(p), r.maxBytesPerRead)
	if toRead > remaining {
		toRead = remaining
	}

	copy(p, r.data[r.pos:r.pos+toRead])
	r.pos += toRead
	return toRead, nil
}
