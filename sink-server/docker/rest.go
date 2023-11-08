package docker

import (
	"fmt"

	"github.com/docker/cli/cli/compose/types"
)

func (e *DockerEngine) newRestFrontend(deploymentID string, dbService string) (conf types.ServiceConfig, motd string) {
	name := fmt.Sprintf("%s-rest", deploymentID)
	localPort := uint32(3000) // TODO: assign dynamically

	conf = types.ServiceConfig{
		Name:          name,
		ContainerName: name,
		Image:         "docker.io/dfuse/sql-wrapper:latest",
		Restart:       "on-failure",
		Ports: []types.ServicePortConfig{
			{
				Published: localPort,
				Target:    3000,
			},
		},
		Links:     []string{dbService + ":clickhouse"},
		DependsOn: []string{dbService},
		Environment: map[string]*string{
			"CLICKHOUSE_URL": deref("tcp://dev-node:insecure-change-me-in-prod@clickhouse:9000/substreams?secure=false&skip_verify=true&connection_timeout=20s"),
		},
	}

	motd = fmt.Sprintf("REST frontend service %q available at URL: 'http://localhost:%d'",
		name,
		localPort,
	)

	return conf, motd
}
