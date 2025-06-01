package main

import (
	"flag"
	"os"

	"github.com/camuzard/crd-watcher/controller-runtime-project/controller"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

func main() {
	// Create a custom FlagSet to avoid conflicts
	fs := flag.NewFlagSet("crd-watcher", flag.ExitOnError)
	kubeconfig := fs.String("kubeconfig", "", "Path to kubeconfig")
	fs.Parse(os.Args[1:])

	// Initialize klog without flag parsing
	klog.InitFlags(nil)

	// Load kubeconfig or in-cluster config
	cfg, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		cfg, err = config.GetConfig()
		if err != nil {
			klog.Fatalf("Failed to load config: %v", err)
		}
	}

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{})
	if err != nil {
		klog.Fatalf("Failed to create manager: %v", err)
	}

	if err := controller.SetupWithManager(mgr); err != nil {
		klog.Fatalf("Failed to setup controller: %v", err)
	}

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		klog.Fatalf("Failed to start manager: %v", err)
	}
}
