package marshaller

import (
	"fmt"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"google.golang.org/protobuf/proto"
)

type Proto struct{}

func (p Proto) Unmarshal(in []byte) (*StoreData, error) {
	stateData := &pbsubstreams.StoreData{}
	if err := proto.Unmarshal(in, stateData); err != nil {
		return nil, fmt.Errorf("unmarshal store: %w", err)
	}
	return &StoreData{
		Kv:             stateData.GetKv(),
		DeletePrefixes: stateData.GetDeletePrefixes(),
	}, nil
}

func (p Proto) Marshal(data *StoreData) ([]byte, error) {
	stateData := &pbsubstreams.StoreData{
		Kv:             data.Kv,
		DeletePrefixes: data.DeletePrefixes,
	}
	return proto.Marshal(stateData)
}
