package tools

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// reserveAddr returns an address nothing is listening on, so that the first dial to it is
// refused rather than merely slow.
func reserveAddr(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := listener.Addr().String()
	require.NoError(t, listener.Close())

	return addr
}

// An endpoint that refuses the first dial and accepts a moment later is a rolling restart, not
// a dead endpoint. The connect budget exists to cover exactly that window, so the poll must
// succeed as long as the backend comes up inside it.
func TestWaitForConnReady_RecoversWithinConnectBudget(t *testing.T) {
	addr := reserveAddr(t)

	const backendDownFor = 500 * time.Millisecond
	const connectTimeout = 5 * time.Second

	server := grpc.NewServer()
	t.Cleanup(server.Stop)

	serving := make(chan struct{})
	go func() {
		time.Sleep(backendDownFor)

		listener, err := net.Listen("tcp", addr)
		if err != nil {
			close(serving)
			return
		}

		close(serving)
		_ = server.Serve(listener)
	}()

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()

	begin := time.Now()
	err = waitForConnReady(ctx, conn)
	elapsed := time.Since(begin)

	<-serving

	require.NoError(t, err, "endpoint came up after %s, well inside the %s connect budget, but the poll gave up after %s", backendDownFor, connectTimeout, elapsed)
	require.Greater(t, elapsed, backendDownFor, "returned before the backend was listening, so readiness was never actually established")
}

// A backend that stays down for longer than the connect budget must still be reported, and the
// budget must be spent waiting rather than returned immediately.
func TestWaitForConnReady_GivesUpAfterConnectBudget(t *testing.T) {
	addr := reserveAddr(t)

	const connectTimeout = 500 * time.Millisecond

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	ctx, cancel := context.WithTimeoutCause(context.Background(), connectTimeout, context.DeadlineExceeded)
	defer cancel()

	begin := time.Now()
	err = waitForConnReady(ctx, conn)
	elapsed := time.Since(begin)

	require.Error(t, err)
	require.GreaterOrEqual(t, elapsed, connectTimeout, "gave up after %s without spending the %s connect budget", elapsed, connectTimeout)
}
