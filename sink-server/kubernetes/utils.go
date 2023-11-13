package kubernetes

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"

	"github.com/streamingfast/substreams/manifest"
	pbsinksvc "github.com/streamingfast/substreams/pb/sf/substreams/sink/service/v1"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func ref[T any](v T) *T { return &v }

func getPodLogs(ctx context.Context, clientset *kubernetes.Clientset, namespace, podName string) (string, error) {
	podLogOpts := corev1.PodLogOptions{
		TailLines: ref(int64(500)),
	}

	req := clientset.CoreV1().Pods(namespace).GetLogs(podName, &podLogOpts)
	podLogs, err := req.Stream(ctx)
	if err != nil {
		return "", fmt.Errorf("error in opening stream: %w", err)
	}
	defer podLogs.Close()

	logs, err := ioutil.ReadAll(podLogs)
	if err != nil {
		return "", fmt.Errorf("error in reading logs: %w", err)
	}

	return string(logs), nil
}

func (k *KubernetesEngine) getProgressBlock(ctx context.Context, serviceName, deploymentID string, zlog *zap.Logger) uint64 {
	out, err := getPodLogs(ctx, k.clientSet, k.namespace, fmt.Sprintf("%s-%s-0", serviceName, deploymentID))
	if err != nil {
		zlog.Debug("cannot get logs from pod", zap.Error(err))
		return 0
	}

	lines := strings.Split(string(out), "\n")
	if len(lines) == 0 {
		zlog.Debug("got no output lines from sink for progress")
		return 0
	}

	type StreamStats struct {
		LastBlock string `json:"last_block"`
	}

	for i := len(lines) - 1; i >= 0; i-- {
		if strings.Contains(lines[i], "postgres sink stats") { // postgres sink can be ahead of substreams sink, so we use the former
			stats := &StreamStats{}
			if err := json.Unmarshal([]byte(lines[i]), stats); err == nil {
				parts := strings.Split(stats.LastBlock, " ")
				if len(parts) == 2 {
					blk := strings.TrimPrefix(parts[0], "#")
					blknum, err := strconv.ParseUint(blk, 10, 64)
					if err == nil {
						return blknum
					} else {
						zlog.Debug("cannot parse blocknum in stream stats", zap.Error(err), zap.String("blk", blk))
					}
				}
			} else {
				zlog.Info("cannot unmarshal sink stream stats", zap.Error(err))
			}
		}
	}
	return 0

}

func (k *KubernetesEngine) getPackageInfo(ctx context.Context, deploymentId string) (*pbsinksvc.PackageInfo, error) {
	// get spkg from the configmap of name "sink-<deploymentId>"
	cm, err := k.clientSet.CoreV1().ConfigMaps(k.namespace).Get(ctx, fmt.Sprintf("sink-%s", deploymentId), metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting configmap: %w", err)
	}

	pkg := &pbsubstreams.Package{}
	err = proto.Unmarshal([]byte(cm.BinaryData["substreams.spkg"]), pkg)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling package info: %w", err)
	}

	pkgMeta := pkg.PackageMeta[0]
	hash, err := getModuleHash(pkg.SinkModule, pkg)
	if err != nil {
		return nil, fmt.Errorf("getting module hash: %w", err)
	}

	info := &pbsinksvc.PackageInfo{
		Name:             pkgMeta.Name,
		Version:          pkgMeta.Version,
		OutputModuleName: pkg.SinkModule,
		OutputModuleHash: hash,
	}

	return info, nil
}

func getModuleHash(mod string, pkg *pbsubstreams.Package) (hash string, err error) {
	graph, err := manifest.NewModuleGraph(pkg.Modules.Modules)
	if err != nil {
		return "", fmt.Errorf("creating module graph: %w", err)
	}

	hashes := manifest.NewModuleHashes()

	for _, module := range pkg.Modules.Modules {
		if module.Name != mod {
			continue
		}
		h, err := hashes.HashModule(pkg.Modules, module, graph)
		if err != nil {
			return "", fmt.Errorf("hashing module: %w", err)
		}
		hash = hex.EncodeToString(h)
	}
	if hash == "" {
		return hash, fmt.Errorf("cannot find module %s", mod)
	}
	return
}
