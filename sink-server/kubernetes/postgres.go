package kubernetes

import (
	"context"
	"fmt"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"
)

func (k *KubernetesEngine) newPostgres(ctx context.Context, deploymentID string, pkg *pbsubstreams.Package) (createFunc, error) {
	//create a stateful set object
	name := fmt.Sprintf("postgres-%s", deploymentID)

	labels := map[string]string{
		"expiration": getExpirationLabelValue(ctx),
		"deployment": deploymentID,
		"app":        "postgres",
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
					Port:     5432,
					Name:     "postgres",
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
							Image: "postgres:14",
							Ports: []corev1.ContainerPort{
								{
									Name:          "postgres",
									Protocol:      corev1.ProtocolTCP,
									ContainerPort: 5432,
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
									MountPath: "/opt/subservices/data",
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "POSTGRES_USER",
									Value: "dev-node",
								},
								{
									Name:  "POSTGRES_PASSWORD",
									Value: "insecure-change-me-in-prod",
								},
								{
									Name:  "POSTGRES_DB",
									Value: "substreams",
								},
								{
									Name:  "POSTGRES_INITDB_ARGS",
									Value: "-E UTF8 --locale=C",
								},
								{
									Name:  "POSTGRES_HOST_AUTH_METHOD",
									Value: "md5",
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

	k.dbDSNs[deploymentID] = fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		"dev-node",
		"insecure-change-me-in-prod",
		name,
		5432,
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
