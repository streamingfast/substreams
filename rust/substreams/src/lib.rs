//! A library for writing Substreams handlers.
//!
//! Substreams consts of a numbmbder of modules that provide struct and macros for
//! implementing Substreams handlers. The handlers are defined in your Manifest.
//!Learn more about Substreams at <https://substreams.streamingfast.io>
//!
//! ## Handler Examples
//!
//! Below are a few `map` handler examples. The signature of then handler function is based
//! on the inputs and output defined in the `map` module definition in the Manifest. There
//! are a few things to note:
//! * Best practice is to name your `map` module function `map_<your_action>'
//! * `map` module function should *always* return a Result
//! * The Result should have an Error type set to `subtreams::error:Error`
//!
//! ```no_run
//! use substreams::{errors::Error, store};
//!
//! // map handler that takes a source as input
//! #[substreams::handlers::map]
//! fn map_transfers(blk: eth::Block) -> Result<proto::Custom, Error> {
//!     // do something
//! }
//!
//! // map handler that takes a source, and a store in get mode as inputs
//! #[substreams::handlers::map]
//! fn map_ownerships(blk: eth::Block, store: store::StoreGet) -> Result<proto::Custom, Error> {
//!     // do something
//! }
//!
//! // map handler that takes a source, another map, and a store in get mode as inputs
//! #[substreams::handlers::map]
//! fn map_mints(blk: eth::Block, mints: proto::Custom, store: store::StoreGet) -> Result<proto::Custom, Error> {
//!     // do something
//! }
//! //!
//! // map handler that takes a source, another map, and a store in delta mode as inputs
//! #[substreams::handlers::map]
//! fn map_db(blk: eth::Block, mints: proto::Custom, store_deltas: store::Deltas) -> Result<proto::Custom, Error> {
//!     // do something
//! }
//! ```
//!
//! Below are a few `store` handler examples. The signature of the handler function is based
//! on the inputs defined in the `stopre` module definition in the Manifest. There
//! are a few things to note:
//! * Best practice is to name your `map` module function `store_<your_action>'
//! * `store` module function should *return nothing*
//!
//! ```no_run
//! use substreams::store;
//!
//! #[substreams::handlers::store]
//! fn store_transfers(objects: proto::Custom, output: store::StoreAddInt64) {
//!     // to something
//! }
//!
//! #[substreams::handlers::store]
//! fn store_ownerships(objects: proto::Custom, store: store::StoreGet, output: store::StoreAddInt64) {
//!     // to something
//! }
//!
//! #[substreams::handlers::store]
//! fn store_mints(objects: proto::Custom, store: store::StoreGet, another_store: store::StoreGet, store_deltas: store::Deltas, output: store::StoreAddInt64) {
//!     // to something
//! }
//!```
//!
//!
pub mod errors;
mod externs;
pub mod handlers;
mod hex;
pub mod log;
pub mod memory;

/// Protobuf generated Substream models
pub mod pb;
pub mod proto;
mod state;
pub mod store;
pub use crate::hex::Hex;

pub fn output<M: prost::Message>(msg: M) {
    // Need to return the buffer and forget about it issue occured when trying to write large data
    // wasm was "dropping" the data before we could write to it, which causes us to have garbage
    // value. By forgetting the data we can properly call external output function to write the
    // msg to heap.
    let (ptr, len, _buffer) = proto::encode_to_ptr(&msg).unwrap();
    std::mem::forget(&_buffer);
    unsafe { externs::output(ptr, len as u32) }
}

///
pub fn output_raw(data: Vec<u8>) {
    unsafe { externs::output(data.as_ptr(), data.len() as u32) }
}

/// Registers a Substreams custom panic hook. The panic hook is invoked when then handler panics
pub fn register_panic_hook() {
    use std::sync::Once;
    static SET_HOOK: Once = Once::new();
    SET_HOOK.call_once(|| {
        std::panic::set_hook(Box::new(hook));
    });
}

fn hook(info: &std::panic::PanicInfo<'_>) {
    let error_msg = info
        .payload()
        .downcast_ref::<String>()
        .map(String::as_str)
        .or_else(|| info.payload().downcast_ref::<&'static str>().copied())
        .unwrap_or("");
    let location = info.location();

    unsafe {
        let _ = match location {
            Some(loc) => {
                let file = loc.file();
                let line = loc.line();
                let column = loc.column();

                externs::register_panic(
                    error_msg.as_ptr(),
                    error_msg.len() as u32,
                    file.as_ptr(),
                    file.len() as u32,
                    line,
                    column,
                )
            }
            None => externs::register_panic(
                error_msg.as_ptr(),
                error_msg.len() as u32,
                std::ptr::null(),
                0,
                0,
                0,
            ),
        };
    }
}
