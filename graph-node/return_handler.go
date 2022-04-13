package graphnode

import (
	"fmt"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/streamingfast/bstream"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"google.golang.org/protobuf/types/known/anypb"
)

type GraphNodeImporter struct {
	definitions *EntityDefinitions
}

func (gni *GraphNodeImporter) ToSqlValue(tableName string, field *pbsubstreams.Field) string {
	typeString, err := gni.definitions.GetPostgresType(tableName, field.Name)
	if err != nil {
		panic(err)
	}

	switch typeString {
	case "text not null", "text":
		return fmt.Sprintf("'%s'", field.NewValue)
	case "bytea not null", "bytea":
		panic("WTF DO WE DOOOOO")
	default:
		return fmt.Sprintf("%s", field.NewValue)
	}
}

func (gni *GraphNodeImporter) ReturnHandler(any *anypb.Any, step bstream.StepType, cursor *bstream.Cursor) error {
	var databaseChanges *pbsubstreams.DatabaseChanges

	data := any.GetValue()
	err := proto.Unmarshal(data, databaseChanges)
	if err != nil {
		return fmt.Errorf("unmarshaling proto: %w", err)
	}

	err = databaseChanges.Squash()
	if err != nil {
		return fmt.Errorf("squashing database changes: %w", err)
	}

	var sqlStatements []string

	for _, tc := range databaseChanges.TableChanges {
		if len(tc.Fields) == 0 {
			continue
		}

		var fieldNames []string
		var fieldValues []string
		for _, field := range tc.Fields {
			fieldNames = append(fieldNames, fmt.Sprintf("`%s`", field.Name))
			fieldValues = append(fieldValues, gni.ToSqlValue(tc.Table, field))
		}

		tableName := fmt.Sprintf("`%s`.`%s`", gni.definitions.PostgresSchema, tc.Table)

		switch tc.Operation {
		case pbsubstreams.TableChange_CREATE:
			sqlStatement := fmt.Sprintf(
				"INSERT INTO %s (%s) VALUES (%s)",
				tableName,
				strings.Join(fieldNames, ", "),
				strings.Join(fieldValues, ", "),
			)
			sqlStatements = append(sqlStatements, sqlStatement)
		case pbsubstreams.TableChange_DELETE:
			sqlStatement := fmt.Sprintf("DELETE * FROM %s WHERE `id` = %s", tableName, tc.Pk)
			sqlStatements = append(sqlStatements, sqlStatement)
		case pbsubstreams.TableChange_UPDATE:
			var updates []string
			for i := 0; i < len(tc.Fields); i++ {
				updates = append(updates, "`%s`=%s", fieldNames[i], fieldValues[i])
			}
			sqlStatement := fmt.Sprintf(
				"UPDATE %s SET %s WHERE `id` = %s",
				tableName,
				strings.Join(updates, ", "),
				tc.Pk,
			)
			sqlStatements = append(sqlStatements, sqlStatement)
		}
	}

	///TODO: execute sql statements

	return nil
}
