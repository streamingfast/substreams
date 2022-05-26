use proc_macro::TokenStream;

mod handler;
mod config;
mod store;
mod errors;


/// Marks function to setup substream map handler wasm boilerplate
///
/// ## Usage
///
///
/// ```rust
/// #[substreams::handlers::map]
/// fn map_handler(blk: eth::Block) -> Result<eth::TransactionTrace, SubstreamError> {
///     Ok(blk.transactions_traces[0])
/// }
/// ```
///
/// Equivalent code not using `#[substream::handlers::map]`
///
/// ```rust
/// #[no_mangle]
/// pub extern "C" fn map_handler(blk_ptr: *mut u8, blk_len: usize) {
///     substreams::register_panic_hook();
///     let func = || -> Result<eth::TransactionTrace, SubstreamError> {
///         let blk: eth::Block = substreams::proto::decode_ptr(blk_ptr, blk_len).unwrap();
///         {
///		Ok(blk.transactions_traces[0])
///         }
///     };
///     let result = func();
///     if result.is_err() {
///         panic!(result.err().unwrap())
///     }
///     substreams::output(proto::encode(result).unwrap());
/// }
/// ```
#[proc_macro_attribute]
pub fn map(args: TokenStream, item: TokenStream) -> TokenStream {
    return handler::main(args,item,config::ModuleType::Map);
}

/// Marks function to setup substream store handler wasm boilerplate
/// ## Usage
///
///
/// ```rust
/// #[substreams::handlers::store]
/// fn build_nft_state(transfers: erc721::Transfers, s: store::SumInt64Writer, pairs: store::Reader, tokens: store::Reader) {
///     println("Length {:?}", transfers.len());
/// }
/// ```
///
/// Equivalent code not using `#[substream::handlers::store]`
///
/// ```rust
/// #[no_mangle]
/// pub extern "C" fn build_nft_state(transfers_ptr: *mut u8,transfers_len: usize,pairs_idx: u32,tokens_idx: u32) {
///    substreams::register_panic_hook();
///    let transfers: erc721::Transfers = substreams::proto::decode_ptr(transfers_ptr, transfers_len).unwrap();
///    let pairs: store::Reader = store::Reader::new(pairs_idx);
///    let tokens: store::Reader = store::Reader::new(tokens_idx);
///    let s: store::SumInt64Writer = store::SumInt64Writer::new();
///    {
///        println("Length {:?}", transfers.len());
///    }
/// }
/// ```
#[proc_macro_attribute]
pub fn store(args: TokenStream, item: TokenStream) -> TokenStream {
    return handler::main(args,item,config::ModuleType::Store);
}

#[proc_macro_derive(StoreWriter)]
pub fn derive(input: TokenStream) -> TokenStream { store::main(input)}
