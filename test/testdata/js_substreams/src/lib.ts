import { substreams, getClock } from "@substreams/sdk";
import { create } from "@bufbuild/protobuf";
import { Block, BlockSchema, MapResultSchema } from "./pb/sf/substreams/v1/test/test_pb";

export default class Substreams {
	@substreams.handlers.map([BlockSchema], MapResultSchema)
	js_test_map(blk: Block) {
		return create(MapResultSchema, {
			blockNumber: blk.number,
			blockHash: blk.id,
		});
	}
	@substreams.handlers.map([BlockSchema], MapResultSchema)
	js_test_map_clock() {
		const clock = getClock();
		return create(MapResultSchema, {
			blockNumber: clock.number,
			blockHash: clock.id,
		});
	}
}
