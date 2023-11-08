package docker

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	pbsql "github.com/streamingfast/substreams-sink-sql/pb/sf/substreams/sink/sql/v1"

	"github.com/docker/cli/cli/compose/types"
	"gopkg.in/yaml.v2"
)

func (e *DockerEngine) newDBT(deploymentID string, serviceName string, config *pbsql.DBTConfig, engine string) (types.ServiceConfig, string, error) {
	name := fmt.Sprintf("%s-dbt", deploymentID)

	var conf types.ServiceConfig

	dbtFiles, err := getFiles(config.Files)
	if err != nil {
		return conf, "", fmt.Errorf("unable to get dbt files: %w", err)
	}

	//get the dbt_project.yml file
	dbtProjectYml, ok := dbtFiles["dbt_project.yml"]
	if !ok {
		return conf, "", fmt.Errorf("unable to get dbt_project.yml file")
	}

	//get profile name from yaml file
	dbtProfileName, err := extractProfileName(dbtProjectYml)
	if err != nil {
		return conf, "", fmt.Errorf("unable to extract profile name from dbt_project.yml: %w", err)
	}

	//create dbt_project.yml file
	dbtProfileYaml, err := createDbtProfileYml(dbtProfileName, engine)
	if err != nil {
		return conf, "", fmt.Errorf("unable to create dbt_project.yml file: %w", err)
	}

	runInterval := config.GetRunIntervalSeconds()
	if runInterval == 0 {
		runInterval = 300 //default to 5 minutes
	}

	//create dbt start script
	dbtStartScript := createDbtStartScript(runInterval)

	//create a volume for the files
	profileFolder := filepath.Join(e.dir, deploymentID, "data", "profile")
	if err := os.MkdirAll(profileFolder, 0755); err != nil {
		return conf, "", fmt.Errorf("creating folder %q: %w", profileFolder, err)
	}

	dataFolder := filepath.Join(e.dir, deploymentID, "data", "dbt")
	if err := os.MkdirAll(dataFolder, 0755); err != nil {
		return conf, "", fmt.Errorf("creating folder %q: %w", profileFolder, err)
	}

	scriptFolder := filepath.Join(e.dir, deploymentID, "data", "scripts")
	if err := os.MkdirAll(scriptFolder, 0755); err != nil {
		return conf, "", fmt.Errorf("creating folder %q: %w", scriptFolder, err)
	}

	err = os.WriteFile(filepath.Join(e.dir, deploymentID, "data", "profile", "profiles.yml"), dbtProfileYaml, 0644)
	if err != nil {
		return conf, "", fmt.Errorf("writing profiles.yml file: %w", err)
	}

	//copy all dbt files
	for fileName, fileContent := range dbtFiles {
		//create the path if it doesn't exist
		if err := os.MkdirAll(filepath.Join(e.dir, deploymentID, "data", "dbt", filepath.Dir(fileName)), 0755); err != nil {
			return conf, "", fmt.Errorf("creating folder %q: %w", filepath.Join(e.dir, deploymentID, "data", "dbt", filepath.Dir(fileName)), err)
		}

		err = os.WriteFile(filepath.Join(e.dir, deploymentID, "data", "dbt", fileName), fileContent, 0644)
		if err != nil {
			return conf, "", fmt.Errorf("writing dbt file %s: %w", fileName, err)
		}
	}

	//copy dbt start script
	err = os.WriteFile(filepath.Join(e.dir, deploymentID, "data", "scripts", "start.sh"), dbtStartScript, 0755)
	if err != nil {
		return conf, "", fmt.Errorf("writing dbt start script: %w", err)
	}

	//chmod +x the start script
	err = os.Chmod(filepath.Join(e.dir, deploymentID, "data", "scripts", "start.sh"), 0755)
	if err != nil {
		return conf, "", fmt.Errorf("chmod +x start script: %w", err)
	}

	conf = types.ServiceConfig{
		Name:          name,
		ContainerName: name,
		Image:         getDBTDockerImage(engine),
		Restart:       "on-failure",
		Environment: map[string]*string{
			"DBT_PROFILES_DIR": deref("/opt/data/profile"),
		},
		Entrypoint: []string{
			"/bin/bash",
		},
		Command: []string{
			"/opt/data/scripts/start.sh",
		},
		Links:     []string{serviceName + ":" + engine},
		DependsOn: []string{serviceName},
		Volumes: []types.ServiceVolumeConfig{
			{
				Type:   "bind",
				Source: "./data/profile",
				Target: "/opt/data/profile",
			},
			{
				Type:   "bind",
				Source: "./data/dbt",
				Target: "/opt/data/dbt",
			},
			{
				Type:   "bind",
				Source: "./data/scripts",
				Target: "/opt/data/scripts",
			},
		},
		HealthCheck: &types.HealthCheckConfig{
			Test:     []string{"CMD", "dbt", "debug"},
			Interval: toDuration(time.Second * 300),
			Timeout:  toDuration(time.Second * 60),
			Retries:  deref(uint64(10)),
		},
	}

	return conf, name, nil
}

func getDBTDockerImage(engine string) string {
	switch engine {
	case "postgres":
		return "ghcr.io/dbt-labs/dbt-postgres:1.4.9"
	case "clickhouse":
		return "docker.io/dfuse/dbt-clickhouse:1.4.9"
	default:
		return ""
	}
}

func getFiles(archive []byte) (map[string][]byte, error) {
	contentMap := make(map[string][]byte)

	// Read the zipped content from the Files field
	r, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, err
	}

	// Loop over the files in the archive
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()

		content, err := io.ReadAll(rc)
		if err != nil {
			return nil, err
		}

		contentMap[f.Name] = content
	}
	return contentMap, nil
}

func extractProfileName(dbtProjectYaml []byte) (string, error) {
	//unmarshal yaml file bytes
	var data map[string]interface{}
	err := yaml.Unmarshal(dbtProjectYaml, &data)
	if err != nil {
		return "", err
	}

	if profileValue, ok := data["profile"]; ok {
		profileName := profileValue.(string)
		return profileName, nil
	}

	return "", fmt.Errorf("unable to extract profile name from dbt_project.yml")
}

func createDbtProfileYml(profileName string, engine string) ([]byte, error) {
	port := 5432
	host := "postgres"
	switch engine {
	case "clickhouse":
		port = 9000
		host = "clickhouse"
	}

	data := fmt.Sprintf(`%s:
  outputs:
    dev:
      driver: native
      type: %s
      threads: 1
      host: %s
      port: %d
      user: dev-node
      password: insecure-change-me-in-prod
      dbname: substreams
      schema: public
  target: dev
`, profileName, engine, host, port)

	return []byte(data), nil
}

func createDbtStartScript(runInterval int32) []byte {
	return []byte(fmt.Sprintf(`#!/bin/bash

# Set the working directory
cd /opt/data/dbt

while true; do
	dbt run --profiles-dir /opt/data/profile --target dev
    sleep %d  
done
`, runInterval))
}
