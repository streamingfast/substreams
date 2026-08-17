package spool

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestAwaitQuotaAllowsAnOversizedSegmentWhenSpoolIsOtherwiseEmpty(t *testing.T) {
	spool := &Spool{
		options: Options{MaxBytes: 10},
		logger:  zap.NewNop(),
	}

	require.NoError(t, spool.awaitQuota(context.Background(), 11))
}

func TestSizerRespectsMaxBytesBelowSegmentFloor(t *testing.T) {
	sizer := newSizer(time.Second, 1)

	require.Equal(t, int64(1), sizer.size())
}

func TestOptionsClampSegmentMaxToSpoolMax(t *testing.T) {
	options := (Options{MaxBytes: 10, SegmentMaxBytes: 20}).withDefaults()

	require.Equal(t, int64(10), options.SegmentMaxBytes)
}

func TestSegmentBytesIncludesRowLogBytes(t *testing.T) {
	manifest := &Manifest{
		LogBytes: 123,
		Tables: []TableRecord{
			{Bytes: 7},
		},
	}

	require.Equal(t, int64(130), segmentBytes(manifest))
}
