package docker

import (
	"fmt"

	"github.com/docker/cli/cli/compose/types"
)

func (e *DockerEngine) newSinkInfo(deploymentID string, dbService string, dbType string) (conf types.ServiceConfig, motd string) {
	name := fmt.Sprintf("%s-sinkinfo", deploymentID)
	localPort := uint32(8282) // TODO: assign dynamically

	conf = types.ServiceConfig{
		Name:          name,
		ContainerName: name,
		Image:         "dfuse/sqlsinkinfo:latest",
		Restart:       "on-failure",
		Ports: []types.ServicePortConfig{
			{
				Published: localPort,
				Target:    8282,
			},
		},
		Command: []string{
			fmt.Sprintf("%s://dev-node:insecure-change-me-in-prod@postgres:5432/substreams?sslmode=disable", dbType),
		},
		Links:     []string{dbService + ":" + dbType},
		DependsOn: []string{dbService},
	}

	motd = fmt.Sprintf("Sink info service %q available at URL: 'http://localhost:%d/sinkinfo'",
		name,
		localPort,
	)

	return conf, motd

}
