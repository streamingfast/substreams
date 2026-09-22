package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveRemoteSquashQuietPeriod(t *testing.T) {
	got, err := ResolveRemoteSquashQuietPeriod(0)
	require.NoError(t, err)
	assert.Equal(t, DefaultRemoteSquashQuietPeriod, got)
	assert.Equal(t, 5*time.Minute, got)

	got, err = ResolveRemoteSquashQuietPeriod(time.Minute)
	require.NoError(t, err)
	assert.Equal(t, time.Minute, got)

	_, err = ResolveRemoteSquashQuietPeriod(-time.Second)
	require.EqualError(t, err, "remote squash quiet period must not be negative, got -1s")
}
