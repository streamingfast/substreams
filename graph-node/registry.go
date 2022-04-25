package graphnode

import (
	"reflect"
	"strings"

	"github.com/iancoleman/strcase"
)

type Registry struct {
	entities   []Entity
	types      map[string]reflect.Type
	interfaces map[reflect.Type]Entity
}

func NewRegistry(entities ...Entity) *Registry {
	r := &Registry{
		types:      map[string]reflect.Type{},
		interfaces: map[reflect.Type]Entity{},
	}
	r.Register(entities...)
	r.Register(&POI{})
	return r
}

func (r *Registry) Len() int {
	return len(r.types)
}

func (r *Registry) Data() map[string]reflect.Type {
	//TODO: should we rlock here?
	return r.types
}

func (r *Registry) Entities() []Entity {
	//TODO: should we rlock here?
	return r.entities
}

func GetTableName(entity Entity) string {
	if v, ok := entity.(NamedEntity); ok {
		return v.TableName()
	}

	return GetTableNameFromType(reflect.TypeOf(entity))
}

func GetTableNameFromType(entity reflect.Type) string {
	el := strings.Split(entity.String(), ".")[1]
	return strcase.ToSnake(el)
}

func (r *Registry) GetType(tableName string) (reflect.Type, bool) {
	res, ok := r.types[tableName]
	return res, ok
}

func (r *Registry) GetInterface(tableName string) (Entity, bool) {
	t, ok := r.types[tableName]
	if !ok {
		return nil, false
	}
	instance := reflect.New(t)

	return instance.Interface().(Entity), ok
}

func (r *Registry) Register(entities ...Entity) {
	r.entities = append(r.entities, entities...)

	for _, ent := range entities {
		r.types[GetTableName(ent)] = reflect.TypeOf(ent).Elem()
		r.interfaces[reflect.TypeOf(ent).Elem()] = ent
	}
}
