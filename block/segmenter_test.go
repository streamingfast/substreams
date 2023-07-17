package block

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSegmenter_Count(t *testing.T) {
	type fields struct {
		interval           int
		initialBlock       int
		linearHandoffBlock int
	}
	tests := []struct {
		name   string
		fields fields
		count  int
	}{
		{
			"beginning",
			fields{10, 12, 31},
			3, // the 10-20, 20-30, 30-31 segments
		},
		{
			"further down",
			fields{10, 112, 131},
			3, // the 110-120, 120-130, 130-131 segments
		},
		{
			"first module segment",
			fields{10, 112, 129},
			2,
		},
		{
			"first module segment is further down",
			fields{10, 112, 135},
			3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewSegmenter(uint64(tt.fields.interval), uint64(tt.fields.initialBlock), uint64(tt.fields.linearHandoffBlock))
			assert.Equalf(t, tt.count, s.Count(), "Count()")
		})
	}
}

func TestSegmenter_IndexForStartBlock(t *testing.T) {
	s := Segmenter{interval: 10}
	assert.Equal(t, 0, s.IndexForStartBlock(5))
	assert.Equal(t, 0, s.IndexForStartBlock(9))
	assert.Equal(t, 1, s.IndexForStartBlock(10))
	assert.Equal(t, 1, s.IndexForStartBlock(11))
	assert.Equal(t, 1, s.IndexForStartBlock(19))
	assert.Equal(t, 2, s.IndexForStartBlock(20))
	assert.Equal(t, 2, s.IndexForStartBlock(21))
	assert.Equal(t, 4, s.IndexForStartBlock(45))
}

func TestSegmenter_IndexForEndBlock(t *testing.T) {
	s := Segmenter{
		interval: 10,
	}
	assert.Equal(t, 0, s.IndexForEndBlock(5))
	assert.Equal(t, 0, s.IndexForEndBlock(9))
	assert.Equal(t, 0, s.IndexForEndBlock(10))
	assert.Equal(t, 1, s.IndexForEndBlock(11))
	assert.Equal(t, 1, s.IndexForEndBlock(19))
	assert.Equal(t, 1, s.IndexForEndBlock(20))
	assert.Equal(t, 2, s.IndexForEndBlock(21))
	assert.Equal(t, 4, s.IndexForEndBlock(45))
}

func TestSegmenter_firstRange(t *testing.T) {
	s := &Segmenter{interval: 10, initialBlock: 1, exclusiveEndBlock: 100}
	assert.Equal(t, NewRange(1, 10), s.firstRange())
	s = &Segmenter{interval: 10, initialBlock: 0, exclusiveEndBlock: 100}
	assert.Equal(t, NewRange(0, 10), s.firstRange())
	s = &Segmenter{interval: 10, initialBlock: 9, exclusiveEndBlock: 100}
	assert.Equal(t, NewRange(9, 10), s.firstRange())
	s = &Segmenter{interval: 10, initialBlock: 10, exclusiveEndBlock: 100}
	assert.Equal(t, NewRange(10, 20), s.firstRange())
	s = &Segmenter{interval: 10, initialBlock: 11, exclusiveEndBlock: 100}
	assert.Equal(t, NewRange(11, 20), s.firstRange())
	s = &Segmenter{interval: 10, initialBlock: 11, exclusiveEndBlock: 15}
	assert.Equal(t, NewRange(11, 15), s.firstRange())

	s = &Segmenter{interval: 10, initialBlock: 11, exclusiveEndBlock: 10}
	assert.Nil(t, s.firstRange())
}

func TestSegmenter_followingRange(t *testing.T) {
	s := NewSegmenter(10, 1, 100)
	assert.Equal(t, ParseRange("0-10"), s.followingRange(0))
	s = NewSegmenter(10, 1, 100)
	assert.Equal(t, ParseRange("10-20"), s.followingRange(1))
	s = NewSegmenter(10, 1, 15)
	assert.Equal(t, ParseRange("10-15"), s.followingRange(1))
	s = NewSegmenter(10, 1, 25)
	assert.Equal(t, ParseRange("20-25"), s.followingRange(2))
	s = NewSegmenter(10, 15, 25)
	assert.Equal(t, ParseRange("20-25"), s.followingRange(2))
	s = NewSegmenter(10, 15, 25)
	assert.Equal(t, ParseRange("10-20"), s.followingRange(1))
}

func TestSegmenter_Range(t *testing.T) {
	s := NewSegmenter(10, 1, 100)
	assert.Nil(t, s.Range(-1))

	s = NewSegmenter(10, 15, 25)
	assert.Nil(t, s.Range(0))
	assert.Equal(t, 1, s.FirstIndex())
	assert.Equal(t, 2, s.LastIndex())
	assert.Equal(t, ParseRange("15-20"), s.Range(1))
	assert.Equal(t, ParseRange("20-25"), s.Range(2))
	assert.Nil(t, s.Range(3))

	s = NewSegmenter(10, 1, 99)
	assert.Equal(t, 10, s.Count())
	assert.Equal(t, 0, s.FirstIndex())
	assert.Equal(t, 9, s.LastIndex())
	assert.Equal(t, ParseRange("90-99"), s.Range(9))
	assert.False(t, s.EndsOnInterval(9))

	s = NewSegmenter(10, 1, 15)
	assert.Equal(t, NewRange(10, 15), s.Range(1))

	s = NewSegmenter(10, 1, 20)
	assert.Equal(t, 2, s.Count())
	assert.Equal(t, 0, s.FirstIndex())
	assert.Equal(t, 1, s.LastIndex())
	assert.Equal(t, ParseRange("10-20"), s.Range(1))
	assert.True(t, s.EndsOnInterval(1))

}
