package entity

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type BlockRange struct {
	StartBlock uint64
	EndBlock   uint64
}

func (b *BlockRange) UnmarshalJSON(data []byte) error {
	// if data is a raw string representation of a block range eg: [23,2314) or "[23,2314)"
	if string(data[0]) == "[" || string(data[0:2]) == `"[` {
		return b.ParseBytes(data)
	}

	//otherwise, we unmarshal the usual way:
	var t struct {
		StartBlock uint64
		EndBlock   uint64
	}
	err := json.Unmarshal(data, &t)
	if err != nil {
		return err
	}
	b.StartBlock = t.StartBlock
	b.EndBlock = t.EndBlock
	return nil
}

func (b *BlockRange) Value() (driver.Value, error) {
	return b.String(), nil
}

func (b *BlockRange) MarshalCSV() ([]byte, error) {
	return []byte(b.String()), nil
}

func (b *BlockRange) String() string {
	if b.StartBlock == 0 && b.EndBlock == 0 {
		panic("string(): empty block range not allowed")
	}

	s := fmt.Sprintf("[%d,", b.StartBlock)
	if b.EndBlock > 0 {
		s += fmt.Sprintf("%d", b.EndBlock)
	}
	s += ")"
	return s
}

func (b *BlockRange) Scan(value interface{}) error {
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

	return b.ParseBytes(bs)
}

func (b *BlockRange) ParseBytes(bs []byte) error {
	v := string(bs)
	v = strings.Trim(v, `"`) //in case of raw string, remove quotes
	v = v[1 : len(v)-1]

	vals := strings.Split(v, ",")

	if vals[0] != "" {
		startBlock, err := strconv.ParseUint(vals[0], 10, 64)
		if err != nil {
			return err
		}

		b.StartBlock = startBlock
	}

	if vals[1] != "" {
		endBlock, err := strconv.ParseUint(vals[0], 10, 64)
		if err != nil {
			return err
		}

		b.EndBlock = endBlock
	}

	if b.StartBlock == 0 && b.EndBlock == 0 {
		panic("scan(): empty block range not allowed")
	}

	return nil
}
