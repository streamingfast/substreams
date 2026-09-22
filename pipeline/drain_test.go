package pipeline

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPipeline_Drain(t *testing.T) {
	t.Run("later blocks are refused", func(t *testing.T) {
		p := &Pipeline{}
		require.NoError(t, p.Drain("test"))
		assert.ErrorIs(t, p.ProcessBlock(nil, nil), ErrShuttingDown)
	})

	t.Run("waits for the block in flight", func(t *testing.T) {
		p := &Pipeline{}
		p.blockMu.Lock() // a block is being processed

		done := make(chan struct{})
		go func() {
			_ = p.Drain("test")
			close(done)
		}()

		select {
		case <-done:
			t.Fatal("Drain returned while a block was being processed")
		case <-time.After(50 * time.Millisecond):
		}

		p.blockMu.Unlock()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("Drain did not return once the block was processed")
		}
	})
}
