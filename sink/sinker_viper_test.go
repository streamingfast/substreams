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
				FlagForceV2,
			},
		},
		{
			"ignore one",
			[]FlagInclusionExclusion{FlagExcludeDefault(FlagInsecure)},
			[]string{
				FlagEndpoint,
				FlagStartBlock,
				FlagStopBlock,
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
				FlagForceV2,
			},
		},
		{
			"ignore one multiple",
			[]FlagInclusionExclusion{FlagExcludeDefault(FlagInsecure, FlagPlaintext)},
			[]string{
				FlagEndpoint,
				FlagStartBlock,
				FlagStopBlock,
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
				FlagForceV2,
			},
		},
		{
			"ignore multiple",
			[]FlagInclusionExclusion{FlagExcludeDefault(FlagInsecure), FlagExcludeDefault(FlagPlaintext)},
			[]string{
				FlagEndpoint,
				FlagStartBlock,
				FlagStopBlock,
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
				FlagForceV2,
			},
		},
		{
			"ignore mixed",
			[]FlagInclusionExclusion{FlagExcludeDefault(FlagInsecure), FlagExcludeDefault(FlagPlaintext, FlagLiveBlockTimeDelta), FlagExcludeDefault(FlagProtoPath, FlagProtoDescriptorSet)},
			[]string{
				FlagEndpoint,
				FlagStartBlock,
				FlagStopBlock,
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
				FlagForceV2,
			},
		},
		{
			"ignore final block",
			[]FlagInclusionExclusion{FlagExcludeDefault(FlagFinalBlocksOnly), FlagExcludeDefault(FlagProtoPath, FlagProtoDescriptorSet)},
			[]string{
				FlagEndpoint,
				FlagStartBlock,
				FlagStopBlock,
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
				FlagForceV2,
			},
		},
		{
			"ignore final block but include cursor",
			[]FlagInclusionExclusion{
				FlagIncludeOptional(FlagCursor),
				FlagExcludeDefault(FlagFinalBlocksOnly),
				FlagExcludeDefault(FlagProtoPath, FlagProtoDescriptorSet),
			},
			[]string{
				FlagEndpoint,
				FlagStartBlock,
				FlagStopBlock,
				FlagParams,
				FlagNetwork,
				FlagCursor,
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
				FlagForceV2,
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
