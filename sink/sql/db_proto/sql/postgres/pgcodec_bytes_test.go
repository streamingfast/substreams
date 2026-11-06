package postgres

import (
	"encoding/binary"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	sqlbytes "github.com/streamingfast/substreams/sink/sql/bytes"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/postgres/pgcopy"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/spool"
	"github.com/stretchr/testify/require"
)

func TestPGSegmentCopyUsesConfiguredBytesEncoding(t *testing.T) {
	const tableName = "payloads"

	codec := newPGCodec(
		spool.FormatPGCopy,
		map[string]*pgcopy.Table{
			tableName: {
				Schema: "public",
				Name:   tableName,
				Columns: []pgcopy.Column{
					{Name: "payload", OID: pgtype.TextOID},
				},
			},
		},
		sqlbytes.EncodingHex,
	)
	segment := &pgSegment{
		dir:    t.TempDir(),
		codec:  codec,
		tables: map[string]*pgTableFile{},
	}

	require.NoError(t, segment.WriteRow(tableName, []any{[]byte{0xde, 0xad, 0xbe, 0xef}}))

	table := segment.tables[tableName]
	require.NoError(t, table.writer.Close())
	encoded, err := os.ReadFile(table.path)
	require.NoError(t, err)

	fieldCountOffset := pgcopy.HeaderSize
	require.Equal(t, int16(1), int16(binary.BigEndian.Uint16(encoded[fieldCountOffset:])))

	fieldLengthOffset := fieldCountOffset + 2
	fieldLength := int(binary.BigEndian.Uint32(encoded[fieldLengthOffset:]))
	fieldOffset := fieldLengthOffset + 4
	require.Equal(t, "deadbeef", string(encoded[fieldOffset:fieldOffset+fieldLength]))
}

func TestPGCopyNormalizationUsesConfiguredBytesEncodingForArrays(t *testing.T) {
	values := []any{[]any{
		[]byte{0xde, 0xad, 0xbe, 0xef},
		[]byte{0x00, 0xff},
	}}
	columns := []pgcopy.Column{{Name: "payloads", OID: pgtype.TextArrayOID}}

	require.NoError(t, pgcopy.NormalizeRowWithEncoding(columns, values, sqlbytes.EncodingHex))
	require.Equal(t, []string{"deadbeef", "00ff"}, values[0])
}
