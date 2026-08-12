package clickhouse

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/spool"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// chCodec spools rows as typed values, one file per table.
//
// ClickHouse inserts columnar and typed rather than as SQL text, so rendering to literals
// the way the PostgreSQL formats do would change both the insert path and how types are
// handled. Keeping the values means a spooled row is appended back into exactly the same
// column builders the accumulator has always used.
type chCodec struct{}

func newCHCodec() *chCodec { return &chCodec{} }

func (c *chCodec) Format() spool.Format { return spool.FormatValues }

func (c *chCodec) OpenSegment(dir string) (spool.SegmentWriter, error) {
	return &chSegment{dir: dir, tables: map[string]*chTableFile{}}, nil
}

// Verify checks each file's length against the manifest. The format is length-framed, so
// that is the whole of it: a torn write leaves the file short of what was recorded.
func (c *chCodec) Verify(dir string, manifest *spool.Manifest) error {
	for _, table := range manifest.Tables {
		path := filepath.Join(dir, table.File)

		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("%s is missing: %w", table.File, err)
		}
		if info.Size() != table.Bytes {
			return fmt.Errorf("%s is %d bytes, the manifest recorded %d", table.File, info.Size(), table.Bytes)
		}
	}

	return nil
}

type chTableFile struct {
	path   string
	file   *os.File
	writer *spool.FrameWriter
	rows   int64
}

type chSegment struct {
	dir    string
	tables map[string]*chTableFile
}

func (s *chSegment) WriteRow(table string, values []any) error {
	target, ok := s.tables[table]
	if !ok {
		path := filepath.Join(s.dir, spool.SanitizeFileName(table)+".values")
		file, err := os.Create(path)
		if err != nil {
			return fmt.Errorf("creating %s: %w", path, err)
		}

		target = &chTableFile{path: path, file: file, writer: spool.NewFrameWriter(file)}
		s.tables[table] = target
	}

	encoded, err := encodeValues(values)
	if err != nil {
		return fmt.Errorf("encoding a row of %q: %w", table, err)
	}
	target.rows++

	return target.writer.WriteRecord(encoded)
}

func (s *chSegment) PendingBytes() int64 {
	var total int64
	for _, target := range s.tables {
		total += target.writer.Bytes()
	}

	return total
}

func (s *chSegment) Seal(manifest *spool.Manifest) error {
	for name, target := range s.tables {
		if err := target.writer.Close(); err != nil {
			return fmt.Errorf("closing the stream of %q: %w", name, err)
		}

		info, err := os.Stat(target.path)
		if err != nil {
			return fmt.Errorf("sizing %s: %w", target.path, err)
		}

		manifest.Tables = append(manifest.Tables, spool.TableRecord{
			Name:  name,
			File:  filepath.Base(target.path),
			Rows:  target.rows,
			Bytes: info.Size(),
		})
	}

	return nil
}

func (s *chSegment) Discard() {
	for _, target := range s.tables {
		target.file.Close()
	}
	os.RemoveAll(s.dir)
}

// Value tags. Every type the accumulator's column switch consumes has one, and anything
// else is an error at spool time rather than a silent mangling at apply time.
const (
	tagNil byte = iota
	tagBool
	tagInt32
	tagInt64
	tagUint32
	tagUint64
	tagFloat32
	tagFloat64
	tagString
	tagBytes
	tagTime
	tagList
)

// encodeValues writes one row as tag-prefixed values.
//
// A *timestamppb.Timestamp is normalised to time.Time on the way in: the accumulator
// accepts either, and carrying one shape rather than two keeps the decoder honest.
func encodeValues(values []any) (string, error) {
	out := make([]byte, 0, 64)

	var err error
	for _, value := range values {
		out, err = appendValue(out, value)
		if err != nil {
			return "", err
		}
	}

	return string(out), nil
}

func appendValue(out []byte, value any) ([]byte, error) {
	switch v := value.(type) {
	case nil:
		return append(out, tagNil), nil
	case bool:
		if v {
			return append(out, tagBool, 1), nil
		}
		return append(out, tagBool, 0), nil
	case int32:
		return binary.BigEndian.AppendUint32(append(out, tagInt32), uint32(v)), nil
	case int:
		return binary.BigEndian.AppendUint64(append(out, tagInt64), uint64(int64(v))), nil
	case int64:
		return binary.BigEndian.AppendUint64(append(out, tagInt64), uint64(v)), nil
	case uint32:
		return binary.BigEndian.AppendUint32(append(out, tagUint32), v), nil
	case uint:
		return binary.BigEndian.AppendUint64(append(out, tagUint64), uint64(v)), nil
	case uint64:
		return binary.BigEndian.AppendUint64(append(out, tagUint64), v), nil
	case float32:
		return binary.BigEndian.AppendUint32(append(out, tagFloat32), math.Float32bits(v)), nil
	case float64:
		return binary.BigEndian.AppendUint64(append(out, tagFloat64), math.Float64bits(v)), nil
	case string:
		return appendBlob(append(out, tagString), []byte(v)), nil
	case []byte:
		return appendBlob(append(out, tagBytes), v), nil
	case time.Time:
		return binary.BigEndian.AppendUint64(append(out, tagTime), uint64(v.UnixNano())), nil
	case *timestamppb.Timestamp:
		return binary.BigEndian.AppendUint64(append(out, tagTime), uint64(v.AsTime().UnixNano())), nil
	case []any:
		out = binary.BigEndian.AppendUint32(append(out, tagList), uint32(len(v)))
		var err error
		for _, element := range v {
			out, err = appendValue(out, element)
			if err != nil {
				return nil, err
			}
		}

		return out, nil
	}

	return nil, fmt.Errorf("cannot spool a value of type %T", value)
}

func appendBlob(out []byte, blob []byte) []byte {
	return append(binary.BigEndian.AppendUint32(out, uint32(len(blob))), blob...)
}

// decodeValues reads back one row.
func decodeValues(encoded string) ([]any, error) {
	data := []byte(encoded)

	var values []any
	for len(data) > 0 {
		value, rest, err := decodeValue(data)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
		data = rest
	}

	return values, nil
}

func decodeValue(data []byte) (any, []byte, error) {
	if len(data) == 0 {
		return nil, nil, fmt.Errorf("a row ended mid-value")
	}

	tag, rest := data[0], data[1:]
	switch tag {
	case tagNil:
		return nil, rest, nil
	case tagBool:
		if len(rest) < 1 {
			return nil, nil, fmt.Errorf("a bool ended mid-value")
		}
		return rest[0] == 1, rest[1:], nil
	case tagInt32:
		v, rest, err := take4(rest)
		return int32(v), rest, err
	case tagInt64:
		v, rest, err := take8(rest)
		return int64(v), rest, err
	case tagUint32:
		v, rest, err := take4(rest)
		return v, rest, err
	case tagUint64:
		v, rest, err := take8(rest)
		return v, rest, err
	case tagFloat32:
		v, rest, err := take4(rest)
		return math.Float32frombits(v), rest, err
	case tagFloat64:
		v, rest, err := take8(rest)
		return math.Float64frombits(v), rest, err
	case tagString:
		blob, rest, err := takeBlob(rest)
		return string(blob), rest, err
	case tagBytes:
		blob, rest, err := takeBlob(rest)
		return blob, rest, err
	case tagTime:
		v, rest, err := take8(rest)
		return time.Unix(0, int64(v)).UTC(), rest, err
	case tagList:
		count, rest, err := take4(rest)
		if err != nil {
			return nil, nil, err
		}
		list := make([]any, 0, count)
		for range count {
			var element any
			element, rest, err = decodeValue(rest)
			if err != nil {
				return nil, nil, err
			}
			list = append(list, element)
		}

		return list, rest, nil
	}

	return nil, nil, fmt.Errorf("unknown value tag %d", tag)
}

func take4(data []byte) (uint32, []byte, error) {
	if len(data) < 4 {
		return 0, nil, fmt.Errorf("a 4 byte value ended after %d", len(data))
	}

	return binary.BigEndian.Uint32(data), data[4:], nil
}

func take8(data []byte) (uint64, []byte, error) {
	if len(data) < 8 {
		return 0, nil, fmt.Errorf("an 8 byte value ended after %d", len(data))
	}

	return binary.BigEndian.Uint64(data), data[8:], nil
}

func takeBlob(data []byte) ([]byte, []byte, error) {
	size, rest, err := take4(data)
	if err != nil {
		return nil, nil, err
	}
	if uint32(len(rest)) < size {
		return nil, nil, fmt.Errorf("a %d byte blob ended after %d", size, len(rest))
	}

	return rest[:size], rest[size:], nil
}
