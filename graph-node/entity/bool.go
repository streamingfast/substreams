package entity

import (
	"database/sql/driver"
	"fmt"
)

type Bool bool

func NewBool(v bool) Bool {
	return Bool(v)
}

func (b *Bool) Value() (driver.Value, error) {
	if b == nil {
		return nil, nil
	}
	return bool(*b), nil
}

func (b *Bool) Scan(value interface{}) error {
	if value == nil {
		return nil
	}

	sv, err := driver.Bool.ConvertValue(value)
	if err != nil {
		return err
	}

	bv, ok := sv.(bool)
	if !ok {
		return fmt.Errorf("could not convert data to boolean")
	}

	*b = NewBool(bv)
	return nil
}

func (b Bool) Ptr() *Bool { return &b }
