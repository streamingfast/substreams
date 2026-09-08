package db

import (
	"fmt"
	"reflect"
)

//go:generate go-enum -f=$GOFILE --marshal --names -nocase

// ENUM(
//
//	 Ignore
//		Warn
//		Error
//
// )
type OnModuleHashMismatch uint

type TableInfo struct {
	schema         string
	schemaEscaped  string
	name           string
	nameEscaped    string
	columnsByName  map[string]*ColumnInfo
	primaryColumns []*ColumnInfo

	// Identifier is equivalent to 'escape(<schemaName>).escape(<name>)' but pre-computed
	// for usage when computing queries.
	identifier string
}

func NewTableInfo(schema, name string, pkList []string, columnsByName map[string]*ColumnInfo) (*TableInfo, error) {
	schemaEscaped := EscapeIdentifier(schema)
	nameEscaped := EscapeIdentifier(name)
	primaryColumns := make([]*ColumnInfo, len(pkList))

	for i, primaryKeyColumnName := range pkList {
		primaryColumn, found := columnsByName[primaryKeyColumnName]
		if !found {
			return nil, fmt.Errorf("primary key column %q not found", primaryKeyColumnName)
		}
		primaryColumns[i] = primaryColumn

	}
	if len(primaryColumns) == 0 {
		return nil, fmt.Errorf("sql sink requires a primary key in every table, none was found in table %s.%s", schema, name)
	}

	return &TableInfo{
		schema:         schema,
		schemaEscaped:  schemaEscaped,
		name:           name,
		nameEscaped:    nameEscaped,
		identifier:     schemaEscaped + "." + nameEscaped,
		primaryColumns: primaryColumns,
		columnsByName:  columnsByName,
	}, nil
}

type ColumnInfo struct {
	name             string
	escapedName      string
	databaseTypeName string
	scanType         reflect.Type
}

func NewColumnInfo(name string, databaseTypeName string, scanType any) *ColumnInfo {
	return &ColumnInfo{
		name:             name,
		escapedName:      EscapeIdentifier(name),
		databaseTypeName: databaseTypeName,
		scanType:         reflect.TypeOf(scanType),
	}
}
