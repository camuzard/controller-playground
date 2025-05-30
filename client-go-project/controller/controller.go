package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

// Controller struct to hold the components needed to watch Deployments, clientset is the interface to the Kubernetes API.
// *kubernetes.Clientset is a typed client for interacting with Kubernetes resources like Deployments. We’ll use it to set up the watcher.
type Controller struct {
	clientset *kubernetes.Clientset
}

// NewController takes a kubeconfig string (path to the kubeconfig file, or empty for in-cluster config).
// It returns a *Controller (pointer to the Controller struct) and an error for initialization failures.
func NewController(kubeconfig string) (*Controller, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, err
		}
	}

	// Initialize the Kubernetes clientset using the provided kubeconfig.
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	// We create a new Controller instance, setting its clientset field to the initialized clientset.
	// Return a pointer to the Controller, and nil for the error.
	return &Controller{
		clientset: clientset,
	}, nil
}

// This method starts the controller and sets up the watcher for Deployments.
// It takes a context.Context for cancellation and returns an error if something goes wrong.
func (c *Controller) Run(ctx context.Context) error {
	// Creating an informer in the argocd namespace using the k8s watch API. Informers caches resources locally and provide event handlers.
	informerFactory := informers.NewSharedInformerFactoryWithOptions(c.clientset, 0, informers.WithNamespace("argocd"))
	deploymentInformer := informerFactory.Apps().V1().Deployments().Informer()

	// AddEventHandler to registers callbacks for Add, Update, and Delete events.
	// cache.ResourceEventHandlerFuncs, struct from client-go that defines event handler functions.
	deploymentInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			deployment := obj.(*appsv1.Deployment)
			replicas := int32(0)
			if deployment.Spec.Replicas != nil {
				replicas = *deployment.Spec.Replicas
			}
			klog.Infof("Deployment created : %s/%s => Replicas=%d", deployment.Namespace, deployment.Name, replicas)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			deployment := newObj.(*appsv1.Deployment)
			var replicas int32
			if deployment.Spec.Replicas != nil {
				replicas = *deployment.Spec.Replicas
			}
			klog.Infof("Deployment updated: %s/%s => Replicas=%d", deployment.Namespace, deployment.Name, replicas)
		},
		DeleteFunc: func(obj interface{}) {
			deployment := obj.(*appsv1.Deployment)
			klog.Infof("Deployment deleted: %s/%s", deployment.Namespace, deployment.Name)
		},
	})

	// Starts the informer in a separate goroutine.
	go deploymentInformer.Run(ctx.Done())

	// Ensure the informer’s cache is populated before proceeding, ie. it has the current state of all Deployments.
	if !cache.WaitForCacheSync(ctx.Done(), deploymentInformer.HasSynced) {
		return fmt.Errorf("failed to sync informer cache")
	}

	// Loop unntil the context is canceled. The informer runs in the background, and event handlers process Deployment events asynchronously.
	<-ctx.Done()

	// Clean exit
	return nil
}
