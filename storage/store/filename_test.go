package store

import (
	"fmt"
	"testing"

	"github.com/streamingfast/substreams/block"
	"github.com/stretchr/testify/assert"
)

func Test_parseFileName(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     *FileInfo
		want1    bool
	}{
		{
			"partial legacy",
			fmt.Sprintf("%010d-%010d.partial", 100, 0),
			&FileInfo{Filename: "0000000100-0000000000.partial", Range: block.NewRange(0, 100), TraceID: "", Partial: true},
			true,
		},
		{
			"partial",
			fmt.Sprintf("%010d-%010d.abcdef.partial", 100, 0),
			&FileInfo{Filename: "0000000100-0000000000.abcdef.partial", Range: block.NewRange(0, 100), TraceID: "abcdef", Partial: true},
			true,
		},
		{
			"full",
			fmt.Sprintf("%010d-%010d.kv", 100, 0),
			&FileInfo{Filename: "0000000100-0000000000.kv", Range: block.NewRange(0, 100), TraceID: "", Partial: false},
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := parseFileName(tt.filename)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.want1, got1)
		})
	}
}
