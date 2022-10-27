package marshaller

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_readMapStringBytes(t *testing.T) {
	type args struct {
		in []byte
	}
	tests := []struct {
		name    string
		args    args
		want    map[string][]byte
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "missing entry count",
			args: args{
				in: []byte{
					0x01,
				},
			},
			want:    nil,
			wantErr: assert.Error,
		},
		{
			name: "missing key bytes",
			args: args{
				in: []byte{
					0x01,
					0x02, 0x00,
				},
			},
			want:    nil,
			wantErr: assert.Error,
		},
		{
			name: "missing value content bytes",
			args: args{
				in: []byte{
					0x01,
					0x01, 0x6B, // k
					0x01,
				},
			},
			want:    nil,
			wantErr: assert.Error,
		},
		{
			name: "valid bytes",
			args: args{
				in: []byte{
					0x01,
					0x01, 0x6B, // k
					0x01, 0x76, // v
				},
			},
			want: map[string][]byte{
				"k": {
					0x76,
				},
			},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := readMapStringBytes(tt.args.in)
			if !tt.wantErr(t, err, fmt.Sprintf("readMapStringBytes(%v)", tt.args.in)) {
				return
			}
			assert.Equalf(t, tt.want, got, "readMapStringBytes(%v)", tt.args.in)
		})
	}
}
