package docker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/cli/cli/compose/types"
	pbsql "github.com/streamingfast/substreams-sink-sql/pb/sf/substreams/sink/sql/v1"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"google.golang.org/protobuf/proto"
)

func (e *DockerEngine) newSink(deploymentID string, dbService string, pkg *pbsubstreams.Package, sinkConfig *pbsql.Service) (conf types.ServiceConfig, motd string, err error) {
	name := sinkServiceName(deploymentID)

	configFolder := filepath.Join(e.dir, deploymentID, "config", "sink")
	if err := os.MkdirAll(configFolder, 0755); err != nil {
		return conf, motd, fmt.Errorf("creating folder %q: %w", configFolder, err)
	}

	dataFolder := filepath.Join(e.dir, deploymentID, "data", "sink")
	if err := os.MkdirAll(dataFolder, 0755); err != nil {
		return conf, motd, fmt.Errorf("creating folder %q: %w", dataFolder, err)
	}

	var dsn string
	var serviceName string
	switch sinkConfig.Engine {
	case pbsql.Service_clickhouse:
		dsn = "clickhouse://dev-node:insecure-change-me-in-prod@clickhouse:9000/substreams"
		if sinkConfig.PostgraphileFrontend != nil && sinkConfig.PostgraphileFrontend.Enabled {
			return conf, motd, fmt.Errorf("postgraphile not supported on clickhouse")
		}
		serviceName = "clickhouse"
	case pbsql.Service_postgres:
		dsn = "postgres://dev-node:insecure-change-me-in-prod@postgres:5432/substreams?sslmode=disable"
		serviceName = "postgres"
	default:
		return conf, motd, fmt.Errorf("unknown service %q", sinkConfig.Engine)
	}

	conf = types.ServiceConfig{
		Name:          name,
		ContainerName: name,
		Image:         "ghcr.io/streamingfast/substreams-sink-sql:v3.0.4",
		Restart:       "on-failure",
		Entrypoint: []string{
			"/opt/subservices/config/start.sh",
		},
		Volumes: []types.ServiceVolumeConfig{
			{
				Type:   "bind",
				Source: "./data/sink",
				Target: "/opt/subservices/data",
			},
			{
				Type:   "bind",
				Source: "./config/sink",
				Target: "/opt/subservices/config",
			},
		},
		Links:     []string{dbService + ":" + serviceName},
		DependsOn: []string{dbService},
		Environment: map[string]*string{
			"DSN":                  &dsn,
			"OUTPUT_MODULE":        &pkg.SinkModule,
			"SUBSTREAMS_API_TOKEN": &e.token,
		},
	}

	motd = fmt.Sprintf("Sink service (no exposed port). Use 'substreams alpha sink-info %s' to see last processed block or 'docker logs %s' to see the logs.", strings.ReplaceAll(name, "-sink", ""), name)

	pkgContent, err := proto.Marshal(pkg)
	if err != nil {
		return conf, motd, fmt.Errorf("marshalling package: %w", err)
	}

	if err := os.WriteFile(filepath.Join(configFolder, "substreams.spkg"), pkgContent, 0644); err != nil {
		return conf, motd, fmt.Errorf("writing file: %w", err)
	}

	withPostgraphile := ""
	if sinkConfig.PostgraphileFrontend != nil && sinkConfig.PostgraphileFrontend.Enabled {
		withPostgraphile = "--postgraphile"
	}

	startScript := []byte(fmt.Sprintf(`#!/bin/bash
set -xeu

if [ ! -f /opt/subservices/data/setup-complete ]; then
    /app/substreams-sink-sql setup $DSN /opt/subservices/config/substreams.spkg %s && touch /opt/subservices/data/setup-complete
fi

/app/substreams-sink-sql run $DSN /opt/subservices/config/substreams.spkg --on-module-hash-mistmatch=warn
`, withPostgraphile))
	if err := os.WriteFile(filepath.Join(configFolder, "start.sh"), startScript, 0755); err != nil {
		fmt.Println("")
		return conf, motd, fmt.Errorf("writing file: %w", err)
	}

	return conf, motd, nil
}

func sinkServiceName(deploymentID string) string {
	return deploymentID + "-sink"
}
