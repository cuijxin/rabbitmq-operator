package rabbitmq

import (
	"context"
	"reflect"

	rabbitmqv1alpha1 "github.com/cuijxin/rabbitmq-operator/pkg/apis/rabbitmq/v1alpha1"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func (r *ReconcileRabbitmq) reconcileService(reqLogger logr.Logger, cr *rabbitmqv1alpha1.Rabbitmq, service *corev1.Service) (reconcile.Result, error) {
	reqLogger.Info("Started reconciling service", "Service.Namespace", service.Namespace, "Service.Name", service.Name)

	if err := controllerutil.SetControllerReference(cr, service, r.scheme); err != nil {
		reqLogger.Info("Error setting controller reference for service", "Service.Namespace", service.Namespace, "Service.Name", service.Name)
		return reconcile.Result{}, err
	}

	// Check if this Pod already exists
	found := &corev1.Service{}
	reqLogger.Info("Getting service", "Service.Namespace", service.Namespace, "Service.Name", service.Name)
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, found)
	if err != nil && apierrors.IsNotFound(err) {
		reqLogger.Info("No service found, creating new", "Service.Namespace", service.Namespace, "Service.Name", service.Name)
		err = r.client.Create(context.TODO(), service)

		found = service

		if err != nil {
			reqLogger.Info("Error creating new service", "Service.Namespace", service.Namespace, "Service.Name", service.Name)
			return reconcile.Result{}, err
		}
	} else if err != nil {
		reqLogger.Info("Error getting service", "Service.Namespace", service.Namespace, "Service.Name", service.Name)
		return reconcile.Result{}, err
	}

	if !reflect.DeepEqual(found.Spec.Ports, service.Spec.Ports) {
		reqLogger.Info("Ports not deep equal", "Service.Namespace", service.Namespace, "Service.Name", service.Name)
		found.Spec.Ports = service.Spec.Ports
	}

	if !reflect.DeepEqual(found.Spec.Selector, service.Spec.Selector) {
		reqLogger.Info("Selector not deep equal", "Service.Namespace", service.Namespace, "Service.Name", service.Name)
		found.Spec.Selector = service.Spec.Selector
	}

	if err = r.client.Update(context.TODO(), found); err != nil {
		reqLogger.Info("Error updating service", "Service.Namespace", service.Namespace, "Service.Name", service.Name)
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileRabbitmq) reconcileDiscoveryService(reqLogger logr.Logger, cr *rabbitmqv1alpha1.Rabbitmq) (reconcile.Result, error) {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.Namespace,
			Labels:    returnLabels(cr),
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "None",
			Selector:  returnLabels(cr),
			Ports: []corev1.ServicePort{
				{
					Port: 5672,
					Name: "amqp",
				},
				{
					Port: 4369,
					Name: "epmd",
				},
				{
					Port: 25672,
					Name: "rabbitmq-dist",
				},
			},
		},
	}

	reconcileResult, err := r.reconcileService(reqLogger, cr, service)

	return reconcileResult, err
}

func (r *ReconcileRabbitmq) reconcileManagementService(reqLogger logr.Logger, cr *rabbitmqv1alpha1.Rabbitmq) (reconcile.Result, error) {
	reqLogger.Info("Custom rabbitmq cluster service type is ", "service.type:", cr.Spec.ServiceType)
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-management",
			Namespace: cr.Namespace,
			Labels:    returnLabels(cr),
		},
		Spec: corev1.ServiceSpec{
			Type:     getServiceType(cr.Spec.ServiceType),
			Selector: returnLabels(cr),
			Ports: []corev1.ServicePort{
				{
					Port: 15672,
					Name: "http",
				},
			},
		},
	}

	reconcileResult, err := r.reconcileService(reqLogger, cr, service)

	return reconcileResult, err
}

func getServiceType(serviceType string) corev1.ServiceType {
	var result corev1.ServiceType
	switch serviceType {
	case "NodePort":
		result = corev1.ServiceTypeNodePort
	case "LoadBalancer":
		result = corev1.ServiceTypeLoadBalancer
	default:
		result = corev1.ServiceTypeNodePort
	}
	return result
}
