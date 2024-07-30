// The entry file of your WebAssembly module.
import { Protobuf } from "as-proto/assembly";
import { Block } from "./generated/my/Block";

// export function map_test(ptr: i32, size: i32): i32 {
//   const bytes = new Uint8Array(0) // .wrap(memory.data(), ptr, size);
//   const block = Protobuf.decode<Block>(bytes, Block.decode);
 
 
export function map_test(bytes: Uint8Array): i32 {
  const block = Protobuf.decode<Block>(bytes, Block.decode);
  block.number += 1
  
  //println()

  output(Protobuf.encode<Block>(block, Block.encode))

  return 0
}



@external("env", "output")
declare function _output(ptr: i32, size: i32): void
// @external("env", "log")
// declare function log(bytes: Uint8Array): void

function output(bytes: Uint8Array): void {
  bytes.byteOffset
  const ptr = changetype<usize>(bytes.buffer);
  const size = bytes.byteLength;
  _output(i32(ptr), i32(size))
}

export function alloc(size: i32): i32 {
  return i32(heap.alloc(usize(size)))
}
export function dealloc(size: i32): void {
  heap.free(usize(size))
}
