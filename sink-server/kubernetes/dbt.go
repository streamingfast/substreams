package kubernetes

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	pbsql "github.com/streamingfast/substreams-sink-sql/pb/sf/substreams/sink/sql/v1"
	"io"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"

	"gopkg.in/yaml.v2"
)

func (k *KubernetesEngine) newDBT(ctx context.Context, deploymentID string, config *pbsql.DBTConfig, engine string) (createFunc, error) {
	name := fmt.Sprintf("dbt-%s", deploymentID)

	labels := map[string]string{
		"expiration": getExpirationLabelValue(ctx),
		"deployment": deploymentID,
		"app":        "dbt",
		"component":  "substreams-sink-sql",
	}

	dbtFiles, err := getFiles(config.Files)
	if err != nil {
		return nil, fmt.Errorf("unable to get dbt files: %w", err)
	}

	//get the dbt_project.yml file
	dbtProjectYml, ok := dbtFiles["dbt_project.yml"]
	if !ok {
		return nil, fmt.Errorf("unable to get dbt_project.yml file")
	}

	//get profile name from yaml file
	dbtProfileName, err := extractProfileName(dbtProjectYml)
	if err != nil {
		return nil, fmt.Errorf("unable to extract profile name from dbt_project.yml: %w", err)
	}

	dbtProfileYaml, err := createDbtProfileYml(dbtProfileName, engine)
	if err != nil {
		return nil, fmt.Errorf("unable to create dbt_project.yml file: %w", err)
	}

	//create a configmap for this
	profileConfigMap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:   fmt.Sprintf("dbt-profile-%s", deploymentID),
			Labels: labels,
		},
		BinaryData: map[string][]byte{
			"profiles.yml": dbtProfileYaml,
		},
	}

	//files
	dbtFilesMap := make(map[string][]byte)
	for fileName, fileContent := range dbtFiles {
		dbtFilesMap[fileName] = fileContent
	}
	filesConfigMap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:   fmt.Sprintf("dbt-files-%s", deploymentID),
			Labels: labels,
		},
		BinaryData: dbtFilesMap,
	}

	//start script
	startScript := createDbtStartScript(config.GetRunIntervalSeconds())
	startScriptConfigMap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:   fmt.Sprintf("dbt-start-%s", deploymentID),
			Labels: labels,
		},
		BinaryData: map[string][]byte{
			"start.sh": startScript,
		},
	}

	// Create Deployment or StatefulSet
	sts := v1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
		Spec: v1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Replicas: ref(int32(1)),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "script-volume",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
						{
							Name: startScriptConfigMap.Name,
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: startScriptConfigMap.Name,
									},
								},
							},
						},
						{
							Name: profileConfigMap.Name,
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: profileConfigMap.Name,
									},
								},
							},
						},
					},
					InitContainers: []corev1.Container{
						{
							Name:  "init",
							Image: "ubuntu:20.04",
							Command: []string{
								"/bin/bash",
								"-c",
							},
							Args: []string{
								"cp /opt/data/config/start.sh /opt/data/script/start.sh && chmod +x /opt/data/script/start.sh",
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      startScriptConfigMap.Name,
									MountPath: "/opt/data/config",
								},
								{
									Name:      "script-volume",
									MountPath: "/opt/data/script",
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:  "sink",
							Image: getDBTDockerImage(engine),
							Command: []string{
								"/bin/bash",
								"-c",
							},
							Args: []string{
								"/opt/data/script/start.sh",
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("500Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("1Gi"),
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      profileConfigMap.Name,
									MountPath: "/opt/data/profile",
								},
								{
									Name:      filesConfigMap.Name,
									MountPath: "/opt/data/dbt",
								},
								{
									Name:      "script-volume",
									MountPath: "/opt/data/script",
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "DBT_PROFILES_DIR",
									Value: "/opt/data/profile",
								},
							},
						},
					},
				},
			},
		},
	}

	return func(ctx context.Context) ([]*metav1.ObjectMeta, error) {
		var res []*metav1.ObjectMeta

		_, err := k.clientSet.CoreV1().ConfigMaps(k.namespace).Create(ctx, &profileConfigMap, metav1.CreateOptions{})
		if err != nil {
			return nil, fmt.Errorf("creating configmap: %w", err)
		}
		res = append(res, &profileConfigMap.ObjectMeta)

		_, err = k.clientSet.CoreV1().ConfigMaps(k.namespace).Create(ctx, &filesConfigMap, metav1.CreateOptions{})
		if err != nil {
			return nil, fmt.Errorf("creating configmap: %w", err)
		}
		res = append(res, &filesConfigMap.ObjectMeta)

		_, err = k.clientSet.CoreV1().ConfigMaps(k.namespace).Create(ctx, &startScriptConfigMap, metav1.CreateOptions{})
		if err != nil {
			return nil, fmt.Errorf("creating configmap: %w", err)
		}
		res = append(res, &startScriptConfigMap.ObjectMeta)

		_, err = k.clientSet.AppsV1().StatefulSets(k.namespace).Create(ctx, &sts, metav1.CreateOptions{})
		if err != nil {
			return nil, fmt.Errorf("creating statefulset: %w", err)
		}
		if err := waitForStateFulSet(ctx, k.clientSet, k.namespace, name, 5*time.Minute); err != nil {
			return res, fmt.Errorf("waiting for statefulset: %w", err)
		}
		res = append(res, &sts.ObjectMeta)

		return res, nil
	}, nil
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
