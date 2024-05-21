mod pb;

use crate::pb::test;
use solana_program::pubkey::Pubkey;
use substreams;

pub use solana_sdk_macro::wasm_bindgen_stub as wasm_bindgen;

#[substreams::handlers::map]
fn map_block(blk: test::Block) -> Result<test::MapResult, substreams::errors::Error> {
    let out = test::MapResult {
        block_number: blk.number,
        block_hash: blk.id,
    };

    Ok(out)
}
