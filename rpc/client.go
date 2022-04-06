package rpc

import (
	"net/http"
	"time"

	"github.com/streamingfast/eth-go/rpc"
)

func NewClient(endpoint string) *rpc.Client {
	httpClient := &http.Client{
		Transport: &http.Transport{
			DisableKeepAlives: true, // don't reuse connections
		},
		Timeout: 3 * time.Second,
	}

	return rpc.NewClient(endpoint, rpc.WithHttpClient(httpClient))
}
