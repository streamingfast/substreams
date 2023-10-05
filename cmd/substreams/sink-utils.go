package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/bufbuild/connect-go"
	pbsinksvc "github.com/streamingfast/substreams/pb/sf/substreams/sink/service/v1"
	"github.com/streamingfast/substreams/pb/sf/substreams/sink/service/v1/pbsinksvcconnect"
)

var fuzzyMatchPreferredStatusOrder = []pbsinksvc.DeploymentStatus{
	pbsinksvc.DeploymentStatus_RUNNING,
	pbsinksvc.DeploymentStatus_PAUSED,
	pbsinksvc.DeploymentStatus_FAILING,
	pbsinksvc.DeploymentStatus_STOPPED,
	pbsinksvc.DeploymentStatus_UNKNOWN,
}

func fuzzyMatchDeployment(ctx context.Context, q string, cli pbsinksvcconnect.ProviderClient, preferredStatusOrder []pbsinksvc.DeploymentStatus) (*pbsinksvc.DeploymentWithStatus, error) {
	resp, err := cli.List(ctx, connect.NewRequest(&pbsinksvc.ListRequest{}))
	if err != nil {
		return nil, fmt.Errorf("error fetching existing deployments")
	}

	matching := make(map[*pbsinksvc.DeploymentWithStatus]pbsinksvc.DeploymentStatus)
	for _, dep := range resp.Msg.Deployments {
		if strings.HasPrefix(dep.Id, q) {
			matching[dep] = dep.Status
		}
	}
	if len(matching) == 0 {
		return nil, fmt.Errorf("cannot find any deployment matching %q", q)
	}
	if len(matching) == 1 {
		for k := range matching {
			return k, nil
		}
	}
	// more than one match, take the best status...

	for i := len(preferredStatusOrder) - 1; i >= 0; i-- {
		for k, v := range matching {
			if v == preferredStatusOrder[i] {
				delete(matching, k)
			}
		}
		if len(matching) == 1 {
			for k := range matching {
				return k, nil
			}
		}
		if len(matching) == 0 {
			break
		}
	}
	return nil, fmt.Errorf("cannot determine which deployment should match %q", q)
}

func printServices(outputs map[string]string) {
	fmt.Printf("Services:\n")
	var keys []string
	for k := range outputs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		lines := strings.Split(outputs[k], "\n")
		prefixLen := len(k) + 6
		var withMargin string
		for i, line := range lines {
			if i == 0 {
				withMargin = line + "\n"
				continue
			}
			withMargin += strings.Repeat(" ", prefixLen) + line + "\n"
		}
		fmt.Printf("  - %s: %s", k, withMargin)
	}

}

func userConfirm() bool {
	var resp string
	_, err := fmt.Scan(&resp)
	if err != nil {
		panic(err)
	}

	switch strings.ToLower(resp) {
	case "y", "yes":
		return true
	}
	return false
}

func interceptConnectionError(err error) error {
	if connectError, ok := err.(*connect.Error); ok {
		if connectError.Code() == connect.CodeUnavailable {
			return fmt.Errorf("Cannot connect to sink service: %w. Are you running `substreams alpha sink-serve` ?", err)
		}
	}
	return err
}
