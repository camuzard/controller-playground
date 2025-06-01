package controller

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
)

// Controller struct to hold the components needed to watch Deployments, clientset is the interface to the Kubernetes API.
// *kubernetes.Clientset is a typed client for interacting with Kubernetes resources like Deployments. We’ll use it to set up the watcher.
type Controller struct {
	clientset *kubernetes.Clientset
	// The controller enqueues string keys from cache.MetaNamespaceKeyFunc, so we’ll use TypedRateLimitingInterface[string]
	queue workqueue.TypedRateLimitingInterface[string]
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

	// Create a type-safe rate-limiting queue for string items.
	queue := workqueue.NewTypedRateLimitingQueue[string](workqueue.DefaultTypedControllerRateLimiter[string]())

	// We create a new Controller instance, setting its clientset field to the initialized clientset.
	// Return a pointer to the Controller, and nil for the error.
	return &Controller{
		clientset: clientset,
		queue:     queue,
	}, nil
}

// This method starts the controller and sets up the watcher for Deployments.
// It takes a context.Context for cancellation and returns an error if something goes wrong.
func (c *Controller) Run(ctx context.Context) error {
	// Creating an informer in the argocd namespace using the k8s watch API. Informers caches resources locally and provide event handlers.
	informerFactory := informers.NewSharedInformerFactoryWithOptions(c.clientset, 0, informers.WithNamespace("test"))
	deploymentInformer := informerFactory.Apps().V1().Deployments().Informer()

	// AddEventHandler to registers callbacks for Add, Update, and Delete events.
	// cache.ResourceEventHandlerFuncs, struct from client-go that defines event handler functions.
	deploymentInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			// Convert our resource object to a string key
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err == nil {
				// Add the key to the queue for processing
				c.queue.Add(key)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(newObj)
			if err == nil {
				c.queue.Add(key)
			}
		},
		DeleteFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err == nil {
				c.queue.Add(key)
			}
		},
	})

	// Starts the informer in a separate goroutine.
	go deploymentInformer.Run(ctx.Done())

	// Ensure the informer’s cache is populated before proceeding, ie. it has the current state of all Deployments.
	if !cache.WaitForCacheSync(ctx.Done(), deploymentInformer.HasSynced) {
		return fmt.Errorf("failed to sync informer cache")
	}

	// Calling processNextItem, which handles one key at a time.
	for c.processNextItem(ctx) {
	}

	// Clean exit when the queue is shut down.
	return nil
}

// Processes one item from the queue. Returns false if the queue is shut down, true to continue.
func (c *Controller) processNextItem(ctx context.Context) bool {
	// Get the next key from the queue.
	key, quit := c.queue.Get()
	if quit {
		return false // Queue has been shut down
	}
	// Mark the key as processed, removing it from the queue’s active set.
	defer c.queue.Done(key)

	err := c.processItem(ctx, key)
	// If processing fails, re-queue the key with rate limiting.
	if err != nil {
		c.queue.AddRateLimited(key)
		klog.Errorf("Error processing %s: %v", key, err)
		return true
	}

	// Mark the item as done.
	c.queue.Forget(key)
	return true
}

func (c *Controller) processItem(ctx context.Context, key string) error {
	// Split the key into namespace and name.
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	// Fetch the latest Deployment from the API server using the clientset.
	deployment, err := c.clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			klog.Infof("Deployment deleted: %s/%s", namespace, name)
			return nil
		}
		return err
	}

	replicas := int32(0)
	if deployment.Spec.Replicas != nil {
		replicas = *deployment.Spec.Replicas
	}
	klog.Infof("Deployment: %s/%s, Replicas=%d", namespace, name, replicas)

	// Ensure we run 2 replicas of the Deployment.
	if replicas < 2 {
		klog.Infof("Scaling replicas to 2 for %s/%s", namespace, name)

		// Create a copy of the Deployment to avoid modifying the informer’s cached object, which could cause issues.
		updatedDeployment := deployment.DeepCopy()
		newReplicas := int32(2)
		updatedDeployment.Spec.Replicas = &newReplicas
		// Update the Deployment via the API server.
		_, err = c.clientset.AppsV1().Deployments(namespace).Update(ctx, updatedDeployment, metav1.UpdateOptions{})
		if err != nil {
			klog.Errorf("Failed to update Deployment %s/%s: %v", namespace, name, err)
			// If another process updates the Deployment concurrently, the work queue will retry, fetching the latest state.
			return err
		}
	}

	return nil
}
