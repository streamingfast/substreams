package docker

import (
	"fmt"

	"github.com/docker/cli/cli/compose/types"
)

func (e *DockerEngine) newPGWeb(deploymentID string, pgService string) (conf types.ServiceConfig) {

	name := fmt.Sprintf("%s-pgweb", deploymentID)

	return types.ServiceConfig{
		Name:          name,
		ContainerName: name,
		Image:         "sosedoff/pgweb:0.11.12",
		Restart:       "on-failure",
		Ports: []types.ServicePortConfig{
			{
				Published: 8081,
				Target:    8081,
			},
		},
		Command: []string{
			"pgweb",
			"--bind=0.0.0.0",
			"--listen=8081",
			"--binary-codec=hex",
		},
		Links:     []string{pgService + ":postgres"},
		DependsOn: []string{pgService},
		Environment: map[string]*string{
			"DATABASE_URL": deref("postgres://dev-node:insecure-change-me-in-prod@postgres:5432/dev-node?sslmode=disable"),
		},
	}

}
