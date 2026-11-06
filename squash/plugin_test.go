package squash

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewEmptyAndLocalAreInProcess(t *testing.T) {
	logger := zap.NewNop()
	c, err := New("", logger)
	require.NoError(t, err)
	assert.Nil(t, c)

	c, err = New("local://", logger)
	require.NoError(t, err)
	assert.Nil(t, c)
}

func TestNewUnknownScheme(t *testing.T) {
	_, err := New("grpc://localhost:9002", zap.NewNop())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no Squasher plugin named \"grpc\"")
}
