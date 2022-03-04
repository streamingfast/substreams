package wasm

import "testing"

func Test_SumBigInt(t *testing.T) {

	wasmBytes := []byte(`
		(module
		  (type $sum_big_int_t (func (param i32) (param i32) (param i32) (param i32) (param i32)))
		  (memory $mem 1)
		  (func $sum_big_int (type $sum_int_64_t) (param $ord i32) (param $key_ptr i32) (param $key_len i32) (param $val_ptr i32) (param $val_len i32)
		    (i32.store (local.get $idx) (local.get $val)))

		  (func $mem_size (type $mem_size_t) (result i32)
		    (memory.size))
		  (export "sum_int_64" (func $sum_big_int))
		  (export "set_at" (func $set_at))
		  (export "mem_size" (func $mem_size))
		  (export "memory" (memory $mem)))
	`)

}
