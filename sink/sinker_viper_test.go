package sink

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/streamingfast/bstream"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddFlagsToSet(t *testing.T) {
	tests := []struct {
		name          string
		ignore        []FlagIgnored
		expectedFlags []string
	}{
		{
			"default",
			nil,
			[]string{
				FlagParams,
				FlagNetwork,
				FlagInsecure,
				FlagPlaintext,
				FlagUndoBufferSize,
				FlagLiveBlockTimeDelta,
				FlagDevelopmentMode,
				FlagFinalBlocksOnly,
				FlagInfiniteRetry,
				FlagIrreversibleOnly,
				FlagSkipPackageValidation,
				FlagExtraHeaders,
				FlagAPIKeyEnvvar,
				FlagAPITokenEnvvar,
				FlagNoopMode,
			},
		},
		{
			"ignore one",
			[]FlagIgnored{FlagIgnore(FlagInsecure)},
			[]string{
				FlagParams,
				FlagNetwork,
				FlagPlaintext,
				FlagUndoBufferSize,
				FlagLiveBlockTimeDelta,
				FlagDevelopmentMode,
				FlagFinalBlocksOnly,
				FlagInfiniteRetry,
				FlagIrreversibleOnly,
				FlagSkipPackageValidation,
				FlagExtraHeaders,
				FlagAPIKeyEnvvar,
				FlagAPITokenEnvvar,
				FlagNoopMode,
			},
		},
		{
			"ignore one multiple",
			[]FlagIgnored{FlagIgnore(FlagInsecure, FlagPlaintext)},
			[]string{
				FlagParams,
				FlagNetwork,
				FlagUndoBufferSize,
				FlagLiveBlockTimeDelta,
				FlagDevelopmentMode,
				FlagFinalBlocksOnly,
				FlagInfiniteRetry,
				FlagIrreversibleOnly,
				FlagSkipPackageValidation,
				FlagExtraHeaders,
				FlagAPIKeyEnvvar,
				FlagAPITokenEnvvar,
				FlagNoopMode,
			},
		},
		{
			"ignore multiple",
			[]FlagIgnored{FlagIgnore(FlagInsecure), FlagIgnore(FlagPlaintext)},
			[]string{
				FlagParams,
				FlagNetwork,
				FlagUndoBufferSize,
				FlagLiveBlockTimeDelta,
				FlagDevelopmentMode,
				FlagFinalBlocksOnly,
				FlagInfiniteRetry,
				FlagIrreversibleOnly,
				FlagSkipPackageValidation,
				FlagExtraHeaders,
				FlagAPIKeyEnvvar,
				FlagAPITokenEnvvar,
				FlagNoopMode,
			},
		},
		{
			"ignore mixed",
			[]FlagIgnored{FlagIgnore(FlagInsecure), FlagIgnore(FlagPlaintext, FlagLiveBlockTimeDelta)},
			[]string{
				FlagParams,
				FlagNetwork,
				FlagUndoBufferSize,
				FlagDevelopmentMode,
				FlagFinalBlocksOnly,
				FlagInfiniteRetry,
				FlagIrreversibleOnly,
				FlagSkipPackageValidation,
				FlagExtraHeaders,
				FlagAPIKeyEnvvar,
				FlagAPITokenEnvvar,
				FlagNoopMode,
			},
		},
		{
			"ignore final block",
			[]FlagIgnored{FlagIgnore(FlagFinalBlocksOnly)},
			[]string{
				FlagParams,
				FlagNetwork,
				FlagInsecure,
				FlagPlaintext,
				FlagUndoBufferSize,
				FlagLiveBlockTimeDelta,
				FlagDevelopmentMode,
				FlagInfiniteRetry,
				FlagSkipPackageValidation,
				FlagExtraHeaders,
				FlagAPIKeyEnvvar,
				FlagAPITokenEnvvar,
				FlagNoopMode,
			},
		},
		{
			"ignore irreversible only",
			[]FlagIgnored{FlagIgnore(FlagIrreversibleOnly)},
			[]string{
				FlagParams,
				FlagNetwork,
				FlagInsecure,
				FlagPlaintext,
				FlagUndoBufferSize,
				FlagLiveBlockTimeDelta,
				FlagDevelopmentMode,
				FlagFinalBlocksOnly,
				FlagInfiniteRetry,
				FlagSkipPackageValidation,
				FlagExtraHeaders,
				FlagAPIKeyEnvvar,
				FlagAPITokenEnvvar,
				FlagNoopMode,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flagSet := pflag.NewFlagSet("", pflag.ContinueOnError)

			AddFlagsToSet(flagSet, tt.ignore...)

			var actualFlags []string
			flagSet.VisitAll(func(f *pflag.Flag) {
				actualFlags = append(actualFlags, f.Name)
			})

			assert.ElementsMatch(t, tt.expectedFlags, actualFlags)
		})
	}
}

func Test_readBlockRange(t *testing.T) {
	errorIs := func(errString string) require.ErrorAssertionFunc {
		return func(tt require.TestingT, err error, i ...interface{}) {
			require.EqualError(tt, err, errString, i...)
		}
	}

	openRange := func(start uint64) *bstream.Range {
		return bstream.NewOpenRange(start)
	}

	closedRange := func(start, end uint64) *bstream.Range {
		return bstream.NewRangeExcludingEnd(start, end)
	}

	type args struct {
		moduleStartBlock uint64
		blockRangeArg    string
	}
	tests := []struct {
		name      string
		args      args
		want      *bstream.Range
		assertion require.ErrorAssertionFunc
	}{
		// Single
		{"single empty is full range", args{5, ""}, openRange(5), nil},
		{"single -1 is full range", args{5, "-1"}, openRange(5), nil},
		{"single : is full range", args{5, ":"}, openRange(5), nil},
		{"single is stop block, start inferred", args{5, "11"}, closedRange(5, 11), nil},
		{"single +relative is stop block, start inferred", args{5, "+10"}, closedRange(5, 15), nil},

		{"range start, stop", args{5, "10:12"}, closedRange(10, 12), nil},
		{"range <empty>, stop", args{5, ":12"}, closedRange(5, 12), nil},
		{"range start, <empty>", args{5, "10:"}, openRange(10), nil},

		{"range start+, stop", args{5, "+10:20"}, closedRange(15, 20), nil},
		{"range <empty>, stop+", args{5, ":+10"}, closedRange(5, 15), nil},
		{"range start+, <empty>", args{5, "+10:"}, openRange(15), nil},
		{"range start+, stop+", args{5, "+10:+10"}, closedRange(15, 25), nil},

		{"range start, stop+", args{5, "10:+10"}, closedRange(10, 20), nil},

		{"error invalid range, equal", args{0, "10:10"}, nil, errorIs("invalid range: start block 10 is equal or above stop block 10 (exclusive)")},
		{"error invalid range, over", args{0, "11:10"}, nil, errorIs("invalid range: start block 11 is equal or above stop block 10 (exclusive)")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			module := &pbsubstreams.Module{
				InitialBlock: tt.args.moduleStartBlock,
			}

			got, err := ReadBlockRange(module, tt.args.blockRangeArg)

			if tt.assertion == nil {
				tt.assertion = require.NoError
			}

			tt.assertion(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
