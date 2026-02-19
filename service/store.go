package service

import "go.uber.org/zap/zapcore"

type usedStore struct {
	Name string
	Hash string
}

func (s *usedStore) MarshalLogObject(e zapcore.ObjectEncoder) error {
	e.AddString("name", s.Name)
	e.AddString("hash", s.Hash)
	return nil
}

type UsedFoundationalStore struct {
	Identifier string
	ModuleHash string
}

func (s *UsedFoundationalStore) MarshalLogObject(e zapcore.ObjectEncoder) error {
	e.AddString("identifier", s.Identifier)
	e.AddString("module_hash", s.ModuleHash)

	return nil
}
