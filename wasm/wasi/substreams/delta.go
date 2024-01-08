package substreams

import (
	"fmt"
	"math/big"
	"strconv"

	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
)

type Delta[T any] interface {
	From(d *pbsubstreamsrpc.StoreDelta) error
}

type deltaCommon struct {
	Operation pbsubstreamsrpc.StoreDelta_Operation
	Key       string
	Ordinal   uint64
}

type DeltaInt64 struct {
	*deltaCommon
	OldValue int64
	NewValue int64
}

func (d *DeltaInt64) From(sd *pbsubstreamsrpc.StoreDelta) error {
	d.deltaCommon = &deltaCommon{
		Operation: sd.Operation,
		Key:       sd.Key,
		Ordinal:   sd.Ordinal,
	}

	ov, err := strconv.Atoi(string(sd.OldValue))
	if err != nil {
		return fmt.Errorf("unable to convert old value to int64: %w", err)
	}
	d.OldValue = int64(ov)

	nv, err := strconv.Atoi(string(sd.NewValue))
	if err != nil {
		return fmt.Errorf("unable to convert new value to int64: %w", err)
	}
	d.NewValue = int64(nv)

	return nil
}

type DeltaInt32 struct {
	*deltaCommon
	OldValue int32
	NewValue int32
}

func (d *DeltaInt32) From(sd *pbsubstreamsrpc.StoreDelta) error {
	d.deltaCommon = &deltaCommon{
		Operation: sd.Operation,
		Key:       sd.Key,
		Ordinal:   sd.Ordinal,
	}

	ov, err := strconv.Atoi(string(sd.OldValue))
	if err != nil {
		return fmt.Errorf("unable to convert old value to int32: %w", err)
	}
	d.OldValue = int32(ov)

	nv, err := strconv.Atoi(string(sd.NewValue))
	if err != nil {
		return fmt.Errorf("unable to convert new value to int32: %w", err)
	}
	d.NewValue = int32(nv)

	return nil
}

type DeltaFloat64 struct {
	*deltaCommon
	OldValue float64
	NewValue float64
}

func (d *DeltaFloat64) From(sd *pbsubstreamsrpc.StoreDelta) error {
	d.deltaCommon = &deltaCommon{
		Operation: sd.Operation,
		Key:       sd.Key,
		Ordinal:   sd.Ordinal,
	}

	ov, err := strconv.ParseFloat(string(sd.OldValue), 64)
	if err != nil {
		return fmt.Errorf("unable to convert old value to float64: %w", err)
	}
	d.OldValue = ov

	nv, err := strconv.ParseFloat(string(sd.NewValue), 64)
	if err != nil {
		return fmt.Errorf("unable to convert new value to float64: %w", err)
	}
	d.NewValue = nv

	return nil
}

type DeltaBigInt struct {
	*deltaCommon
	OldValue *big.Int
	NewValue *big.Int
}

func (d *DeltaBigInt) From(sd *pbsubstreamsrpc.StoreDelta) error {
	d.deltaCommon = &deltaCommon{
		Operation: sd.Operation,
		Key:       sd.Key,
		Ordinal:   sd.Ordinal,
	}

	ov := new(big.Int)
	ov.SetString(string(sd.OldValue), 10)
	d.OldValue = ov

	nv := new(big.Int)
	nv.SetString(string(sd.NewValue), 10)
	d.NewValue = nv

	return nil
}

type DeltaBigFloat struct {
	*deltaCommon
	OldValue *big.Float
	NewValue *big.Float
}

func (d *DeltaBigFloat) From(sd *pbsubstreamsrpc.StoreDelta) error {
	d.deltaCommon = &deltaCommon{
		Operation: sd.Operation,
		Key:       sd.Key,
		Ordinal:   sd.Ordinal,
	}

	ov := new(big.Float)
	ov.SetString(string(sd.OldValue))
	d.OldValue = ov

	nv := new(big.Float)
	nv.SetString(string(sd.NewValue))
	d.NewValue = nv

	return nil
}

type DeltaBool struct {
	*deltaCommon
	OldValue bool
	NewValue bool
}

func (d *DeltaBool) From(sd *pbsubstreamsrpc.StoreDelta) error {
	d.deltaCommon = &deltaCommon{
		Operation: sd.Operation,
		Key:       sd.Key,
		Ordinal:   sd.Ordinal,
	}

	containsZero := func(b []byte) bool {
		for _, v := range b {
			if v == 0 {
				return true
			}
		}
		return false
	}

	d.OldValue = !containsZero(sd.OldValue)
	d.NewValue = !containsZero(sd.NewValue)

	return nil
}

type DeltaString struct {
	*deltaCommon
	OldValue string
	NewValue string
}

func (d *DeltaString) From(sd *pbsubstreamsrpc.StoreDelta) error {
	d.deltaCommon = &deltaCommon{
		Operation: sd.Operation,
		Key:       sd.Key,
		Ordinal:   sd.Ordinal,
	}

	d.OldValue = string(sd.OldValue)
	d.NewValue = string(sd.NewValue)

	return nil
}

type DeltaBytes struct {
	*deltaCommon
	OldValue []byte
	NewValue []byte
}

func (d *DeltaBytes) From(sd *pbsubstreamsrpc.StoreDelta) error {
	d.deltaCommon = &deltaCommon{
		Operation: sd.Operation,
		Key:       sd.Key,
		Ordinal:   sd.Ordinal,
	}

	d.OldValue = sd.OldValue
	d.NewValue = sd.NewValue

	return nil
}
