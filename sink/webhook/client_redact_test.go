package webhook

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestRedactURL(t *testing.T) {
	assert.Equal(t, "https://user:xxxxx@host/hook?x=1", RedactURL("https://user:s3cret@host/hook?x=1"))
	assert.Equal(t, "https://host/hook", RedactURL("https://host/hook"))
	assert.Equal(t, "<invalid URL>", RedactURL("https://user:s3c/ret@host/hook"))
}

func TestCall_ErrorsDoNotContainPassword(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	client := NewClient(Config{Timeout: time.Second, MaxRetries: 0, MaxInterval: time.Millisecond}, zap.NewNop())

	withPassword := strings.Replace(server.URL, "http://", "http://user:s3cret@", 1)
	err := client.Call(context.Background(), withPassword, []byte(`{}`), 1)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "s3cret")
	var deliveryErr *DeliveryError
	require.True(t, errors.As(err, &deliveryErr))
	assert.NotContains(t, deliveryErr.URL, "s3cret")

	err = client.Call(context.Background(), "http://user:s3c/ret@host/hook", []byte(`{}`), 1)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "s3c")
}
