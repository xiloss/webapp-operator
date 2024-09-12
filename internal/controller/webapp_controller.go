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
	"reflect"

	"github.com/prometheus/client_golang/prometheus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	// webAppCreatedCounter is the prometheus counter metric to count WebApp resources creation
	// its label is webapp_created_total
	webAppCreatedCounter = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "webapp_created_total",
			Help: "Number of WebApp resources created",
		},
	)
	// webAppDeletedCounter is the prometheus counter metric to count WebApp resources deletion
	// its label is webapp_deleted_total
	webAppDeletedCounter = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "webapp_delete_total",
			Help: "Number of WebApp resources deleted",
		},
	)
	// deploymentCreatedCounter is the prometheus counter metric to count a deployment creation in the WebApp resource
	// its label is deployment_created_total
	deploymentCreatedCounter = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "deployment_created_total",
			Help: "Number of Deployment resources created in WebApp",
		},
	)
	// deploymentUpdatedCounter is the prometheus counter metric to count a deployment update in the WebApp resource
	// its label is deployment_updated_total
	deploymentUpdatedCounter = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "deployment_updated_total",
			Help: "Number of Deployment resources updated in WebApp",
		},
	)
	// serviceCreatedCounter is the prometheus counter metric to count a service creation in the WebApp resource
	// its label is service_created_total
	serviceCreatedCounter = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "service_created_total",
			Help: "Number of Service resources created for WebApp",
		},
	)
	// serviceUpdatedCounter is the prometheus counter metric to count a service update in the WebApp resource
	// its label is service_updated_total
	serviceUpdatedCounter = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "service_updated_total",
			Help: "Number of Service resources created for WebApp",
		},
	)
)

// webAppFinalizer is the constant defining the local controller finalizer; it allows logging when an object is deleted
const webAppFinalizer = "finalizer.webapp.kubebuilder.demo"

func init() {
	// Register custom metrics with the global Prometheus registry
	metrics.Registry.MustRegister(
		webAppCreatedCounter,
		webAppDeletedCounter,
		deploymentCreatedCounter,
		deploymentUpdatedCounter,
		serviceCreatedCounter,
		serviceUpdatedCounter,
	)
}

// WebAppReconciler reconciles a WebApp object
type WebAppReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

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
	// instantiating a new logger
	logger := log.FromContext(ctx).WithName("controller-reconcile")

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
			// Log the deletion start on the WebApp resource
			logger.Info("deleting WebApp resource",
				"webapp_name", webApp.Name,
				"webapp_namespace", webApp.Namespace)
			// Perform any additional cleanup logic if necessary
			// Remove finalizer
			controllerutil.RemoveFinalizer(webApp, webAppFinalizer)
			err := r.Update(ctx, webApp)
			if err != nil {
				return ctrl.Result{}, err
			}
			// increment counter when a WebApp resource is deleted
			webAppDeletedCounter.Inc()
			// Log the deletion start on the WebApp resource
			logger.Info("deleted WebApp resource",
				"webapp_name", webApp.Name,
				"webapp_namespace", webApp.Namespace)
		}
		// Stop reconciliation as the object is deleted
		return ctrl.Result{}, nil
	}

	// Add a finalizer to control the deletion of the resource from the operator
	if !controllerutil.ContainsFinalizer(webApp, webAppFinalizer) {
		controllerutil.AddFinalizer(webApp, webAppFinalizer)
		err := r.Update(ctx, webApp)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	// Reconcile Deployment Section
	deployment := createDeployment(webApp)
	// Set WebApp instance as the owner and controller of the deployment
	if err := controllerutil.SetControllerReference(webApp, deployment, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	foundDeployment, err := r.getDeployment(ctx, webApp)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info(
				"creating a new deployment",
				"deployment_name", deployment.Name,
				"deployment_namespace", deployment.Namespace,
			)
			if err := r.Create(ctx, deployment); err != nil {
				return ctrl.Result{}, err
			}
			logger.Info(
				"deployment created",
				"deployment_name", deployment.Name,
				"deployment_namespace", deployment.Namespace,
			)
			deploymentCreatedCounter.Inc()
			// on deployment creation, the webapp is always created too
			webAppCreatedCounter.Inc()
		} else {
			return ctrl.Result{}, err
		}
	} else {
		if updateDeploymentSpec(foundDeployment, webApp) {
			logger.Info("updating deployment",
				"deployment_name", foundDeployment.Name,
				"deployment_namespace", foundDeployment.Namespace,
			)
			if err := r.Update(ctx, foundDeployment); err != nil {
				return ctrl.Result{}, err
			}
			logger.Info(
				"deployment updated",
				"deployment_name", deployment.Name,
				"deployment_namespace", deployment.Namespace,
			)
			deploymentUpdatedCounter.Inc()
		}
	}

	service := createService(webApp)
	// Set WebApp instance as the owner and controller of the service
	if err := controllerutil.SetControllerReference(webApp, service, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	// Check if the service already exists
	foundService, err := r.getService(ctx, webApp)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("creating a new service",
				"service_namespace", service.Namespace,
				"service_name", service.Name,
			)
			if err := r.Create(ctx, service); err != nil {
				return ctrl.Result{}, err
			}
			logger.Info(
				"service created",
				"service_name", service.Name,
				"service_namespace", service.Namespace,
			)
			serviceCreatedCounter.Inc()
		} else {
			return ctrl.Result{}, err
		}
	} else {
		if updateServiceSpec(foundService, webApp) {
			logger.Info("updating Service",
				"service_name", foundService.Name,
				"service_namespace", foundService.Namespace,
			)
			if err := r.Update(ctx, foundService); err != nil {
				return ctrl.Result{}, err
			}
			logger.Info(
				"service updated",
				"service_name", service.Name,
				"service_namespace", service.Namespace,
			)
			serviceUpdatedCounter.Inc()
		}
	}

	// Update status
	if err := r.updateStatus(ctx, webApp, foundDeployment, foundService,
		"ReconcileComplete", "reconciliation completed successfully"); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("skip reconcile: all resources are reconciled",
		"deployment_name", foundDeployment.Name,
		"deployment_namespace", foundDeployment.Namespace,
		"service_name", foundService.Name,
		"service_namespace", foundService.Namespace,
	)

	return ctrl.Result{}, nil
}

// createDeployment is used to create an empty deployment prototype to be reconciled
func createDeployment(webApp *appsv1alpha1.WebApp) *appsv1.Deployment {
	// return the desired deployment prototype
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      webApp.Name,
			Namespace: webApp.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &webApp.Spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": webApp.Name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
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
}

func createService(webApp *appsv1alpha1.WebApp) *corev1.Service {
	// return the desired service prototype
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
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
			Type: corev1.ServiceTypeClusterIP,
		},
	}
}

func (r *WebAppReconciler) getDeployment(ctx context.Context, webApp *appsv1alpha1.WebApp) (*appsv1.Deployment, error) {
	foundDeployment := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: webApp.Name, Namespace: webApp.Namespace}, foundDeployment)
	return foundDeployment, err
}

func (r *WebAppReconciler) getService(ctx context.Context, webApp *appsv1alpha1.WebApp) (*corev1.Service, error) {
	foundService := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: webApp.Name, Namespace: webApp.Namespace}, foundService)
	return foundService, err
}

func updateDeploymentSpec(deployment *appsv1.Deployment, webApp *appsv1alpha1.WebApp) bool {
	updated := false
	if *deployment.Spec.Replicas != webApp.Spec.Replicas {
		deployment.Spec.Replicas = &webApp.Spec.Replicas
		updated = true
	}
	if deployment.Spec.Template.Spec.Containers[0].Image != webApp.Spec.Image {
		deployment.Spec.Template.Spec.Containers[0].Image = webApp.Spec.Image
		updated = true
	}
	if deployment.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort != webApp.Spec.Port {
		deployment.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort = webApp.Spec.Port
		updated = true
	}
	return updated
}

func updateServiceSpec(service *corev1.Service, webApp *appsv1alpha1.WebApp) bool {
	updated := false
	if len(service.Spec.Ports) > 0 && service.Spec.Ports[0].Port != webApp.Spec.Port {
		service.Spec.Ports[0].Port = webApp.Spec.Port
		updated = true
	}
	return updated
}

func mergeConditions(conditions []metav1.Condition, newCondition metav1.Condition) []metav1.Condition {
	for i, cond := range conditions {
		if cond.Type == newCondition.Type {
			if cond.Status != newCondition.Status || cond.Reason != newCondition.Reason || cond.Message != newCondition.Message {
				conditions[i] = newCondition
			}
			return conditions
		}
	}
	return append(conditions, newCondition)
}

func (r *WebAppReconciler) updateStatus(ctx context.Context, webApp *appsv1alpha1.WebApp, deployment *appsv1.Deployment, service *corev1.Service, reason, message string) error {
	logger := log.FromContext(ctx)

	updated := false
	newStatus := webApp.Status.DeepCopy()

	if deployment != nil {
		if newStatus.ReadyReplicas != deployment.Status.ReadyReplicas {
			newStatus.ReadyReplicas = deployment.Status.ReadyReplicas
			updated = true
		}
		if newStatus.AvailableReplicas != deployment.Status.AvailableReplicas {
			newStatus.AvailableReplicas = deployment.Status.AvailableReplicas
			updated = true
		}
	}

	if service != nil && len(service.Spec.Ports) > 0 {
		if newStatus.ServicePort != service.Spec.Ports[0].Port {
			newStatus.ServicePort = service.Spec.Ports[0].Port
			updated = true
		}
		if newStatus.ServiceType != string(service.Spec.Type) {
			newStatus.ServiceType = string(service.Spec.Type)
			updated = true
		}
	}

	newCondition := metav1.Condition{
		Type:               "ReconcileCompleted",
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}

	newStatus.Conditions = mergeConditions(newStatus.Conditions, newCondition)

	if !reflect.DeepEqual(webApp.Status, *newStatus) {
		webApp.Status = *newStatus
		updated = true
	}

	if updated {
		if err := r.Status().Update(ctx, webApp); err != nil {
			logger.Error(err, "Failed to update WebApp status")
			return err
		}
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *WebAppReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		// Looping on the WebApp custom resource
		For(&appsv1alpha1.WebApp{}).
		// Deployment is included in ownership here
		Owns(&appsv1.Deployment{}).
		// Service is included in ownership here
		Owns(&corev1.Service{}).
		// completing the returning loop
		Complete(r)
}
