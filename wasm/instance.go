package wasm

import (
	"github.com/tetratelabs/wazero"
)

// An Instance is a copy of the WASM VM for a given Module, with its own memory,
// and recreated at each module within a given block.
type Instance struct {
	name string

	CurrentCall *Call
	entrypoint  string
	wazInstance wazero.CompiledModule

	isClosed bool
}

//
//func (i *Instance) registerStateImports() error {
//	functions := map[string]interface{}{}
//	functions["set"] = i.set
//	functions["set_if_not_exists"] = i.setIfNotExists
//	functions["append"] = i.append
//	functions["delete_prefix"] = i.deletePrefix
//	functions["add_bigint"] = i.addBigInt
//	functions["add_bigdecimal"] = i.addBigDecimal
//	functions["add_bigfloat"] = i.addBigDecimal
//	functions["add_int64"] = i.addInt64
//	functions["add_float64"] = i.addFloat64
//	functions["set_min_int64"] = i.setMinInt64
//	functions["set_min_bigint"] = i.setMinBigint
//	functions["set_min_float64"] = i.setMinFloat64
//	functions["set_min_bigdecimal"] = i.setMinBigDecimal
//	functions["set_min_bigfloat"] = i.setMinBigDecimal
//	functions["set_max_int64"] = i.setMaxInt64
//	functions["set_max_bigint"] = i.setMaxBigInt
//	functions["set_max_float64"] = i.setMaxFloat64
//	functions["set_max_bigdecimal"] = i.setMaxBigDecimal
//	functions["set_max_bigfloat"] = i.setMaxBigDecimal
//	functions["get_at"] = i.getAt
//	functions["get_first"] = i.getFirst
//	functions["get_last"] = i.getLast
//	functions["has_at"] = i.hasAt
//	functions["has_first"] = i.hasFirst
//	functions["has_last"] = i.hasLast
//
//	return nil
//}
