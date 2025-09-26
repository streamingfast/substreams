package sink

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
)

func TestAddFlagsToSet(t *testing.T) {
	tests := []struct {
		name          string
		ignore        []FlagInclusionExclusion
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
				FlagMaxRetries,
				FlagSkipPackageValidation,
				FlagExtraHeaders,
				FlagAPIKeyEnvvar,
				FlagAPITokenEnvvar,
				FlagNoopMode,
				FlagProtoPath,
				FlagProtoDescriptorSet,
				FlagPrometheusAddr,
			},
		},
		{
			"ignore one",
			[]FlagInclusionExclusion{FlagExcludeDefault(FlagInsecure)},
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
				FlagMaxRetries,
				FlagSkipPackageValidation,
				FlagExtraHeaders,
				FlagAPIKeyEnvvar,
				FlagAPITokenEnvvar,
				FlagNoopMode,
				FlagProtoPath,
				FlagProtoDescriptorSet,
				FlagPrometheusAddr,
			},
		},
		{
			"ignore one multiple",
			[]FlagInclusionExclusion{FlagExcludeDefault(FlagInsecure, FlagPlaintext)},
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
				FlagMaxRetries,
				FlagSkipPackageValidation,
				FlagExtraHeaders,
				FlagAPIKeyEnvvar,
				FlagAPITokenEnvvar,
				FlagNoopMode,
				FlagProtoPath,
				FlagProtoDescriptorSet,
				FlagPrometheusAddr,
			},
		},
		{
			"ignore multiple",
			[]FlagExcludeDefaultd{FlagExcludeDefault(FlagInsecure), FlagExcludeDefault(FlagPlaintext)},
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
				FlagMaxRetries,
				FlagSkipPackageValidation,
				FlagExtraHeaders,
				FlagAPIKeyEnvvar,
				FlagAPITokenEnvvar,
				FlagNoopMode,
				FlagProtoPath,
				FlagProtoDescriptorSet,
				FlagPrometheusAddr,
			},
		},
		{
			"ignore mixed",
			[]FlagExcludeDefaultd{FlagExcludeDefault(FlagInsecure), FlagExcludeDefault(FlagPlaintext, FlagLiveBlockTimeDelta), FlagExcludeDefault(FlagProtoPath, FlagProtoDescriptorSet)},
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
				FlagMaxRetries,
				FlagSkipPackageValidation,
				FlagExtraHeaders,
				FlagAPIKeyEnvvar,
				FlagAPITokenEnvvar,
				FlagNoopMode,
				FlagPrometheusAddr,
			},
		},
		{
			"ignore final block",
			[]FlagExcludeDefaultd{FlagExcludeDefault(FlagFinalBlocksOnly), FlagExcludeDefault(FlagProtoPath, FlagProtoDescriptorSet)},
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
				FlagMaxRetries,
				FlagSkipPackageValidation,
				FlagExtraHeaders,
				FlagAPIKeyEnvvar,
				FlagAPITokenEnvvar,
				FlagNoopMode,
				FlagPrometheusAddr,
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
