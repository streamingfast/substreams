package postgres

import (
	"reflect"
	"time"

	graphnode "github.com/streamingfast/substreams/graph-node"
)

var cacheHits int
var cacheMiss int
var cacheRemove int
var cacheOther int

func newEntityCache() *entityCache {
	return &entityCache{
		entityMap: map[string]map[string]graphnode.Entity{},
	}
}

type entityCache struct {
	entityMap map[string]map[string]graphnode.Entity

	CacheAll bool
}

func (c entityCache) getTable(name string) map[string]graphnode.Entity {
	m := c.entityMap
	table, found := m[name]
	if found {
		return table
	}
	m[name] = make(map[string]graphnode.Entity)
	return m[name]
}

func (c entityCache) GetEntity(tableName, id string, out graphnode.Entity) (found bool) {
	table := c.getTable(tableName)
	if e, found := table[id]; found {
		cacheHits++
		ve := reflect.ValueOf(out).Elem()
		ve.Set(reflect.ValueOf(e).Elem())
		return true
	}
	cacheMiss++

	return false
}

func (c entityCache) purgeCache(blockNum uint64, blockTime time.Time) {
	for _, rows := range c.entityMap {
		for id, ent := range rows {
			if purgeableEntity, ok := ent.(graphnode.Finalizable); ok {
				if purgeableEntity.IsFinal(blockNum, blockTime) {
					delete(rows, id)
				}
			}
		}
	}
}

func (c entityCache) SetEntity(tableName string, entity graphnode.Entity) {
	table := c.getTable(tableName)
	id := entity.GetID()

	table[id] = entity
}

func (c entityCache) Invalidate(tableName string, id string) {
	delete(c.entityMap[tableName], id)
}
