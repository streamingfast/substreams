// Import rust's io and filesystem module
use std::io::prelude::*;
use std::fs;

// Entry point to our WASI applications
fn main() {

    // Print out hello world!
    // This will handle writing to stdout for us using the WASI APIs (e.g fd_write)
    println!("Hello world!");

    // // Create a file
    // // We are creating a `helloworld.txt` file in the `/helloworld` directory
    // // This code requires the Wasi host to provide a `/helloworld` directory on the guest.
    // // If the `/helloworld` directory is not available, the unwrap() will cause this program to panic.
    // // For example, in Wasmtime, if you want to map the current directory to `/helloworld`,
    // // invoke the runtime with the flag/argument: `--mapdir /helloworld::.`
    // // This will map the `/helloworld` directory on the guest, to  the current directory (`.`) on the host
    // let mut file = fs::File::create("/helloworld/helloworld.txt").unwrap();
    //
    // // Write the text to the file we created
    // write!(file, "Hello world!\n").unwrap();
}

#[no_mangle]
pub extern "C" fn map_block(blk_ptr: *mut u8, blk_len: usize) {
    println!("Hello world! {}", blk_len);

    // let mut file = fs::File::create("/helloworld/helloworld.txt").unwrap();
    // write!(file, "Hello world!\n").unwrap();

    let mut buf = Vec::with_capacity(50000);
    let ptr = buf.as_mut_ptr();
    unsafe {
        output(ptr, 44957);
    }
}

#[no_mangle]
pub fn alloc(size: usize) -> *mut u8 {
    let mut buf = Vec::with_capacity(size);
    let ptr = buf.as_mut_ptr();

    // Runtime is responsible of calling dealloc when no longer needed
    std::mem::forget(buf);
    ptr
}

#[no_mangle]
pub unsafe fn dealloc(ptr: *mut u8, size: usize) {
    std::mem::drop(Vec::from_raw_parts(ptr, size, size))
}

#[link(wasm_import_module = "env")]
extern "C" {
    pub fn output(ptr: *const u8, len: u32);
    pub fn register_panic(
        msg_ptr: *const u8,
        msg_len: u32,
        file_ptr: *const u8,
        file_len: u32,
        line: u32,
        column: u32,
    );
}