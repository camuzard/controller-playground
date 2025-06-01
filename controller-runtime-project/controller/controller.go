package controller

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

type DeploymentReconciler struct {
	// Client is controller-runtime’s API client, replacing client-go’s Clientset. It provides methods like Get, Update, and handles caching.
	client.Client
}

// Implements the reconciliation logic, called for each event.
func (r *DeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Create an empty Deployment object to store the fetched resource
	deployment := &appsv1.Deployment{}
	// req ctrl.Request contains namespace and name,excample test/test-deployment
	// Get fetches the Deployment using the client, which uses the manager’s cache, reducing API calls compared to client-go’s Clientset.
	if err := r.Get(ctx, req.NamespacedName, deployment); err != nil {
		if client.IgnoreNotFound(err) != nil {
			klog.Errorf("Error fetching Deployment %s: %v", req.NamespacedName, err)
			return ctrl.Result{}, err
		}
		klog.Infof("Deployment deleted: %s", req.NamespacedName)
		return ctrl.Result{}, nil // No requeue
	}

	replicas := int32(0)
	if deployment.Spec.Replicas != nil {
		replicas = *deployment.Spec.Replicas
	}
	klog.Infof("Deployment: %s, Replicas=%d", req.NamespacedName, replicas)

	if replicas < 2 {
		klog.Infof("Scaling replicas to 2 for %s", req.NamespacedName)

		// Create a copy of the Deployment to avoid modifying the informer’s cached object, which could cause issues.
		updatedDeployment := deployment.DeepCopy()
		newReplicas := int32(2)
		updatedDeployment.Spec.Replicas = &newReplicas
		// Update the Deployment using the client’s Update method.
		// The client handles retries and conflicts internally, unlike client-go’s raw API calls.
		if err := r.Update(ctx, updatedDeployment); err != nil {
			klog.Errorf("Failed to update Deployment %s to 2 replicas: %v", req.NamespacedName, err)
			return ctrl.Result{}, err // No requeue
		}
	}

	return ctrl.Result{}, nil // No requeue
}

// Configures the controller with the manager, to watch resources and handle events.
func SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		// For specifies the primary resource to watch
		For(&appsv1.Deployment{}).
		WithEventFilter(predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return e.Object.GetNamespace() == "test" },
			UpdateFunc: func(e event.UpdateEvent) bool { return e.ObjectNew.GetNamespace() == "test" },
			DeleteFunc: func(e event.DeleteEvent) bool { return e.Object.GetNamespace() == "test" },
		}).
		Complete(&DeploymentReconciler{Client: mgr.GetClient()})
}
