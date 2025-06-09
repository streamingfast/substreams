import { substreams } from "@substreams/sdk";
import { create } from "@bufbuild/protobuf";
import { Block, BlockSchema, MapResultSchema } from "./pb/sf/substreams/v1/test/test_pb";

export default class Substreams {
	@substreams.handlers.map([BlockSchema], MapResultSchema)
	test_map(blk: Block) {
		return create(MapResultSchema, {
			blockNumber: 14n,
			blockHash: "testdata",
		});
	}
}
