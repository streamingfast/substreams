package orchestrator

import (
	"strconv"
	"strings"
	"testing"

	"github.com/streamingfast/substreams/block"
	"github.com/stretchr/testify/assert"
)

func rng(in string) *block.Range {
	if in == "" {
		return nil
	}
	ch := strings.Split(in, "-")
	lo, err := strconv.ParseInt(ch[0], 10, 64)
	if err != nil {
		panic(err)
	}
	hi, err := strconv.ParseInt(ch[1], 10, 64)
	if err != nil {
		panic(err)
	}
	return &block.Range{StartBlock: uint64(lo), ExclusiveEndBlock: uint64(hi)}
}

func rngs(in string) (out block.Ranges) {
	for _, e := range strings.Split(in, ",") {
		out = append(out, rng(strings.Trim(e, " ")))
	}
	return
}

type splitTestCase struct {
	name string

	storeSplit  uint64 // storeSaveInterval, boundaries at which we want a new store snapshot
	subreqSplit uint64 // boundaries at which we want to split sharded queries

	modInitBlock uint64 // ModuleInitialBlock
	lastBlock    uint64 // store's Last block saved from the store's Info file
	reqStart     uint64 // the request's absolute start block

	expectStoreInit *block.Range // used both to LoadFrom() in the Squasher, and to send an initial Progress notification
	expectSubreqs   string
}

func splitTest(name string, storeSplit, subreqSplit uint64, modInitBlock, lastBlock, reqStart uint64, expectProgress, expectSubreqs string,
) splitTestCase {
	return splitTestCase{
		name:            name,
		storeSplit:      storeSplit,
		subreqSplit:     subreqSplit,
		lastBlock:       lastBlock,
		modInitBlock:    modInitBlock,
		reqStart:        reqStart,
		expectStoreInit: rng(expectProgress),
		expectSubreqs:   expectSubreqs,
	}
}

func TestSplitWork(t *testing.T) {
	for _, tt := range []splitTestCase{
		splitTest("simple", 10, 10,
			/* modInit, lastSaved, reqStart */
			50, 0, 100,
			/* expected initial _Progress_, expected requests(store chunks) */
			"", "50-60, 60-70, 70-80, 80-90, 90-100",
		),
		splitTest("reqStart", 10, 10,
			50, 0, 100,
			"", "50-60, 60-70, 70-80, 80-90, 90-100",
		),
		splitTest("different splits for store and reqs, 10 blocks already processed", 10, 20,
			50, 60, 90,
			"50-60", "60-80(60-70,70-80), 80-90",
		),
		splitTest("different splits for store and reqs, 40 blocks already processed", 10, 20,
			50, 80, 100,
			"50-80", "80-100(80-90,90-100)",
		),
		splitTest("different splits for store and reqs, no blocks processed", 10, 20,
			55, 0, 92,
			"", "55-60,60-80(60-70,70-80),80-92(80-90,TMP:90-92)",
		),
		// noEqSplit("start at zero", 100, 200, 50, "0-300", "50-100, 100-200, 200-300"),
		// splitTest("start at initial block", 100, 50, "50-300", "50-100, 100-200, 200-300"),
		// splitTest("start after start block, on boundary", 100, 50, "100-300", "100-200,200-300"),
		// splitTest("start after start block, random block", 100, 50, "127-300", "127-200,200-300"),
		splitTest("store synchronized at modInit, which shouldn't happen", 10, 10,
			50, 50, 100,
			"", "50-60, 60-70, 70-80, 80-90, 90-100",
		),
		splitTest("modInit off bounds, reqStart off bound too", 10, 10,
			55, 0, 85,
			"", "55-60, 60-70, 70-80, 80-85(TMP:80-85)",
		),
		splitTest("reqStart just above the modInit, and lower bound lower than modInit", 10, 10,
			55, 0, 60,
			"", "55-60",
		),
		splitTest("reqStart off bound just above the modInit, and lower bound lower than modInit", 10, 10,
			55, 0, 59,
			"", "55-59(TMP:55-59)",
		),
		splitTest("reqStart equal to lastSaved, on bound", 10, 10,
			50, 90, 90,
			"50-90", "",
		),
		splitTest("reqStart equal to lastSaved, off bound", 10, 10,
			50, 80, 92,
			"50-80", "80-90,90-92(TMP:90-92)",
		),
		splitTest("nothing saved, reqStart off bound", 10, 10,
			50, 0, 72,
			"", "50-60,60-70,70-72(TMP:70-72)",
		),
		splitTest("nothing saved, reqStart on bound", 10, 10,
			50, 0, 70,
			"", "50-60,60-70",
		),
		splitTest("nothing saved, reqStart on bound", 10, 10,
			50, 0, 70,
			"", "50-60,60-70",
		),
		splitTest("reqStart after last saved but below init block, can't have last saved below module's init block", 10, 10,
			50, 20, 40,
			"", "PANIC",
		),
		splitTest("reqStart before last saved but below init block, can't have last saved below module's init block", 10, 10,
			50, 30, 10,
			"", "PANIC",
		),
		splitTest("reqStart before module init, don't process anything and start with a clean store", 10, 10,
			50, 0, 10,
			"", "",
		),
	} {
		t.Run(tt.name, func(t *testing.T) {
			var work *SplitWork
			f := func() {
				work = splitWork("mod", tt.storeSplit, tt.subreqSplit, tt.modInitBlock, tt.lastBlock, tt.reqStart)
			}
			if tt.expectSubreqs == "PANIC" {
				assert.Panics(t, f, "bob")
			} else {
				f()
				assert.Equal(t, tt.expectStoreInit, work.loadInitialStore)
				var reqChunks []string
				for _, rc := range work.reqChunks {
					reqChunks = append(reqChunks, rc.String())
				}
				assert.Equal(t,
					strings.Replace(tt.expectSubreqs, " ", "", -1),
					strings.Replace(strings.Join(reqChunks, ","), " ", "", -1),
				)
			}
		})
	}
}

func TestComputeExclusiveEndBlock(t *testing.T) {
	tests := []struct {
		name      string
		lastSaved int
		target    int
		expect    int
	}{
		{
			name:      "target equal to last saved, on bound",
			lastSaved: 90,
			target:    90,
			expect:    90,
		},
		{
			name:      "target later than last saved, on bound",
			lastSaved: 100,
			target:    90,
			expect:    90,
		},
		{
			name:      "target later than last saved, off bound",
			lastSaved: 100,
			target:    91,
			expect:    90,
		},
		{
			name:      "target later than last saved, off bound",
			lastSaved: 100,
			target:    91,
			expect:    90,
		},
		{
			name:      "target prior to last saved, on bound",
			lastSaved: 80,
			target:    90,
			expect:    80,
		},
		{
			name:      "target prior to last saved, off bound",
			lastSaved: 80,
			target:    92,
			expect:    80,
		},
		{
			name:      "nothing saved, target off bound",
			lastSaved: 0,
			target:    92,
			expect:    0,
		},
		{
			name:      "nothing saved, target on bound",
			lastSaved: 0,
			target:    80,
			expect:    0,
		},
	}
	moduleInitBlock := 50
	saveInterval := 10

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			res := computeStoreExclusiveEndBlock(uint64(test.lastSaved), uint64(test.target), uint64(saveInterval), uint64(moduleInitBlock))
			assert.Equal(t, test.expect, int(res))
		})
	}
}
