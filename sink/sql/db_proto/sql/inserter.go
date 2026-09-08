package sql

type Inserter interface {
	Insert(table string, values []any) error
}
