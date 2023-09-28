package docker

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/docker/cli/cli/compose/types"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
   	pbsql "github.com/streamingfast/substreams-sink-sql/pb/sf/substreams/sink/sql/v1"
	"google.golang.org/protobuf/proto"
)

func (e *DockerEngine) newSink(deploymentID string, pgService string, pkg *pbsubstreams.Package) (conf types.ServiceConfig, err error) {

    // FIXME: this should be provided by the request, more checks should be done here
    token := os.Getenv("SUBSTREAMS_API_TOKEN")

	name := fmt.Sprintf("%s-sink", deploymentID)

    configFolder := filepath.Join(e.dir, deploymentID, "config", "sink")
	if err := os.MkdirAll(configFolder, 0755); err != nil {
		return conf, fmt.Errorf("creating folder %q: %w", configFolder, err)
	}

	dataFolder := filepath.Join(e.dir, deploymentID, "data", "sink")
	if err := os.MkdirAll(dataFolder, 0755); err != nil {
		return conf, fmt.Errorf("creating folder %q: %w", dataFolder, err)
	}

	endpoint := "api.streamingfast.io:443"
	switch pkg.Network {
	case "mainnet":
		break
	case "polygon":
		endpoint = "polygon.streamingfast.io:443"
	case "mumbai":
		endpoint = "mumbai.streamingfast.io:443"
	case "arbitrum", "arb-one":
		endpoint = "arb-one.streamingfast.io:443"
	case "solana", "sol-mainnet":
		endpoint = "mainnet.sol.streamingfast.io:443"
	}

	conf = types.ServiceConfig{
		Name:          name,
		ContainerName: name,
		Image:         "ghcr.io/streamingfast/substreams-sink-sql:36cf706",
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
			"DATABASE_URL":  deref("postgres://dev-node:insecure-change-me-in-prod@postgres:5432/dev-node?sslmode=disable"),
			"ENDPOINT":      &endpoint,
			"OUTPUT_MODULE": &pkg.SinkModule,
            "SUBSTREAMS_API_TOKEN": &token,
		},
	}

	pkgContent, err := proto.Marshal(pkg)
	if err != nil {
		return conf, fmt.Errorf("marshalling package: %w", err)
	}

	if err := ioutil.WriteFile(filepath.Join(configFolder, "substreams.spkg"), pkgContent, 0644); err != nil {
		return conf, fmt.Errorf("writing file: %w", err)
	}

    if pkg.SinkConfig.TypeUrl != "sf.substreams.sink.sql.v1.Service" {
        return conf, fmt.Errorf("invalid sinkconfig type: %q", pkg.SinkConfig.TypeUrl)
    }
        sqlSvc := &pbsql.Service{}
		if err := pkg.SinkConfig.UnmarshalTo(sqlSvc); err != nil {
			return types.ServiceConfig{}, fmt.Errorf("failed to proto unmarshal: %w", err)
		}

	if err := ioutil.WriteFile(filepath.Join(configFolder, "schema.sql"), []byte(sqlSvc.Schema), 0644); err != nil {
		return conf, fmt.Errorf("writing file: %w", err)
	}

	startScript := []byte(`#!/bin/bash
set -xeu

if [ ! -f /opt/subservices/data/setup-complete ]; then
    /app/substreams-sink-sql setup /opt/subservices/config/substreams.spkg
    touch /opt/subservices/data/setup-complete
fi

/app/substreams-sink-sql run /opt/subservices/config/substreams.spkg
`)
	if err := ioutil.WriteFile(filepath.Join(configFolder, "start.sh"), startScript, 0755); err != nil {
		fmt.Println("")
		return conf, fmt.Errorf("writing file: %w", err)
	}

	return conf, nil
}
