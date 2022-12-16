package marshaller

import (
	"fmt"

	pbsubstreams "github.com/streamingfast/substreams/storage/store/marshaller/pb"
	"google.golang.org/protobuf/proto"
)

type Proto struct{}

func (p *Proto) Unmarshal(in []byte) (*StoreData, uint64, error) {
	stateData := &pbsubstreams.StoreData{}
	if err := proto.Unmarshal(in, stateData); err != nil {
		return nil, 0, fmt.Errorf("unmarshal store: %w", err)
	}
	return &StoreData{
		Kv:             stateData.GetKv(),
		DeletePrefixes: stateData.GetDeletePrefixes(),
	}, 0, nil
}

func (p *Proto) Marshal(data *StoreData) ([]byte, error) {
	stateData := &pbsubstreams.StoreData{
		Kv:             data.Kv,
		DeletePrefixes: data.DeletePrefixes,
	}
	return proto.Marshal(stateData)
}
