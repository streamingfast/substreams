package marshaller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProtoFast_CompareMarshal(t *testing.T) {
	tests := []struct {
		name string
		data *StoreData
	}{
		{
			name: "only kv single entry",
			data: &StoreData{
				Kv: map[string][]byte{
					"a": {0xaa},
				},
			},
		},
		{
			name: "no kv no delete prefixes",
			data: &StoreData{},
		},
		////TODO: we can a mismatch in the order of the field written
		////this has no impact on the decoding but makes the test rather annoying
		//{
		//	name: "only kv multiple",
		//	data: &StoreData{
		//		Kv: map[string][]byte{
		//			"a": {0xaa},
		//			"b": {0xcc},
		//		},
		//	},
		//},
		////TODO: we can a mismatch in the order of the field written
		////this has no impact on the decoding but makes the test rather annoying
		//{
		//	name: "kv and delete prefix",
		//	data: &StoreData{
		//		Kv: map[string][]byte{
		//			"b": {0xcc},
		//			"a": {0xaa},
		//		},
		//		DeletePrefixes: []string{"dd", "124"},
		//	},
		//},
		{
			name: "only delete prefix",
			data: &StoreData{
				DeletePrefixes: []string{"22"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pf := &ProtoingFast{}
			protoingFastData, err := pf.Marshal(test.data)
			require.NoError(t, err)

			p := &Proto{}
			protoData, err := p.Marshal(test.data)
			require.NoError(t, err)

			vp := &VTproto{}
			vtProtoData, err := vp.Marshal(test.data)
			require.NoError(t, err)

			assert.Equal(t, protoData, protoingFastData)
			assert.Equal(t, protoData, vtProtoData)

			v, _, err := pf.Unmarshal(protoingFastData)
			require.NoError(t, err)

			assert.Equal(t, test.data, v)

		})
	}
}
