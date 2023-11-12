package kubernetes

import (
	"context"
	"fmt"
	pbsql "github.com/streamingfast/substreams-sink-sql/pb/sf/substreams/sink/sql/v1"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"google.golang.org/protobuf/proto"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"
)

func (k *KubernetesEngine) newSink(ctx context.Context, deploymentID string, dbService string, pkg *pbsubstreams.Package, sinkConfig *pbsql.Service) (createFunc, error) {
	name := fmt.Sprintf("sink-%s", deploymentID)

	labels := map[string]string{
		"expiration": getExpirationLabelValue(ctx),
		"deployment": deploymentID,
		"app":        "sink",
		"component":  "substreams-sink-sql",
	}

	startScriptConfigMap := "postgres-start"
	if sinkConfig.PostgraphileFrontend != nil && sinkConfig.PostgraphileFrontend.Enabled {
		startScriptConfigMap = "postgres-postgraphile-start"
	}

	pkgContent, err := proto.Marshal(pkg)
	if err != nil {
		return nil, fmt.Errorf("marshaling package: %w", err)
	}

	// Create ConfigMap for the package content
	cm := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
		BinaryData: map[string][]byte{
			"substreams.spkg": pkgContent,
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
							Name: "datadir",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: fmt.Sprintf("datadir-%s", name),
								},
							},
						},
						{
							Name: "spkg",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: name,
									},
								},
							},
						},
						{
							Name: startScriptConfigMap,
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: startScriptConfigMap,
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
								"cp /opt/subservices/config/start.sh /opt/subservices/script/start.sh && chmod +x /opt/subservices/script/start.sh",
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      startScriptConfigMap,
									MountPath: "/opt/subservices/config",
								},
								{
									Name:      "script-volume",
									MountPath: "/opt/subservices/script",
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:  "sink",
							Image: "ghcr.io/streamingfast/substreams-sink-sql:v3.0.4",
							Command: []string{
								"/opt/subservices/script/start.sh",
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
									Name:      "datadir",
									MountPath: "/opt/subservices/data",
								},
								{
									Name:      "script-volume",
									MountPath: "/opt/subservices/script",
								},
								{
									Name:      "spkg",
									MountPath: "/opt/subservices/config",
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "DSN",
									Value: k.dbDSNs[deploymentID],
								},
								{
									Name:  "OUTPUT_MODULE",
									Value: pkg.SinkModule,
								},
								{
									Name:  "SUBSTREAMS_API_TOKEN",
									Value: k.apiToken,
								},
							},
						},
					},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:   fmt.Sprintf("datadir"),
						Labels: labels,
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{
							corev1.ReadWriteOnce,
						},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse("32Gi"),
							},
						},
						StorageClassName: ref("gcpssd-lazy"),
						VolumeMode:       ref(corev1.PersistentVolumeFilesystem),
					},
				},
			},
		},
	}

	return func(ctx context.Context) ([]*metav1.ObjectMeta, error) {
		var res []*metav1.ObjectMeta

		_, err := k.clientSet.CoreV1().ConfigMaps(k.namespace).Create(ctx, &cm, metav1.CreateOptions{})
		if err != nil {
			return nil, fmt.Errorf("creating configmap: %w", err)
		}
		res = append(res, &cm.ObjectMeta)

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
