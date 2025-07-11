package sink

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
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
				FlagEndpoint,
				FlagStartBlock,
				FlagStopBlock,
				FlagCursor,
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
				FlagProtoPath,
				FlagProtoDescriptorSet,
			},
		},
		{
			"ignore one",
			[]FlagIgnored{FlagIgnore(FlagInsecure)},
			[]string{
				FlagEndpoint,
				FlagStartBlock,
				FlagStopBlock,
				FlagCursor,
				FlagParams,
				FlagNetwork,
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
				FlagProtoPath,
				FlagProtoDescriptorSet,
			},
		},
		{
			"ignore one multiple",
			[]FlagIgnored{FlagIgnore(FlagInsecure, FlagPlaintext)},
			[]string{
				FlagEndpoint,
				FlagStartBlock,
				FlagStopBlock,
				FlagCursor,
				FlagParams,
				FlagNetwork,
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
				FlagProtoPath,
				FlagProtoDescriptorSet,
			},
		},
		{
			"ignore multiple",
			[]FlagIgnored{FlagIgnore(FlagInsecure), FlagIgnore(FlagPlaintext)},
			[]string{
				FlagEndpoint,
				FlagStartBlock,
				FlagStopBlock,
				FlagCursor,
				FlagParams,
				FlagNetwork,
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
				FlagProtoPath,
				FlagProtoDescriptorSet,
			},
		},
		{
			"ignore mixed",
			[]FlagIgnored{FlagIgnore(FlagInsecure), FlagIgnore(FlagPlaintext, FlagLiveBlockTimeDelta)},
			[]string{
				FlagEndpoint,
				FlagStartBlock,
				FlagStopBlock,
				FlagCursor,
				FlagParams,
				FlagNetwork,
				FlagUndoBufferSize,
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
		{
			"ignore final block",
			[]FlagIgnored{FlagIgnore(FlagFinalBlocksOnly)},
			[]string{
				FlagEndpoint,
				FlagStartBlock,
				FlagStopBlock,
				FlagCursor,
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
