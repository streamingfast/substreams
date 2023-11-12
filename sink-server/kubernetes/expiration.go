package kubernetes

import (
	"context"
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strconv"
	"time"
)

func getExpirationLabelValue(ctx context.Context) string {
	//default 24 hours from now. string unix timestamp
	// return fmt.Sprintf("%s", time.Now().Add(24*time.Hour).Unix())
	return fmt.Sprintf("%d", time.Now().Add(7*time.Minute).Unix())
}

func getIntFromString(s string) int64 {
	i, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return int64(i)
}

func (k *KubernetesEngine) DeleteExpiredResources(ctx context.Context) error {
	k.resourceMutex.Lock()
	defer k.resourceMutex.Unlock()

	now := time.Now().Unix()

	// use the label selector to find all resources that are expired
	labelSelector := "component=substreams-sink-sql"

	svcs, err := k.clientSet.CoreV1().Services(k.namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return fmt.Errorf("unable to list services: %w", err)
	}
	for _, svc := range svcs.Items {
		//get expiration value from label
		exp, ok := svc.Labels["expiration"]
		if !ok {
			continue
		}

		if now < getIntFromString(exp) {
			continue
		}

		if err := k.clientSet.CoreV1().Services(k.namespace).Delete(ctx, svc.Name, metav1.DeleteOptions{}); err != nil {
			return fmt.Errorf("unable to delete service %q: %w", svc.Name, err)
		}
	}

	sts, err := k.clientSet.AppsV1().StatefulSets(k.namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return fmt.Errorf("unable to list statefulsets: %w", err)
	}
	for _, st := range sts.Items {
		//get expiration value from label
		exp, ok := st.Labels["expiration"]
		if !ok {
			continue
		}

		if now < getIntFromString(exp) {
			continue
		}

		if err := k.clientSet.AppsV1().StatefulSets(k.namespace).Delete(ctx, st.Name, metav1.DeleteOptions{}); err != nil {
			return fmt.Errorf("unable to delete statefulset %q: %w", st.Name, err)
		}
	}

	deployments, err := k.clientSet.AppsV1().Deployments(k.namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return fmt.Errorf("unable to list deployments: %w", err)
	}
	for _, deployment := range deployments.Items {
		//get expiration value from label
		exp, ok := deployment.Labels["expiration"]
		if !ok {
			continue
		}

		if now < getIntFromString(exp) {
			continue
		}

		if err := k.clientSet.AppsV1().Deployments(k.namespace).Delete(ctx, deployment.Name, metav1.DeleteOptions{}); err != nil {
			return fmt.Errorf("unable to delete deployment %q: %w", deployment.Name, err)
		}
	}

	pvcs, err := k.clientSet.CoreV1().PersistentVolumeClaims(k.namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return fmt.Errorf("unable to list pvcs: %w", err)
	}
	for _, pvc := range pvcs.Items {
		//get expiration value from label
		exp, ok := pvc.Labels["expiration"]
		if !ok {
			continue
		}

		if now < getIntFromString(exp) {
			continue
		}

		if err := k.clientSet.CoreV1().PersistentVolumeClaims(k.namespace).Delete(ctx, pvc.Name, metav1.DeleteOptions{}); err != nil {
			return fmt.Errorf("unable to delete pvc %q: %w", pvc.Name, err)
		}
	}

	configMaps, err := k.clientSet.CoreV1().ConfigMaps(k.namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return fmt.Errorf("unable to list configmaps: %w", err)
	}
	for _, configMap := range configMaps.Items {
		//get expiration value from label
		exp, ok := configMap.Labels["expiration"]
		if !ok {
			continue
		}

		if now < getIntFromString(exp) {
			continue
		}

		if err := k.clientSet.CoreV1().ConfigMaps(k.namespace).Delete(ctx, configMap.Name, metav1.DeleteOptions{}); err != nil {
			return fmt.Errorf("unable to delete configmap %q: %w", configMap.Name, err)
		}
	}

	return nil
}
