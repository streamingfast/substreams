package pipeline

import (
	"fmt"

	"github.com/streamingfast/substreams/storage/store"

	"github.com/streamingfast/substreams"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
)

func (p *Pipeline) sendSnapshots(storeMap store.Map, snapshotModules []string) error {
	if len(snapshotModules) == 0 {
		return nil
	}

	for _, modName := range snapshotModules {
		store, found := storeMap.Get(modName)
		if !found {
			return fmt.Errorf("store %q not found", modName)
		}

		send := func(count uint64, total uint64, deltas []*pbsubstreamsrpc.StoreDelta) {
			data := &pbsubstreamsrpc.InitialSnapshotData{
				ModuleName: modName,
				Deltas:     deltas,
				SentKeys:   count,
				TotalKeys:  total,
			}
			p.respFunc(substreams.NewSnapshotData(data))
		}

		var count uint64
		total := store.Length()
		var accum []*pbsubstreamsrpc.StoreDelta

		store.Iter(func(k string, v []byte) error {
			count++
			accum = append(accum, &pbsubstreamsrpc.StoreDelta{
				Operation: pbsubstreamsrpc.StoreDelta_CREATE,
				Key:       k,
				NewValue:  v,
			})

			if count%100 == 0 {
				send(count, total, accum)
				accum = nil
			}
			return nil
		})

		if len(accum) != 0 {
			send(count, total, accum)
		}
	}

	p.respFunc(substreams.NewSnapshotComplete())

	return nil
}
