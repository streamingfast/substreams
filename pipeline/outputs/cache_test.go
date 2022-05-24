package outputs

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/require"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"google.golang.org/protobuf/proto"
)

func TestFoo(t *testing.T) {
	data, err := base64.StdEncoding.DecodeString("CnsIARABGjB0b2tlbjoweDg0ZWIwN2IxNGFiMTE2NDlkMDg1YWVmNjFlMmUwY2ZjZTQ4Yzk0ODgqQwoqMHg4NGViMDdiMTRhYjExNjQ5ZDA4NWFlZjYxZTJlMGNmY2U0OGM5NDg4Eg9EcmFjaG1hIERpZ2l0YWwaAkREIBI=")
	//data, err := base64.StdEncoding.DecodeString("CnoKeAgBEAEaMHRva2VuOjB4Y2ZmOGZkYTM0NzI5Y2YzMDVkNzcwMjJiMzQ1Yzc3YjA2MGRlYzlhYypACioweGNmZjhmZGEzNDcyOWNmMzA1ZDc3MDIyYjM0NWM3N2IwNjBkZWM5YWMSCE1PT05ET0dFGghNT09ORE9HRQp9CnsIARABGjB0b2tlbjoweDg0ZWIwN2IxNGFiMTE2NDlkMDg1YWVmNjFlMmUwY2ZjZTQ4Yzk0ODgqQwoqMHg4NGViMDdiMTRhYjExNjQ5ZDA4NWFlZjYxZTJlMGNmY2U0OGM5NDg4Eg9EcmFjaG1hIERpZ2l0YWwaAkREIBI=")
	require.NoError(t, err)
	deltas := &pbsubstreams.StoreDeltas{}
	err = proto.Unmarshal(data, deltas)
	require.NoError(t, err)
}
