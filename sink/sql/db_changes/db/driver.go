package db

import (
	// Register the PostgreSQL driver, this package opens connections through database/sql.
	_ "github.com/lib/pq"
)
