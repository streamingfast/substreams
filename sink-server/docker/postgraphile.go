package docker

import (
	"fmt"

	"github.com/docker/cli/cli/compose/types"
)

func (e *DockerEngine) newPostgraphile(deploymentID string, pgService string) (conf types.ServiceConfig, motd string) {
	name := fmt.Sprintf("%s-postgraphile", deploymentID)
	localPort := uint32(3000) // TODO: assign dynamically

	conf = types.ServiceConfig{
		Name:          name,
		ContainerName: name,
		Image:         "graphile/postgraphile:4",
		Restart:       "on-failure",
		Ports: []types.ServicePortConfig{
			{
				Published: localPort,
				Target:    5000,
			},
		},
		Command: []string{
			"--connection",
			"postgres://dev-node:insecure-change-me-in-prod@postgres:5432/substreams?sslmode=disable",
			"--watch",
		},
		Links:     []string{pgService + ":postgres"},
		DependsOn: []string{pgService},
	}

	motd = fmt.Sprintf("Postgraphile service %q available at URL: 'http://localhost:%d/graphiql' (API at 'http://localhost:%d/graphql')",
		name,
		localPort,
		localPort,
	)

	return conf, motd

}
