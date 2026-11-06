package grpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"regexp"

	"github.com/streamingfast/dgrpc"
	pbsquasher "github.com/streamingfast/substreams/pb/sf/substreams/squasher/v1"
	"github.com/streamingfast/substreams/squash"
	gogrpc "google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

var portSuffixRegex = regexp.MustCompile(":[0-9]{2,5}$")

// Client calls a remote Squasher over gRPC and implements squash.Client.
type Client struct {
	grpc     pbsquasher.SquasherClient
	callOpts []gogrpc.CallOption
	headers  map[string]string
}

type DialOptions struct {
	PlainText bool
	Insecure  bool
	Secret    string
}

func Dial(endpoint string, opts DialOptions) (*Client, func() error, error) {
	if !portSuffixRegex.MatchString(endpoint) {
		return nil, nil, fmt.Errorf("invalid squasher endpoint %q: suffix must be ':<port>'", endpoint)
	}

	var dialOptions []gogrpc.DialOption
	switch {
	case opts.PlainText:
		dialOptions = []gogrpc.DialOption{gogrpc.WithTransportCredentials(insecure.NewCredentials())}
	case opts.Insecure:
		dialOptions = []gogrpc.DialOption{gogrpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{InsecureSkipVerify: true}))}
	}

	conn, err := dgrpc.NewExternalClientConn(endpoint, dialOptions...)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to create squasher gRPC client: %w", err)
	}

	c := &Client{grpc: pbsquasher.NewSquasherClient(conn)}
	if opts.Secret != "" {
		c.headers = map[string]string{"authorization": opts.Secret}
	}
	return c, conn.Close, nil
}

func (c *Client) Squash(ctx context.Context, req squash.Request) (*squash.Result, error) {
	if len(c.headers) > 0 {
		kvs := make([]string, 0, len(c.headers)*2)
		for k, v := range c.headers {
			kvs = append(kvs, k, v)
		}
		ctx = metadata.AppendToOutgoingContext(ctx, kvs...)
	}

	ranges := make([]*pbsquasher.Range, len(req.Ranges))
	for i, r := range req.Ranges {
		ranges[i] = &pbsquasher.Range{StartBlock: r.StartBlock, ExclusiveEndBlock: r.ExclusiveEndBlock}
	}

	resp, err := c.grpc.Squash(ctx, &pbsquasher.SquashRequest{
		ModuleName:           req.ModuleName,
		ModuleHash:           req.ModuleHash,
		ModuleInitialBlock:   req.ModuleInitialBlock,
		UpdatePolicy:         req.UpdatePolicy,
		ValueType:            req.ValueType,
		StateStore:           req.StateStore,
		StateStoreDefaultTag: req.StateStoreDefaultTag,
		StoreSizeLimit:       req.StoreSizeLimit,
		Ranges:               ranges,
	}, c.callOpts...)
	if err != nil {
		return nil, err
	}
	return &squash.Result{
		ExclusiveEndBlock: resp.ExclusiveEndBlock,
		StoreSizeBytes:    resp.StoreSizeBytes,
		LoadedExisting:    resp.LoadedExistingFullKv,
	}, nil
}
