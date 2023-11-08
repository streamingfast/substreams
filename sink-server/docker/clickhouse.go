package docker

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/docker/cli/cli/compose/types"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func (e *DockerEngine) newClickhouse(deploymentID string, pkg *pbsubstreams.Package) (types.ServiceConfig, string, error) {
	name := fmt.Sprintf("%s-clickhouse", deploymentID)

	dataFolder := filepath.Join(e.dir, deploymentID, "data", "clickhouse")
	if err := os.MkdirAll(dataFolder, 0755); err != nil {
		return types.ServiceConfig{}, "", fmt.Errorf("creating folder %q: %w", dataFolder, err)
	}

	pgPort := uint32(5432) // TODO: assign dynamically
	clickhousePort := uint32(9000)

	conf := types.ServiceConfig{
		Name:          name,
		ContainerName: name,
		Image:         "clickhouse/clickhouse-server:23.3-alpine",
		Restart:       "on-failure",
		Ports: []types.ServicePortConfig{
			{
				Published: pgPort,
				Target:    9005,
			},
			{
				Published: clickhousePort,
				Target:    9000,
			},
		},

		Environment: map[string]*string{

			"CLICKHOUSE_USER":                      deref("dev-node"),
			"CLICKHOUSE_PASSWORD":                  deref("insecure-change-me-in-prod"),
			"CLICKHOUSE_DB":                        deref("substreams"),
			"CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT": deref("1"),
			//"POSTGRES_INITDB_ARGS":      deref("-E UTF8 --locale=C"),
			//"POSTGRES_HOST_AUTH_METHOD": deref("md5"),
		},
		Volumes: []types.ServiceVolumeConfig{
			{
				Type:   "bind",
				Source: "./data/clickhouse",
				Target: "/var/lib/clickhouse",
			},
		},
		HealthCheck: &types.HealthCheckConfig{
			Test:     []string{"CMD", "clickhouse", "status"},
			Interval: toDuration(time.Second * 5),
			Timeout:  toDuration(time.Second * 4),
			Retries:  deref(uint64(10)),
		},
	}

	motd := fmt.Sprintf("Clickhouse service %q available at DSN: 'clickhouse://%s:%s@localhost:%d/%s', connect to CLI using 'docker exec -ti %s clickhouse client'",
		name,
		*conf.Environment["CLICKHOUSE_USER"],
		*conf.Environment["CLICKHOUSE_PASSWORD"],
		clickhousePort,
		*conf.Environment["CLICKHOUSE_DB"],
		name,
	)

	return conf, motd, nil
}
