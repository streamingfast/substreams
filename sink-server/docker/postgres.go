package docker

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/docker/cli/cli/compose/types"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func (e *DockerEngine) newPostgres(deploymentID string, pkg *pbsubstreams.Package) (types.ServiceConfig, error) {

	name := fmt.Sprintf("%s-postgres", deploymentID)

	dataFolder := filepath.Join(e.dir, deploymentID, "data", "postgres")
	if err := os.MkdirAll(dataFolder, 0755); err != nil {
		return types.ServiceConfig{}, fmt.Errorf("creating folder %q: %w", dataFolder, err)
	}

	return types.ServiceConfig{
		Name:          name,
		ContainerName: name,
		Image:         "postgres:14",
		Restart:       "on-failure",
		Ports: []types.ServicePortConfig{
			{
				Published: 5432,
				Target:    5432,
			},
		},
		Command: []string{
			"postgres",
			"-cshared_preload_libraries=pg_stat_statements",
		},
		Environment: map[string]*string{
			"POSTGRES_USER":             deref("dev-node"),
			"POSTGRES_PASSWORD":         deref("insecure-change-me-in-prod"),
			"POSTGRES_DB":               deref("dev-node"),
			"POSTGRES_INITDB_ARGS":      deref("-E UTF8 --locale=C"),
			"POSTGRES_HOST_AUTH_METHOD": deref("md5"),
		},
		Volumes: []types.ServiceVolumeConfig{
			{
				Type:   "bind",
				Source: "./data/postgres",
				Target: "/var/lib/postgresql/data",
			},
		},
		HealthCheck: &types.HealthCheckConfig{
			Test:     []string{"CMD", "nc", "-z", "localhost", "5432"},
			Interval: toDuration(time.Second * 30),
			Timeout:  toDuration(time.Second * 10),
			Retries:  deref(uint64(15)),
		},
	}, os.MkdirAll(dataFolder, 0755)
}
