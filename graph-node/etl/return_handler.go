package etl

import (
	"context"
	"fmt"
	"reflect"
	"time"

	graphnode "github.com/streamingfast/substreams/graph-node"

	"github.com/golang/protobuf/proto"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/substreams/graph-node/storage"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/anypb"
)

type Loader struct {
	store    storage.Store
	registry *graphnode.Registry

	// cached entities
	current map[string]map[string]graphnode.Entity
	updates map[string]map[string]graphnode.Entity
}

func NewLoader(store storage.Store, registry *graphnode.Registry) *Loader {
	return &Loader{
		store:    store,
		registry: registry,
	}
}

func (l *Loader) save(ent graphnode.Entity) error {
	tableName := graphnode.GetTableName(ent)

	updateTable, found := l.updates[tableName]
	if !found {
		updateTable = make(map[string]graphnode.Entity)
		l.updates[tableName] = updateTable
	}

	ent.SetExists(true)
	updateTable[ent.GetID()] = ent

	return nil
}

func (l *Loader) load(entity graphnode.Entity, blockNum uint64) error {
	tableName := graphnode.GetTableName(entity)
	id := entity.GetID()

	zlog.Debug("loading entity",
		zap.String("id", id),
		zap.String("table", tableName),
		zap.Uint64("vid", entity.GetVID()),
	)
	if id == "" {
		return fmt.Errorf("id was not set before calling load")
	}

	// First check from updates
	updateTable, found := l.updates[tableName]
	if !found {
		updateTable = make(map[string]graphnode.Entity)
		l.updates[tableName] = updateTable
	}

	cachedEntity, found := updateTable[id]
	if found {
		if cachedEntity == nil {
			return nil
		}
		ve := reflect.ValueOf(entity).Elem()
		ve.Set(reflect.ValueOf(cachedEntity).Elem())
		return nil
	}

	// Load from DB otherwise
	currentTable, found := l.current[tableName]
	if !found {
		currentTable = make(map[string]graphnode.Entity)
		l.current[tableName] = currentTable
	}

	cachedEntity, found = currentTable[id]
	if found {
		if cachedEntity == nil {
			return nil
		}
		ve := reflect.ValueOf(entity).Elem()
		ve.Set(reflect.ValueOf(cachedEntity).Elem())
		return nil
	}

	if err := l.store.Load(context.TODO(), id, entity, blockNum); err != nil {
		return fmt.Errorf("failed loading entity: %w", err)
	}

	if entity.Exists() {
		reflectType, ok := l.registry.GetType(tableName) //subgraph.MainSubgraphDef.Entities.GetType(tableName)
		if !ok {
			return fmt.Errorf("unable to retrieve entity type")
		}
		clone := reflect.New(reflectType).Interface()
		ve := reflect.ValueOf(clone).Elem()
		ve.Set(reflect.ValueOf(entity).Elem())
		currentTable[id] = clone.(graphnode.Entity)
	} else {
		currentTable[id] = nil
	}

	return nil
}

func (l *Loader) Flush(cursor string, blockNum uint64, blockID string, blockTime time.Time) error {
	return l.store.BatchSave(context.TODO(), blockNum, blockID, blockTime, l.updates, cursor)
}

func (l *Loader) ReturnHandler(any *anypb.Any, blockNum uint64, blockID string, blockTime time.Time, step bstream.StepType, cursor *bstream.Cursor) error {
	var databaseChanges *pbsubstreams.DatabaseChanges

	l.current = make(map[string]map[string]graphnode.Entity)
	l.updates = make(map[string]map[string]graphnode.Entity)

	data := any.GetValue()
	err := proto.Unmarshal(data, databaseChanges)
	if err != nil {
		return fmt.Errorf("unmarshaling database changes proto: %w", err)
	}

	//todo: should be applied in a transform inside the firehose, not here.
	err = databaseChanges.Squash()
	if err != nil {
		return fmt.Errorf("squashing database changes: %w", err)
	}

	for _, change := range databaseChanges.TableChanges {
		ent, ok := l.registry.GetInterface(change.Table)
		if !ok {
			return fmt.Errorf("unknown entity for table %s", change.Table)
		}

		err = l.load(ent, blockNum)
		if err != nil {
			return fmt.Errorf("loading entity %w", err)
		}

		err := graphnode.ApplyTableChange(change, ent)
		if err != nil {
			return fmt.Errorf("applying table change: %w", err)
		}

		err = l.save(ent)
		if err != nil {
			return fmt.Errorf("saving entity: %w", err)
		}
	}

	err = l.Flush(cursor.String(), blockNum, blockID, blockTime)
	if err != nil {
		return fmt.Errorf("flushing block changes: %w", err)
	}

	return nil
}
