package state

import (
	"reflect"
	"testing"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/storage/execout"
)

func TestNewExecOutputStorageState(t *testing.T) {
	t.Skip("test the generated ranges")
	type args struct {
		config             *execout.Config
		saveInterval       uint64
		requestStartBlock  uint64
		linearHandoffBlock uint64
		snapshots          block.Ranges
	}
	tests := []struct {
		name    string
		args    args
		wantOut *ExecOutputStorageState
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOut, err := NewExecOutputStorageState(tt.args.config, tt.args.saveInterval, tt.args.requestStartBlock, tt.args.linearHandoffBlock, tt.args.snapshots)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewExecOutputStorageState() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotOut, tt.wantOut) {
				t.Errorf("NewExecOutputStorageState() gotOut = %v, want %v", gotOut, tt.wantOut)
			}
		})
	}
}
