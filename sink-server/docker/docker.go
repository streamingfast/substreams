package docker

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	types "github.com/docker/cli/cli/compose/types"
	"github.com/streamingfast/substreams/manifest"
	pbsinksvc "github.com/streamingfast/substreams/pb/sf/substreams/sink/service/v1"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type DockerEngine struct {
	mutex sync.Mutex
	dir   string
	token string
}

func NewEngine(dir string, sf_token string) *DockerEngine {
	return &DockerEngine{
		dir:   dir,
		token: sf_token,
	}
}

type deploymentInfo struct {
	PackageInfo *pbsinksvc.PackageInfo
	ServiceInfo map[string]string
	UsedPorts   []uint32
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

func (e *DockerEngine) writeDeploymentInfo(deploymentID string, usedPorts []uint32, svcInfo map[string]string, pkg *pbsubstreams.Package) error {

	pkgMeta := pkg.PackageMeta[0]
	hash, err := getModuleHash(pkg.SinkModule, pkg)
	if err != nil {
		return err
	}

	depInfo := &deploymentInfo{
		ServiceInfo: svcInfo,
		UsedPorts:   usedPorts,
		PackageInfo: &pbsinksvc.PackageInfo{
			Name:             pkgMeta.Name,
			Version:          pkgMeta.Version,
			OutputModuleName: pkg.SinkModule,
			OutputModuleHash: hash,
		},
	}

	json, err := json.Marshal(depInfo)
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(e.dir, deploymentID, "info.json"), json, 0644)
}
func (e *DockerEngine) readDeploymentInfo(deploymentID string) (info *deploymentInfo, err error) {
	content, err := os.ReadFile(filepath.Join(e.dir, deploymentID, "info.json"))
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(content, &info); err != nil {
		return nil, err
	}
	return info, nil
}

func (e *DockerEngine) Apply(deploymentID string, pkg *pbsubstreams.Package, zlog *zap.Logger) error {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	manifest, usedPorts, serviceInfo, err := e.createManifest(deploymentID, e.token, pkg)
	if err != nil {
		return fmt.Errorf("creating manifest from package: %w", err)
	}

	if err := e.writeDeploymentInfo(deploymentID, usedPorts, serviceInfo, pkg); err != nil {
		return fmt.Errorf("cannot write Service Info: %w", err)
	}

	output, err := e.applyManifest(deploymentID, manifest)
	if err != nil {
		return fmt.Errorf("applying manifest: %w", err)
	}
	_ = output // TODO save somewhere maybe
	return nil
}

var reasonInternalError = "internal error"

func (e *DockerEngine) Info(deploymentID string, zlog *zap.Logger) (pbsinksvc.DeploymentStatus, string, map[string]string, *pbsinksvc.PackageInfo, error) {
	cmd := exec.Command("docker", "compose", "ps", "--format", "json")
	cmd.Dir = filepath.Join(e.dir, deploymentID)
	out, err := cmd.Output()
	if err != nil {
		return pbsinksvc.DeploymentStatus_UNKNOWN, reasonInternalError, nil, nil, fmt.Errorf("getting status from `docker compose ps` command: %q, %w", out, err)
	}

	var status pbsinksvc.DeploymentStatus

	sc := bufio.NewScanner(bytes.NewReader(out))
	if !sc.Scan() {
		return 0, reasonInternalError, nil, nil, fmt.Errorf("no output from command")
	}
	line := sc.Bytes()

	var outputs []*dockerComposePSOutput
	if err := json.Unmarshal(line, &outputs); err != nil {
		return 0, reasonInternalError, nil, nil, fmt.Errorf("unmarshalling docker output: %w", err)
	}

	info, err := e.readDeploymentInfo(deploymentID)
	if err != nil {
		return status, reasonInternalError, nil, nil, fmt.Errorf("cannot read Service Info: %w", err)
	}

	seen := make(map[string]bool, len(info.ServiceInfo))
	var reason string

	for _, output := range outputs {
		seen[output.Name] = true
		switch output.State {
		case "running":
			if status == pbsinksvc.DeploymentStatus_UNKNOWN { // anything else has priority
				status = pbsinksvc.DeploymentStatus_RUNNING
			}
		case "paused":
			if status != pbsinksvc.DeploymentStatus_FAILING {
				status = pbsinksvc.DeploymentStatus_PAUSED
			}
		default:
			status = pbsinksvc.DeploymentStatus_FAILING
			reason += fmt.Sprintf("%s: %q", strings.TrimPrefix(output.Name, deploymentID+"-"), output.Status)
		}
	}
	if len(seen) == 0 {
		status = pbsinksvc.DeploymentStatus_PAUSED
	} else {
		for k := range info.ServiceInfo {
			if !seen[k] {
				status = pbsinksvc.DeploymentStatus_FAILING
				reason += fmt.Sprintf("%s: missing, ", strings.TrimPrefix(k, deploymentID+"-"))
			}
		}
	}

	return status, reason, info.ServiceInfo, info.PackageInfo, nil
}

type dockerComposePSOutput struct {
	State  string `json:"State"`
	Status string `json:"Status"`
	Name   string `json:"Name"`
}

func (e *DockerEngine) List(zlog *zap.Logger) (out []*pbsinksvc.DeploymentWithStatus, err error) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	files, err := os.ReadDir(e.dir)
	if err != nil {
		return nil, err
	}

	for _, f := range files {
		id := f.Name()
		status, reason, _, info, err := e.Info(id, zlog)
		if err != nil {
			zlog.Warn("cannot get info for deployment", zap.String("id", id))
			continue
		}
		out = append(out, &pbsinksvc.DeploymentWithStatus{
			Id:     id,
			Status: status,
			Reason: reason,
            PackageInfo: info,
		})
	}
	return out, nil
}

func (e *DockerEngine) Resume(deploymentID string, _ *zap.Logger) (string, error) {
	cmd := exec.Command("docker", "compose", "up", "-d")
	cmd.Dir = filepath.Join(e.dir, deploymentID)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("resuming docker compose: %q, %w", out, err)
	}
	return string(out), nil
}

func (e *DockerEngine) Pause(deploymentID string, zlog *zap.Logger) (string, error) {
	cmd := exec.Command("docker", "compose", "down")
	cmd.Dir = filepath.Join(e.dir, deploymentID)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("pausing docker compose: %q, %w", out, err)
	}
	return string(out), nil
}

func (e *DockerEngine) Remove(deploymentID string, zlog *zap.Logger) (string, error) {
	cmd := exec.Command("docker", "compose", "down")
	cmd.Dir = filepath.Join(e.dir, deploymentID)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("pausing docker compose: %q, %w", out, err)
	}
	err = os.RemoveAll(filepath.Join(e.dir, deploymentID))
	return string(out), err
}

func (e *DockerEngine) applyManifest(deployment string, man []byte) (string, error) {
	w, err := os.Create(filepath.Join(e.dir, deployment, "docker-compose.yaml"))
	if err != nil {
		return "", fmt.Errorf("creating docker-compose file: %w", err)
	}
	r := bytes.NewReader(man)
	if _, err := io.Copy(w, r); err != nil {
		return "", fmt.Errorf("writing docker-compose file: %w", err)
	}

	cmd := exec.Command("docker", "compose", "up", "-d")
	cmd.Dir = filepath.Join(e.dir, deployment)

	out, err := cmd.CombinedOutput()

	return string(out), err
}

func deref[T any](in T) *T {
	return &in
}

func toDuration(in time.Duration) *types.Duration {
	return deref(types.Duration(in))
}

func (e *DockerEngine) Shutdown(zlog *zap.Logger) (err error) {
	deps, err := e.List(zlog)
	if err != nil {
		return fmt.Errorf("cannot list deployments: %w", err)
	}
	for _, dep := range deps {
		if dep.Status != pbsinksvc.DeploymentStatus_RUNNING && dep.Status != pbsinksvc.DeploymentStatus_FAILING {
			continue
		}
		zlog.Info("shutting down deployment", zap.String("deploymentID", dep.Id))
		if _, e := e.Pause(dep.Id, zlog); e != nil {
			err = errors.Join(err, e)
		}
	}
	return err
}

func (e *DockerEngine) createManifest(deploymentID string, token string, pkg *pbsubstreams.Package) (content []byte, usedPorts []uint32, services map[string]string, err error) {

	services = make(map[string]string)

	pg, pgMotd, err := e.newPostgres(deploymentID, pkg)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("creating postgres deployment: %w", err)
	}
	services[pg.Name] = pgMotd

	pgweb, pgwebMotd := e.newPGWeb(deploymentID, pg.Name)
	services[pgweb.Name] = pgwebMotd

	sink, sinkMotd, err := e.newSink(deploymentID, pg.Name, pkg)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("creating postgres deployment: %w", err)
	}
	services[sink.Name] = sinkMotd

	config := types.Config{
		Version: "3",
		Services: []types.ServiceConfig{
			pg,
			pgweb,
			sink,
		},
	}

	for _, svc := range config.Services {
		for _, port := range svc.Ports {
			usedPorts = append(usedPorts, port.Published)
		}
	}

	content, err = yaml.Marshal(config)
	return

	//  clickhouse:
	//    container_name: clickhouse-ssp
	//    image: clickhouse/clickhouse-server
	//    user: "101:101"
	//    hostname: clickhouse
	//    volumes:
	//      - ${PWD}/devel/clickhouse-server/config.xml:/etc/clickhouse-server/config.d/config.xml
	//      - ${PWD}/devel/clickhouse-server/users.xml:/etc/clickhouse-server/users.d/users.xml
	//    ports:
	//      - "8123:8123"
	//      - "9000:9000"
	//      - "9005:9005"
	//  sink:
}
