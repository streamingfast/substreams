package work

import (
	"context"
	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"testing"
)

func Test_jobRunnerPool_Borrow_Return(t *testing.T) {
	ctx := context.Background()
	p := NewJobRunnerPool(ctx, 2, func(logger *zap.Logger) JobRunner {
		return func(ctx context.Context, request *pbsubstreams.Request, respFunc substreams.ResponseFunc) ([]*block.Range, error) {
			return nil, nil
		}
	})
	assert.Len(t, p.workers, 2)
	jobRunner := p.Borrow(ctx)
	assert.Len(t, p.workers, 1)
	p.Return(jobRunner)
	assert.Len(t, p.workers, 2)
}

func Test_jobRunnerPool_Borrow_Return_Canceled_Ctx(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := NewJobRunnerPool(ctx, 1, func(logger *zap.Logger) JobRunner {
		return func(ctx context.Context, request *pbsubstreams.Request, respFunc substreams.ResponseFunc) ([]*block.Range, error) {
			return nil, nil
		}
	})
	assert.Len(t, p.workers, 1)
	<-p.workers
	assert.Len(t, p.workers, 0)
	jobRunner := p.Borrow(ctx)
	assert.Nil(t, jobRunner)

}
