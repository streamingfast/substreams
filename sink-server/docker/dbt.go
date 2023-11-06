package docker

import (
	"archive/zip"
	"bytes"
	"fmt"
	"github.com/docker/cli/cli/compose/types"
	pbsql "github.com/streamingfast/substreams-sink-sql/pb/sf/substreams/sink/sql/v1"
	"gopkg.in/yaml.v2"
	"io"
	"os"
	"path/filepath"
)

func (e *DockerEngine) newPostgresDBT(deploymentID string, serviceName string, config *pbsql.DBTConfig) (types.ServiceConfig, string, error) {
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
	dbtProfileYaml, err := createDbtProfileYml(dbtProfileName)
	if err != nil {
		return conf, "", fmt.Errorf("unable to create dbt_project.yml file: %w", err)
	}

	//create dbt start script
	dbtStartScript := createDbtStartScript()

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

	name := fmt.Sprintf("%s-dbt", deploymentID)
	conf = types.ServiceConfig{
		Name:          name,
		ContainerName: name,
		Image:         "ghcr.io/dbt-labs/dbt-postgres:1.4.9",
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
		Links:     []string{serviceName + ":postgres"},
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
	}

	return conf, name, nil
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

func createDbtProfileYml(profileName string) ([]byte, error) {
	data := fmt.Sprintf(`%s:
  outputs:
    dev:
      type: postgres
      threads: 1
      host: postgres
      port: 5432
      user: dev-node
      pass: insecure-change-me-in-prod
      dbname: substreams
      schema: public
  target: dev
`, profileName)

	return []byte(data), nil
}

func createDbtStartScript() []byte {
	return []byte(`#!/bin/bash

# Set the working directory
cd /opt/data/dbt

while true; do
	dbt run --profiles-dir /opt/data/profile --target dev
    sleep 60  # Sleep for 60 seconds (1 minute)
done
`)
}
