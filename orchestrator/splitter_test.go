package orchestrator

import (
	"strings"
	"testing"

	"github.com/streamingfast/substreams/block"
	"github.com/stretchr/testify/assert"
)

var parseRange = block.ParseRange
var parseRanges = block.ParseRanges

func parseSnapshotSpec(in string) *Snapshots {
	out := &Snapshots{}
	if in == "" {
		return out
	}
	for _, el := range strings.Split(in, ",") {
		el = strings.Trim(el, " ")
		partial := strings.Contains(el, "p")
		partRange := block.ParseRange(strings.Trim(el, "p"))
		if partial {
			out.Partials = append(out.Partials, partRange)
		} else {
			out.Completes = append(out.Completes, partRange)
		}
	}
	out.Sort()
	return out
}

func TestSplitSomeWork(t *testing.T) {
	type splitTestCase struct {
		name string

		storeSplit uint64 // storeSaveInterval, boundaries at which we want a new store snapshot

		modInitBlock uint64     // ModuleInitialBlock
		snapshots    *Snapshots // store's Last block saved from the store's Info file
		reqStart     uint64     // the request's absolute start block

		expectInitLoad *block.Range // Used for LoadFrom()
		expectMissing  block.Ranges // sent to the user as already processed, and passed to the Squasher, the first Covered is expected to match the expectStoreInit
		expectPresent  block.Ranges // sent to the user as already processed, and passed to the Squasher, the first Covered is expected to match the expectStoreInit
	}

	splitTest := func(name string, storeSplit uint64, modInitBlock uint64, snapshotsSpec string, reqStart uint64, expectInitLoad, expectMissing, expectPresent string,
	) splitTestCase {
		c := splitTestCase{
			name:         name,
			storeSplit:   storeSplit,
			snapshots:    parseSnapshotSpec(snapshotsSpec),
			modInitBlock: modInitBlock,
			reqStart:     reqStart,
		}
		c.expectInitLoad = block.ParseRange(expectInitLoad)
		c.expectMissing = block.ParseRanges(expectMissing)
		c.expectPresent = block.ParseRanges(expectPresent)
		return c
	}

	for _, tt := range []splitTestCase{
		splitTest("simple", 10,
			/* modInit, snapshots, reqStart */
			50, "", 100,
			/* expected: initial progress, covered ranges, partials missing, present */
			"", "50-60, 60-70, 70-80, 80-90, 90-100", "",
		),
		splitTest("nothing to work for, nothing to initialize", 10,
			55, "", 55,
			"", "", "",
		),
		splitTest("reqStart before module init, don't process anything and start with a clean store", 10,
			50, "", 10,
			"", "", "",
		),
		splitTest("one case", 10,
			0, "0-20,p20-30", 20,
			"0-20", "", "",
		),
		splitTest("10 blocks already processed", 10, // 20,
			50, "50-60,p70-80", 90,
			"50-60", "60-70,80-90", "70-80",
		),
		splitTest("40 blocks already processed", 10,
			50, "50-60,p60-70,p70-80", 100,
			"50-60", "80-90,90-100", "60-70,70-80",
		),
		splitTest("multiple complete", 10,
			50, "50-60,50-70,50-80,p80-90", 100, // would they be sorted this way? should we run `sort` on the snapshots first?
			"50-80", "90-100", "80-90",
		),
		splitTest("off bounds, no blocks processed", 10,
			55, "", 92,
			"", "55-60,60-70,70-80,80-90,90-92", "",
		),
		splitTest("reqStart just above the modInit, and lower bound lower than modInit", 10,
			55, "", 60,
			"", "55-60", "",
		),
		splitTest("reqStart just above the modInit, and lower bound lower than modInit, off bound", 10,
			55, "", 59,
			"", "55-59", "",
		),
		splitTest("reqStart just above the modInit, and lower bound lower than modInit, lastBlock higher", 10,
			55, "55-60,p60-70,p70-80", 60,
			"55-60", "", "",
		),
		splitTest("reqStart off bound just above the modInit, and lower bound lower than modInit, lastBlock higher", 10,
			55, "55-60,p60-70", 59,
			"", "55-59", "",
		),
		splitTest("reqStart equal to lastSaved, on bound", 10,
			50, "50-60,p60-70,p70-80,p80-90", 90,
			"50-60", "", "60-70,70-80,80-90",
		),
		splitTest("reqStart equal to lastSaved, off bound", 10,
			50, "50-60,p60-70,p70-80,p80-90", 92,
			"50-60", "90-92", "60-70,70-80,80-90",
		),
	} {
		t.Run(tt.name, func(t *testing.T) {
			work := SplitSomeWork("mod", tt.storeSplit, tt.modInitBlock, tt.reqStart, tt.snapshots)
			assert.Equal(t, tt.expectInitLoad, work.loadInitialStore)
			assert.Equal(t,
				tt.expectMissing.String(),
				work.partialsMissing.String(),
			)
			assert.Equal(t,
				tt.expectPresent.String(),
				work.partialsPresent.String(),
			)
		})
	}
}

func TestSplitWorkComputeRequests(t *testing.T) {
	t.Skip("why would we ever want to produce a larger number of partials than the subreqSplit value? oh its because the Squasher will use those partials and create a store snapshot at the partial interval.. if it doesn,t have partials, it doesn't have the boundaries to write a snapshot at the given interval")
	// perhaps the "backprocessing" request should return in the TRAILERS of the request, the ranges of stores it has produced
	// and the Squasher can take that in simply.. so it doesn't rely on the computations and boundaries that it itself calculates (and where they both must align to be detected).
	// if the backend says it produced stores for X-y,y-z, etc.. then the squasher can simply use that to merge, ensuring it has contiguity, and trusting the
	// content was duly written because it trusts the Object Store's permissions to have protected its writes.
	// the "partials file writer" then becomes the unit at which the squasher is expected to squash, and can be separate
	// and driven by the amount of bytes in each partials, etc..
	// all future requests will align on whatever is IN the Snapshots, so they can have different sizes and it wouldn't matter.
}

// 	type splitTestCase struct {
// 		name string

// 		storeSplit uint64 // storeSaveInterval, boundaries at which we want a new store snapshot

// 		modInitBlock uint64     // ModuleInitialBlock
// 		snapshots    *Snapshots // store's Last block saved from the store's Info file
// 		reqStart     uint64     // the request's absolute start block

// 		expectInitLoad *block.Range // Used for LoadFrom()
// 		expectMissing  block.Ranges // sent to the user as already processed, and passed to the Squasher, the first Covered is expected to match the expectStoreInit
// 		expectPresent  block.Ranges // sent to the user as already processed, and passed to the Squasher, the first Covered is expected to match the expectStoreInit
// 	}

// 	splitTest := func(name string, storeSplit uint64, modInitBlock uint64, snapshotsSpec string, reqStart uint64, expectInitLoad, expectMissing, expectPresent string,
// 	) splitTestCase {
// 		c := splitTestCase{
// 			name:         name,
// 			storeSplit:   storeSplit,
// 			snapshots:    parseSnapshotSpec(snapshotsSpec),
// 			modInitBlock: modInitBlock,
// 			reqStart:     reqStart,
// 		}
// 		c.expectInitLoad = block.ParseRange(expectInitLoad)
// 		c.expectMissing = block.ParseRanges(expectMissing)
// 		c.expectPresent = block.ParseRanges(expectPresent)
// 		return c
// 	}

// 	for _, tt := range []splitTestCase{
// 		splitTest("simple", 10,
// 			/* modInit, snapshots, reqStart */
// 			50, "", 100,
// 			/* expected: initial progress, covered ranges, partials missing, present */
// 			"", "50-60, 60-70, 70-80, 80-90, 90-100", "",
// 		),
// 		splitTest("nothing to work for, nothing to initialize", 10,
// 			55, "", 55,
// 			"", "", "",
// 		),
// 		splitTest("reqStart before module init, don't process anything and start with a clean store", 10,
// 			50, "", 10,
// 			"", "", "",
// 		),
// 		splitTest("one case", 10,
// 			0, "0-20,p20-30", 20,
// 			"0-20", "", "",
// 		),
// 		splitTest("10 blocks already processed", 10, // 20,
// 			50, "50-60,p70-80", 90,
// 			"50-60", "60-70,80-90", "70-80",
// 		),
// 		splitTest("40 blocks already processed", 10,
// 			50, "50-60,p60-70,p70-80", 100,
// 			"50-60", "80-90,90-100", "60-70,70-80",
// 		),
// 		splitTest("multiple complete", 10,
// 			50, "50-60,50-70,50-80,p80-90", 100, // would they be sorted this way? should we run `sort` on the snapshots first?
// 			"50-80", "90-100", "80-90",
// 		),
// 		splitTest("off bounds, no blocks processed", 10,
// 			55, "", 92,
// 			"", "55-60,60-70,70-80,80-90,90-92", "",
// 		),
// 		splitTest("reqStart just above the modInit, and lower bound lower than modInit", 10,
// 			55, "", 60,
// 			"", "55-60", "",
// 		),
// 		splitTest("reqStart just above the modInit, and lower bound lower than modInit, off bound", 10,
// 			55, "", 59,
// 			"", "55-59", "",
// 		),
// 		splitTest("reqStart just above the modInit, and lower bound lower than modInit, lastBlock higher", 10,
// 			55, "55-60,p60-70,p70-80", 60,
// 			"55-60", "", "",
// 		),
// 		splitTest("reqStart off bound just above the modInit, and lower bound lower than modInit, lastBlock higher", 10,
// 			55, "55-60,p60-70", 59,
// 			"", "55-59", "",
// 		),
// 		splitTest("reqStart equal to lastSaved, on bound", 10,
// 			50, "50-60,p60-70,p70-80,p80-90", 90,
// 			"50-60", "", "60-70,70-80,80-90",
// 		),
// 		splitTest("reqStart equal to lastSaved, off bound", 10,
// 			50, "50-60,p60-70,p70-80,p80-90", 92,
// 			"50-60", "90-92", "60-70,70-80,80-90",
// 		),
// 	} {
// 		t.Run(tt.name, func(t *testing.T) {
// 			work := SplitSomeWork("mod", tt.storeSplit, tt.modInitBlock, tt.reqStart, tt.snapshots)
// 			assert.Equal(t, tt.expectInitLoad, work.loadInitialStore)
// 			assert.Equal(t,
// 				tt.expectMissing.String(),
// 				work.partialsMissing.String(),
// 			)
// 			assert.Equal(t,
// 				tt.expectPresent.String(),
// 				work.partialsPresent.String(),
// 			)
// 		})
// 	}
// }

// func TestComputeExclusiveEndBlock(t *testing.T) {
// 	tests := []struct {
// 		name      string
// 		lastSaved int
// 		target    int
// 		expect    int
// 	}{
// 		{
// 			name:      "target equal to last saved, on bound",
// 			lastSaved: 90,
// 			target:    90,
// 			expect:    90,
// 		},
// 		{
// 			name:      "target later than last saved, on bound",
// 			lastSaved: 100,
// 			target:    90,
// 			expect:    90,
// 		},
// 		{
// 			name:      "target later than last saved, off bound",
// 			lastSaved: 100,
// 			target:    91,
// 			expect:    90,
// 		},
// 		{
// 			name:      "target later than last saved, off bound",
// 			lastSaved: 100,
// 			target:    91,
// 			expect:    90,
// 		},
// 		{
// 			name:      "target prior to last saved, on bound",
// 			lastSaved: 80,
// 			target:    90,
// 			expect:    80,
// 		},
// 		{
// 			name:      "target prior to last saved, off bound",
// 			lastSaved: 80,
// 			target:    92,
// 			expect:    80,
// 		},
// 		{
// 			name:      "nothing saved, target off bound",
// 			lastSaved: 0,
// 			target:    92,
// 			expect:    0,
// 		},
// 		{
// 			name:      "nothing saved, target on bound",
// 			lastSaved: 0,
// 			target:    80,
// 			expect:    0,
// 		},
// 	}
// 	moduleInitBlock := 50
// 	saveInterval := 10

// 	for _, test := range tests {
// 		t.Run(test.name, func(t *testing.T) {
// 			res := computeStoreExclusiveEndBlock(uint64(test.lastSaved), uint64(test.target), uint64(saveInterval), uint64(moduleInitBlock))
// 			assert.Equal(t, test.expect, int(res))
// 		})
// 	}
// }
