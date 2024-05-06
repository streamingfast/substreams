## Javy

A tool that compiles JavaScript ES2020+ code into a `wasm32-wasi` `.wasm` file that can then be run on any compatible `WASM` `WASI` VM like `wasmtime`.

Links:
- Javy @ https://github.com/bytecodealliance/javy
- Wasmtime @ https://github.com/bytecodealliance/wasmtime

### Observations

You compile your JavaScript code into a `.wasm` using:

`javy compile /tmp/test.js -o /tmp/test.wasm`

Which produces a ready made `.wasm` with the JavaScript in it as well as the required JavaScript runtime. The produced `.wasm` file has the following functions that be used:

```
$ wasm-decompile /tmp/test.wasm | grep "import function"
import function wasi_snapshot_preview1_clock_time_get(a:int, b:long, c:int):int;
import function wasi_snapshot_preview1_random_get(a:int, b:int):int;
import function wasi_snapshot_preview1_fd_write(a:int, b:int, c:int, d:int):int;
import function wasi_snapshot_preview1_fd_read(a:int, b:int, c:int, d:int):int;
import function wasi_snapshot_preview1_environ_get(a:int, b:int):int;
import function wasi_snapshot_preview1_environ_sizes_get(a:int, b:int):int;
import function wasi_snapshot_preview1_fd_close(a:int):int;
import function wasi_snapshot_preview1_fd_fdstat_get(a:int, b:int):int;
import function wasi_snapshot_preview1_fd_seek(a:int, b:long, c:int, d:int):int;
import function wasi_snapshot_preview1_proc_exit(a:int);
```

There is those functions that are problematic:

```
import function wasi_snapshot_preview1_clock_time_get(a:int, b:long, c:int):int;
import function wasi_snapshot_preview1_random_get(a:int, b:int):int;
import function wasi_snapshot_preview1_environ_get(a:int, b:int):int;
import function wasi_snapshot_preview1_environ_sizes_get(a:int, b:int):int;
```

Those should be "disabled" somehow, either at runtime or removing from the exported VM if possible.
