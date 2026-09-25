package service

import (
	"io"
	"net/http"
	"sync"
	"time"
)

const (
	drainBodyMaxBytes = 4 * 1024 * 1024
	drainBodyTimeout  = 2 * time.Second
)

// drainRejectedRequestBody reads and discards what is left of a request body once
// the request is answered, up to drainBodyMaxBytes and for at most drainBodyTimeout.
//
// A request rejected before its body is read (authentication, compression
// enforcement) is answered while the client is still uploading. Go's HTTP/2 server
// then ends the stream with RST_STREAM(NO_ERROR) right after the response, and some
// load balancers in front of tier1 then sometimes answer the client with a 502 or an
// INTERNAL error instead of forwarding the response. Reading the rest of the
// body lets the stream end normally. A body that is already fully read, which is the
// case for every accepted request, drains nothing.
func drainRejectedRequestBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body == nil || r.Body == http.NoBody {
			next.ServeHTTP(w, r)
			return
		}

		body := &drainOnCloseBody{ReadCloser: r.Body, rc: http.NewResponseController(w)}
		r.Body = body

		next.ServeHTTP(w, r)

		// Handlers that answer without ever closing the body (e.g. compression
		// enforcement) still get it drained.
		body.Close()
	})
}

// drainOnCloseBody drains the body before closing it. grpc-go closes the request body
// as soon as the call ends, before its ServeHTTP returns, so the drain has to happen
// in Close and not only after the handler.
type drainOnCloseBody struct {
	io.ReadCloser
	rc *http.ResponseController

	once sync.Once
	err  error
}

func (b *drainOnCloseBody) Close() error {
	b.once.Do(func() {
		// Not every ResponseWriter supports deadlines (e.g. in tests); the byte limit
		// still bounds the drain there.
		_ = b.rc.SetReadDeadline(time.Now().Add(drainBodyTimeout))
		_, _ = io.Copy(io.Discard, io.LimitReader(b.ReadCloser, drainBodyMaxBytes))
		b.err = b.ReadCloser.Close()
	})
	return b.err
}
