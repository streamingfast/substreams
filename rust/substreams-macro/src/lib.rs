use proc_macro::TokenStream;

mod handler;


/// Marks function to setup substream wasm boilerplate
/// ## Usage
///
/// ### Using map handler
///
/// ```rust
/// #[substream::handler(type = "map")]
/// fn map_handler(blk: eth::Block) -> Result<u64, SubstreamError> {
///     Ok(blk.transactions_traces.len())
/// }
/// ```
///
/// Equivalent code not using `#[substream::handler]`
///
/// ```rust
/// #[no_mangle]
/// pub extern "C" fn map_handler(blk_ptr: *mut u8, blk_len: usize) {
///     substreams::register_panic_hook();
///     let func = || -> Result<u64, SubstreamError> {
///         let blk: eth::Block = substreams::proto::decode_ptr(blk_ptr, blk_len).unwrap();
///         {
///             Ok(blk.transactions_traces.len())
///         }
///     };
///     let result = func();
///     if result.is_err() {
///         panic!(result.err().unwrap())
///     }
///     substreams::output(result.unwrap());
/// }
/// ```
#[proc_macro_attribute]
pub fn handler(args: TokenStream, item: TokenStream) -> TokenStream {
    handler::main(args, item)
}
