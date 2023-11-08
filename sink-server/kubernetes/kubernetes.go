package kubernetes

import (
	"k8s.io/client-go/kubernetes"

	pbsinksvc "github.com/streamingfast/substreams/pb/sf/substreams/sink/service/v1"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap"
)

type KubernetesEngine struct {
	clientSet kubernetes.Clientset
}

func NewEngine() (*KubernetesEngine, error) {
	return &KubernetesEngine{}, nil
}

func (k *KubernetesEngine) Create(deploymentID string, pkg *pbsubstreams.Package, zlog *zap.Logger) error {
	//TODO implement me
	panic("implement me")
}

func (k *KubernetesEngine) Update(deploymentID string, pkg *pbsubstreams.Package, reset bool, zlog *zap.Logger) error {
	//TODO implement me
	panic("implement me")
}

func (k *KubernetesEngine) Resume(deploymentID string, currentState pbsinksvc.DeploymentStatus, zlog *zap.Logger) (string, error) {
	//TODO implement me
	panic("implement me")
}

func (k *KubernetesEngine) Pause(deploymentID string, zlog *zap.Logger) (string, error) {
	//TODO implement me
	panic("implement me")
}

func (k *KubernetesEngine) Stop(deploymentID string, zlog *zap.Logger) (string, error) {
	//TODO implement me
	panic("implement me")
}

func (k *KubernetesEngine) Remove(deploymentID string, zlog *zap.Logger) (string, error) {
	//TODO implement me
	panic("implement me")
}

func (k *KubernetesEngine) Info(deploymentID string, zlog *zap.Logger) (pbsinksvc.DeploymentStatus, string, map[string]string, *pbsinksvc.PackageInfo, *pbsinksvc.SinkProgress, error) {
	//TODO implement me
	panic("implement me")
}

func (k *KubernetesEngine) List(zlog *zap.Logger) ([]*pbsinksvc.DeploymentWithStatus, error) {
	//TODO implement me
	panic("implement me")
}

func (k *KubernetesEngine) Shutdown(zlog *zap.Logger) error {
	return nil
}
