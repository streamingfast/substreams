// Package sinktest provides a fake Substreams stream server and builders for its responses, to test
// sinks end to end without a real Substreams endpoint.
package sinktest

import (
	"fmt"
	"iter"
	"net"
	"sync"
	"testing"

	"github.com/streamingfast/substreams/client"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// SetupFakeSubstreamsServer starts a [FakeStreamServer] serving pattern (see [NewFakeStreamServer]),
// stopped when the test ends, and returns the client config to reach it.
func SetupFakeSubstreamsServer(t testing.TB, pattern ...any) *client.SubstreamsClientConfig {
	t.Helper()

	// Bind to 127.0.0.1 explicitly to avoid IPv6 issues. Binding to `:0` results
	// in `[::]:port` which causes gRPC connection failures in some environments
	// (e.g., AI sandboxes with proxy configurations). Using 127.0.0.1 works
	// consistently in both local development and sandboxed environments.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := grpc.NewServer()
	pbsubstreamsrpc.RegisterStreamServer(server, NewFakeStreamServer(pattern))

	go func() {
		if err := server.Serve(listener); err != nil {
			t.Logf("gRPC server error: %v", err)
		}
	}()

	t.Cleanup(server.Stop)

	return client.NewSubstreamsClientConfig(client.SubstreamsClientConfigOptions{
		Endpoint:  listener.Addr().String(),
		AuthToken: "",
		AuthType:  client.None,
		Insecure:  false,
		PlainText: true,
		Agent:     "sink-test",
	})
}

// messageBucket is the sequence of responses one Blocks call sends, ended by EndOfStreamError (nil for
// a clean end of stream).
type messageBucket struct {
	Responses        []*pbsubstreamsrpc.Response
	EndOfStreamError error
}

func (b *messageBucket) iterator() iter.Seq2[*pbsubstreamsrpc.Response, error] {
	return func(yield func(*pbsubstreamsrpc.Response, error) bool) {
		for _, response := range b.Responses {
			if !yield(response, nil) {
				return
			}
		}

		yield(nil, b.EndOfStreamError)
	}
}

// FakeStreamServer implements pbsubstreamsrpc.StreamServer for testing.
// It supports buckets of message sequences, where each call to Blocks processes the next bucket.
type FakeStreamServer struct {
	pbsubstreamsrpc.UnimplementedStreamServer
	buckets       []messageBucket
	currentBucket int
	mu            sync.Mutex
}

// NewFakeStreamServer splits pattern into one bucket per Blocks call:
//   - a *pbsubstreamsrpc.Response is sent on the stream,
//   - an error ends the bucket, the stream closing with that error,
//   - nil ends the bucket, the stream closing cleanly (end of stream).
//
// Once the buckets are exhausted, Blocks fails with a fatal error.
func NewFakeStreamServer(pattern []any) *FakeStreamServer {
	buckets := []messageBucket{}
	currentBucket := []*pbsubstreamsrpc.Response{}

	rollBucket := func(endOfStreamError error) {
		if len(currentBucket) > 0 {
			buckets = append(buckets, messageBucket{
				Responses:        currentBucket,
				EndOfStreamError: endOfStreamError,
			})
			currentBucket = []*pbsubstreamsrpc.Response{}
		}
	}

	for _, item := range pattern {
		switch v := item.(type) {
		case *pbsubstreamsrpc.Response:
			currentBucket = append(currentBucket, v)
		case error:
			// If we encounter an error, it indicates the end of the current bucket
			rollBucket(v)
		default:
			// If the item is nil, it's an of stream signal and mark a new bucket
			if v == nil {
				rollBucket(nil)
				continue
			}
		}
	}

	// Roll the last bucket if it has items
	rollBucket(nil)

	return &FakeStreamServer{
		buckets:       buckets,
		currentBucket: 0,
	}
}

// Blocks implements the Stream RPC method
// It uses the iterator to get messages and handles them according to the iterator semantics.
func (s *FakeStreamServer) Blocks(req *pbsubstreamsrpc.Request, stream pbsubstreamsrpc.Stream_BlocksServer) error {
	bucket := s.nextBucket()
	if bucket == nil {
		// We use Unauthenticated because it's a fatal error in the sinker which will stop processing
		return status.Error(codes.Unauthenticated, "test mock data exhausted: no more message buckets available")
	}

	// First send SessionInit message
	sessionInit := &pbsubstreamsrpc.Response{
		Message: &pbsubstreamsrpc.Response_Session{
			Session: &pbsubstreamsrpc.SessionInit{
				TraceId:            "test-trace-id",
				ResolvedStartBlock: uint64(req.StartBlockNum),
				LinearHandoffBlock: uint64(req.StartBlockNum),
				MaxParallelWorkers: 1,
			},
		},
	}

	if err := stream.Send(sessionInit); err != nil {
		return fmt.Errorf("failed to send session init: %w", err)
	}

	for response, err := range bucket.iterator() {
		if err != nil {
			return err
		}

		if response == nil {
			// This indicates the end of the stream, we can return nil to indicate EOF
			return nil
		}

		if err := stream.Send(response); err != nil {
			return fmt.Errorf("failed to send message: %w", err)
		}
	}

	return nil
}

func (s *FakeStreamServer) nextBucket() *messageBucket {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.currentBucket >= len(s.buckets) {
		return nil
	}

	bucket := &s.buckets[s.currentBucket]
	s.currentBucket++

	return bucket
}
