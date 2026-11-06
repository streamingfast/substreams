package clickhouse

import (
	"context"
	"net"
	"sync/atomic"
)

// wireCounter counts the bytes the sink actually put on the connection to ClickHouse.
//
// It exists because the spool's own byte count answers a different question. That one is
// the size of the segment on disk, in the tag-encoded format the codec writes, and it is
// what the sizer steers; what reaches the server is a ch-go columnar block, optionally
// LZ4-compressed when the DSN carries compress=true. The two differ by whatever the
// encoding and the compression do, which is exactly the ratio an operator sizing a
// network link or reading a cloud egress bill is asking about.
//
// Counting at the connection means it also covers the cursor writes, the schema probes
// and the protocol framing. That is deliberate: all of it is bytes this sink sent to that
// database, which is what the number claims.
// It is itself the ch.Dialer, so every connection the client opens is counted — including
// the ones a retry replaces the cached client with.
type wireCounter struct {
	written atomic.Int64
	dialer  net.Dialer
}

func (c *wireCounter) BytesWritten() int64 { return c.written.Load() }

func (c *wireCounter) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	conn, err := c.dialer.DialContext(ctx, network, address)
	if err != nil {
		return nil, err
	}

	return &countingConn{Conn: conn, counter: c}, nil
}

type countingConn struct {
	net.Conn
	counter *wireCounter
}

func (c *countingConn) Write(b []byte) (int, error) {
	n, err := c.Conn.Write(b)
	c.counter.written.Add(int64(n))

	return n, err
}
