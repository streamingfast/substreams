package graph_node

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/iancoleman/strcase"
	"github.com/vektah/gqlparser/ast"
)

type EntityDefinitions struct {
	definitions    map[string]*EntityDefinition
	enums          map[string]*Enum
	PostgresSchema string
}

type EntityDefinition struct {
	Fields   map[string]*Field
	Name     string
	IsEntity bool

	CacheSkipDBLookup bool
}
type Enum struct {
	Name   string
	Fields []string
}

type Field struct {
	Name         string
	Type         string
	Nullable     bool
	Array        bool
	Derived      bool
	Hidden       bool
	PostgresType string
	//PostgresIndex string
}

func NewEntityDefinitions(postgresSchema string, graphqlSchemaDoc *ast.SchemaDocument) (*EntityDefinitions, error) {
	enums := ParseEnums(graphqlSchemaDoc)
	definitions, err := ParseDefinitions(graphqlSchemaDoc, enums, postgresSchema)
	if err != nil {
		return nil, fmt.Errorf("parsing definition: %w", err)
	}

	return &EntityDefinitions{
		definitions:    definitions,
		enums:          enums,
		PostgresSchema: postgresSchema,
	}, nil
}

func ParseEnums(graphqlSchemaDoc *ast.SchemaDocument) map[string]*Enum {
	enums := map[string]*Enum{}
	for _, d := range graphqlSchemaDoc.Definitions {
		if def := ParseEnum(d); def != nil {
			enums[ToLowerSnakeCase(d.Name)] = def
		}
	}
	return enums
}

func ParseDefinitions(graphqlSchemaDoc *ast.SchemaDocument, enums map[string]*Enum, postgresSchema string) (map[string]*EntityDefinition, error) {
	defs := map[string]*EntityDefinition{}
	for _, d := range graphqlSchemaDoc.Definitions {
		def, err := parseObject(d, enums, postgresSchema)
		if err != nil {
			return nil, err
		}
		if def != nil && def.IsEntity {
			defs[ToLowerSnakeCase(d.Name)] = def
		}
	}

	return defs, nil
}

func ParseEnum(def *ast.Definition) *Enum {
	if def.Kind != "ENUM" {
		return nil
	}

	vals := make([]string, 0, len(def.EnumValues))
	for _, val := range def.EnumValues {
		vals = append(vals, val.Name)
	}

	return &Enum{
		Name:   def.Name,
		Fields: vals,
	}
}

func parseObject(def *ast.Definition, enums map[string]*Enum, postgresSchema string) (*EntityDefinition, error) {
	if def.Kind != "OBJECT" {
		return nil, nil
	}
	fields := map[string]*Field{}
	for _, field := range def.Fields {
		fieldDef, err := ParseFieldDefinition(field, enums, postgresSchema)
		if err != nil {
			return nil, fmt.Errorf("entity %q: %w", def.Name, err)
		}
		fields[ToLowerSnakeCase(field.Name)] = fieldDef
	}

	out := &EntityDefinition{
		Fields: fields,
		Name:   def.Name,
	}

	for _, dir := range def.Directives {
		if dir.Name == "entity" {
			out.IsEntity = true
		}
	}

	if !out.IsEntity {
		return out, nil
	}

	// this is only applied for entities
	for _, dir := range def.Directives {
		if dir.Name == "cache" {
			for _, arg := range dir.Arguments {
				switch arg.Name {
				case "skip_db_lookup":
					if arg.Value == nil {
						return nil, fmt.Errorf("'skip_db_lookup' argument to @cache directive requires a boolean parameter")
					}
					val, err := strconv.ParseBool(arg.Value.Raw)
					if err != nil {
						return nil, fmt.Errorf("invalid bool value for 'skip_db_lookup' argument to @cache directive: %w", err)
					}
					if val {
						out.CacheSkipDBLookup = true
					}
				}
			}
		}
	}

	return out, nil
}

func ParseFieldDefinition(field *ast.FieldDefinition, enums map[string]*Enum, postgresSchema string) (*Field, error) {
	f := &Field{
		Name:   field.Name,
		Type:   field.Type.Name(),
		Array:  field.Type.Elem != nil,
		Hidden: field.Name == "id", // defined in entity.Base, so not needed in our codegen
	}
	if field.Type.Elem != nil {
		f.Nullable = !field.Type.Elem.NonNull
	} else {
		f.Nullable = !field.Type.NonNull
	}

	f.PostgresType = getPostgresType(f, enums, postgresSchema)

	return f, nil
}

var fieldTypesNotNullable = map[string]string{
	"String":     "text not null",
	"Boolean":    "boolean not null",
	"Bytes":      "bytea not null",
	"Int":        "numeric not null",
	"Float":      "numeric not null",
	"BigInt":     "numeric not null",
	"BigDecimal": "numeric not null",
}
var fieldTypesNullable = map[string]string{
	"String":     "text",
	"Boolean":    "boolean",
	"Bytes":      "bytea",
	"Int":        "numeric",
	"Float":      "numeric",
	"BigInt":     "numeric",
	"BigDecimal": "numeric",
}

func (d *EntityDefinitions) GetPostgresTypeFromName(tableName string, fieldName string) (string, error) {
	entity, found := d.definitions[tableName]
	if !found {
		return "", fmt.Errorf("table with name '%s' not found", tableName)
	}

	field, found := entity.Fields[fieldName]
	if !found {
		return "", fmt.Errorf("field '%s' of table with name '%s' not found", fieldName, tableName)
	}

	return getPostgresType(field, d.enums, d.PostgresSchema), nil
}
func getPostgresType(field *Field, enums map[string]*Enum, postgresSchema string) string {

	switch {
	case field.Nullable:
		if dt, ok := fieldTypesNullable[field.Type]; ok {
			return dt
		}
	default:
		if dt, ok := fieldTypesNotNullable[field.Type]; ok {
			return dt
		}
	}

	if field.Array {
		if field.Nullable {
			return "text[]"
		}
		return "text[] not null"
	}

	if _, ok := enums[field.Type]; ok {
		return fmt.Sprintf(`%s."%s"`, postgresSchema, field.Name)
	}

	//this should be an ID
	if field.Nullable {
		return "text"
	}
	return "text not null"

}

func ToLowerSnakeCase(input string) string {
	return strings.ToLower(strcase.ToSnake(input))
}
