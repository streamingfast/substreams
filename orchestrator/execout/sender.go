package execout

import (
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreamsrpcv4 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v4"
)

// dataStream is where the walker and the message buffer send block data.
type dataStream interface {
	BlockScopedData(*pbsubstreamsrpc.BlockScopedData) error
	BlockScopedDatas(*pbsubstreamsrpcv4.BlockScopedDatas) error
}

// asyncStream delivers messages to the underlying stream on its own goroutine, so
// the walker decodes the next batch while the previous one is compressed and
// written. The channel is unbuffered: at most one batch is being built while one
// is being sent. Messages go out in the order they were handed in.
//
// Once a send fails, every later call returns that error and nothing else is
// sent. close waits for the goroutine, so a walker that closes before reporting a
// segment done never lets the linear pipeline send ahead of cached data.
type asyncStream struct {
	out  dataStream
	jobs chan func() error
	done chan struct{}
	err  error // written by run before done is closed, read only after done
}

func newAsyncStream(out dataStream) *asyncStream {
	s := &asyncStream{
		out:  out,
		jobs: make(chan func() error),
		done: make(chan struct{}),
	}
	go s.run()
	return s
}

func (s *asyncStream) run() {
	defer close(s.done)
	for send := range s.jobs {
		if err := send(); err != nil {
			s.err = err
			return
		}
	}
}

func (s *asyncStream) enqueue(send func() error) error {
	select {
	case s.jobs <- send:
		return nil
	case <-s.done:
		return s.err
	}
}

func (s *asyncStream) BlockScopedData(msg *pbsubstreamsrpc.BlockScopedData) error {
	return s.enqueue(func() error { return s.out.BlockScopedData(msg) })
}

func (s *asyncStream) BlockScopedDatas(msg *pbsubstreamsrpcv4.BlockScopedDatas) error {
	return s.enqueue(func() error { return s.out.BlockScopedDatas(msg) })
}

// close stops accepting messages, waits for the ones already handed in to be sent
// and returns the first send error, if any. It must be called exactly once, from
// the goroutine that enqueues.
func (s *asyncStream) close() error {
	close(s.jobs)
	<-s.done
	return s.err
}
