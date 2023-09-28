package docker

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	types "github.com/docker/cli/cli/compose/types"
	pbsinksvc "github.com/streamingfast/substreams/pb/sf/substreams/sink/service/v1"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type DockerEngine struct {
	mutex sync.Mutex
	dir   string
}

func NewEngine(dir string) *DockerEngine {
	return &DockerEngine{
		dir: dir,
	}
}

func (e *DockerEngine) Apply(deploymentID string, pkg *pbsubstreams.Package, zlog *zap.Logger) error {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	// TODO: pass substreams token from the original request
	token := os.Getenv("STREAMING_FAST_API_TOKEN")
	manifest, err := e.createManifest(deploymentID, token, pkg)
	if err != nil {
		return fmt.Errorf("creating manifest from package: %w", err)
	}

	output, err := e.applyManifest(deploymentID, manifest)
	fmt.Println("applied manifest", output)
	if err != nil {
		return fmt.Errorf("applying manifest: %w", err)
	}
	return nil
}

func (e *DockerEngine) Info(deploymentID string, zlog *zap.Logger) (pbsinksvc.DeploymentStatus, map[string]string, error) {
	cmd := exec.Command("docker", "compose", "ps", "--format", "json")
	cmd.Dir = filepath.Join(e.dir, deploymentID)
	out, err := cmd.Output()
	if err != nil {
		return pbsinksvc.DeploymentStatus_UNKNOWN, nil, fmt.Errorf("getting status from `docker compose ps` command: %q, %w", out, err)
	}

	var status pbsinksvc.DeploymentStatus

	sc := bufio.NewScanner(bytes.NewReader(out))
	if !sc.Scan() {
		return 0, nil, fmt.Errorf("no output from command")
	}
	line := sc.Bytes()

	var outputs []*dockerComposePSOutput
	if err := json.Unmarshal(line, &outputs); err != nil {
		return 0, nil, fmt.Errorf("unmarshalling docker output: %w", err)
	}

	for _, output := range outputs {
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
		}
	}
	return status, map[string]string{}, nil
}

type dockerComposePSOutput struct {
	State  string `json:"State"`
	Status string `json:"Status"`
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
		status, _, err := e.Info(id, zlog)
		if err != nil {
			zlog.Warn("cannot get info for deployment", zap.String("id", id))
			continue
		}
		out = append(out, &pbsinksvc.DeploymentWithStatus{
			Id:     id,
			Status: status,
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
		return "", fmt.Errorf("creating temporary docker-compose file: %w", err)
	}
	r := bytes.NewReader(man)
	if _, err := io.Copy(w, r); err != nil {
		return "", fmt.Errorf("writing temporary docker-compose file: %w", err)
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
		zlog.Info("shutting down deployment", zap.String("deploymentID", dep.Id))
		if _, e := e.Pause(dep.Id, zlog); e != nil {
			err = errors.Join(err, e)
		}
	}
	return err
}

func (e *DockerEngine) createManifest(deploymentID string, token string, pkg *pbsubstreams.Package) (content []byte, err error) {

	if err := os.MkdirAll(deploymentID, 0755); err != nil {
		return nil, fmt.Errorf("creating deploymentID folder %q: %w", deploymentID, err)
	}

	pg, err := e.newPostgres(deploymentID, pkg)
	if err != nil {
		return nil, fmt.Errorf("creating postgres deployment: %w", err)
	}

	pgweb := e.newPGWeb(deploymentID, pg.Name)

	sink, err := e.newSink(deploymentID, pg.Name, pkg)
	if err != nil {
		return nil, fmt.Errorf("creating postgres deployment: %w", err)
	}

	config := types.Config{
		Version: "3",
		Services: []types.ServiceConfig{
			pg,
			pgweb,
			sink,
		},
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
