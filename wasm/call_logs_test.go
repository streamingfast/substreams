package wasm

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAppendLog_NotTruncatedBetweenPerMessageAndTotalCaps(t *testing.T) {
	c := &Call{}
	msg := strings.Repeat("a", 256*1024) // 256 KiB, under the 512 KiB per-message cap

	// 4 messages = 1 MiB cumulative: above the 512 KiB per-message cap,
	// but well under the 2 MiB total cap. Nothing must be truncated.
	for i := 0; i < 4; i++ {
		c.AppendLog(msg)
	}

	require.False(t, c.ReachedLogsMaxByteCount())
	require.False(t, c.logsTruncated)
	require.Len(t, c.Logs, 4)
	for _, log := range c.Logs {
		require.NotContains(t, log, "truncated")
	}
}

func TestAppendLog_TruncatedAboveTotalCap(t *testing.T) {
	c := &Call{}
	msg := strings.Repeat("a", 256*1024) // 256 KiB per message

	// 8 messages = 2 MiB cumulative, reaching the total cap.
	for i := 0; i < 8; i++ {
		c.AppendLog(msg)
	}
	require.True(t, c.ReachedLogsMaxByteCount())

	c.AppendLog("this must be dropped")
	require.True(t, c.logsTruncated)
	require.Len(t, c.Logs, 9) // 8 messages + 1 truncation marker
	require.Contains(t, c.Logs[8], "truncated")
}
