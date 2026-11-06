package grpc

import (
	"context"
	"testing"

	pbsquasher "github.com/streamingfast/substreams/pb/sf/substreams/squasher/v1"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/squash"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gogrpc "google.golang.org/grpc"
)

type stubSquasherClient struct {
	calls   int
	lastReq *pbsquasher.SquashRequest
}

func (s *stubSquasherClient) Squash(_ context.Context, in *pbsquasher.SquashRequest, _ ...gogrpc.CallOption) (*pbsquasher.SquashResponse, error) {
	s.calls++
	s.lastReq = in
	return &pbsquasher.SquashResponse{
		ExclusiveEndBlock:    30,
		StoreSizeBytes:       7,
		LoadedExistingFullKv: false,
	}, nil
}

func TestClientMapsRangesToOneRPC(t *testing.T) {
	stub := &stubSquasherClient{}
	c := &Client{grpc: stub}

	resp, err := c.Squash(context.Background(), squash.Request{
		ModuleName:         "mod",
		ModuleHash:         "hash",
		ModuleInitialBlock: 0,
		UpdatePolicy:       pbsubstreams.Module_KindStore_UPDATE_POLICY_SET,
		ValueType:          "string",
		StateStore:         "file://tmp",
		Ranges: []squash.Range{
			{StartBlock: 10, ExclusiveEndBlock: 20},
			{StartBlock: 20, ExclusiveEndBlock: 30},
		},
		StoreSizeLimit: 99,
	})
	require.NoError(t, err)
	require.Equal(t, 1, stub.calls, "the whole run must be one Squash RPC")
	require.NotNil(t, stub.lastReq)
	assert.Zero(t, stub.lastReq.StartBlock)
	assert.Zero(t, stub.lastReq.ExclusiveEndBlock)
	require.Len(t, stub.lastReq.Ranges, 2)
	assert.Equal(t, uint64(10), stub.lastReq.Ranges[0].StartBlock)
	assert.Equal(t, uint64(20), stub.lastReq.Ranges[0].ExclusiveEndBlock)
	assert.Equal(t, uint64(20), stub.lastReq.Ranges[1].StartBlock)
	assert.Equal(t, uint64(30), stub.lastReq.Ranges[1].ExclusiveEndBlock)
	assert.Equal(t, uint64(99), stub.lastReq.StoreSizeLimit)
	assert.Equal(t, uint64(30), resp.ExclusiveEndBlock)
	assert.Equal(t, uint64(7), resp.StoreSizeBytes)
}
