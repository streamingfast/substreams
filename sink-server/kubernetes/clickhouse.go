package kubernetes

import (
	"context"
	"fmt"
	"time"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func (k *KubernetesEngine) newClickhouse(ctx context.Context, deploymentID string) (createFunc, error) {
	name := fmt.Sprintf("clickhouse-%s", deploymentID)

	labels := map[string]string{
		"expiration": getExpirationLabelValue(ctx),
		"deployment": deploymentID,
		"app":        "clickhouse",
		"component":  "substreams-sink-sql",
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "None",
			Selector:  labels,
			Ports: []corev1.ServicePort{
				{
					Port: 5432,
					TargetPort: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: 9005,
					},
					Name:     "pgport",
					Protocol: corev1.ProtocolTCP,
				},
				{
					Port: 9000,
					TargetPort: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: 9000,
					},
					Name:     "clickhouse",
					Protocol: corev1.ProtocolTCP,
				},
			},
		},
	}

	sts := v1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
		Spec: v1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			ServiceName: name,
			Replicas:    ref(int32(1)),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "datadir",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: fmt.Sprintf("datadir-%s", name),
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:  "postgres",
							Image: "clickhouse/clickhouse-server:23.3-alpine",
							Ports: []corev1.ContainerPort{
								{
									Name:          "clickhouse",
									Protocol:      corev1.ProtocolTCP,
									ContainerPort: 9000,
								},
								{
									Name:          "pgport",
									Protocol:      corev1.ProtocolTCP,
									ContainerPort: 9005,
								},
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
									MountPath: "/var/lib/clickhouse",
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "CLICKHOUSE_USER",
									Value: "dev-node",
								},
								{
									Name:  "CLICKHOUSE_PASSWORD",
									Value: "insecure-change-me-in-prod",
								},
								{
									Name:  "CLICKHOUSE_DB",
									Value: "substreams",
								},
								{
									Name:  "CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT",
									Value: "1",
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

	k.dbDSNs[deploymentID] = fmt.Sprintf("clickhouse://%s:%s@%s:%d/%s",
		"dev-node",
		"insecure-change-me-in-prod",
		name,
		9000,
		"substreams",
	)

	return func(ctx context.Context) ([]*metav1.ObjectMeta, error) {
		var res []*metav1.ObjectMeta

		service, err := k.clientSet.CoreV1().Services(k.namespace).Create(ctx, svc, metav1.CreateOptions{})
		if err != nil {
			return res, fmt.Errorf("creating service: %w", err)
		}
		res = append(res, &service.ObjectMeta)

		obj, err := k.clientSet.AppsV1().StatefulSets(k.namespace).Create(ctx, &sts, metav1.CreateOptions{})
		if err != nil {
			return res, fmt.Errorf("creating statefulset: %w", err)
		}
		if err := waitForStateFulSet(ctx, k.clientSet, k.namespace, name, 5*time.Minute); err != nil {
			return res, fmt.Errorf("waiting for statefulset: %w", err)
		}
		res = append(res, &obj.ObjectMeta)

		return res, nil
	}, nil
}
