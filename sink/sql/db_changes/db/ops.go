package db

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"go.uber.org/zap"
)

// Insert a row in the DB, it is assumed the table exists, you can do a
// check before with HasTable()
func (l *Loader) Insert(tableName string, primaryKey map[string]string, data map[string]FieldData, reversibleBlockNum *uint64) error {
	uniqueID := createRowUniqueID(primaryKey)

	if l.tracer.Enabled() {
		l.logger.Debug("processing insert operation", zap.String("table_name", tableName), zap.String("primary_key", uniqueID), zap.Int("field_count", len(data)))
	}

	table, found := l.tables[tableName]
	if !found {
		return fmt.Errorf("unknown table %q", tableName)
	}

	entry, found := l.entries.Get(tableName)
	if !found {
		if l.tracer.Enabled() {
			l.logger.Debug("adding tracking of table never seen before", zap.String("table_name", tableName))
		}

		entry = NewOrderedMap[string, *Operation]()
		l.entries.Set(tableName, entry)
	}

	if operation, found := entry.Get(uniqueID); found {
		switch operation.opType {
		case OperationTypeInsert:
			if !l.dialect.AllowPkDuplicates() {
				return fmt.Errorf("attempting to insert in table %q a primary key %q, that is already scheduled for insertion, insert should only be called once for a given primary key", tableName, primaryKey)
			}
		case OperationTypeDelete:
			return fmt.Errorf("attempting to insert an object with primary key %q, that is scheduled to be deleted", primaryKey)
		case OperationTypeUpdate:
			return fmt.Errorf("attempting to insert an object with primary key %q, that is scheduled to be updated", primaryKey)
		case OperationTypeUpsert:
			return fmt.Errorf("attempting to insert an object with primary key %q, that is scheduled to be upserted", primaryKey)
		}
	}

	if l.tracer.Enabled() {
		l.logger.Debug("primary key entry never existed for table, adding insert operation", zap.String("primary_key", uniqueID), zap.String("table_name", tableName))
	}

	// We need to make sure to add the primary key(s) in the data so that those column get created correctly, but only if there is data
	for _, primary := range l.tables[tableName].primaryColumns {
		if dataFromPrimaryKey, ok := primaryKey[primary.name]; ok {
			data[primary.name] = FieldData{Value: dataFromPrimaryKey, UpdateOp: UpdateOpSet}
		}
	}

	entry.Set(uniqueID, l.newInsertOperation(table, primaryKey, data, l.NextBatchOrdinal(), reversibleBlockNum))
	l.entriesCount++
	return nil
}

func createRowUniqueID(m map[string]string) string {
	if len(m) == 1 {
		for _, v := range m {
			return v
		}
	}

	keys := slices.Collect(maps.Keys(m))
	slices.Sort(keys)

	values := make([]string, len(keys))
	for i, key := range keys {
		values[i] = m[key]
	}

	return strings.Join(values, "/")
}

func (l *Loader) GetPrimaryKey(tableName string, pk string) (map[string]string, error) {
	primaryKeyColumns := l.tables[tableName].primaryColumns

	switch len(primaryKeyColumns) {
	case 0:
		return nil, fmt.Errorf("substreams sent a single primary key, but our sql table has none, this is unsupported")
	case 1:
		return map[string]string{primaryKeyColumns[0].name: pk}, nil
	}

	cols := make([]string, len(primaryKeyColumns))
	for i := range primaryKeyColumns {
		cols[i] = primaryKeyColumns[i].name
	}
	return nil, fmt.Errorf("substreams sent a single primary key, but our sql table has a composite primary key (columns: %s), this is unsupported", strings.Join(cols, ","))
}

// Upsert a row in the DB, it is assumed the table exists, you can do a
// check before with HasTable().
func (l *Loader) Upsert(tableName string, primaryKey map[string]string, data map[string]FieldData, reversibleBlockNum *uint64) error {
	if l.dialect.OnlyInserts() {
		return fmt.Errorf("update operation is not supported by the current database")
	}

	uniqueID := createRowUniqueID(primaryKey)
	if l.tracer.Enabled() {
		l.logger.Debug("processing update operation", zap.String("table_name", tableName), zap.String("primary_key", uniqueID), zap.Int("field_count", len(data)))
	}

	table, found := l.tables[tableName]
	if !found {
		return fmt.Errorf("unknown table %q", tableName)
	}

	if len(table.primaryColumns) == 0 {
		return fmt.Errorf("trying to perform an UPSERT operation but table %q don't have a primary key(s) set, this is not accepted", tableName)
	}

	entry, found := l.entries.Get(tableName)
	if !found {
		if l.tracer.Enabled() {
			l.logger.Debug("adding tracking of table never seen before", zap.String("table_name", tableName))
		}

		entry = NewOrderedMap[string, *Operation]()
		l.entries.Set(tableName, entry)
	}

	if op, found := entry.Get(uniqueID); found {
		switch op.opType {
		case OperationTypeInsert:
			return fmt.Errorf("attempting to upsert an object with primary key %q, that is scheduled to be inserted, insert and upsert are exclusive", primaryKey)
		case OperationTypeDelete:
			return fmt.Errorf("attempting to upsert an object with primary key %q, that is scheduled to be deleted", primaryKey)
		case OperationTypeUpdate:
			// Accept existing update operation but change it to upsert, merge columns together
			op.opType = OperationTypeUpsert
		case OperationTypeUpsert:
			// Fine, merge columns together
		}

		if l.tracer.Enabled() {
			l.logger.Debug("primary key entry already exist for table, merging columns together", zap.String("primary_key", uniqueID), zap.String("table_name", tableName))
		}

		op.mergeOperation(data)
		entry.Set(uniqueID, op)
		return nil
	} else {
		l.entriesCount++
	}

	if l.tracer.Enabled() {
		l.logger.Debug("primary key entry never existed for table, adding upsert operation", zap.String("primary_key", uniqueID), zap.String("table_name", tableName))
	}

	// We need to make sure to add the primary key(s) in the data so that those column get created correctly, but only if there is data
	for _, primary := range l.tables[tableName].primaryColumns {
		if dataFromPrimaryKey, ok := primaryKey[primary.name]; ok {
			data[primary.name] = FieldData{Value: dataFromPrimaryKey, UpdateOp: UpdateOpSet}
		}
	}

	entry.Set(uniqueID, l.newUpsertOperation(table, primaryKey, data, l.NextBatchOrdinal(), reversibleBlockNum))
	return nil
}

// Update a row in the DB, it is assumed the table exists, you can do a
// check before with HasTable()
func (l *Loader) Update(tableName string, primaryKey map[string]string, data map[string]FieldData, reversibleBlockNum *uint64) error {
	if l.dialect.OnlyInserts() {
		return fmt.Errorf("update operation is not supported by the current database")
	}

	uniqueID := createRowUniqueID(primaryKey)
	if l.tracer.Enabled() {
		l.logger.Debug("processing update operation", zap.String("table_name", tableName), zap.String("primary_key", uniqueID), zap.Int("field_count", len(data)))
	}

	table, found := l.tables[tableName]
	if !found {
		return fmt.Errorf("unknown table %q", tableName)
	}

	if len(table.primaryColumns) == 0 {
		return fmt.Errorf("trying to perform an UPDATE operation but table %q don't have a primary key(s) set, this is not accepted", tableName)
	}

	entry, found := l.entries.Get(tableName)
	if !found {
		if l.tracer.Enabled() {
			l.logger.Debug("adding tracking of table never seen before", zap.String("table_name", tableName))
		}

		entry = NewOrderedMap[string, *Operation]()
		l.entries.Set(tableName, entry)
	}

	if op, found := entry.Get(uniqueID); found {
		switch op.opType {
		case OperationTypeInsert:
			// Column is scheduled to be inserted, simply add our fields to the insert without changing its Insert type
		case OperationTypeDelete:
			return fmt.Errorf("attempting to update an object with primary key %q, that is scheduled to be deleted", primaryKey)
		case OperationTypeUpdate:
			// Fine, merge columns together
		case OperationTypeUpsert:
			// Accept existing upsert and our columns to it, but not change its type to keep it as an upsert
		}

		if l.tracer.Enabled() {
			l.logger.Debug("primary key entry already exist for table, merging fields together", zap.String("primary_key", uniqueID), zap.String("table_name", tableName))
		}

		op.mergeOperation(data)
		entry.Set(uniqueID, op)
		return nil
	} else {
		l.entriesCount++
	}

	if l.tracer.Enabled() {
		l.logger.Debug("primary key entry never existed for table, adding update operation", zap.String("primary_key", uniqueID), zap.String("table_name", tableName))
	}

	entry.Set(uniqueID, l.newUpdateOperation(table, primaryKey, data, l.NextBatchOrdinal(), reversibleBlockNum))
	return nil
}

// Delete a row in the DB, it is assumed the table exists, you can do a
// check before with HasTable()
func (l *Loader) Delete(tableName string, primaryKey map[string]string, reversibleBlockNum *uint64) error {
	if l.dialect.OnlyInserts() {
		return fmt.Errorf("delete operation is not supported by the current database")
	}

	uniqueID := createRowUniqueID(primaryKey)
	if l.tracer.Enabled() {
		l.logger.Debug("processing delete operation", zap.String("table_name", tableName), zap.String("primary_key", uniqueID))
	}

	table, found := l.tables[tableName]
	if !found {
		return fmt.Errorf("unknown table %q", tableName)
	}

	if len(table.primaryColumns) == 0 {
		return fmt.Errorf("trying to perform a DELETE operation but table %q don't have a primary key(s) set, this is not accepted", tableName)
	}

	entry, found := l.entries.Get(tableName)
	if !found {
		if l.tracer.Enabled() {
			l.logger.Debug("adding tracking of table never seen before", zap.String("table_name", tableName))
		}

		entry = NewOrderedMap[string, *Operation]()
		l.entries.Set(tableName, entry)
	}

	if _, found := entry.Get(uniqueID); !found {
		if l.tracer.Enabled() {
			l.logger.Debug("primary key entry never existed for table", zap.String("primary_key", uniqueID), zap.String("table_name", tableName))
		}

		l.entriesCount++
	}

	if l.tracer.Enabled() {
		l.logger.Debug("adding deleting operation", zap.String("primary_key", uniqueID), zap.String("table_name", tableName))
	}

	entry.Set(uniqueID, l.newDeleteOperation(table, primaryKey, l.NextBatchOrdinal(), reversibleBlockNum))
	return nil
}
