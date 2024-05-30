mod pb;

use std::str::FromStr;

use crate::pb::test;
use solana_program::pubkey::Pubkey;
use substreams;

#[substreams::handlers::map]
fn map_block(blk: test::Block) -> Result<test::MapResult, substreams::errors::Error> {
    let pubkey = Pubkey::from_str("5oNDL3swdJJF1g9DzJiZ4ynHXgszjAEpUkxVYejchzrY").unwrap();

    let out = test::MapResult {
        block_number: blk.number,
        block_hash: blk.id + &pubkey.to_string(),
    };

    Ok(out)
}
