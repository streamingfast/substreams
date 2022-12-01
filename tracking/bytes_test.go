package tracking

import (
	"errors"
	"testing"

	"github.com/streamingfast/substreams"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/require"
)

func TestBytesMeter_AddBytesRead(t *testing.T) {
	type fields struct {
		modules         map[string]struct{}
		bytesWrittenMap map[string]uint64
		bytesReadMap    map[string]uint64
	}
	type args struct {
		module string
		n      int
	}
	tests := []struct {
		name     string
		fields   fields
		args     args
		validate func(t *testing.T, fields fields, args args)
	}{
		{
			name: "simple",
			fields: fields{
				modules:         map[string]struct{}{},
				bytesWrittenMap: map[string]uint64{},
				bytesReadMap:    map[string]uint64{},
			},
			args: args{
				module: "A",
				n:      1,
			},
			validate: func(t *testing.T, fields fields, args args) {
				expected := uint64(1)
				actual := fields.bytesReadMap[args.module]
				require.Equal(t, expected, actual)
			},
		},
		{
			name: "multiple",
			fields: fields{
				modules: map[string]struct{}{
					"test": {},
				},
				bytesWrittenMap: map[string]uint64{},
				bytesReadMap: map[string]uint64{
					"A": 1,
					"B": 2,
				},
			},
			args: args{
				module: "A",
				n:      1,
			},
			validate: func(t *testing.T, fields fields, args args) {
				expected := uint64(2)
				actual := fields.bytesReadMap[args.module]
				require.Equal(t, expected, actual)

				expected = uint64(2)
				actual = fields.bytesReadMap["B"]
				require.Equal(t, expected, actual)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &bytesMeter{
				modules:         tt.fields.modules,
				bytesWrittenMap: tt.fields.bytesWrittenMap,
				bytesReadMap:    tt.fields.bytesReadMap,
			}
			b.AddBytesRead(tt.args.module, tt.args.n)
			tt.validate(t, tt.fields, tt.args)
		})
	}
}

func TestBytesMeter_AddBytesWritten(t *testing.T) {
	type fields struct {
		modules         map[string]struct{}
		bytesWrittenMap map[string]uint64
		bytesReadMap    map[string]uint64
	}
	type args struct {
		module string
		n      int
	}
	tests := []struct {
		name     string
		fields   fields
		args     args
		validate func(t *testing.T, fields fields, args args)
	}{
		{
			name: "simple",
			fields: fields{
				modules:         map[string]struct{}{},
				bytesWrittenMap: map[string]uint64{},
				bytesReadMap:    map[string]uint64{},
			},
			args: args{
				module: "A",
				n:      1,
			},
			validate: func(t *testing.T, fields fields, args args) {
				expected := uint64(1)
				actual := fields.bytesWrittenMap[args.module]
				require.Equal(t, expected, actual)
			},
		},
		{
			name: "multiple",
			fields: fields{
				modules: map[string]struct{}{
					"test": {},
				},
				bytesWrittenMap: map[string]uint64{
					"A": 1,
					"B": 2,
				},
				bytesReadMap: map[string]uint64{},
			},
			args: args{
				module: "A",
				n:      1,
			},
			validate: func(t *testing.T, fields fields, args args) {
				expected := uint64(2)
				actual := fields.bytesWrittenMap[args.module]
				require.Equal(t, expected, actual)

				expected = uint64(2)
				actual = fields.bytesWrittenMap["B"]
				require.Equal(t, expected, actual)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &bytesMeter{
				modules:         tt.fields.modules,
				bytesWrittenMap: tt.fields.bytesWrittenMap,
				bytesReadMap:    tt.fields.bytesReadMap,
			}
			b.AddBytesWritten(tt.args.module, tt.args.n)
			tt.validate(t, tt.fields, tt.args)
		})
	}
}

func TestBytesMeter_Send(t *testing.T) {
	var respFuncError = errors.New("respFuncError")

	type fields struct {
		modules         map[string]struct{}
		bytesWrittenMap map[string]uint64
		bytesReadMap    map[string]uint64
	}
	tests := []struct {
		name     string
		fields   fields
		err      error
		validate func(t *testing.T, fields fields, resps []*pbsubstreams.Response, err error)
	}{
		{
			name: "baseline",
			fields: fields{
				modules:         map[string]struct{}{},
				bytesWrittenMap: map[string]uint64{},
				bytesReadMap:    map[string]uint64{},
			},
			validate: func(t *testing.T, fields fields, resps []*pbsubstreams.Response, err error) {
				require.Len(t, resps, 0)
				require.Nil(t, err)
			},
		},
		{
			name: "simple",
			fields: fields{
				modules: map[string]struct{}{
					"A": {},
					"B": {},
				},
				bytesWrittenMap: map[string]uint64{
					"A": 1,
				},
				bytesReadMap: map[string]uint64{
					"A": 1,
				},
			},
			validate: func(t *testing.T, fields fields, resps []*pbsubstreams.Response, err error) {
				require.Len(t, resps, 1)
				require.Nil(t, err)
			},
		},
		{
			name: "error",
			fields: fields{
				modules: map[string]struct{}{
					"A": {},
				},
				bytesWrittenMap: map[string]uint64{
					"A": 1,
				},
				bytesReadMap: map[string]uint64{
					"A": 1,
				},
			},
			err: respFuncError,
			validate: func(t *testing.T, fields fields, resps []*pbsubstreams.Response, err error) {
				require.Equal(t, respFuncError, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &bytesMeter{
				modules:         tt.fields.modules,
				bytesWrittenMap: tt.fields.bytesWrittenMap,
				bytesReadMap:    tt.fields.bytesReadMap,
			}

			var resps []*pbsubstreams.Response
			testRespFunc := substreams.ResponseFunc(func(resp *pbsubstreams.Response) error {
				resps = append(resps, resp)
				return tt.err
			})

			err := b.Send(testRespFunc)
			tt.validate(t, tt.fields, resps, err)
		})
	}
}
