package kubernetes

import (
	"context"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

type createFunc func(context.Context) ([]*metav1.ObjectMeta, error)

func waitForStateFulSet(ctx context.Context, clientset *kubernetes.Clientset, namespace, name string, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(ctx, 10*time.Second, timeout, true, func(context.Context) (bool, error) {
		sts, err := clientset.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		if sts.Status.ObservedGeneration < sts.Generation {
			return false, nil
		}

		if sts.Status.ReadyReplicas != *sts.Spec.Replicas {
			return false, nil
		}

		return true, nil
	})
}

func waitForDeployment(ctx context.Context, clientset *kubernetes.Clientset, namespace, name string, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(ctx, 10*time.Second, timeout, true, func(context.Context) (bool, error) {
		depl, err := clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		if depl.Status.ObservedGeneration < depl.Generation {
			return false, nil
		}

		if depl.Status.ReadyReplicas != *depl.Spec.Replicas {
			return false, nil
		}

		return true, nil
	})
}
