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
			"partial",
			fmt.Sprintf("%010d-%010d.partial", 100, 0),
			&FileInfo{ModuleName: "test", Filename: "0000000100-0000000000.partial", Range: block.NewRange(0, 100), Partial: true},
			true,
		},
		{
			"full",
			fmt.Sprintf("%010d-%010d.kv", 100, 0),
			&FileInfo{ModuleName: "test", Filename: "0000000100-0000000000.kv", Range: block.NewRange(0, 100), Partial: false},
			true,
		},
		{
			"old-partial-with-trace-id",
			fmt.Sprintf("%010d-%010d.deadbeefdeadbeefdeadbeefdeadbeef.partial", 100, 0),
			&FileInfo{ModuleName: "test", Filename: "0000000100-0000000000.deadbeefdeadbeefdeadbeefdeadbeef.partial", Range: block.NewRange(0, 100), Partial: true, WithTraceID: true},
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := parseFileName("test", tt.filename)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.want1, got1)
		})
	}
}
