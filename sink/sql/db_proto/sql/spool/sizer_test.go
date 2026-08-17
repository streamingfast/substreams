package spool

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const mib = int64(1) << 20

func TestSizerConvergesOnMeasuredThroughput(t *testing.T) {
	const target = 3 * time.Second
	const maxBytes = 512 * (1 << 20)

	cases := []struct {
		name     string
		current  int64
		bytes    int64
		elapsed  time.Duration
		expected int64
	}{
		{
			// The one the controller exists for: 64 MiB in 3 s is exactly the target.
			name: "a commit at target holds the size", current: 64 * mib,
			bytes: 64 * mib, elapsed: 3 * time.Second, expected: 64 * mib,
		},
		{
			// A short seal — idle, drain or shutdown — applies fast for reasons that say
			// nothing about throughput. The size must not grow on it.
			name: "a short segment applied fast does not grow the size", current: 64 * mib,
			bytes: 1 * mib, elapsed: 100 * time.Millisecond, expected: 32 * mib,
		},
		{
			name: "a slow database shrinks the size, one step at a time", current: 64 * mib,
			bytes: 64 * mib, elapsed: 30 * time.Second, expected: 32 * mib,
		},
		{
			name: "a fast database grows the size, one step at a time", current: 64 * mib,
			bytes: 64 * mib, elapsed: 500 * time.Millisecond, expected: 128 * mib,
		},
		{
			name: "the floor holds", current: segmentFloorBytes,
			bytes: 1 * mib, elapsed: 30 * time.Second, expected: segmentFloorBytes,
		},
		{
			name: "the ceiling holds", current: maxBytes,
			bytes: maxBytes, elapsed: time.Millisecond, expected: maxBytes,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sizer := newSizer(target, maxBytes)
			sizer.current = c.current

			sizer.observe(c.bytes, c.elapsed)

			require.Equal(t, c.expected, sizer.size())
		})
	}
}

// TestSizerIgnoresAnEmptyCommit covers the segment a drain or a shutdown seals with
// nothing in it: it carries no measurement, so it must not move the size at all.
func TestSizerIgnoresAnEmptyCommit(t *testing.T) {
	sizer := newSizer(3*time.Second, 512*mib)
	sizer.current = 64 * mib

	sizer.observe(0, time.Millisecond)
	require.Equal(t, 64*mib, sizer.size())

	sizer.observe(64*mib, 0)
	require.Equal(t, 64*mib, sizer.size())
}
