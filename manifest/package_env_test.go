package manifest

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpandManifestVariables_FoundationalStore(t *testing.T) {
	t.Setenv("MANIFEST_TEST_DEPLOYMENT_ID", "abc-123")

	conv := newManifestConverter("test.yaml", ReaderValidation{}, nil)

	manif := &Manifest{
		Modules: []*Module{
			{
				Name: "store_module",
				Inputs: []*Input{
					{FoundationalStore: "$MANIFEST_TEST_DEPLOYMENT_ID"},
					{Source: "sf.ethereum.type.v2.Block"},
				},
			},
		},
	}

	require.NoError(t, conv.expandManifestVariables(manif))
	assert.Equal(t, "abc-123", manif.Modules[0].Inputs[0].FoundationalStore)
	assert.Equal(t, "sf.ethereum.type.v2.Block", manif.Modules[0].Inputs[1].Source)
}

func TestExpandManifestVariables_FoundationalStoreUndefined(t *testing.T) {
	conv := newManifestConverter("test.yaml", ReaderValidation{}, nil)

	manif := &Manifest{
		Modules: []*Module{
			{
				Name: "store_module",
				Inputs: []*Input{
					{FoundationalStore: "$MANIFEST_TEST_UNSET"},
				},
			},
		},
	}

	err := conv.expandManifestVariables(manif)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "store_module")
	assert.Contains(t, err.Error(), "MANIFEST_TEST_UNSET")
}
