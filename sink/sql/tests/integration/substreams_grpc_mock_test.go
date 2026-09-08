package tests

import (
	"fmt"
	"iter"
	"sync"

	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// MessageResult represents a single result from the message sequence.
// The semantics are:
// - Response: nil, Error: nil => EOF of stream
// - Response: non-nil, Error: nil => Send message to stream
// - Response: <any>, Error: non-nil => Close stream with error
type MessageBucket struct {
	Responses        []*pbsubstreamsrpc.Response
	EndOfStreamError error
}

func (b *MessageBucket) iterator() iter.Seq2[*pbsubstreamsrpc.Response, error] {
	return func(yield func(*pbsubstreamsrpc.Response, error) bool) {
		for _, response := range b.Responses {
			if !yield(response, nil) {
				return
			}
		}

		yield(nil, b.EndOfStreamError)
	}
}

// FakeStreamServer implements pbsubstreamsrpc.StreamServer for testing
// It supports buckets of message sequences, where each call to Blocks processes the next bucket.
type FakeStreamServer struct {
	pbsubstreamsrpc.UnimplementedStreamServer
	buckets       []MessageBucket
	currentBucket int
	mu            sync.Mutex
}

func newFakeStreamServer(pattern []any) *FakeStreamServer {
	buckets := []MessageBucket{}
	currentBucket := []*pbsubstreamsrpc.Response{}

	rollBucket := func(endOfStreamError error) {
		if len(currentBucket) > 0 {
			buckets = append(buckets, MessageBucket{
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

func (s *FakeStreamServer) nextBucket() *MessageBucket {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.currentBucket >= len(s.buckets) {
		return nil
	}

	bucket := &s.buckets[s.currentBucket]
	s.currentBucket++

	return bucket
}
