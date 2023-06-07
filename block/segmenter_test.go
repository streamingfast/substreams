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

func TestSegmenter_IndexWithBlock(t *testing.T) {
	s := Segmenter{
		interval:     10,
		initialBlock: 121,
	}
	assert.Equal(t, -1, s.IndexForBlock(119))
	assert.Equal(t, 0, s.IndexForBlock(120)) // even though the Range() call will yield `nil`
	assert.Equal(t, 0, s.IndexForBlock(121))
	assert.Equal(t, 0, s.IndexForBlock(122))
	assert.Equal(t, 0, s.IndexForBlock(129))
	assert.Equal(t, 1, s.IndexForBlock(130))
	assert.Equal(t, 1, s.IndexForBlock(131))
	assert.Equal(t, 1, s.IndexForBlock(139))
	assert.Equal(t, 2, s.IndexForBlock(140))
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

func TestSegmenter_rangeFromBegin(t *testing.T) {
	s := NewSegmenter(10, 1, 100)
	assert.Equal(t, NewRange(0, 10), s.rangeFromBegin(0))
	s = NewSegmenter(10, 1, 100)
	assert.Equal(t, NewRange(10, 20), s.rangeFromBegin(1))
	s = NewSegmenter(10, 1, 15)
	assert.Equal(t, NewRange(10, 15), s.rangeFromBegin(1))
	s = NewSegmenter(10, 1, 25)
	assert.Equal(t, NewRange(20, 25), s.rangeFromBegin(2))
	s = NewSegmenter(10, 15, 25)
	assert.Equal(t, NewRange(20, 25), s.rangeFromBegin(1))
	s = NewSegmenter(10, 15, 25)
	assert.Equal(t, NewRange(10, 20), s.rangeFromBegin(0))
}

func TestSegmenter_Range(t *testing.T) {
	s := NewSegmenter(10, 1, 100)
	assert.Nil(t, s.Range(-1))

	s = NewSegmenter(10, 1, 100)
	assert.Equal(t, NewRange(1, 10), s.Range(0))

	s = NewSegmenter(10, 1, 15)
	assert.Equal(t, NewRange(10, 15), s.Range(1))
}
