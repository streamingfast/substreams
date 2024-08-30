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
	return fmt.Sprintf(`"%s" %s,`, e.Name, e.Type)
}

type SubgraphEntityType struct {
	Name string
	Type SubgraphType
}

func (e *SubgraphEntityType) ToEntityTypeOut() string {
	switch e.Type {
	case SubgraphBytes, SubgraphString, SubgraphBoolean, SubgraphInt, SubgraphInt8:
		return fmt.Sprintf("input.%s", e.Name)
	case SubgraphBigInt:
		return fmt.Sprintf("BigInt.fromString(input.%s.toString())", e.Name)
	case SubgraphBigDecimal:
		return fmt.Sprintf("BigDecimal.fromString(input.%s.toString())", e.Name)
	case SubgraphTimestamp:
		panic("unsupported type")
	}

	panic("unsupported type")
}

func (e *SubgraphEntityType) ToGraphQLType() string {
	return fmt.Sprintf("%s: %s!", e.Name, e.Type)
}
