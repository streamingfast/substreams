package docker

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/docker/cli/cli/compose/types"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func (e *DockerEngine) newPostgres(deploymentID string, pkg *pbsubstreams.Package) (types.ServiceConfig, string, error) {

	name := fmt.Sprintf("%s-postgres", deploymentID)

	dataFolder := filepath.Join(e.dir, deploymentID, "data", "postgres")
	if err := os.MkdirAll(dataFolder, 0755); err != nil {
		return types.ServiceConfig{}, "", fmt.Errorf("creating folder %q: %w", dataFolder, err)
	}

    localPort := uint32(5432) // TODO: assign dynamically


    conf := types.ServiceConfig{
		Name:          name,
		ContainerName: name,
		Image:         "postgres:14",
		Restart:       "on-failure",
		Ports: []types.ServicePortConfig{
			{
				Published: localPort,
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
	} 

    motd := fmt.Sprintf("PostgreSQL service %q available at DSN: 'postgres://%s:%s@localhost:%d/%s?sslmode=disable'",
        name,
        *conf.Environment["POSTGRES_USER"],
        *conf.Environment["POSTGRES_PASSWORD"],
        localPort,
        *conf.Environment["POSTGRES_DB"],
     )

    return conf, motd, nil
}
