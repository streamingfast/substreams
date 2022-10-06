package store

import (
	"errors"
	"go.uber.org/zap/zapcore"
)

var NotFound = errors.New("store not found")

type Getter interface {
	Get(name string) (Store, bool)
	All() map[string]Store
}

type Setter interface {
	Set(name string, s Store)
}

type Map struct {
	stores map[string]Store
}

func NewMap() *Map {
	return &Map{
		stores: map[string]Store{},
	}
}

func (m *Map) Set(name string, s Store) {
	m.stores[name] = s
}

func (m *Map) Get(name string) (Store, bool) {
	s, found := m.stores[name]
	return s, found
}

func (m *Map) All() map[string]Store {
	return m.stores
}

func (m *Map) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddInt("count", len(m.stores))
	return nil
}
