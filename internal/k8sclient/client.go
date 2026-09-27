// Package k8sclient は Pod 内から k8s API Server に接続するための
// clientset を提供する。
package k8sclient

import (
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// InCluster は Pod 内の ServiceAccount 資格情報を使って clientset を構築する。
func InCluster() (kubernetes.Interface, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("in-cluster config: %w", err)
	}
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("build clientset: %w", err)
	}
	return clientset, nil
}
