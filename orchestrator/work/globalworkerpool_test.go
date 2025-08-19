package work

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pbworker "github.com/streamingfast/worker-pool-protocol/pb/sf/worker/v1"
	"google.golang.org/grpc"
)

// concurrentBroker used by concurrent test
type concurrentBroker struct {
	mu          sync.Mutex
	counter     int
	returnCalls int
}

func (c *concurrentBroker) BorrowWorker(ctx context.Context, req *pbworker.BorrowWorkerRequest, _ ...grpc.CallOption) (*pbworker.BorrowWorkerResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.counter++
	key := c.counter
	return &pbworker.BorrowWorkerResponse{WorkerKey: fmt.Sprintf("kb-%d", key), Status: pbworker.BorrowWorkerResponse_borrowed}, nil
}
func (c *concurrentBroker) ReturnWorker(ctx context.Context, req *pbworker.ReturnWorkerRequest, _ ...grpc.CallOption) (*pbworker.ReturnWorkerResponse, error) {
	c.mu.Lock()
	c.returnCalls++
	c.mu.Unlock()
	return &pbworker.ReturnWorkerResponse{}, nil
}
func (c *concurrentBroker) KeepAlive(ctx context.Context, req *pbworker.KeepAliveRequest, _ ...grpc.CallOption) (*pbworker.KeepAliveResponse, error) {
	return &pbworker.KeepAliveResponse{}, nil
}

// sigBroker signals on channel when KeepAlive is called and delegates to fakeBroker
type sigBroker struct {
	*fakeBroker
	ch chan struct{}
}

func (s *sigBroker) KeepAlive(ctx context.Context, req *pbworker.KeepAliveRequest, _ ...grpc.CallOption) (*pbworker.KeepAliveResponse, error) {
	select {
	case s.ch <- struct{}{}:
	default:
	}
	return s.fakeBroker.KeepAlive(ctx, req)
}

// fakeBroker implements WorkerBroker for tests and records calls
type fakeBroker struct {
	borrowResp     *pbworker.BorrowWorkerResponse
	borrowErr      error
	returnCalls    int
	keepAliveCalls int
}

func (f *fakeBroker) BorrowWorker(ctx context.Context, req *pbworker.BorrowWorkerRequest, _ ...grpc.CallOption) (*pbworker.BorrowWorkerResponse, error) {
	return f.borrowResp, f.borrowErr
}
func (f *fakeBroker) ReturnWorker(ctx context.Context, req *pbworker.ReturnWorkerRequest, _ ...grpc.CallOption) (*pbworker.ReturnWorkerResponse, error) {
	f.returnCalls++
	return &pbworker.ReturnWorkerResponse{}, nil
}
func (f *fakeBroker) KeepAlive(ctx context.Context, req *pbworker.KeepAliveRequest, _ ...grpc.CallOption) (*pbworker.KeepAliveResponse, error) {
	f.keepAliveCalls++
	return &pbworker.KeepAliveResponse{}, nil
}

// match the WorkerBroker signature with grpc.CallOption type for compile compatibility
// the above methods use ...interface{} to avoid importing grpc in test file.

func TestGlobalWorkerPool_RampUpExhaustion(t *testing.T) {
	old := rampupTime
	defer func() { rampupTime = old }()
	// keep ramp-up long so the pool remains in ramping state during test
	rampupTime = time.Hour

	broker := &fakeBroker{borrowResp: &pbworker.BorrowWorkerResponse{WorkerKey: "k1", Status: pbworker.BorrowWorkerResponse_borrowed}}
	// create pool; cast a nil client factory to the expected type via interface conversion
	pool := NewGlobalWorkerPool(context.Background(), "u", "a", "t", 2, broker, nil, time.Second)

	if !pool.RampingUp() {
		t.Fatalf("expected pool to be ramping up")
	}

	w1, err := pool.Borrow(context.Background())
	if err != nil {
		t.Fatalf("expected first borrow to succeed, got err: %v", err)
	}
	if w1 == nil {
		t.Fatalf("expected worker, got nil")
	}

	// second borrow while rampingUp and first worker served should be rejected with ramp-up error
	_, err = pool.Borrow(context.Background())
	if err != ErrorResourceExhaustedRampUp {
		t.Fatalf("expected ramp-up exhaustion error, got: %v", err)
	}

	// return the worker to ensure keepalive goroutine is stopped
	pool.Return(context.Background(), w1)
}

func TestGlobalWorkerPool_BorrowExhaustionFromBroker(t *testing.T) {
	broker := &fakeBroker{borrowResp: &pbworker.BorrowWorkerResponse{WorkerKey: "", Status: pbworker.BorrowWorkerResponse_resource_exhausted}}
	pool := NewGlobalWorkerPool(context.Background(), "u", "a", "t", 2, broker, nil, time.Second)

	_, err := pool.Borrow(context.Background())
	if err != ErrorResourceExhausted {
		t.Fatalf("expected resource exhausted, got: %v", err)
	}
}

func TestGlobalWorkerPool_ReturnCallsBroker(t *testing.T) {
	broker := &fakeBroker{borrowResp: &pbworker.BorrowWorkerResponse{WorkerKey: "xk", Status: pbworker.BorrowWorkerResponse_borrowed}}
	pool := NewGlobalWorkerPool(context.Background(), "u", "a", "t", 2, broker, nil, 1*time.Millisecond)

	w, err := pool.Borrow(context.Background())
	if err != nil {
		t.Fatalf("borrow failed: %v", err)
	}

	pool.Return(context.Background(), w)

	if broker.returnCalls != 1 {
		t.Fatalf("expected broker.ReturnWorker to be called once, got %d", broker.returnCalls)
	}

	if atomic.LoadInt32(&pool.rampUpWorkerServed) != 0 {
		t.Fatalf("expected rampUpWorkerServed to be false after return")
	}
}

func TestGlobalWorkerPool_KeepAliveLifecycle(t *testing.T) {
	// make rampup immediate for this test
	old := rampupTime
	defer func() { rampupTime = old }()
	rampupTime = 0

	// keepAliveDelay small so we observe calls quickly
	keepAliveDelay := 10 * time.Millisecond
	// use a channel to be signalled when KeepAlive is called, for deterministic testing
	kaCh := make(chan struct{}, 10)
	base := &fakeBroker{borrowResp: &pbworker.BorrowWorkerResponse{WorkerKey: "keep-1", Status: pbworker.BorrowWorkerResponse_borrowed}}
	// wrapper that signals kaCh on KeepAlive (use the package-level sigBroker type)
	broker := &sigBroker{fakeBroker: base, ch: kaCh}
	pool := NewGlobalWorkerPool(context.Background(), "u", "a", "t", 2, broker, nil, keepAliveDelay)

	w, err := pool.Borrow(context.Background())
	if err != nil {
		t.Fatalf("borrow failed: %v", err)
	}

	// wait for at least one keepalive to be received
	select {
	case <-kaCh:
		// good
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("timed out waiting for initial keepalive")
	}

	pool.Return(context.Background(), w)

	// capture current count non-deterministically via channel drain, then ensure no more events
	drain := func() int {
		cnt := 0
		for {
			select {
			case <-kaCh:
				cnt++
			default:
				return cnt
			}
		}
	}

	_ = drain()

	// ensure no more keepalive messages after return for a short window
	select {
	case <-kaCh:
		t.Fatalf("unexpected keepalive after return")
	case <-time.After(100 * time.Millisecond):
		// success
	}
}

func TestGlobalWorkerPool_ConcurrentBorrowReturn(t *testing.T) {
	old := rampupTime
	defer func() { rampupTime = old }()
	rampupTime = 0

	cb := &concurrentBroker{}

	pool := NewGlobalWorkerPool(context.Background(), "u", "a", "t", 100, cb, nil, 20*time.Millisecond)

	var wg sync.WaitGroup
	n := 50
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			w, err := pool.Borrow(context.Background())
			if err != nil {
				t.Errorf("borrow error: %v", err)
				return
			}
			// short simulated work
			time.Sleep(5 * time.Millisecond)
			pool.Return(context.Background(), w)
		}()
	}

	wg.Wait()

	if cb.returnCalls != n {
		t.Fatalf("expected %d returnCalls, got %d", n, cb.returnCalls)
	}
}

func TestGlobalWorkerPool_ContextCancelReturnsAllWorkers(t *testing.T) {
	old := rampupTime
	defer func() { rampupTime = old }()
	rampupTime = 0

	cb := &concurrentBroker{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := NewGlobalWorkerPool(ctx, "u", "a", "t", 100, cb, nil, 50*time.Millisecond)

	// borrow N workers
	n := 10
	workers := make([]Worker, 0, n)
	for i := 0; i < n; i++ {
		w, err := pool.Borrow(context.Background())
		if err != nil {
			t.Fatalf("borrow failed: %v", err)
		}
		workers = append(workers, w)
	}

	// sanity: map should contain n entries
	pool.borrowedWorkerMutex.Lock()
	if len(pool.borrowedWorker) != n {
		pool.borrowedWorkerMutex.Unlock()
		t.Fatalf("expected %d borrowed workers, got %d", n, len(pool.borrowedWorker))
	}
	pool.borrowedWorkerMutex.Unlock()

	// cancel the pool context; goroutine should iterate and Return each worker
	cancel()

	// wait for return calls to be observed
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for broker.ReturnWorker calls: got %d, want %d", cb.returnCalls, n)
		default:
			// check if broker recorded all returns
			cb.mu.Lock()
			rc := cb.returnCalls
			cb.mu.Unlock()
			if rc >= n {
				// ensure pool map is empty
				pool.borrowedWorkerMutex.Lock()
				remaining := len(pool.borrowedWorker)
				pool.borrowedWorkerMutex.Unlock()
				if remaining != 0 {
					t.Fatalf("expected borrowedWorker map to be empty after context cancel, still has %d entries", remaining)
				}
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}
