package docker

import (
	"fmt"

	"github.com/docker/cli/cli/compose/types"
)

func (e *DockerEngine) newPGWeb(deploymentID string, dbService string) (conf types.ServiceConfig, motd string) {
	name := fmt.Sprintf("%s-pgweb", deploymentID)
	localPort := uint32(8081) // TODO: assign dynamically

	conf = types.ServiceConfig{
		Name:          name,
		ContainerName: name,
		Image:         "sosedoff/pgweb:0.11.12",
		Restart:       "on-failure",
		Ports: []types.ServicePortConfig{
			{
				Published: localPort,
				Target:    8081,
			},
		},
		Command: []string{
			"pgweb",
			"--bind=0.0.0.0",
			"--listen=8081",
			"--binary-codec=hex",
		},
		Links:     []string{dbService + ":postgres"},
		DependsOn: []string{dbService},
		Environment: map[string]*string{
			"DATABASE_URL": deref("postgres://dev-node:insecure-change-me-in-prod@postgres:5432/substreams?sslmode=disable"),
		},
	}

	motd = fmt.Sprintf("PGWeb service %q available at URL: 'http://localhost:%d'",
		name,
		localPort,
	)

	return conf, motd

}
