package spool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestFrameRoundTripsARecordLargerThanAnyFixedCeiling covers the row a rendered write mode
// can produce and a fixed read ceiling used to refuse.
//
// The writer accepted it, the manifest matched the bytes on disk so Verify passed, and the
// read failed only at apply — after which recovery replayed the same segment on every
// start and the sink could not come up at all.
func TestFrameRoundTripsARecordLargerThanAnyFixedCeiling(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rows.tuples")
	file, err := os.Create(path)
	require.NoError(t, err)

	// Comfortably past the 64MiB ceiling this used to carry.
	field := strings.Repeat("x", (64<<20)+1)

	writer := NewFrameWriter(file)
	require.NoError(t, writer.WriteRecord(field))
	require.NoError(t, writer.Close())

	reader, err := OpenFrameReader(path)
	require.NoError(t, err)
	defer reader.Close()

	read, err := reader.ReadField()
	require.NoError(t, err)
	require.Equal(t, len(field), len(read))
}

// TestFrameRejectsALengthTheFileCannotHold is what replaced the fixed ceiling: a corrupt
// prefix still cannot make the reader allocate, because no record can be longer than what
// is left of the file carrying it.
func TestFrameRejectsALengthTheFileCannotHold(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rows.tuples")
	// A prefix claiming ~4GiB in front of three bytes of payload.
	require.NoError(t, os.WriteFile(path, []byte{0xFF, 0xFF, 0xFF, 0xF0, 'a', 'b', 'c'}, 0o600))

	reader, err := OpenFrameReader(path)
	require.NoError(t, err)
	defer reader.Close()

	_, err = reader.ReadField()
	require.ErrorContains(t, err, "does not fit in the 3 bytes left of the file")
}

// TestFrameReadsSeveralRecordsBack pins that the running bound does not drift, the reader
// having to subtract both the prefix and the payload of every record it hands out.
func TestFrameReadsSeveralRecordsBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rows.log")
	file, err := os.Create(path)
	require.NoError(t, err)

	writer := NewFrameWriter(file)
	require.NoError(t, writer.WriteRecord("customers", "1,'alpha'"))
	require.NoError(t, writer.WriteRecord("orders", "2,'beta'"))
	require.NoError(t, writer.Close())

	reader, err := OpenFrameReader(path)
	require.NoError(t, err)
	defer reader.Close()

	var got []string
	for {
		field, err := reader.ReadField()
		if err != nil {
			break
		}
		got = append(got, field)
	}
	require.Equal(t, []string{"customers", "1,'alpha'", "orders", "2,'beta'"}, got)
}
