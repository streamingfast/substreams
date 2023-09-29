package server

import (
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	pbsinksvc "github.com/streamingfast/substreams/pb/sf/substreams/sink/service/v1"
	docker "github.com/streamingfast/substreams/sink-server/docker"

	"go.uber.org/zap"
)

type Engine interface {

    // Apply can Create or Update a deployment
	Apply(deploymentID string, pkg *pbsubstreams.Package, zlog *zap.Logger) error

    Resume(deploymentID string, zlog *zap.Logger) (string, error)
    Pause(deploymentID string, zlog *zap.Logger) (string, error)

    Remove(deploymentID string, zlog *zap.Logger) (string, error)

    Info(deploymentID string, zlog *zap.Logger) (pbsinksvc.DeploymentStatus, string, map[string]string, error)
    List(zlog *zap.Logger) ([]*pbsinksvc.DeploymentWithStatus, error)

    Shutdown(zlog *zap.Logger) error
}


var _ Engine = &docker.DockerEngine{}
