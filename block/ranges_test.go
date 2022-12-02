package block

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRangeMerged(t *testing.T) {
	assert.Equal(t, ParseRanges("10-40,50-70").String(), ParseRanges("10-20,20-30,30-40,50-60,60-70").Merged().String())
	assert.Equal(t, ParseRanges("10-40,60-70").String(), ParseRanges("10-20,20-30,30-40,60-70").Merged().String())
	assert.Equal(t, ParseRanges("10-40").String(), ParseRanges("10-20,20-30,30-40").Merged().String())
	assert.Equal(t, ParseRanges("1-5,10-12,13-14").String(), ParseRanges("1-2,2-3,3-4,4-5,10-12,13-14").Merged().String())
}

func TestRangeMergedBuckets(t *testing.T) {
	assert.Equal(t,
		ParseRanges("1-10,10-11").String(),
		ParseRanges("1-10,10-11").MergedBuckets(10).String(),
	)
	assert.Equal(t,
		ParseRanges("1-10,10-12").String(),
		ParseRanges("1-10,10-12").MergedBuckets(10).String(),
	)
	assert.Equal(t,
		ParseRanges("10-30,30-40,50-70").String(),
		ParseRanges("10-20,20-30,30-40,50-60,60-70").MergedBuckets(20).String(),
	)
	assert.Equal(t,
		ParseRanges("10-30,30-50,50-60,80-100").String(),
		ParseRanges("10-20,20-30,30-40,40-50,50-60,80-90,90-100").MergedBuckets(20).String(),
	)
	assert.Equal(t,
		ParseRanges("10-20,20-30,30-40").String(),
		ParseRanges("10-20,20-30,30-40").MergedBuckets(5).String(),
	)
	assert.Equal(t,
		ParseRanges("10-20,20-30,30-40,40-50").String(),
		ParseRanges("10-20,20-30,30-40,40-50").MergedBuckets(11).String(),
	)
	assert.Equal(t,
		ParseRanges("10-20,20-30,30-40,40-50").String(),
		ParseRanges("10-20,20-30,30-40,40-50").MergedBuckets(19).String(),
	)
	assert.Equal(t,
		ParseRanges("10-30,30-50").String(),
		ParseRanges("10-20,20-30,30-40,40-50").MergedBuckets(20).String(),
	)
	assert.Equal(t,
		ParseRanges("1-4,4-5,10-12,13-14").String(),
		ParseRanges("1-2,2-3,3-4,4-5,10-12,13-14").MergedBuckets(3).String(),
	)
}
