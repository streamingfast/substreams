package block

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSegmenter_Count(t *testing.T) {
	type fields struct {
		interval               int
		moduleTreeInitialBlock int
		currentModuleInitBlock int
		linearHandoffBlock     int
	}
	tests := []struct {
		name               string
		fields             fields
		countBegin         int
		countModuleInit    int
		firstModuleSegment int
	}{
		{
			"beginning",
			fields{10, 12, 22, 31},
			3, // the 10-20, 20-30, 30-31 segments
			2, // the 20-30, 30-31
			1,
		},
		{
			"further down",
			fields{10, 112, 122, 131},
			3, // the 110-120, 120-130, 130-131 segments
			2, // the 120-130, 130-131
			1,
		},
		{
			"first module segment",
			fields{10, 112, 113, 129},
			2,
			2,
			0,
		},
		{
			"first module segment is further down",
			fields{10, 112, 133, 135},
			3,
			1,
			2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewSegmenter(uint64(tt.fields.interval), uint64(tt.fields.moduleTreeInitialBlock), uint64(tt.fields.currentModuleInitBlock), uint64(tt.fields.linearHandoffBlock))
			assert.Equalf(t, tt.countBegin, s.CountFromBegin(), "CountFromBegin()")
			assert.Equalf(t, tt.countModuleInit, s.CountFromModuleInit(), "CountFromModuleInit()")
			assert.Equalf(t, tt.firstModuleSegment, s.FirstModuleSegment(), "FirstModuleSegment()")
		})
	}
}

func TestSegmenter_IndexWithBlock(t *testing.T) {
	s := Segmenter{
		interval:       10,
		graphInitBlock: 121,
	}
	assert.Panics(t, func() { s.IndexWithBlock(119) })
	assert.Panics(t, func() { s.IndexWithBlock(120) })
	assert.Equal(t, 0, s.IndexWithBlock(121))
	assert.Equal(t, 0, s.IndexWithBlock(122))
	assert.Equal(t, 0, s.IndexWithBlock(129))
	assert.Equal(t, 1, s.IndexWithBlock(130))
	assert.Equal(t, 1, s.IndexWithBlock(131))
	assert.Equal(t, 1, s.IndexWithBlock(139))
	assert.Equal(t, 2, s.IndexWithBlock(140))
}
