package marshaller

import (
	"bytes"
	"io"
	"testing"
)

// TestMarshalStream_LazyEquivalence ensures the lazy streaming marshaler emits
// stable, wire-valid bytes regardless of the caller's read buffer size
// (including reads smaller than a single entry, and values larger than the read
// buffer). Bytes are not compared to Marshal directly because Marshal uses Go
// map iteration order while the stream sorts keys; both are valid protobuf.
func TestMarshalStream_LazyEquivalence(t *testing.T) {
	p := &VTproto{}

	data := &StoreData{
		Kv: map[string][]byte{
			"a":       []byte("1"),
			"b":       bytes.Repeat([]byte("x"), 1024), // value bigger than tiny read buffers
			"cc":      []byte(""),
			"key-zzz": bytes.Repeat([]byte("y"), 4096),
		},
		DeletePrefixes: []string{"pre1", "pre2"},
	}

	var reference []byte
	for _, bufSize := range []int{1, 3, 7, 64, 1024, 1 << 20} {
		r := p.MarshalStream(data, 0)
		got, err := io.ReadAll(&fixedReader{r: r, size: bufSize})
		r.Close()
		if err != nil {
			t.Fatalf("bufSize=%d ReadAll: %v", bufSize, err)
		}

		// Every buffer size must yield identical bytes (partial-read correctness
		// + deterministic sorted-key ordering).
		if reference == nil {
			reference = got
		} else if !bytes.Equal(got, reference) {
			t.Fatalf("bufSize=%d: streamed bytes differ from reference (got %d, want %d)", bufSize, len(got), len(reference))
		}

		// And it must round-trip back to the same store.
		out, _, err := p.Unmarshal(got)
		if err != nil {
			t.Fatalf("bufSize=%d Unmarshal: %v", bufSize, err)
		}
		if len(out.Kv) != len(data.Kv) {
			t.Fatalf("bufSize=%d: kv count %d != %d", bufSize, len(out.Kv), len(data.Kv))
		}
		for k, v := range data.Kv {
			if !bytes.Equal(out.Kv[k], v) {
				t.Fatalf("bufSize=%d: kv[%q] mismatch", bufSize, k)
			}
		}
	}
}

// TestMarshalStream_Empty ensures an empty store streams to empty bytes.
func TestMarshalStream_Empty(t *testing.T) {
	p := &VTproto{}
	r := p.MarshalStream(&StoreData{Kv: map[string][]byte{}}, 0)
	defer r.Close()
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty stream, got %d bytes", len(got))
	}
}

// fixedReader forces Read to be called with a bounded buffer size, exercising
// the marshaler's partial-entry read path.
type fixedReader struct {
	r    io.Reader
	size int
}

func (f *fixedReader) Read(p []byte) (int, error) {
	if len(p) > f.size {
		p = p[:f.size]
	}
	return f.r.Read(p)
}
