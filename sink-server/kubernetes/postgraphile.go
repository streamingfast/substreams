package kubernetes

import (
	"context"
	"fmt"
	"time"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (k *KubernetesEngine) newPostgraphile(ctx context.Context, deploymentID string) (createFunc, error) {
	name := fmt.Sprintf("postgraphile-%s", deploymentID)

	//create a kubernets deployment object
	labels := map[string]string{
		"expiration": getExpirationLabelValue(ctx),
		"deployment": deploymentID,
		"app":        "postgraphile",
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
					Port:     5000,
					Name:     "graphql",
					Protocol: corev1.ProtocolTCP,
				},
			},
		},
	}

	// Create Deployment or StatefulSet
	deployment := v1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
		Spec: v1.DeploymentSpec{
			Replicas: ref(int32(1)),
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  name,
							Image: "graphile/postgraphile:4",
							Args: []string{
								"--connection",
								fmt.Sprintf("postgres://dev-node:insecure-change-me-in-prod@postgres-%s:5432/substreams?sslmode=disable", deploymentID),
								"--watch",
							},
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 5000,
									Name:          "graphql",
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("500Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("500Mi"),
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

		service, err := k.clientSet.CoreV1().Services(k.namespace).Create(ctx, svc, metav1.CreateOptions{})
		if err != nil {
			return res, fmt.Errorf("creating service: %w", err)
		}
		res = append(res, &service.ObjectMeta)

		d, err := k.clientSet.AppsV1().Deployments(k.namespace).Create(ctx, &deployment, metav1.CreateOptions{})
		if err != nil {
			return nil, fmt.Errorf("unable to create deployment: %w", err)
		}
		err = waitForDeployment(ctx, k.clientSet, k.namespace, name, 5*time.Minute)
		if err != nil {
			return nil, fmt.Errorf("waiting for deployment: %w", err)
		}

		res = append(res, &d.ObjectMeta)

		return res, nil
	}, nil
}
