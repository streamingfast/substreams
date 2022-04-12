package entity

import (
	"crypto/md5"
	"encoding/csv"
	"fmt"
	"hash"
	"time"

	"github.com/jszwec/csvutil"
)

type POI struct {
	Base
	Digest Bytes `db:"digest" csv:"digest"`
	md5    hash.Hash
}

func (p *POI) TableName() string {
	return "poi2$"
}

func NewPOI(causalityRegion string) *POI {
	return &POI{
		Base:   NewBase(causalityRegion),
		Digest: nil,
		md5:    md5.New(),
	}
}

func (p *POI) Clear() {
	p.md5 = md5.New()
	p.Digest = nil
}

func (p *POI) IsFinal(_ uint64, _ time.Time) bool {
	return false
}

func (p *POI) RemoveEnt(entityType, entityId string) error {
	if _, err := p.md5.Write([]byte(entityType)); err != nil {
		return fmt.Errorf("unable to encode entity type: %w", err)
	}
	if _, err := p.md5.Write([]byte(entityId)); err != nil {
		return fmt.Errorf("unable to encode entity id: %w", err)
	}
	return nil
}

func (p *POI) AddEnt(entityType string, ent interface{}) error {
	if _, err := p.md5.Write([]byte(entityType)); err != nil {
		return fmt.Errorf("unable to encode entity type: %w", err)
	}

	csvWriter := csv.NewWriter(p.md5)
	enc := csvutil.NewEncoder(csvWriter)
	enc.Tag = "poi"
	enc.AutoHeader = false
	if err := enc.Encode(ent); err != nil {
		return fmt.Errorf("unable to encode serialized entity: %w", err)
	}
	csvWriter.Flush()
	if err := csvWriter.Error(); err != nil {
		return fmt.Errorf("error flushing csv encoder: %w", err)
	}
	return nil
}

func (p *POI) Apply() {
	p.Digest = p.md5.Sum(nil)
}

func (p *POI) AggregateDigest(previousDigest []byte) {
	sum := md5.New()
	_, err := sum.Write(append(previousDigest, p.Digest...))
	if err != nil {
		panic("error generating md5sum")
	}
	p.Digest = sum.Sum(nil)
}
