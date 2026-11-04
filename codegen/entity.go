package codegen

import (
	"fmt"

	"github.com/golang-cz/textcase"
)

type EntityInfo struct {
	HasAnID     bool
	IDFieldName string
}

type EntityType interface {
	ToEntityTypeOut() string
}

type SQLEntityType struct {
	Name string
	Type SQLType
}

func (e *SQLEntityType) ToEntityTypeOut() string {
	return fmt.Sprintf("input.%s", textcase.SnakeCase(e.Name))
}

func (e *SQLEntityType) ToSQLType() string {
	return fmt.Sprintf(`"%s" %s,`, textcase.SnakeCase(e.Name), e.Type)
}
