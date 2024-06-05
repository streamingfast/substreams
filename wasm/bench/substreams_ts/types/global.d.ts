declare namespace Substreams {
  interface Module {
    exports: any
  }
}

declare var module: Substreams.Module

class Buffer {
  static from(input: string | Uint8Array | ArrayBufferLike, encoding?: string): Buffer

  byteLength: number;

  readonly [Symbol.toStringTag]: string

  slice(begin: number, end?: number): Buffer
  toString(encoding?: string): string
}

declare namespace substreams_engine {
  function output(bytes: Uint8Array)
}
