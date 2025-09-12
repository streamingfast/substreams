package work

import (
	"context"

	"github.com/streamingfast/dsession"
	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/reqctx"
)

type WorkerPoolFactory func(ctx context.Context) WorkerPool

type SessionWorkerPoolFactory struct {
	sessionPool   dsession.SessionPool
	clientFactory client.InternalClientFactory
}

func NewSessionWorkerPoolFactory(sessionPool dsession.SessionPool, clientFactory client.InternalClientFactory) *SessionWorkerPoolFactory {
	return &SessionWorkerPoolFactory{
		sessionPool:   sessionPool,
		clientFactory: clientFactory,
	}
}

func (f *SessionWorkerPoolFactory) WorkerPool(ctx context.Context) WorkerPool {
	sessionKey, ok := reqctx.GetSessionKey(ctx)
	if !ok {
		// For backward compatibility with tests that don't set sessionKey
		sessionKey = "test-session"
	}
	return NewSessionWorkerPool(ctx, sessionKey, f.sessionPool, f.clientFactory)
}
