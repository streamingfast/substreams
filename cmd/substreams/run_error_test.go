package main

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const serverLimitError = "rpc error: code = FailedPrecondition desc = request needs to process a total of 3394913 blocks (including 3394000 to prepare the stores) but only 10000 blocks are allowed according to the 'limit-processed-blocks' request argument"

type fakeWork struct {
	total         uint64
	prepareStores uint64
	known         bool
}

func (f fakeWork) SessionWork() (uint64, uint64, bool) { return f.total, f.prepareStores, f.known }

var noSession = fakeWork{}

func TestExplainRunError_PassesThroughUnrelatedErrors(t *testing.T) {
	err := errors.New("rpc error: code = Unavailable desc = connection refused")
	assert.Equal(t, err, explainRunError(err, noSession, 10_000))
	assert.NoError(t, explainRunError(nil, noSession, 10_000))
}

func TestExplainRunError_WithSession(t *testing.T) {
	work := fakeWork{total: 3_394_913, prepareStores: 3_394_000, known: true}

	out := explainRunError(errors.New(serverLimitError), work, 10_000)
	require.Error(t, out)

	assert.Contains(t, out.Error(), "--limit-processed-blocks is set to 10,000")
	assert.Contains(t, out.Error(), "3,394,913 blocks (3,394,000 of them to prepare the stores)")
	assert.Contains(t, out.Error(), "--limit-processed-blocks 3400000")
	assert.Contains(t, out.Error(), "--limit-processed-blocks 0")
	// The raw status is fully restated, so keeping it would only push the fix further away.
	assert.NotContains(t, out.Error(), "FailedPrecondition")
	// One line: hard-wrapping here would fight whatever width the terminal actually has.
	assert.NotContains(t, out.Error(), "\n")
}

// Everything can be in the requested range, with no stores to prepare at all.
func TestExplainRunError_WithoutStorePreparation(t *testing.T) {
	out := explainRunError(errors.New(serverLimitError), fakeWork{total: 50_000, known: true}, 10_000)
	require.Error(t, out)

	assert.Contains(t, out.Error(), "needs to process 50,000 blocks:")
	assert.NotContains(t, out.Error(), "prepare the stores")
}

// The limit can be tripped before a session is established, in which case the real figures are
// unknown and the message must not invent them.
func TestExplainRunError_WithoutSession(t *testing.T) {
	out := explainRunError(errors.New(serverLimitError), noSession, 10_000)
	require.Error(t, out)

	assert.Contains(t, out.Error(), "--limit-processed-blocks allows (10,000)")
	assert.Contains(t, out.Error(), "--limit-processed-blocks 0")
}

func TestSuggestedLimit(t *testing.T) {
	// The suggestion is rounded up so it stays valid if the chain moves before the re-run.
	assert.Equal(t, uint64(4_000), suggestedLimit(3_500))
	assert.Equal(t, uint64(110_000), suggestedLimit(100_001))
	assert.Equal(t, uint64(3_400_000), suggestedLimit(3_394_913))
	assert.Equal(t, uint64(13_000_000), suggestedLimit(12_000_001))
}
