package db

import (
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/streamingfast/logging"
)

var zlog, tracer = logging.PackageLogger("sink-sql", "github.com/streamingfast/substreams/sink/sql/db")

func init() {
	logging.InstantiateLoggers()
}
