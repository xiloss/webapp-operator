/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"github.com/prometheus/client_golang/prometheus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	appsv1alpha1 "kubebuilder-demo/api/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	// ctrlLog is the local instance of the logger of the controller
	ctrlLog = ctrl.Log.WithName("controller")

	webAppCreatedCounter = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "webapp_created_total",
			Help: "Number of WebApp resources created",
		},
	)

	deploymentCreatedCounter = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "deployment_created_total",
			Help: "Number of Deployment resources created for WebApp",
		},
	)

	serviceCreatedCounter = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "service_created_total",
			Help: "Number of Service resources created for WebApp",
		},
	)
)

func init() {
	// Register custom metrics with the global Prometheus registry
	metrics.Registry.MustRegister(webAppCreatedCounter, deploymentCreatedCounter, serviceCreatedCounter)
}

// WebAppReconciler reconciles a WebApp object
type WebAppReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// webAppFinalizer is the constants defining the local controller finalizer; it allows logging when an object is deleted
const webAppFinalizer = "finalizer.webapp.kubebuilder.demo"

// Define RBAC permissions
//+kubebuilder:rbac:groups=apps.kubebuilder.demo,resources=webapps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps.kubebuilder.demo,resources=webapps/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=apps.kubebuilder.demo,resources=webapps/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// Modify the Reconcile function to compare the state specified by
// the WebApp object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *WebAppReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the WebApp instance
	webApp := &appsv1alpha1.WebApp{}
	err := r.Get(ctx, req.NamespacedName, webApp)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return and don't requeue
			return ctrl.Result{}, nil
		}
		// Error reading the object, requeue the request
		return ctrl.Result{}, err
	}

	// Check if the object is being deleted
	if webApp.GetDeletionTimestamp() != nil {
		// The object is being deleted
		if controllerutil.ContainsFinalizer(webApp, webAppFinalizer) {
			// Log the deletion of the WebApp resource
			logger.Info("deleting WebApp resource",
				"webapp", webApp.Name,
				"namespace", webApp.Namespace)
			// Perform any additional cleanup logic if necessary
			// Remove finalizer
			controllerutil.RemoveFinalizer(webApp, webAppFinalizer)
			err := r.Update(ctx, webApp)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
		// Stop reconciliation as the object is being deleted
		return ctrl.Result{}, nil
	}

	// Add a finalizer if it doesn't already have one
	if !controllerutil.ContainsFinalizer(webApp, webAppFinalizer) {
		controllerutil.AddFinalizer(webApp, webAppFinalizer)
		err := r.Update(ctx, webApp)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	// Define the desired deployment
	deployment := &appsv1.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:      webApp.Name,
			Namespace: webApp.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &webApp.Spec.Replicas,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"app": webApp.Name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{"app": webApp.Name},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "webapp",
							Image: webApp.Spec.Image,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: webApp.Spec.Port,
								},
							},
						},
					},
				},
			},
		},
	}

	// Set WebApp instance as the owner and controller
	if err := controllerutil.SetControllerReference(webApp, deployment, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	// Check if the deployment already exists
	foundDeployment := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: webApp.Name, Namespace: webApp.Namespace}, foundDeployment)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Creating a new Deployment", "namespace", deployment.Namespace, "name", deployment.Name)
		err = r.Create(ctx, deployment)
		if err != nil {
			return ctrl.Result{}, err
		}
		deploymentCreatedCounter.Inc() // Increment deployment counter
		webAppCreatedCounter.Inc()     // Increment webApp counter
		// Deployment created successfully - don't requeue
		return ctrl.Result{}, nil
	} else if err != nil {
		return ctrl.Result{}, err
	}

	// Ensure the deployment size is the same as the spec
	if *foundDeployment.Spec.Replicas != webApp.Spec.Replicas {
		foundDeployment.Spec.Replicas = &webApp.Spec.Replicas
		err = r.Update(ctx, foundDeployment)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	// Define the desired service
	service := &corev1.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:      webApp.Name,
			Namespace: webApp.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": webApp.Name},
			Ports: []corev1.ServicePort{
				{
					Port:     webApp.Spec.Port,
					Protocol: corev1.ProtocolTCP,
				},
			},
		},
	}

	// Set WebApp instance as the owner and controller
	if err := controllerutil.SetControllerReference(webApp, service, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	// Check if the service already exists
	foundService := &corev1.Service{}
	if err = r.Get(ctx, types.NamespacedName{Name: webApp.Name, Namespace: webApp.Namespace}, foundService); err != nil && errors.IsNotFound(err) {
		ctrlLog.Info("creating new service", "service", service.Name,
			"namespace", service.Namespace)
		err = r.Create(ctx, service)
		if err != nil {
			return ctrl.Result{}, err
		}
		serviceCreatedCounter.Inc() // Increment service counter
		// Service created successfully - don't requeue
		return ctrl.Result{}, nil
	} else if err != nil {
		return ctrl.Result{}, err
	}

	// Service already exists - don't requeue
	logger.Info("Skip reconcile: Service already exists",
		"namespace", foundService.Namespace,
		"service", foundService.Name)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *WebAppReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1alpha1.WebApp{}).
		// Deployment is included in ownership here
		Owns(&appsv1.Deployment{}).
		// Service is included in ownership here
		Owns(&corev1.Service{}).
		Complete(r)
}
