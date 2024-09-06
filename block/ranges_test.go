package block

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRangeMerged(t *testing.T) {
	assert.Equal(t, MustParseRanges("10-40,50-70").String(), MustParseRanges("10-20,20-30,30-40,50-60,60-70").Merged().String())
	assert.Equal(t, MustParseRanges("10-40,60-70").String(), MustParseRanges("10-20,20-30,30-40,60-70").Merged().String())
	assert.Equal(t, MustParseRanges("10-40").String(), MustParseRanges("10-20,20-30,30-40").Merged().String())
	assert.Equal(t, MustParseRanges("1-5,10-12,13-14").String(), MustParseRanges("1-2,2-3,3-4,4-5,10-12,13-14").Merged().String())
}

func TestRangeMergedBuckets(t *testing.T) {
	assert.Equal(t,
		MustParseRanges("1-10,10-11").String(),
		MustParseRanges("1-10,10-11").MergedBuckets(10).String(),
	)
	assert.Equal(t,
		MustParseRanges("1-10,10-12").String(),
		MustParseRanges("1-10,10-12").MergedBuckets(10).String(),
	)
	assert.Equal(t,
		MustParseRanges("10-30,30-40,50-70").String(),
		MustParseRanges("10-20,20-30,30-40,50-60,60-70").MergedBuckets(20).String(),
	)
	assert.Equal(t,
		MustParseRanges("10-30,30-50,50-60,80-100").String(),
		MustParseRanges("10-20,20-30,30-40,40-50,50-60,80-90,90-100").MergedBuckets(20).String(),
	)
	assert.Equal(t,
		MustParseRanges("10-20,20-30,30-40").String(),
		MustParseRanges("10-20,20-30,30-40").MergedBuckets(5).String(),
	)
	assert.Equal(t,
		MustParseRanges("10-20,20-30,30-40,40-50").String(),
		MustParseRanges("10-20,20-30,30-40,40-50").MergedBuckets(11).String(),
	)
	assert.Equal(t,
		MustParseRanges("10-20,20-30,30-40,40-50").String(),
		MustParseRanges("10-20,20-30,30-40,40-50").MergedBuckets(19).String(),
	)
	assert.Equal(t,
		MustParseRanges("10-30,30-50").String(),
		MustParseRanges("10-20,20-30,30-40,40-50").MergedBuckets(20).String(),
	)
	assert.Equal(t,
		MustParseRanges("1-4,4-5,10-12,13-14").String(),
		MustParseRanges("1-2,2-3,3-4,4-5,10-12,13-14").MergedBuckets(3).String(),
	)
}
