package streamproto

import (
	"bytes"
	"fmt"
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
		compareLength := min(len(encodedFull), len(encodedStream))
		if compareLength > 20 {
			compareLength = 20 // Limit to first 20 bytes for readability
		}

		t.Logf("First %d bytes of full encoding: %v", compareLength, encodedFull[:compareLength])
		t.Logf("First %d bytes of streamed encoding: %v", compareLength, encodedStream[:compareLength])
	} else {
		t.Logf("Both encoding methods produced identical results")
	}

}
