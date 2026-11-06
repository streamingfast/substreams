package clickhouse

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWireCounterCountsEveryConnectionItDials(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_, _ = conn.Write([]byte("pong"))
				_, _ = conn.Read(make([]byte, 16))
			}()
		}
	}()

	counter := &wireCounter{}

	// Two connections, because a retry replaces the cached client and the count has to
	// survive that rather than restart with the new socket.
	for range 2 {
		conn, err := counter.DialContext(context.Background(), "tcp", listener.Addr().String())
		require.NoError(t, err)

		_, err = conn.Write([]byte("hello"))
		require.NoError(t, err)

		_, err = conn.Read(make([]byte, 4))
		require.NoError(t, err)
		require.NoError(t, conn.Close())
	}

	require.Equal(t, int64(10), counter.BytesWritten(), "5 bytes on each of two connections")
}

func TestWireCounterIsZeroBeforeAnythingIsDialed(t *testing.T) {
	require.Zero(t, (&wireCounter{}).BytesWritten())
}
