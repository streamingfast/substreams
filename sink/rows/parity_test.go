package rows

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/protobuf/reflect/protoreflect"

	dbsql "github.com/streamingfast/substreams/sink/sql/db_proto/sql"
	dbschema "github.com/streamingfast/substreams/sink/sql/db_proto/sql/schema"
)

// registryDialect is the smallest dialect the db_proto walk accepts: no version, deleted or row id columns, tables from the db_proto schema.
type registryDialect struct{ tables map[string]*dbschema.Table }

func (registryDialect) SchemaHash() string                         { return "" }
func (registryDialect) FullTableName(table *dbschema.Table) string { return table.Name }
func (d registryDialect) GetTable(table string) *dbschema.Table    { return d.tables[table] }
func (registryDialect) GetTables() []*dbschema.Table               { return nil }
func (registryDialect) UseVersionField() bool                      { return false }
func (registryDialect) UseDeletedField() bool                      { return false }
func (registryDialect) UseRowIDField(string) bool                  { return false }
func (registryDialect) AppendInlineFieldValues(fieldValues []any, _ protoreflect.FieldDescriptor, _ protoreflect.Value, _ protoreflect.Message) ([]any, error) {
	return fieldValues, nil
}

type tableValues struct {
	table  string
	values []any
}

type capturingInserter struct{ inserts []tableValues }

func (c *capturingInserter) Insert(table string, values []any) error {
	c.inserts = append(c.inserts, tableValues{table: table, values: values})
	return nil
}

// TestParity_DBProtoWalk replays the existing db_proto walk and checks the new producer emits the same tables, in the
// same order, for the same data: same row count per table, same pk values, same parent/child nesting order.
func TestParity_DBProtoWalk(t *testing.T) {
	for _, f := range factories() {
		t.Run(f.name, func(t *testing.T) {
			msg := f.make(t, sampleOutput())

			schema, err := dbschema.NewSchema("parity", relationsDescriptor(), true, zap.NewNop())
			require.NoError(t, err)

			database, err := dbsql.NewBaseDatabase("parity", relationsDescriptor(), true, zap.NewNop())
			require.NoError(t, err)

			captured := &capturingInserter{}
			_, err = database.WalkMessageDescriptorAndInsertWithDialect(msg, 100, time.Unix(0, 0), nil, registryDialect{tables: schema.TableRegistry}, captured)
			require.NoError(t, err)

			producer := newRelationsProducer(t)
			var got []string
			for row, err := range producer.Rows(testBlock, msg) {
				require.NoError(t, err)
				got = append(got, rowIdentity(row))
			}

			var want []string
			for _, insert := range captured.inserts {
				table := schema.TableRegistry[insert.table]
				identity := insert.table
				if table.PrimaryKey != nil {
					// db_proto puts blockNum and blockTimestamp first, then the primary key.
					identity += "=" + fmt.Sprint(insert.values[2])
				}
				want = append(want, identity)
			}

			assert.Equal(t, want, got, "same tables, same order, same keys")
		})
	}
}

func rowIdentity(row *Row) string {
	identity := row.Table.Name
	if row.Table.PK.Kind == PKKindSingle {
		value := row.Values[row.Table.PK.Single]
		column := row.Table.Columns[row.Table.PK.Single]
		identity += "=" + fmt.Sprint(plain(column.Type, value))
	}
	return identity
}
