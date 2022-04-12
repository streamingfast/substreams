package entity

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"time"
)

type ExportedEntities struct {
	BlockNum       uint64
	BlockTimestamp time.Time
	EntityName     string
	Entities       Map

	TypeGetter interface {
		GetType(string) (reflect.Type, bool)
	} `json:"-"`
}

type Map map[string]Interface

func (ee *ExportedEntities) UnmarshalJSON(in []byte) error {
	discovery := struct {
		BlockNum       uint64
		BlockTimestamp time.Time
		EntityName     string
		Entities       map[string]json.RawMessage
	}{}
	if err := json.Unmarshal(in, &discovery); err != nil {
		return fmt.Errorf("unable to unmarshal entity map: %w", err)
	}

	tblName := discovery.EntityName
	if tblName == "" {
		tblName = ee.EntityName
	}

	reflectType, ok := ee.TypeGetter.GetType(tblName)
	if !ok {
		return fmt.Errorf("no entity registered for table name %q", tblName)
	}

	var entities = make(map[string]Interface, len(discovery.Entities))

	for id, rawEntity := range discovery.Entities {
		if bytes.Compare(rawEntity, []byte(`null`)) == 0 {
			entities[id] = nil
			continue
		}

		el := reflect.New(reflectType).Interface()
		if err := json.Unmarshal(rawEntity, el); err != nil {
			return fmt.Errorf("unmarshal raw entity: %w", err)
		}
		entities[id] = el.(Interface)
	}

	*ee = ExportedEntities{
		BlockNum:       discovery.BlockNum,
		BlockTimestamp: discovery.BlockTimestamp,
		EntityName:     tblName,
		Entities:       entities,
	}
	return nil
}
