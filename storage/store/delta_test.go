package store

import (
	"testing"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/assert"
)

var baseStoreConfig = &Config{
	totalSizeLimit: 9999,
}

func TestApplyDelta(t *testing.T) {
	tests := []struct {
		name       string
		deltas     []*pbsubstreams.StoreDelta
		expectedKV map[string][]byte
	}{
		{
			name: "creates",
			deltas: []*pbsubstreams.StoreDelta{
				{
					Operation: pbsubstreams.StoreDelta_CREATE,
					Key:       "k1",
					NewValue:  []byte("v1"),
				},
				{
					Operation: pbsubstreams.StoreDelta_CREATE,
					Key:       "k2",
					NewValue:  []byte("v2"),
				},
			},
			expectedKV: map[string][]byte{
				"k1": []byte("v1"),
				"k2": []byte("v2"),
			},
		},
		{
			name: "update",
			deltas: []*pbsubstreams.StoreDelta{
				{
					Operation: pbsubstreams.StoreDelta_CREATE,
					Key:       "k1",
					NewValue:  []byte("v1"),
				},
				{
					Operation: pbsubstreams.StoreDelta_UPDATE,
					Key:       "k1",
					OldValue:  []byte("v1"),
					NewValue:  []byte("v2"),
				},
			},
			expectedKV: map[string][]byte{
				"k1": []byte("v2"),
			},
		},
		{
			name: "delete",
			deltas: []*pbsubstreams.StoreDelta{
				{
					Operation: pbsubstreams.StoreDelta_CREATE,
					Key:       "k1",
					NewValue:  []byte("v1"),
				},
				{
					Operation: pbsubstreams.StoreDelta_CREATE,
					Key:       "k2",
					NewValue:  []byte("v2"),
				},
				{
					Operation: pbsubstreams.StoreDelta_DELETE,
					Key:       "k1",
					OldValue:  []byte("v1"),
				},
			},
			expectedKV: map[string][]byte{
				"k2": []byte("v2"),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := &baseStore{
				Config: baseStoreConfig,
				kvImpl: newMemoryKVImpl(),
			}
			for _, delta := range test.deltas {
				s.ApplyDelta(delta)
			}
			snapshot, err := saveToMap(s.kvImpl.Save())
			assert.NoError(t, err)
			assert.Equal(t, test.expectedKV, snapshot)
		})
	}
}

func TestApplyDeltasReverse(t *testing.T) {
	initialKV := map[string][]byte{
		"k2": []byte("k2-original"),
		"k3": []byte("k3-value"),
		"k4": []byte("k4-value"),
	}

	s := &baseStore{
		Config:                  baseStoreConfig,
		kvImpl:                  newMemoryKVImpl(),
		recentlyDeletedPrefixes: make(DeletedPrefixes),
	}
	var initialSize uint64
	for k, v := range initialKV {
		if err := s.kvImpl.Set(k, v); err != nil {
			t.Fatal(err)
		}
		initialSize += uint64(len(k) + len(v))
	}
	s.totalSizeBytes = initialSize

	deltas := []*pbsubstreams.StoreDelta{
		{
			Operation: pbsubstreams.StoreDelta_CREATE,
			Key:       "k1",
			NewValue:  []byte("k1-created"),
		},
		{
			Operation: pbsubstreams.StoreDelta_UPDATE,
			Key:       "k2",
			OldValue:  []byte("k2-original"),
			NewValue:  []byte("k2-updated"),
		},
		{
			Operation: pbsubstreams.StoreDelta_DELETE,
			Key:       "k3",
			OldValue:  []byte("k3-value"),
		},
		{
			Operation: pbsubstreams.StoreDelta_DELETE,
			Key:       "k4",
			OldValue:  []byte("k4-value"),
		},
	}

	for _, delta := range deltas {
		s.ApplyDelta(delta)
	}

	s.ApplyDeltasReverse(deltas)

	snapshot, err := saveToMap(s.kvImpl.Save())
	assert.NoError(t, err)
	assert.Equal(t, map[string][]byte{
		"k2": []byte("k2-original"),
		"k3": []byte("k3-value"),
		"k4": []byte("k4-value"),
	}, snapshot, "reversing all deltas must restore the original state")
	assert.Equal(t, initialSize, s.totalSizeBytes)
}

func Test_baseStore_SetDeltas(t *testing.T) {
	kvImpl := newMemoryKVImpl()
	kvImpl.Load(mapToIter(map[string][]byte{"A": []byte("a")}))
	
	s := baseStore{
		Config:         baseStoreConfig,
		kvImpl:         kvImpl,
		totalSizeBytes: 2,
	}
	s.SetDeltas([]*pbsubstreams.StoreDelta{
		{
			Key:       "A",
			Operation: pbsubstreams.StoreDelta_DELETE,
			OldValue:  []byte("a"),
		},
		{
			Key:       "B",
			Operation: pbsubstreams.StoreDelta_CREATE,
			NewValue:  []byte("b"),
		},
		{
			Key:       "C",
			Operation: pbsubstreams.StoreDelta_CREATE,
			NewValue:  []byte("c"),
		},
		{
			Key:       "C",
			Operation: pbsubstreams.StoreDelta_UPDATE,
			OldValue:  []byte("c"),
			NewValue:  []byte("d"),
		},
	})
	
	snapshot, err := saveToMap(s.kvImpl.Save())
	assert.NoError(t, err)
	assert.Len(t, snapshot, 2)
	assert.Equal(t, "b", string(snapshot["B"]))
	assert.Equal(t, "d", string(snapshot["C"]))
	assert.Equal(t, uint64(4), s.totalSizeBytes)
	assert.Len(t, s.deltas, 4)
}
