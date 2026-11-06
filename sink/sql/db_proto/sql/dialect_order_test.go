package sql

import (
	"testing"

	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func dialectWith(tables []string, foreignKeys map[string]string) *BaseDialect {
	registry := map[string]*schema.Table{}
	for _, table := range tables {
		registry[table] = &schema.Table{Name: table}
	}

	dialect := NewBaseDialect(registry, nil)
	for referencing, referenced := range foreignKeys {
		dialect.AddForeignKeyReferencing(referencing, referenced, "ALTER TABLE "+referencing+" ADD CONSTRAINT fk FOREIGN KEY (x) REFERENCES "+referenced+"(y)")
	}

	return dialect
}

// before asserts that one table is applied ahead of another, which is the whole point of
// the order: rows of the referenced table must reach the server first.
func before(t *testing.T, ordered []string, first, second string) {
	t.Helper()

	positions := map[string]int{}
	for i, name := range ordered {
		positions[name] = i
	}

	require.Contains(t, positions, first)
	require.Contains(t, positions, second)
	assert.Less(t, positions[first], positions[second], "%q must be applied before %q, got %v", first, second, ordered)
}

func TestTableApplyOrder(t *testing.T) {
	t.Run("the block table always leads", func(t *testing.T) {
		dialect := dialectWith([]string{"customers"}, map[string]string{"customers": DialectTableBlock})

		ordered, err := dialect.TableApplyOrder()
		require.NoError(t, err)
		before(t, ordered, DialectTableBlock, "customers")
	})

	t.Run("deep nesting", func(t *testing.T) {
		// orders -> order_items -> order_item_options -> order_item_option_extras
		dialect := dialectWith(
			[]string{"orders", "order_items", "order_item_options", "order_item_option_extras"},
			map[string]string{
				"order_items":              "orders",
				"order_item_options":       "order_items",
				"order_item_option_extras": "order_item_options",
			},
		)

		ordered, err := dialect.TableApplyOrder()
		require.NoError(t, err)
		before(t, ordered, "orders", "order_items")
		before(t, ordered, "order_items", "order_item_options")
		before(t, ordered, "order_item_options", "order_item_option_extras")
	})

	t.Run("a sibling reference, which nesting depth cannot order", func(t *testing.T) {
		// Both are top-level tables, so they sit at the same depth; only the foreign key
		// says which has to be loaded first.
		dialect := dialectWith(
			[]string{"orders", "customers"},
			map[string]string{"orders": "customers"},
		)

		ordered, err := dialect.TableApplyOrder()
		require.NoError(t, err)
		before(t, ordered, "customers", "orders")
	})

	t.Run("a sibling reference pointing the other way", func(t *testing.T) {
		dialect := dialectWith(
			[]string{"orders", "customers"},
			map[string]string{"customers": "orders"},
		)

		ordered, err := dialect.TableApplyOrder()
		require.NoError(t, err)
		before(t, ordered, "orders", "customers")
	})

	t.Run("nesting and siblings together", func(t *testing.T) {
		dialect := dialectWith(
			[]string{"customers", "orders", "order_items", "products"},
			map[string]string{
				"orders":      "customers",
				"order_items": "orders",
			},
		)
		dialect.AddForeignKeyReferencing("order_items", "products", "ALTER TABLE order_items ADD CONSTRAINT fk_product FOREIGN KEY (p) REFERENCES products(id)")

		ordered, err := dialect.TableApplyOrder()
		require.NoError(t, err)
		before(t, ordered, "customers", "orders")
		before(t, ordered, "orders", "order_items")
		before(t, ordered, "products", "order_items")
		assert.Len(t, ordered, 5, "every table plus the block table")
	})

	t.Run("a table referencing itself is not a cycle", func(t *testing.T) {
		// It constrains the order of rows within one table, which no table-level order
		// can address, so it is left to the write order of the rows themselves.
		dialect := dialectWith([]string{"employees"}, map[string]string{"employees": "employees"})

		ordered, err := dialect.TableApplyOrder()
		require.NoError(t, err)
		assert.Contains(t, ordered, "employees")
	})

	t.Run("a cycle is reported rather than ordered wrong", func(t *testing.T) {
		dialect := dialectWith([]string{"a", "b"}, map[string]string{"a": "b", "b": "a"})

		_, err := dialect.TableApplyOrder()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cycle")
		assert.Contains(t, err.Error(), "a")
		assert.Contains(t, err.Error(), "b")
	})

	t.Run("a longer cycle", func(t *testing.T) {
		dialect := dialectWith([]string{"a", "b", "c"}, map[string]string{"a": "b", "b": "c", "c": "a"})

		_, err := dialect.TableApplyOrder()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cycle")
	})

	t.Run("a table nobody references keeps its place", func(t *testing.T) {
		dialect := dialectWith([]string{"alone", "customers", "orders"}, map[string]string{"orders": "customers"})

		ordered, err := dialect.TableApplyOrder()
		require.NoError(t, err)
		assert.Contains(t, ordered, "alone")
		assert.Len(t, ordered, 4)
	})

	t.Run("ranks match the order", func(t *testing.T) {
		dialect := dialectWith([]string{"customers", "orders"}, map[string]string{"orders": "customers"})

		ordered, err := dialect.TableApplyOrder()
		require.NoError(t, err)

		ranks, err := dialect.TableApplyRanks()
		require.NoError(t, err)
		require.Len(t, ranks, len(ordered))
		for i, name := range ordered {
			assert.Equal(t, i, ranks[name])
		}
	})
}

// TestTableNamesCoversEverySchemaTable is what the reorg path falls back to when the
// foreign keys cannot be ordered. It has to name every table, or an undo would leave rows
// of the ones it skipped behind.
func TestTableNamesCoversEverySchemaTable(t *testing.T) {
	dialect := dialectWith(
		[]string{"orders", "customers"},
		map[string]string{"orders": "customers", "customers": "orders"},
	)

	_, err := dialect.TableApplyOrder()
	require.Error(t, err, "the schema is cyclic on purpose")

	assert.Equal(t, []string{DialectTableBlock, "customers", "orders"}, dialect.TableNames())
}
