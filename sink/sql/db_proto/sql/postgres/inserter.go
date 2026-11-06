package postgres

type pgInserter interface {
	insert(table string, values []any, database *Database) error
}

type pgFlusher interface {
	flush(database *Database) error
}
