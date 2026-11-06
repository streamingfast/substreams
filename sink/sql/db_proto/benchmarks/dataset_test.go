package benchmarks

import (
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"math/big"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"
)

// The benchmark table mirrors the shape the from-proto sink produces for a typical
// entity: the two synthetic block columns, a string primary key, and a spread of the
// column types whose binary encodings are non-trivial (NUMERIC from uint64, BYTEA,
// TIMESTAMP, TEXT[], JSONB).
const (
	benchSchema = "bench"
	benchTable  = "transfers"
)

const createBenchTableSQL = `
CREATE TABLE IF NOT EXISTS bench.transfers (
	_block_number_    BIGINT NOT NULL,
	_block_timestamp_ TIMESTAMP NOT NULL,
	id                VARCHAR(255) NOT NULL,
	tx_hash           BYTEA NOT NULL,
	log_index         INTEGER NOT NULL,
	"from"            VARCHAR(255) NOT NULL,
	"to"              VARCHAR(255) NOT NULL,
	amount            NUMERIC NOT NULL,
	gas_used          NUMERIC NOT NULL,
	success           BOOLEAN NOT NULL,
	fee               DOUBLE PRECISION NOT NULL,
	topics            TEXT[] NOT NULL,
	meta              JSONB NOT NULL
)`

// benchColumnNames is the column order used by every variant, on disk and on the wire.
var benchColumnNames = []string{
	"_block_number_", "_block_timestamp_", "id", "tx_hash", "log_index",
	"from", "to", "amount", "gas_used", "success", "fee", "topics", "meta",
}

// metaColumnIndex is the JSONB column, which the text paths must quote as json rather
// than let the []byte fall through to a bytea literal.
const metaColumnIndex = 12

// row is one generated entity. Field order matches benchColumnNames.
type row struct {
	BlockNumber    int64
	BlockTimestamp time.Time
	ID             string
	TxHash         []byte
	LogIndex       int32
	From           string
	To             string
	Amount         uint64
	GasUsed        uint64
	Success        bool
	Fee            float64
	Topics         []any
	Meta           []byte
}

// values returns the row exactly as the protobuf walker hands it to Inserter.Insert:
// raw Go types, uint64 for the NUMERIC columns, []any for the array.
func (r *row) values() []any {
	return []any{
		r.BlockNumber, r.BlockTimestamp, r.ID, r.TxHash, r.LogIndex,
		r.From, r.To, r.Amount, r.GasUsed, r.Success, r.Fee, r.Topics, r.Meta,
	}
}

// generateRows builds a deterministic dataset. Same seed and count always yields the
// same bytes, so cached on-disk artifacts stay valid across runs.
func generateRows(count int, seed int64) []*row {
	rng := rand.New(rand.NewSource(seed))
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	rows := make([]*row, count)
	for i := range rows {
		blockNum := int64(20_000_000 + i/12) // ~12 entities per block

		txHash := make([]byte, 32)
		rng.Read(txHash)

		topicCount := 1 + rng.Intn(4)
		topics := make([]any, topicCount)
		for j := range topics {
			topics[j] = fmt.Sprintf("0x%016x", rng.Uint64())
		}

		rows[i] = &row{
			BlockNumber:    blockNum,
			BlockTimestamp: base.Add(time.Duration(blockNum-20_000_000) * 12 * time.Second),
			ID:             fmt.Sprintf("%016x-%04d", rng.Uint64(), i%10000),
			TxHash:         txHash,
			LogIndex:       int32(i % 256),
			From:           fmt.Sprintf("0x%040x", rng.Uint64()),
			To:             fmt.Sprintf("0x%040x", rng.Uint64()),
			// Deliberately spread across the whole uint64 range: above 2^63 is exactly
			// where sending a uint64 as an int8 to a NUMERIC column goes wrong.
			Amount:  rng.Uint64(),
			GasUsed: uint64(21000 + rng.Intn(3_000_000)),
			Success: rng.Intn(100) != 0,
			Fee:     rng.Float64() * 1000,
			Topics:  topics,
			Meta:    fmt.Appendf(nil, `{"kind":"transfer","seq":%d,"ok":%t}`, i, rng.Intn(2) == 0),
		}
	}

	return rows
}

// -- verification -----------------------------------------------------------------

// checksum is a cheap fingerprint of a loaded table, computed identically in Go and in
// SQL so every variant can be proven to have loaded the same data. A fast-but-wrong
// load is worth nothing, and binary COPY is exactly the kind of change that can be
// fast and wrong.
type checksum struct {
	Count           int64
	BlockSum        int64
	LogIndexSum     int64
	AmountSum       string // exact sum of the uint64 amounts, as a decimal string
	TopicsSum       int64
	HashLenSum      int64
	HashByte0Sum    int64
	HashByteLastSum int64
	SuccessCount    int64
	FeeAbove500     int64
	MetaKindCount   int64
}

func (c checksum) String() string {
	return fmt.Sprintf("count=%d blocks=%d logidx=%d amount=%s topics=%d hashlen=%d hash0=%d hashN=%d ok=%d fee500=%d meta=%d",
		c.Count, c.BlockSum, c.LogIndexSum, c.AmountSum, c.TopicsSum,
		c.HashLenSum, c.HashByte0Sum, c.HashByteLastSum, c.SuccessCount, c.FeeAbove500, c.MetaKindCount)
}

func expectedChecksum(rows []*row) checksum {
	out := checksum{Count: int64(len(rows))}
	amount := new(big.Int)

	for _, r := range rows {
		out.BlockSum += r.BlockNumber
		out.LogIndexSum += int64(r.LogIndex)
		out.TopicsSum += int64(len(r.Topics))
		amount.Add(amount, new(big.Int).SetUint64(r.Amount))

		out.HashLenSum += int64(len(r.TxHash))
		out.HashByte0Sum += int64(r.TxHash[0])
		out.HashByteLastSum += int64(r.TxHash[len(r.TxHash)-1])

		if r.Success {
			out.SuccessCount++
		}
		if r.Fee > 500 {
			out.FeeAbove500++
		}
		out.MetaKindCount++ // every generated row has "kind":"transfer"
	}
	out.AmountSum = amount.String()

	return out
}

// checksumSQL recomputes the same fingerprint server-side.
//
// sum(length(tx_hash)) alone catches the classic bytea double-encoding failure (64
// bytes instead of 32); the two byte probes catch content corruption at the same
// length. meta->>'kind' proves the JSONB actually parsed rather than landing as text.
// Every aggregate is cast explicitly because sum() over an integer type returns
// numeric, which does not scan into an int64.
const checksumSQL = `
SELECT
	count(*)::bigint,
	coalesce(sum(_block_number_), 0)::bigint,
	coalesce(sum(log_index), 0)::bigint,
	coalesce(sum(amount), 0)::text,
	coalesce(sum(array_length(topics, 1)), 0)::bigint,
	coalesce(sum(length(tx_hash)), 0)::bigint,
	coalesce(sum(get_byte(tx_hash, 0)), 0)::bigint,
	coalesce(sum(get_byte(tx_hash, length(tx_hash) - 1)), 0)::bigint,
	count(*) FILTER (WHERE success)::bigint,
	count(*) FILTER (WHERE fee > 500)::bigint,
	count(*) FILTER (WHERE meta->>'kind' = 'transfer')::bigint
FROM bench.transfers`

// -- on-disk artifacts ------------------------------------------------------------
//
// Every file below is materialised in full before the comparison starts, so a measured
// duration is transport plus server work and never generation.

// writeCSV materialises the dataset for COPY ... FROM STDIN (FORMAT CSV).
func writeCSV(path string, rows []*row) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating %s: %w", path, err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	record := make([]string, len(benchColumnNames))

	for _, r := range rows {
		record[0] = strconv.FormatInt(r.BlockNumber, 10)
		record[1] = r.BlockTimestamp.Format("2006-01-02 15:04:05.999999")
		record[2] = r.ID
		record[3] = `\x` + hex.EncodeToString(r.TxHash)
		record[4] = strconv.FormatInt(int64(r.LogIndex), 10)
		record[5] = r.From
		record[6] = r.To
		record[7] = strconv.FormatUint(r.Amount, 10)
		record[8] = strconv.FormatUint(r.GasUsed, 10)
		record[9] = strconv.FormatBool(r.Success)
		record[10] = strconv.FormatFloat(r.Fee, 'g', 17, 64)
		record[11] = pgArrayLiteral(r.Topics)
		record[12] = string(r.Meta)

		if err := writer.Write(record); err != nil {
			return fmt.Errorf("writing csv record: %w", err)
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return fmt.Errorf("flushing csv: %w", err)
	}

	return file.Sync()
}

// pgArrayLiteral renders {"a","b"} as Postgres array input expects. The csv writer
// quotes the resulting field itself.
func pgArrayLiteral(values []any) string {
	var b strings.Builder
	b.WriteByte('{')
	for i, v := range values {
		if i > 0 {
			b.WriteByte(',')
		}
		escaped := strings.ReplaceAll(fmt.Sprint(v), `\`, `\\`)
		escaped = strings.ReplaceAll(escaped, `"`, `\"`)
		b.WriteByte('"')
		b.WriteString(escaped)
		b.WriteByte('"')
	}
	b.WriteByte('}')

	return b.String()
}

// writeMultiRowSQL materialises complete multi-row INSERT statements, length-prefixed
// so they stream back without re-parsing. This isolates the server-side cost of the
// current accumulator strategy from the client-side cost of building its SQL.
func writeMultiRowSQL(path string, rows []*row, batchSize int) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating %s: %w", path, err)
	}
	defer file.Close()

	frames := newFrameWriter(file)
	for start := 0; start < len(rows); start += batchSize {
		end := min(start+batchSize, len(rows))

		if err := frames.write([]byte(buildMultiRowInsert(rows[start:end]))); err != nil {
			return fmt.Errorf("writing frame: %w", err)
		}
	}

	if err := frames.flush(); err != nil {
		return err
	}

	return file.Sync()
}

// buildMultiRowInsert is the current AccumulatorInserter.flush strategy: one text
// statement whose length grows with the batch.
func buildMultiRowInsert(rows []*row) string {
	var b strings.Builder
	b.Grow(len(rows) * 400)

	b.WriteString(insertPrefix())
	for i, r := range rows {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteByte('(')
		for j, v := range r.values() {
			if j > 0 {
				b.WriteByte(',')
			}
			b.WriteString(sqlLiteral(j, v))
		}
		b.WriteByte(')')
	}

	return b.String()
}

func insertPrefix() string {
	quoted := make([]string, len(benchColumnNames))
	for i, name := range benchColumnNames {
		quoted[i] = `"` + name + `"`
	}

	return fmt.Sprintf(`INSERT INTO %s.%s (%s) VALUES `, benchSchema, benchTable, strings.Join(quoted, ", "))
}

// sqlLiteral mirrors postgres.ValueToString, which is what AccumulatorInserter uses
// today, restricted to the types this dataset produces.
func sqlLiteral(columnIndex int, value any) string {
	if columnIndex == metaColumnIndex {
		return quoteString(string(value.([]byte)))
	}

	switch v := value.(type) {
	case nil:
		return "NULL"
	case string:
		return quoteString(v)
	case int32:
		return strconv.FormatInt(int64(v), 10)
	case int64:
		return strconv.FormatInt(v, 10)
	case uint64:
		return strconv.FormatUint(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'g', 17, 64)
	case bool:
		return strconv.FormatBool(v)
	case time.Time:
		return "'" + v.UTC().Format("2006-01-02 15:04:05.999999") + "'"
	case []byte:
		return `'\x` + hex.EncodeToString(v) + `'::bytea`
	case []any:
		elements := make([]string, len(v))
		for i, e := range v {
			elements[i] = sqlLiteral(-1, e)
		}
		return "array[" + strings.Join(elements, ",") + "]"
	default:
		panic(fmt.Sprintf("sqlLiteral: unsupported type %T", v))
	}
}

func quoteString(v string) string {
	return "'" + strings.ReplaceAll(v, "'", "''") + "'"
}
