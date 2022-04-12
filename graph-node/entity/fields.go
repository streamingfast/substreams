package entity

import (
	"reflect"

	"github.com/jmoiron/sqlx/reflectx"
)

var typesMapper = reflectx.NewMapper("db")

// Note: this function is simple and doesn't support embedded structs,
// except a special case for the entity.Base object.
//
// It can be extended in the future, as the `reflectx` lib for
// `typesMapper`supports all the goodies out of the box.

func DBFields(entityType reflect.Type) (out []*FieldTag) {
	baseType := reflect.TypeOf(Base{})
	res := typesMapper.TypeMap(entityType)
	for _, el := range res.Index {
		if el.Field.Type == baseType {
			for _, el := range el.Children {
				if el == nil {
					continue
				}
				out = append(out, &FieldTag{
					Name:       el.Field.Name,
					ColumnName: el.Path,
					Base:       true,
				})
			}
		} else if len(el.Index) == 1 && !el.Field.Anonymous && !el.Embedded {
			_, isOptional := el.Options["nullable"]
			out = append(out, &FieldTag{
				Name:       el.Field.Name,
				ColumnName: el.Path,
				Optional:   isOptional,
			})
		}
	}
	return out
}

type FieldTag struct {
	Name       string
	Base       bool
	ColumnName string
	Optional   bool
}
