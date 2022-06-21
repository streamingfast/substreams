//! Handler macros for Substreams.
//!
//! This create exports useful macros that you can use to develop
//! Substreams handlers. The goal of these macros is to significantly reduce boilerplate
//! code and ensure that your handler are more readable

/// Marks function to setup substream map handler WASM boilerplate
///
/// ## Usage
///
///
/// ```rust
/// # mod eth { pub type Block = (); }
/// # mod proto { pub type Custom = (); }
///
/// #[substreams::handlers::map]
/// fn map_handler(blk: eth::Block) -> Result<proto::Custom, substreams::errors::Error> {
///     unimplemented!("do something");
/// }
/// ```
///
/// Equivalent code not using `#[substream::handlers::map]`
///
/// ```rust
/// # mod eth { pub type Block = (); }
/// # mod proto {
/// #  #[derive(Debug)]
/// #  pub struct Custom(u8);
/// #    impl prost::Message for Custom {
/// #  fn encode_raw<B: prost::bytes::BufMut>(&self, _: &mut B) where Self: Sized { todo!() }
/// #  fn merge_field<B: prost::bytes::Buf>(&mut self, _: u32, _: prost::encoding::WireType, _: &mut B, _: prost::encoding::DecodeContext) -> Result<(), prost::DecodeError> where Self: Sized { todo!() }
/// #  fn encoded_len(&self) -> usize { todo!() }
/// #  fn clear(&mut self) { todo!() }
/// #  }
/// # }
///
/// #[no_mangle]
/// pub extern "C" fn map_handler(blk_ptr: *mut u8, blk_len: usize) {
///     substreams::register_panic_hook();
///     let func = || -> Result<proto::Custom, substreams::errors::Error> {
///         let blk: eth::Block = substreams::proto::decode_ptr(blk_ptr, blk_len).unwrap();
///         {
///             unimplemented!("do something");
///         }
///     };
///     let result = func();
///     if result.is_err() {
///         panic!(result.err().unwrap())
///     }
///     substreams::output(substreams::proto::encode(&result.unwrap()).unwrap());
/// }
/// ```
pub use substreams_macro::map;

/// Marks function to setup substream store handler WASM boilerplate
/// ## Usage
///
///
/// ```rust
/// use substreams::{log, store};
/// # mod proto { pub type Custom = (); }
///
/// #[substreams::handlers::store]
/// fn build_nft_state(data: proto::Custom, s: store::StoreAddInt64, pairs: store::StoreGet, tokens: store::StoreGet) {
///     unimplemented!("do something");
/// }
/// ```
///
/// Equivalent code not using `#[substream::handlers::store]`
///
/// ```rust
/// use substreams::{log, store};
/// # mod proto { pub type Custom = (); }
///
/// #[no_mangle]
/// pub extern "C" fn build_nft_state(data_ptr: *mut u8, data_len: usize, pairs_idx: u32, tokens_idx: u32) {
///    substreams::register_panic_hook();
///    let data: proto::Custom = substreams::proto::decode_ptr(data_ptr, data_len).unwrap();
///    let pairs: store::StoreGet = store::StoreGet::new(pairs_idx);
///    let tokens: store::StoreGet = store::StoreGet::new(tokens_idx);
///    let s: store::StoreAddInt64 = store::StoreAddInt64::new();
///    {
///        unimplemented!("do something");
///    }
/// }
/// ```
pub use substreams_macro::store;
