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
	p := NewJobRunnerPool(context.Background(), 2, func(logger *zap.Logger) JobRunner {
		return func(ctx context.Context, request *pbsubstreams.Request, respFunc substreams.ResponseFunc) ([]*block.Range, error) {
			return nil, nil
		}
	})
	assert.Len(t, p.workers, 2)
	jobRunner := p.Borrow()
	assert.Len(t, p.workers, 1)
	p.Return(jobRunner)
	assert.Len(t, p.workers, 2)
}
