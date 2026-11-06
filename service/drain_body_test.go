package service

import (
	"bytes"
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

// rejectEarly answers like grpc-go does when an interceptor rejects a call: the
// response is flushed and the body closed before the request body was read.
var rejectEarly = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusForbidden)
	w.(http.Flusher).Flush()
	r.Body.Close()
})

func TestDrainRejectedRequestBody(t *testing.T) {
	t.Run("without drain the stream is reset after the response", func(t *testing.T) {
		assert.True(t, streamResetAfterEarlyResponse(t, rejectEarly))
	})

	t.Run("with drain the stream ends without a reset", func(t *testing.T) {
		assert.False(t, streamResetAfterEarlyResponse(t, drainRejectedRequestBody(rejectEarly)))
	})
}

func TestDrainOnCloseBodyStopsAtLimit(t *testing.T) {
	src := bytes.NewReader(make([]byte, drainBodyMaxBytes+1024))
	r := httptest.NewRequest(http.MethodPost, "/", src)

	drainRejectedRequestBody(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})).
		ServeHTTP(httptest.NewRecorder(), r)

	assert.Equal(t, 1024, src.Len())
}

// streamResetAfterEarlyResponse sends a request whose body is split in two DATA
// frames, the second one only after the response is complete, and reports whether
// the server reset the stream.
func streamResetAfterEarlyResponse(t *testing.T, handler http.Handler) bool {
	t.Helper()

	srv := httptest.NewUnstartedServer(handler)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	defer srv.Close()

	conn, err := tls.Dial("tcp", srv.Listener.Addr().String(), &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"h2"}})
	require.NoError(t, err)
	defer conn.Close()

	_, err = io.WriteString(conn, http2.ClientPreface)
	require.NoError(t, err)

	framer := http2.NewFramer(conn, conn)
	require.NoError(t, framer.WriteSettings())

	var headers bytes.Buffer
	enc := hpack.NewEncoder(&headers)
	for _, f := range []hpack.HeaderField{
		{Name: ":method", Value: "POST"},
		{Name: ":scheme", Value: "https"},
		{Name: ":authority", Value: srv.Listener.Addr().String()},
		{Name: ":path", Value: "/"},
		{Name: "content-type", Value: "application/grpc"},
	} {
		require.NoError(t, enc.WriteField(f))
	}
	require.NoError(t, framer.WriteHeaders(http2.HeadersFrameParam{StreamID: 1, BlockFragment: headers.Bytes(), EndHeaders: true}))
	require.NoError(t, framer.WriteData(1, false, make([]byte, 16*1024)))

	// Like a gRPC client or a load balancer, the rest of the body goes out as soon as
	// the response starts, without waiting for the response to end.
	responseStarted, responseEnded, secondHalfSent := false, false, false
	deadline := time.Now().Add(5 * time.Second)
	for {
		if responseStarted && !secondHalfSent {
			require.NoError(t, framer.WriteData(1, true, make([]byte, 16*1024)))
			secondHalfSent = true
		}

		require.NoError(t, conn.SetReadDeadline(deadline))
		frame, err := framer.ReadFrame()
		if err != nil {
			require.True(t, responseEnded, "response not complete before %s: %s", deadline, err)
			return false
		}

		switch f := frame.(type) {
		case *http2.SettingsFrame:
			if !f.IsAck() {
				require.NoError(t, framer.WriteSettingsAck())
			}
		case *http2.HeadersFrame, *http2.DataFrame:
			if frame.Header().StreamID != 1 {
				continue
			}
			responseStarted = true
			if frame.Header().Flags.Has(http2.FlagHeadersEndStream) && !responseEnded {
				responseEnded = true
				// A reset, when there is one, follows the end of the response right away.
				deadline = time.Now().Add(500 * time.Millisecond)
			}
		case *http2.RSTStreamFrame:
			if f.StreamID == 1 {
				return true
			}
		}
	}
}
