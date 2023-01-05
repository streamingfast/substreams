package pbsubstreams

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_ValidateRequest(t *testing.T) {
	tests := []struct {
		name         string
		request      *Request
		isSubrequest bool
		expect       error
	}{
		{"negative start block num", TestNewRequest(-1), false, fmt.Errorf("negative start block -1 is not accepted")},
		{"no modules found in request", &Request{StartBlockNum: 1}, false, fmt.Errorf("no modules found in request")},
		{"multiple output modules is not accepted", TestNewRequest(1, withTestOutputModules([][]string{{"output_mod_1", "store"}, {"output_mod_1", "kind"}}, true)), false, fmt.Errorf("multiple output modules is not accepted")},
		{"store output module is accepted for sub-request", TestNewRequest(1, withTestOutputModule("output_mod_1", "store", false)), true, nil},
		{"production mode should fail with debug flag", TestNewRequest(1), false, fmt.Errorf("to fill")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateRequest(test.request, test.isSubrequest)
			if test.expect != nil {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
