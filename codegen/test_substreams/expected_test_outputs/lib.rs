mod pb;
mod generated;
use substreams::prelude::*;
use substreams::errors::Error;

impl generated::substreams::SubstreamsTrait for generated::substreams::Substreams{

    fn map_block(
        _block: substreams_ethereum::pb::eth::v2::Block,
    ) -> Result<pb::my_types_v1::Tests, Error> {
		todo!()
	}

    fn map_block_i64(
        _block: substreams_ethereum::pb::eth::v2::Block,
    ) -> Result<i64, Error> {
		todo!()
	}

    fn store_test(
        _block: substreams_ethereum::pb::eth::v2::Block,
        _map_block: pb::my_types_v1::Tests,
        _store: substreams::store::StoreSetProto<pb::my_types_v1::Test>,
    ) {
		todo!()
	}

    fn store_append_string(
        _block: substreams_ethereum::pb::eth::v2::Block,
        _store: substreams::store::StoreAppend<String>,
    ) {
		todo!()
	}

    fn store_bigint(
        _block: substreams_ethereum::pb::eth::v2::Block,
        _store: substreams::store::StoreSetBigInt,
    ) {
		todo!()
	}

    fn store_test2(
        _block: substreams_ethereum::pb::eth::v2::Block,
        _map_block: pb::my_types_v1::Tests,
        _store_test: substreams::store::StoreGetProto<pb::my_types_v1::Test>,
        _store_test_deltas: substreams::store::Deltas<substreams::store::DeltaProto<pb::my_types_v1::Test>>,
        _map_block_i64: i64,
        _store_bigint: substreams::store::StoreGetBigInt,
        _store_bigint_deltas: substreams::store::Deltas<substreams::store::DeltaBigInt>,
        _store_append_string: substreams::store::StoreGetRaw,
        _store_append_string_deltas: substreams::store::Deltas<substreams::store::DeltaArray<String>>,
        _store: substreams::store::StoreSetProto<pb::my_types_v1::Test>,
    ) {
		todo!()
	}
}



