package store

import (
	"testing"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestDeleteKey(t *testing.T) {
	initialKV := map[string][]byte{
		"a":  []byte("1"),
		"ab": []byte("2"),
		"b":  []byte("3"),
	}

	tests := []struct {
		name           string
		ops            func(s *baseStore)
		expectedKV     map[string][]byte
		expectedDeltas []*pbsubstreams.StoreDelta
	}{
		{
			name: "deletes only the exact key",
			ops:  func(s *baseStore) { s.DeleteKey(1, "a") },
			expectedKV: map[string][]byte{
				"ab": []byte("2"),
				"b":  []byte("3"),
			},
			expectedDeltas: []*pbsubstreams.StoreDelta{
				{Operation: pbsubstreams.StoreDelta_DELETE, Ordinal: 1, Key: "a", OldValue: []byte("1")},
			},
		},
		{
			name:       "absent key emits no delta",
			ops:        func(s *baseStore) { s.DeleteKey(1, "c") },
			expectedKV: initialKV,
		},
		{
			name: "set then delete in one block",
			ops: func(s *baseStore) {
				s.Set(1, "c", "4")
				s.DeleteKey(2, "c")
			},
			expectedKV: initialKV,
			expectedDeltas: []*pbsubstreams.StoreDelta{
				{Operation: pbsubstreams.StoreDelta_CREATE, Ordinal: 1, Key: "c", NewValue: []byte("4")},
				{Operation: pbsubstreams.StoreDelta_DELETE, Ordinal: 2, Key: "c", OldValue: []byte("4")},
			},
		},
		{
			name: "delete then set in one block",
			ops: func(s *baseStore) {
				s.DeleteKey(1, "a")
				s.Set(2, "a", "5")
			},
			expectedKV: map[string][]byte{
				"a":  []byte("5"),
				"ab": []byte("2"),
				"b":  []byte("3"),
			},
			expectedDeltas: []*pbsubstreams.StoreDelta{
				{Operation: pbsubstreams.StoreDelta_DELETE, Ordinal: 1, Key: "a", OldValue: []byte("1")},
				{Operation: pbsubstreams.StoreDelta_CREATE, Ordinal: 2, Key: "a", NewValue: []byte("5")},
			},
		},
		{
			name: "ordinals, not call order, decide",
			ops: func(s *baseStore) {
				s.Set(2, "a", "5")
				s.DeleteKey(1, "a")
			},
			expectedKV: map[string][]byte{
				"a":  []byte("5"),
				"ab": []byte("2"),
				"b":  []byte("3"),
			},
			expectedDeltas: []*pbsubstreams.StoreDelta{
				{Operation: pbsubstreams.StoreDelta_DELETE, Ordinal: 1, Key: "a", OldValue: []byte("1")},
				{Operation: pbsubstreams.StoreDelta_CREATE, Ordinal: 2, Key: "a", NewValue: []byte("5")},
			},
		},
	}

	for _, backend := range []string{"mmap", "memory"} {
		t.Run(backend, func(t *testing.T) {
			t.Setenv("SUBSTREAMS_STORE_BACKEND", backend)

			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					s := createTestStore(t, "delete_key", pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, "")
					defer s.Close()

					for k, v := range initialKV {
						s.Set(0, k, string(v))
					}
					require.NoError(t, s.Flush())
					s.Reset()

					test.ops(s)
					require.NoError(t, s.Flush())

					gotKV, err := saveToMap(s.kvImpl.Save())
					require.NoError(t, err)
					assert.Equal(t, test.expectedKV, gotKV)

					gotDeltas := s.GetDeltas()
					require.Len(t, gotDeltas, len(test.expectedDeltas))
					for i, expected := range test.expectedDeltas {
						assert.Truef(t, proto.Equal(expected, gotDeltas[i]), "delta %d: expected %v, got %v", i, expected, gotDeltas[i])
					}
				})
			}
		})
	}
}
