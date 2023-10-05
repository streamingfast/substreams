package docker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/cli/cli/compose/types"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"google.golang.org/protobuf/proto"
)

func (e *DockerEngine) newSink(deploymentID string, pgService string, pkg *pbsubstreams.Package) (conf types.ServiceConfig, motd string, err error) {

	name := sinkServiceName(deploymentID)

	configFolder := filepath.Join(e.dir, deploymentID, "config", "sink")
	if err := os.MkdirAll(configFolder, 0755); err != nil {
		return conf, motd, fmt.Errorf("creating folder %q: %w", configFolder, err)
	}

	dataFolder := filepath.Join(e.dir, deploymentID, "data", "sink")
	if err := os.MkdirAll(dataFolder, 0755); err != nil {
		return conf, motd, fmt.Errorf("creating folder %q: %w", dataFolder, err)
	}

	conf = types.ServiceConfig{
		Name:          name,
		ContainerName: name,
		Image:         "ghcr.io/streamingfast/substreams-sink-sql:v3.0.2",
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
		Links:     []string{pgService + ":postgres"},
		DependsOn: []string{pgService},
		Environment: map[string]*string{
			"DSN":                  deref("postgres://dev-node:insecure-change-me-in-prod@postgres:5432/dev-node?sslmode=disable"),
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

	startScript := []byte(`#!/bin/bash
set -xeu

if [ ! -f /opt/subservices/data/setup-complete ]; then
    /app/substreams-sink-sql setup $DSN /opt/subservices/config/substreams.spkg --postgraphile && touch /opt/subservices/data/setup-complete
fi

/app/substreams-sink-sql run $DSN /opt/subservices/config/substreams.spkg --on-module-hash-mistmatch=warn
`)
	if err := os.WriteFile(filepath.Join(configFolder, "start.sh"), startScript, 0755); err != nil {
		fmt.Println("")
		return conf, motd, fmt.Errorf("writing file: %w", err)
	}

	return conf, motd, nil
}

func sinkServiceName(deploymentID string) string {
	return deploymentID + "-sink"
}
