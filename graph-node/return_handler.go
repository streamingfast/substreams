package graphnode

import (
	"context"
	"fmt"
	"reflect"

	"github.com/golang/protobuf/proto"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/substreams/graph-node/storage"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/anypb"
)

type Importer struct {
	store    storage.Store
	registry *Registry

	// cached entities
	current map[string]map[string]Entity
	updates map[string]map[string]Entity
}

func NewImporter(store storage.Store, registry *Registry) *Importer {
	return &Importer{
		store:    store,
		registry: registry,
	}
}

func (i *Importer) save(ent Entity) error {
	tableName := GetTableName(ent)

	updateTable, found := i.updates[tableName]
	if !found {
		updateTable = make(map[string]Entity)
		i.updates[tableName] = updateTable
	}

	ent.SetExists(true)
	updateTable[ent.GetID()] = ent

	return nil
}

func (i *Importer) load(ent Entity, block *bstream.Block) error {
	tableName := GetTableName(ent)
	id := ent.GetID()

	zlog.Debug("loading entity",
		zap.String("id", id),
		zap.String("table", tableName),
		zap.Uint64("vid", ent.GetVID()),
	)
	if id == "" {
		return fmt.Errorf("id was not set before calling load")
	}

	// First check from updates
	updateTable, found := i.updates[tableName]
	if !found {
		updateTable = make(map[string]Entity)
		i.updates[tableName] = updateTable
	}

	cachedEntity, found := updateTable[id]
	if found {
		if cachedEntity == nil {
			return nil
		}
		ve := reflect.ValueOf(ent).Elem()
		ve.Set(reflect.ValueOf(cachedEntity).Elem())
		return nil
	}

	// Load from DB otherwise
	currentTable, found := i.current[tableName]
	if !found {
		currentTable = make(map[string]Entity)
		i.current[tableName] = currentTable
	}

	cachedEntity, found = currentTable[id]
	if found {
		if cachedEntity == nil {
			return nil
		}
		ve := reflect.ValueOf(ent).Elem()
		ve.Set(reflect.ValueOf(cachedEntity).Elem())
		return nil
	}

	if err := i.store.Load(context.TODO(), id, ent, block.Num()); err != nil {
		return fmt.Errorf("failed loading entity: %w", err)
	}

	if ent.Exists() {
		reflectType, ok := i.registry.GetType(tableName) //subgraph.MainSubgraphDef.Entities.GetType(tableName)
		if !ok {
			return fmt.Errorf("unable to retrieve entity type")
		}
		clone := reflect.New(reflectType).Interface()
		ve := reflect.ValueOf(clone).Elem()
		ve.Set(reflect.ValueOf(ent).Elem())
		currentTable[id] = clone.(Entity)
	} else {
		currentTable[id] = nil
	}

	return nil
}

func (i *Importer) Flush(cursor string, block *bstream.Block) error {
	return i.store.BatchSave(context.TODO(), block.Num(), block.ID(), block.Time(), i.updates, cursor)
}

func (i *Importer) ReturnHandler(any *anypb.Any, block *bstream.Block, step bstream.StepType, cursor *bstream.Cursor) error {
	var databaseChanges *pbsubstreams.DatabaseChanges

	i.current = make(map[string]map[string]Entity)
	i.updates = make(map[string]map[string]Entity)

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
		ent, ok := i.registry.GetInterface(change.Table)
		if !ok {
			return fmt.Errorf("unknown entity for table %s", change.Table)
		}

		err = i.load(ent, block)
		if err != nil {
			return fmt.Errorf("loading entity %w", err)
		}

		err := ApplyTableChange(change, ent)
		if err != nil {
			return fmt.Errorf("applying table change: %w", err)
		}

		err = i.save(ent)
		if err != nil {
			return fmt.Errorf("saving entity: %w", err)
		}
	}

	err = i.Flush(cursor.String(), block)
	if err != nil {
		return fmt.Errorf("flushing block changes: %w", err)
	}

	return nil
}
