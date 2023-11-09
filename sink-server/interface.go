package server

import (
	"context"
	pbsinksvc "github.com/streamingfast/substreams/pb/sf/substreams/sink/service/v1"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	docker "github.com/streamingfast/substreams/sink-server/docker"
	"github.com/streamingfast/substreams/sink-server/kubernetes"

	"go.uber.org/zap"
)

type Engine interface {
	Create(ctx context.Context, deploymentID string, pkg *pbsubstreams.Package, zlog *zap.Logger) error
	Update(ctx context.Context, deploymentID string, pkg *pbsubstreams.Package, reset bool, zlog *zap.Logger) error

	Resume(ctx context.Context, deploymentID string, currentState pbsinksvc.DeploymentStatus, zlog *zap.Logger) (string, error)
	Pause(ctx context.Context, deploymentID string, zlog *zap.Logger) (string, error)
	Stop(ctx context.Context, deploymentID string, zlog *zap.Logger) (string, error)

	Remove(ctx context.Context, deploymentID string, zlog *zap.Logger) (string, error)

	Info(ctx context.Context, deploymentID string, zlog *zap.Logger) (pbsinksvc.DeploymentStatus, string, map[string]string, *pbsinksvc.PackageInfo, *pbsinksvc.SinkProgress, error)
	List(ctx context.Context, zlog *zap.Logger) ([]*pbsinksvc.DeploymentWithStatus, error)

	Shutdown(ctx context.Context, zlog *zap.Logger) error
}

var _ Engine = &docker.DockerEngine{}
var _ Engine = &kubernetes.KubernetesEngine{}
