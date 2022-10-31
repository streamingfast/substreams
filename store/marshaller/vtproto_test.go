package marshaller

import (
	"encoding/hex"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestVTproto_Marshal(t *testing.T) {
	sd := &StoreData{
		Kv: map[string][]byte{
			"b": {0xcc},
			"a": {0xaa},
		},
		DeletePrefixes: []string{"dd", "124"},
	}

	vt := &VTproto{}
	data, err := vt.Marshal(sd)
	require.NoError(t, err)
	fmt.Println(hex.EncodeToString(data))
	v, err := vt.Unmarshal(data)
	require.NoError(t, err)
	assert.Equal(t, sd, v)
	fmt.Println(hex.EncodeToString(v.Kv["a"]))
	data[7] = 0xbb
	fmt.Println(hex.EncodeToString(v.Kv["a"]))

}
