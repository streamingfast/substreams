mod allocators;
mod pb;
mod bench_ethereum;
mod bench_solana;

// Re-export all benchmark entrypoints
pub use bench_ethereum::*;
pub use bench_solana::*;

/// Verification entrypoint: outputs which allocator is compiled into this module
#[no_mangle]
pub extern "C" fn get_allocator_name(_ptr: *mut u8, _len: usize) {
    substreams::register_panic_hook();

    let name = allocators::allocator_name();

    // Output the allocator name as a simple string message
    let output = pb::bench::BenchOutput {
        count: name.len() as u64,
        allocator_name: name.to_string(),
    };

    substreams::skip_empty_output();
    substreams::output(output);
}
