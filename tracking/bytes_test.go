package tracking

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"github.com/streamingfast/substreams"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"github.com/stretchr/testify/require"
)

func TestBytesMeter_AddBytesRead(t *testing.T) {
	type fields struct {
		bytesWritten uint64
		bytesRead    uint64
	}
	type args struct {
		module string
		n      int
	}
	tests := []struct {
		name     string
		fields   fields
		args     args
		expected uint64
	}{
		{
			name:   "simple",
			fields: fields{},
			args: args{
				n: 1,
			},
			expected: uint64(1),
		},
		{
			name: "multiple",
			fields: fields{
				bytesRead: 1,
			},
			args: args{
				n: 1,
			},
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &bytesMeter{
				bytesWritten: tt.fields.bytesWritten,
				bytesRead:    tt.fields.bytesRead,

				logger: zap.NewNop(),
			}
			b.AddBytesRead(tt.args.n)
			actual := b.bytesRead
			expected := tt.expected
			require.Equal(t, expected, actual)
		})
	}
}

func TestBytesMeter_AddBytesWritten(t *testing.T) {
	type fields struct {
		bytesWritten uint64
		bytesRead    uint64
	}
	type args struct {
		n int
	}
	tests := []struct {
		name     string
		fields   fields
		args     args
		expected uint64
	}{
		{
			name:   "simple",
			fields: fields{},
			args: args{
				n: 1,
			},
			expected: 1,
		},
		{
			name: "multiple",
			fields: fields{
				bytesWritten: 1,
			},
			args: args{
				n: 1,
			},
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &bytesMeter{
				bytesWritten: tt.fields.bytesWritten,
				bytesRead:    tt.fields.bytesRead,

				logger: zap.NewNop(),
			}
			b.AddBytesWritten(tt.args.n)
			actual := b.bytesWritten
			expected := tt.expected
			require.Equal(t, expected, actual)
		})
	}
}

func TestBytesMeter_Send(t *testing.T) {
	var respFuncError = errors.New("respFuncError")

	type fields struct {
		bytesWritten uint64
		bytesRead    uint64
	}
	tests := []struct {
		name         string
		fields       fields
		err          error
		requiredMsgs int
		requiredErr  error
		validate     func(t *testing.T, fields fields, resps []*pbsubstreamsrpc.Response, err error)
	}{
		{
			name:         "simple",
			fields:       fields{},
			requiredMsgs: 1,
			requiredErr:  nil,
		},
		{
			name:         "error",
			fields:       fields{},
			err:          respFuncError,
			requiredMsgs: 0,
			requiredErr:  respFuncError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &bytesMeter{
				bytesWritten: tt.fields.bytesWritten,
				bytesRead:    tt.fields.bytesRead,

				logger: zap.NewNop(),
			}

			var resps []*pbsubstreamsrpc.Response
			testRespFunc := substreams.ResponseFunc(func(respAny substreams.ResponseFromAnyTier) error {
				if tt.err != nil {
					return tt.err
				}

				resp := respAny.(*pbsubstreamsrpc.Response)
				resps = append(resps, resp)
				return nil
			})

			err := b.Send(context.TODO(), testRespFunc)
			require.Equal(t, len(resps), tt.requiredMsgs)
			require.Equal(t, err, tt.requiredErr)
		})
	}
}
