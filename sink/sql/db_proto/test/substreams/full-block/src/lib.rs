mod pb;
use substreams_ethereum::pb::eth::v2::Block;

#[allow(unused_imports)]
use num_traits::cast::ToPrimitive;
substreams_ethereum::init!();

#[substreams::handlers::map]
fn full_block(blk: Block) -> Result<Block, substreams::errors::Error> {
    Ok(blk)
}
