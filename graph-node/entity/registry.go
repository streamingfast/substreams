package entity

import (
	"reflect"
	"strings"

	"github.com/iancoleman/strcase"
)

type Registry struct {
	entities   []Interface
	data       map[string]reflect.Type
	interfaces map[reflect.Type]Interface
}

func NewRegistry(entities ...Interface) *Registry {
	r := &Registry{
		data:       map[string]reflect.Type{},
		interfaces: map[reflect.Type]Interface{},
	}
	r.Register(entities...)
	r.Register(&POI{})
	return r
}

func (r *Registry) Len() int {
	return len(r.data)
}

func (r *Registry) Data() map[string]reflect.Type {
	//TODO: should we rlock here?
	return r.data
}

func (r *Registry) Entities() []Interface {
	//TODO: should we rlock here?
	return r.entities
}

func GetTableName(entity Interface) string {
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
	res, ok := r.data[tableName]
	return res, ok
}

func (r *Registry) GetInterface(tableName string) (Interface, bool) {
	t, ok := r.data[tableName]
	if !ok {
		return nil, false
	}

	res, ok := r.interfaces[t]
	return res, ok
}

func (r *Registry) Register(entities ...Interface) {
	r.entities = append(r.entities, entities...)

	for _, ent := range entities {
		r.data[GetTableName(ent)] = reflect.TypeOf(ent).Elem()
		r.interfaces[reflect.TypeOf(ent).Elem()] = ent
	}
}
