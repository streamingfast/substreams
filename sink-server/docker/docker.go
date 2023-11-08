package docker

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	pbsql "github.com/streamingfast/substreams-sink-sql/pb/sf/substreams/sink/sql/v1"
	"github.com/streamingfast/substreams/manifest"
	pbsinksvc "github.com/streamingfast/substreams/pb/sf/substreams/sink/service/v1"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"

	types "github.com/docker/cli/cli/compose/types"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

type DockerEngine struct {
	mutex sync.Mutex
	dir   string
	token string
}

func NewEngine(dir string, sf_token string) (*DockerEngine, error) {
	out := &DockerEngine{
		dir:   dir,
		token: sf_token,
	}
	if err := out.CheckVersion(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	return out, nil
}

type deploymentInfo struct {
	PackageInfo *pbsinksvc.PackageInfo
	ServiceInfo map[string]string
	UsedPorts   []uint32
	RunMeFirst  []string
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

func (e *DockerEngine) CheckVersion() error {
	cmd := exec.Command("docker", "compose", "version", "--short")

	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("cannot run `docker compose version`: %q, %w", out, err)
	}

	ver := string(out)
	match, _ := regexp.MatchString("v?[1-9]", ver)
	if match {
		return nil
	}

	return fmt.Errorf("Cannot determine docker compose version %q. Upgrade your Docker engine here: https://docs.docker.com/engine/install/", ver)
}

func (e *DockerEngine) writeDeploymentInfo(deploymentID string, usedPorts []uint32, runMeFirst []string, svcInfo map[string]string, pkg *pbsubstreams.Package) error {
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
		RunMeFirst: runMeFirst,
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

func (e *DockerEngine) Create(deploymentID string, pkg *pbsubstreams.Package, zlog *zap.Logger) error {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	if e.otherDeploymentIsActive("!NO_MATCH!", zlog) {
		return fmt.Errorf("this substreams-sink engine only supports a single active deployment. Stop any active sink before launching another one or use `sink-update`")
	}

	manifest, usedPorts, serviceInfo, runMeFirst, err := e.createManifest(deploymentID, e.token, pkg)
	if err != nil {
		return fmt.Errorf("creating manifest from package: %w", err)
	}

	if err := e.writeDeploymentInfo(deploymentID, usedPorts, runMeFirst, serviceInfo, pkg); err != nil {
		return fmt.Errorf("cannot write Service Info: %w", err)
	}

	output, err := e.applyManifest(deploymentID, manifest, runMeFirst, false)
	if err != nil {
		return fmt.Errorf("applying manifest: %w\noutput: %s", err, output)
	}
	_ = output // TODO save somewhere maybe
	return nil
}

func (e *DockerEngine) Update(deploymentID string, pkg *pbsubstreams.Package, reset bool, zlog *zap.Logger) error {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	if reset {
		if _, err := e.Stop(deploymentID, zlog); err != nil {
			return err
		}

		if err := os.RemoveAll(filepath.Join(e.dir, deploymentID)); err != nil {
			return fmt.Errorf("cannot cleanup the deployment folder: %w", err)
		}
		if err := os.MkdirAll(filepath.Join(e.dir, deploymentID), 0755); err != nil {
			return fmt.Errorf("cannot re-create the deployment folder: %w", err)
		}
	}

	manifest, usedPorts, serviceInfo, runMeFirst, err := e.createManifest(deploymentID, e.token, pkg)
	if err != nil {
		return fmt.Errorf("creating manifest from package: %w", err)
	}

	if err := e.writeDeploymentInfo(deploymentID, usedPorts, runMeFirst, serviceInfo, pkg); err != nil {
		return fmt.Errorf("cannot write Service Info: %w", err)
	}

	output, err := e.applyManifest(deploymentID, manifest, runMeFirst, true)
	if err != nil {
		return fmt.Errorf("applying manifest: %w\noutput: %s", err, output)
	}
	_ = output // TODO save somewhere maybe
	return nil
}

func (e *DockerEngine) otherDeploymentIsActive(deploymentID string, zlog *zap.Logger) bool {
	if deps, _ := e.list(zlog); deps != nil {
		for _, dep := range deps {
			if dep.Id == deploymentID {
				continue
			}
			switch dep.Status {
			case pbsinksvc.DeploymentStatus_PAUSED:
				return true
			case pbsinksvc.DeploymentStatus_RUNNING:
				return true
			case pbsinksvc.DeploymentStatus_FAILING:
				return true
			case pbsinksvc.DeploymentStatus_STOPPED:
				continue
			case pbsinksvc.DeploymentStatus_UNKNOWN:
				zlog.Info("cannot determine if deployment is active: unknown", zap.String("deployment_id", dep.Id))
				continue
			}
		}
	}
	return false
}

var reasonInternalError = "internal error"

func (e *DockerEngine) Info(deploymentID string, zlog *zap.Logger) (pbsinksvc.DeploymentStatus, string, map[string]string, *pbsinksvc.PackageInfo, *pbsinksvc.SinkProgress, error) {
	cmd := exec.Command("docker", "compose", "ps", "--format", "json")
	cmd.Dir = filepath.Join(e.dir, deploymentID)
	out, err := cmd.Output()
	if err != nil {
		return pbsinksvc.DeploymentStatus_UNKNOWN, reasonInternalError, nil, nil, nil, fmt.Errorf("getting status from `docker compose ps` command: %q, %w", out, err)
	}

	var line []byte
	// If the output does not start with a '[', add some
	if !bytes.HasPrefix(out, []byte("[")) {
		// Split the output by lines
		lines := bytes.Split(out, []byte("\n"))

		// Remove empty lines
		var cleanedLines [][]byte
		for _, l := range lines {
			if len(l) > 0 {
				cleanedLines = append(cleanedLines, l)
			}
		}

		// Join the cleaned lines with commas
		line = bytes.Join(cleanedLines, []byte(","))

		// Wrap the result in square brackets
		line = append([]byte{'['}, line...)
		line = append(line, ']')
	} else {
		line = out
	}

	var outputs []*dockerComposePSOutput
	if err := json.Unmarshal(line, &outputs); err != nil {
		return 0, reasonInternalError, nil, nil, nil, fmt.Errorf("unmarshalling docker output: %w", err)
	}

	var status pbsinksvc.DeploymentStatus

	info, err := e.readDeploymentInfo(deploymentID)
	if err != nil {
		return status, reasonInternalError, nil, nil, nil, fmt.Errorf("cannot read Service Info: %w", err)
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
		case "exited":
			// some versions of 'docker compose' use 'exited' state for stopped containers, we ignore those here and treat them later
			seen[output.Name] = false
		default:
			status = pbsinksvc.DeploymentStatus_FAILING
			reason += fmt.Sprintf("%s: %q", strings.TrimPrefix(output.Name, deploymentID+"-"), output.Status)
		}
	}
	if len(seen) == 0 {
		status = pbsinksvc.DeploymentStatus_STOPPED
	} else {
		for k := range info.ServiceInfo {
			if !seen[k] {
				if k == sinkServiceName(deploymentID) {
					status = pbsinksvc.DeploymentStatus_PAUSED
					continue
				}
				status = pbsinksvc.DeploymentStatus_FAILING
				reason += fmt.Sprintf("%s: missing, ", strings.TrimPrefix(k, deploymentID+"-"))
			}
		}
	}

	var sinkProgress *pbsinksvc.SinkProgress
	blk := getProgressBlock(sinkServiceName(deploymentID), filepath.Join(e.dir, deploymentID), zlog)
	if blk != 0 {
		sinkProgress = &pbsinksvc.SinkProgress{
			LastProcessedBlock: blk,
		}
	}

	return status, reason, info.ServiceInfo, info.PackageInfo, sinkProgress, nil
}

func getProgressBlock(serviceName, dir string, zlog *zap.Logger) uint64 {
	cmd := exec.Command("docker", "compose", "logs", serviceName, "--no-log-prefix")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		zlog.Debug("cannot get progress block", zap.Error(err))
		return 0
	}

	lines := strings.Split(string(out), "\n")
	if len(lines) == 0 {
		zlog.Debug("got no output lines from sink for progress")
		return 0
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

type StreamStats struct {
	LastBlock string `json:"last_block"`
}

type dockerComposePSOutput struct {
	State  string `json:"State"`
	Status string `json:"Status"`
	Name   string `json:"Name"`
}

func (e *DockerEngine) List(zlog *zap.Logger) (out []*pbsinksvc.DeploymentWithStatus, err error) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	return e.list(zlog)
}

func (e *DockerEngine) list(zlog *zap.Logger) (out []*pbsinksvc.DeploymentWithStatus, err error) {
	files, err := os.ReadDir(e.dir)
	if err != nil {
		return nil, err
	}

	for _, f := range files {
		id := f.Name()
		status, reason, _, info, _, err := e.Info(id, zlog)
		if err != nil {
			zlog.Warn("cannot get info for deployment", zap.String("id", id))
			continue
		}
		out = append(out, &pbsinksvc.DeploymentWithStatus{
			Id:          id,
			Status:      status,
			Reason:      reason,
			PackageInfo: info,
		})
	}
	return out, nil
}

func (e *DockerEngine) Resume(deploymentID string, _ pbsinksvc.DeploymentStatus, zlog *zap.Logger) (string, error) {
	if e.otherDeploymentIsActive(deploymentID, zlog) {
		return "", fmt.Errorf("this substreams-sink engine only supports a single active deployment. Stop any active sink before launching another one")
	}

	info, err := e.readDeploymentInfo(deploymentID)
	if err != nil {
		return "", err
	}
	// these services need to be healthy first
	if info.RunMeFirst != nil {
		args := append([]string{"compose", "up", "-d", "--wait"}, info.RunMeFirst...)
		cmd := exec.Command("docker", args...)
		cmd.Dir = filepath.Join(e.dir, deploymentID)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return string(out), err
		}
	}

	var cmd *exec.Cmd
	cmd = exec.Command("docker", "compose", "up", "-d", "--wait")
	cmd.Dir = filepath.Join(e.dir, deploymentID)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("resuming docker compose: %q, %w", out, err)
	}
	return string(out), nil
}

func (e *DockerEngine) Pause(deploymentID string, zlog *zap.Logger) (string, error) {
	cmd := exec.Command("docker", "compose", "stop", sinkServiceName(deploymentID)) // stop the sink process, keeping the database up
	cmd.Dir = filepath.Join(e.dir, deploymentID)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("pausing docker compose: %q, %w", out, err)
	}
	return string(out), nil
}

func (e *DockerEngine) Stop(deploymentID string, zlog *zap.Logger) (string, error) {
	cmd := exec.Command("docker", "compose", "down")
	cmd.Dir = filepath.Join(e.dir, deploymentID)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("stopping docker compose: %q, %w", out, err)
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

func (e *DockerEngine) applyManifest(deployment string, man []byte, runMeFirst []string, restartSink bool) (string, error) {
	w, err := os.Create(filepath.Join(e.dir, deployment, "docker-compose.yaml"))
	if err != nil {
		return "", fmt.Errorf("creating docker-compose file: %w", err)
	}
	r := bytes.NewReader(man)
	if _, err := io.Copy(w, r); err != nil {
		return "", fmt.Errorf("writing docker-compose file: %w", err)
	}
	// these services need to be healthy first
	if runMeFirst != nil {
		args := append([]string{"compose", "up", "-d", "--wait"}, runMeFirst...)
		cmd := exec.Command("docker", args...)
		cmd.Dir = filepath.Join(e.dir, deployment)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return string(out), err
		}
	}

	cmd := exec.Command("docker", "compose", "up", "-d", "--wait")
	cmd.Dir = filepath.Join(e.dir, deployment)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), err
	}

	if restartSink {
		cmd := exec.Command("docker", "compose", "restart", sinkServiceName(deployment))
		cmd.Dir = filepath.Join(e.dir, deployment)
		out2, err2 := cmd.CombinedOutput()
		out = append(out, out2...)
		err = err2
	}

	return string(out), err
}

func deref[T any](in T) *T {
	return &in
}

func toDuration(in time.Duration) *types.Duration {
	return deref(types.Duration(in))
}

func (e *DockerEngine) Shutdown(zlog *zap.Logger) (err error) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	deps, err := e.list(zlog)
	if err != nil {
		return fmt.Errorf("cannot list deployments: %w", err)
	}
	for _, dep := range deps {
		if dep.Status != pbsinksvc.DeploymentStatus_RUNNING && dep.Status != pbsinksvc.DeploymentStatus_FAILING {
			continue
		}
		zlog.Info("shutting down deployment", zap.String("deploymentID", dep.Id))
		if _, e := e.Stop(dep.Id, zlog); e != nil {
			err = errors.Join(err, e)
		}
	}
	return err
}

func (e *DockerEngine) createManifest(deploymentID string, token string, pkg *pbsubstreams.Package) (content []byte, usedPorts []uint32, servicesDesc map[string]string, runMeFirst []string, err error) {

	if pkg.SinkConfig.TypeUrl != "sf.substreams.sink.sql.v1.Service" {
		return nil, nil, nil, nil, fmt.Errorf("invalid sinkconfig type: %q. Only sf.substreams.sink.sql.v1.Service is supported for now.", pkg.SinkConfig.TypeUrl)
	}
	sinkConfig := &pbsql.Service{}
	if err := pkg.SinkConfig.UnmarshalTo(sinkConfig); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("cannot unmarshal sinkconfig: %w", err)
	}

	servicesDesc = make(map[string]string)
	var services []types.ServiceConfig

	var dbServiceName string
	var isPostgres, isClickhouse bool

	switch sinkConfig.Engine {
	case pbsql.Service_clickhouse:
		db, dbMotd, err := e.newClickhouse(deploymentID, pkg)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("creating clickhouse deployment: %w", err)
		}
		dbServiceName = db.Name
		servicesDesc[db.Name] = dbMotd
		runMeFirst = append(runMeFirst, db.Name)
		services = append(services, db)
		isClickhouse = true

	case pbsql.Service_postgres, pbsql.Service_unset:
		pg, pgMotd, err := e.newPostgres(deploymentID, pkg)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("creating postgres deployment: %w", err)
		}
		dbServiceName = pg.Name
		servicesDesc[pg.Name] = pgMotd
		runMeFirst = append(runMeFirst, pg.Name)
		services = append(services, pg)
		isPostgres = true
	}

	sink, sinkMotd, err := e.newSink(deploymentID, dbServiceName, pkg, sinkConfig)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("creating postgres deployment: %w", err)
	}
	servicesDesc[sink.Name] = sinkMotd
	services = append(services, sink)

	if sinkConfig.PgwebFrontend != nil && sinkConfig.PgwebFrontend.Enabled {
		pgweb, motd := e.newPGWeb(deploymentID, dbServiceName)
		servicesDesc[pgweb.Name] = motd
		services = append(services, pgweb)
	}

	if sinkConfig.PostgraphileFrontend != nil && sinkConfig.PostgraphileFrontend.Enabled {
		postgraphile, motd := e.newPostgraphile(deploymentID, dbServiceName)
		servicesDesc[postgraphile.Name] = motd
		services = append(services, postgraphile)
	}

	if sinkConfig.DbtConfig != nil && sinkConfig.DbtConfig.Files != nil {
		var engine string
		if isPostgres {
			engine = "postgres"
		} else if isClickhouse {
			engine = "clickhouse"
		}

		if engine != "" {
			dbt, motd, err := e.newDBT(deploymentID, dbServiceName, sinkConfig.DbtConfig, engine)
			if err != nil {
				return nil, nil, nil, nil, fmt.Errorf("creating dbt deployment: %w", err)
			}
			servicesDesc[dbt.Name] = motd
			services = append(services, dbt)
		}
	}

	if sinkConfig.RestFrontend != nil && sinkConfig.RestFrontend.Enabled {
		rest, motd := e.newRestFrontend(deploymentID, dbServiceName)
		servicesDesc[rest.Name] = motd
		services = append(services, rest)
	}

	for _, svc := range services {
		for _, port := range svc.Ports {
			usedPorts = append(usedPorts, port.Published)
		}
	}

	config := types.Config{
		Version:  "3",
		Services: services,
	}
	content, err = yaml.Marshal(config)
	return
}
