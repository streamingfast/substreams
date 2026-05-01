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
		s, found := storeMap.Get(modName)
		if !found {
			return fmt.Errorf("store %q not found", modName)
		}

		// BadgerBackedStore state lives in the foundational store service, not in the
		// local kv cache. Enumerating all keys would require a full scan over gRPC which
		// is not yet implemented. Skip snapshot generation and send an empty snapshot so
		// the client receives a well-formed response rather than a silently partial one.
		if _, isBadger := s.(*store.BadgerBackedStore); isBadger {
			p.respFunc(substreams.NewSnapshotData(&pbsubstreamsrpc.InitialSnapshotData{
				ModuleName: modName,
				SentKeys:   0,
				TotalKeys:  0,
			}))
			continue
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
		total := s.Length()
		var accum []*pbsubstreamsrpc.StoreDelta

		s.Iter(func(k string, v []byte) error {
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
