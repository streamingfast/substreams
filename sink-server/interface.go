package server

import (
	pbsinksvc "github.com/streamingfast/substreams/pb/sf/substreams/sink/service/v1"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	docker "github.com/streamingfast/substreams/sink-server/docker"
	"github.com/streamingfast/substreams/sink-server/kubernetes"

	"go.uber.org/zap"
)

type Engine interface {
	Create(deploymentID string, pkg *pbsubstreams.Package, zlog *zap.Logger) error
	Update(deploymentID string, pkg *pbsubstreams.Package, reset bool, zlog *zap.Logger) error

	Resume(deploymentID string, currentState pbsinksvc.DeploymentStatus, zlog *zap.Logger) (string, error)
	Pause(deploymentID string, zlog *zap.Logger) (string, error)
	Stop(deploymentID string, zlog *zap.Logger) (string, error)

	Remove(deploymentID string, zlog *zap.Logger) (string, error)

	Info(deploymentID string, zlog *zap.Logger) (pbsinksvc.DeploymentStatus, string, map[string]string, *pbsinksvc.PackageInfo, *pbsinksvc.SinkProgress, error)
	List(zlog *zap.Logger) ([]*pbsinksvc.DeploymentWithStatus, error)

	Shutdown(zlog *zap.Logger) error
}

var _ Engine = &docker.DockerEngine{}
var _ Engine = &kubernetes.KubernetesEngine{}
