package entity

import (
	"database/sql/driver"
	"encoding/hex"
	"fmt"
	"strings"
)

type Bytes []byte

func (b Bytes) Value() (driver.Value, error) {
	if len(b) == 0 {
		return nil, nil
	}
	return []byte(b), nil
}

func (b *Bytes) Scan(value interface{}) error {
	if value == nil {
		return nil
	}

	*b = value.([]byte)
	return nil
}

func (b Bytes) MarshalCSV() ([]byte, error) {
	return []byte(fmt.Sprintf("\\x%s", hex.EncodeToString(b))), nil
}

func (b *Bytes) UnmarshalCSV(hexStr []byte) error {
	d, err := hex.DecodeString(strings.TrimPrefix(string(hexStr), "\\x"))
	if err != nil {
		return fmt.Errorf("failed to hex decode BYTES: %w", err)
	}
	*b = d
	return nil
}
