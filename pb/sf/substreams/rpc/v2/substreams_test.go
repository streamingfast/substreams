package pbsubstreamsrpc

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ValidateRequest(t *testing.T) {
	tests := []struct {
		name    string
		request *Request
		expect  error
	}{
		{"correct", TestNewRequest(1, withTestOutputModule("output_mod_1"), withTestMapModule("output_mod_1")), nil},
		{"output module not found", TestNewRequest(1, withTestOutputModule("output_mod_1"), withTestMapModule("output_mod_other")), fmt.Errorf("output module \"output_mod_1\" not found in modules")},
		{"negative start block num", TestNewRequest(-1, withTestOutputModule("output_mod_1"), withTestMapModule("output_mod_1")), nil},
		{"no modules found in request", &Request{StartBlockNum: 1}, fmt.Errorf("no modules found in request")},
		{"store output module is accepted for sub-request", TestNewRequest(1, withTestOutputModule("output_mod_1"), withTestStoreModule("output_mod_1")), fmt.Errorf("output module must be of kind 'map'")},
		{"production mode should fail with debug flag", TestNewRequest(1, withTestOutputModule("output_mod_1"), withTestMapModule("output_mod_1"), withProductionMode(), withDebugSnapshotsModule("output_mod_1")), fmt.Errorf("cannot set 'debug-modules-initial-snapshot' in 'production-mode'")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.request.Validate()
			if test.expect != nil {
				require.Error(t, err)
				assert.Equal(t, err.Error(), test.expect.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}
