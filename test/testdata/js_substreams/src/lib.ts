import { substreams, getClock } from "@substreams/sdk";
import { fromBinary, create } from "@bufbuild/protobuf";
import { Block, BlockSchema, MapResultSchema } from "./pb/sf/substreams/v1/test/test_pb";
import { ClockSchema } from "./pb/sf/substreams/v1/clock_pb";

export default class Substreams {
	@substreams.handlers.map([BlockSchema], MapResultSchema)
	test_map(blk: Block) {
		return create(MapResultSchema, {
			blockNumber: blk.number,
			blockHash: blk.id,
		});
	}
	@substreams.handlers.map([ClockSchema], MapResultSchema)
	test_map_clock() {
		const clock = getClock();
		return create(MapResultSchema, {
			blockNumber: clock.number,
			blockHash: clock.id,
		});
	}
}
