package kubernetes

import (
	"context"
	"fmt"
	pbsql "github.com/streamingfast/substreams-sink-sql/pb/sf/substreams/sink/sql/v1"
	pbsinksvc "github.com/streamingfast/substreams/pb/sf/substreams/sink/service/v1"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type KubernetesEngine struct {
	clientSet *kubernetes.Clientset
	namespace string
	apiToken  string
}

func NewEngine(configPath string, namespace string, token string) (*KubernetesEngine, error) {
	var config *rest.Config
	var err error
	if configPath == "" {
		config, err = rest.InClusterConfig()
	} else {
		config, err = clientcmd.BuildConfigFromFlags("", configPath)
		if err != nil {
			panic(err.Error())
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	return &KubernetesEngine{
		clientSet: clientset,
		namespace: namespace,
		apiToken:  token,
	}, nil
}

func (k *KubernetesEngine) Create(ctx context.Context, deploymentID string, pkg *pbsubstreams.Package, zlog *zap.Logger) error {
	if pkg.SinkConfig.TypeUrl != "sf.substreams.sink.sql.v1.Service" {
		return fmt.Errorf("invalid sinkconfig type: %q. Only sf.substreams.sink.sql.v1.Service is supported for now", pkg.SinkConfig.TypeUrl)
	}

	var k8sCreateFuncs []createFunc

	sinkConfig := &pbsql.Service{}
	if err := pkg.SinkConfig.UnmarshalTo(sinkConfig); err != nil {
		return fmt.Errorf("cannot unmarshal sinkconfig: %w", err)
	}

	switch sinkConfig.GetEngine() {
	case pbsql.Service_unset:
		// nothing to do
	case pbsql.Service_clickhouse:
		return fmt.Errorf("clickhouse engine is not supported yet")
	case pbsql.Service_postgres:
		// create a postgres stateful set
		cf, err := k.newPostgres(deploymentID, pkg)
		if err != nil {
			return fmt.Errorf("error creating postgres stateful set: %w", err)
		}
		k8sCreateFuncs = append(k8sCreateFuncs, cf)
	}

	createdObjects := make([]*v1.ObjectMeta, 0)
	for _, f := range k8sCreateFuncs {
		oms, err := f(ctx)
		if err != nil {
			return fmt.Errorf("error creating kubernetes resources: %w", err)
		}

		createdObjects = append(createdObjects, oms...)
	}

	for _, om := range createdObjects {
		zlog.Info("created object", zap.String("name", om.Name))
	}

	return nil
}

func (k *KubernetesEngine) Update(ctx context.Context, deploymentID string, pkg *pbsubstreams.Package, reset bool, zlog *zap.Logger) error {
	//TODO implement me
	panic("implement me")
}

func (k *KubernetesEngine) Resume(ctx context.Context, deploymentID string, currentState pbsinksvc.DeploymentStatus, zlog *zap.Logger) (string, error) {
	//TODO implement me
	panic("implement me")
}

func (k *KubernetesEngine) Pause(ctx context.Context, deploymentID string, zlog *zap.Logger) (string, error) {
	//TODO implement me
	panic("implement me")
}

func (k *KubernetesEngine) Stop(ctx context.Context, deploymentID string, zlog *zap.Logger) (string, error) {
	//TODO implement me
	panic("implement me")
}

func (k *KubernetesEngine) Remove(ctx context.Context, deploymentID string, zlog *zap.Logger) (string, error) {
	//TODO implement me
	panic("implement me")
}

func (k *KubernetesEngine) Info(ctx context.Context, deploymentID string, zlog *zap.Logger) (pbsinksvc.DeploymentStatus, string, map[string]string, *pbsinksvc.PackageInfo, *pbsinksvc.SinkProgress, error) {
	//TODO implement me
	panic("implement me")
}

func (k *KubernetesEngine) List(ctx context.Context, zlog *zap.Logger) ([]*pbsinksvc.DeploymentWithStatus, error) {
	//TODO implement me
	panic("implement me")
}

func (k *KubernetesEngine) Shutdown(ctx context.Context, zlog *zap.Logger) error {
	return nil
}
