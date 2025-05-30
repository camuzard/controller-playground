package main

import (
	"context"
	"flag"

	"github.com/camuzard/crd-watcher/client-go-project/controller"
	"k8s.io/klog/v2"
)

func main() {
	kubeconfig := flag.String("kubeconfig", "", "Path to kubeconfig file")
	flag.Parse()

	ctrl, err := controller.NewController(*kubeconfig)
	if err != nil {
		klog.Fatalf("Failed to create controller: %v", err)
	}

	ctx := context.Background()
	if err := ctrl.Run(ctx); err != nil {
		klog.Fatalf("Failed to run controller: %v", err)
	}
}
