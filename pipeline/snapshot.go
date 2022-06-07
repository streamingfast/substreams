package pipeline

import (
	"fmt"

	"github.com/streamingfast/substreams"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func (p *Pipeline) sendSnapshots(snapshotModules []string, respFunc func(resp *pbsubstreams.Response) error) error {
	for _, modName := range snapshotModules {
		store, found := p.storeMap[modName]
		if !found {
			return fmt.Errorf("store %q not found", modName)
		}

		send := func(count uint64, total uint64, deltas []*pbsubstreams.StoreDelta) {
			data := &pbsubstreams.InitialSnapshotData{
				ModuleName: store.Name,
				Deltas: &pbsubstreams.StoreDeltas{
					Deltas: deltas,
				},
				SentKeys:  count,
				TotalKeys: total,
			}
			respFunc(substreams.NewSnapshotData(data))
		}

		var count uint64
		total := uint64(len(store.KV))
		var accum []*pbsubstreams.StoreDelta
		for k, v := range store.KV {
			count++

			accum = append(accum, &pbsubstreams.StoreDelta{
				Operation: pbsubstreams.StoreDelta_CREATE,
				Key:       k,
				NewValue:  v,
			})

			if count%100 == 0 {
				send(count, total, accum)
				accum = nil
			}
		}
		if len(accum) != 0 {
			send(count, total, accum)
		}
	}

	respFunc(substreams.NewSnapshotComplete())

	return nil
}
