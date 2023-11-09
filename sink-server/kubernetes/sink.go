package kubernetes

import (
	pbsql "github.com/streamingfast/substreams-sink-sql/pb/sf/substreams/sink/sql/v1"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func (k *KubernetesEngine) newSink(deploymentID string, dbService string, pkg *pbsubstreams.Package, sinkConfig *pbsql.Service) ([]createFunc, error) {
	return nil, nil
}
