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

type Map map[string]Store

func NewMap() Map {
	return map[string]Store{}
}

func (m Map) Set(s Store) {
	m[s.Name()] = s
}

func (m Map) Get(name string) (Store, bool) {
	s, found := m[name]
	return s, found
}

func (m Map) All() map[string]Store {
	return m
}

func (m Map) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddInt("count", len(m))
	return nil
}
