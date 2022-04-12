package entity

import (
	"database/sql/driver"
	"fmt"
)

type Enum string

func (e Enum) String() string {
	return string(e)
}

func (e *Enum) Value() (driver.Value, error) {
	if e == nil {
		return nil, nil
	}

	if *e == "" {
		return nil, nil
	}

	str := string(*e)
	return str, nil
}

func (e *Enum) Scan(value interface{}) error {

	if value == nil {
		return nil
	}

	sv, err := driver.String.ConvertValue(value)
	if err != nil {
		return err
	}

	bs, ok := sv.([]byte)
	if !ok {
		return fmt.Errorf("could not convert data to byte array")
	}
	str := string(bs)

	*e = Enum(str)
	return nil
}
