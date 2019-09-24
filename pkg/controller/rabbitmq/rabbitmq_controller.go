package rabbitmq

import (
	"context"
	"fmt"
	"reflect"

	rabbitmqv1alpha1 "github.com/cuijxin/rabbitmq-operator/pkg/apis/rabbitmq/v1alpha1"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const initRabbitmqScript = `if [ -z "$(grep rabbitmq /etc/resolv.conf)" ]; then
  sed "s/^search \([^ ]\+\)/search rabbitmq.\1 \1/" /etc/resolv.conf > /etc/resolv.conf.new;
  cat /etc/resolv.conf.new > /etc/resolv.conf;
  rm /etc/resolv.conf.new;
fi;
until rabbitmqctl node_health_check; do sleep 1; done;
if [ -z "$(rabbitmqctl cluster_status | grep rabbitmq-0)" ]; then
  touch /gotit
  rabbitmqctl stop_app;
  rabbitmqctl reset;
  rabbitmqctl join_cluster rabbit@rabbitmq-0;
  rabbitmqctl start_app;
else
  touch /notget
fi;`

const defaultImageName = "rabbitmq"
const defaultImageTag = "3.7-rc-management"

var log = logf.Log.WithName("controller_rabbitmq")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new Rabbitmq Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileRabbitmq{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("rabbitmq-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource Rabbitmq
	err = c.Watch(&source.Kind{Type: &rabbitmqv1alpha1.Rabbitmq{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Pods and requeue the owner Rabbitmq
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &rabbitmqv1alpha1.Rabbitmq{},
	})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource StatefulSets and requeue the owner
	err = c.Watch(&source.Kind{Type: &v1.StatefulSet{}}, &handler.EnqueueRequestForOwner{
		OwnerType: &rabbitmqv1alpha1.Rabbitmq{},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileRabbitmq implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileRabbitmq{}

// ReconcileRabbitmq reconciles a Rabbitmq object
type ReconcileRabbitmq struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a Rabbitmq object and makes changes based on the state read
// and what is in the Rabbitmq.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileRabbitmq) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling Rabbitmq")

	// Fetch the Rabbitmq instance
	instance := &rabbitmqv1alpha1.Rabbitmq{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		reqLogger.Error(err, "Failed to get Rabbitmq")
		return reconcile.Result{}, err
	}

	// Define a new StatefulSet Object
	statefulset := newStatefulSet(instance)
	if err := controllerutil.SetControllerReference(instance, statefulset, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	// Check if the statefulset already exists, if not create a new one
	statefulsetFound := &v1.StatefulSet{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: statefulset.Name, Namespace: statefulset.Namespace}, statefulsetFound)
	if err != nil && errors.IsNotFound(err) {
		reqLogger.Info("Creating a new statefulset", "statefulset.Namespace", statefulset.Namespace, "statefulset.Name", statefulset.Name)
		err = r.client.Create(context.TODO(), statefulset)
		if err != nil {
			reqLogger.Error(err, "Failed to create new Statefulset", "Statefulset.Namespace", statefulset.Namespace, "Statefulset.Name", statefulset.Name)
			return reconcile.Result{}, err
		}

		// statefulset created successfully - don't requeue
		return reconcile.Result{}, nil
	} else if err != nil {
		reqLogger.Error(err, "Failed to get Statefulset")
		return reconcile.Result{}, err
	}

	if !reflect.DeepEqual(statefulsetFound.Spec, statefulset.Spec) {
		statefulsetFound.Spec.Replicas = statefulset.Spec.Replicas
		statefulsetFound.Spec.Template = statefulset.Spec.Template
	}

	if !reflect.DeepEqual(statefulsetFound.Labels, statefulset.Labels) {
		statefulsetFound.Labels = statefulset.Labels
	}

	reqLogger.Info("Reconcile statefulset", "statefulset.Namespace", statefulsetFound.Namespace, "statefulset.Name", statefulsetFound.Name)
	if err = r.client.Update(context.TODO(), statefulsetFound); err != nil {
		reqLogger.Info("Reconcile statefulset error", "statefulset.Namespace", statefulsetFound.Namespace, "statefulset.Name", statefulsetFound.Name)
		return reconcile.Result{}, err
	}

	// creating services
	reqLogger.Info("Reconciling services")

	_, err = r.reconcileManagementService(reqLogger, instance)
	if err != nil {
		return reconcile.Result{}, err
	}

	_, err = r.reconcileDiscoveryService(reqLogger, instance)
	if err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func newStatefulSet(cr *rabbitmqv1alpha1.Rabbitmq) *v1.StatefulSet {
	// prepare containers for pod
	podContainers := []corev1.Container{}
	cmd := make([]string, 0)
	exec := &corev1.ExecAction{
		Command: newCommand(cmd),
	}

	// 容器启动后，立刻执行的指定操作
	postStart := &corev1.Handler{
		Exec: exec,
	}

	lifecycle := &corev1.Lifecycle{
		PostStart: postStart,
	}

	// container with rabbitmq
	env := make([]corev1.EnvVar, 0)
	ports := make([]corev1.ContainerPort, 0)
	image := fmt.Sprintf("%s:%s", cr.Spec.K8SImage.Name, cr.Spec.K8SImage.Tag)
	rabbitmqContainer := corev1.Container{
		Name:      "rabbitmq",
		Image:     image, //"rabbitmq:3.7-rc-management",
		Lifecycle: lifecycle,
		Env:       newContainerEnvs(env),
		Ports:     newContainerPorts(ports),
	}

	podContainers = append(podContainers, rabbitmqContainer)

	podTemplate := corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: returnLabels(cr),
		},
		Spec: corev1.PodSpec{
			Containers: podContainers,
		},
	}

	return &v1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.Namespace,
			Labels:    returnLabels(cr),
		},
		Spec: v1.StatefulSetSpec{
			Replicas:    &cr.Spec.RabbitmqReplicas,
			ServiceName: cr.Name,
			Selector: &metav1.LabelSelector{
				MatchLabels: returnLabels(cr),
			},
			Template: podTemplate,
		},
	}
}

func newCommand(cmd []string) []string {
	cmd = append(cmd, "/bin/sh")
	cmd = append(cmd, "-c")
	cmd = append(cmd, initRabbitmqScript)

	return cmd
}

func newContainerEnvs(env []corev1.EnvVar) []corev1.EnvVar {
	return append(env,
		corev1.EnvVar{
			Name: "MY_POD_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.name",
				},
			},
		},
		corev1.EnvVar{
			Name:  "RABBITMQ_ERLANG_COOKIE",
			Value: "YZSDHWMFSMKEMBDHSGGZ",
		},
		corev1.EnvVar{
			Name:  "RABBITMQ_NODENAME",
			Value: "rabbit@$(MY_POD_NAME)",
		},
	)
}

func newContainerPorts(ports []corev1.ContainerPort) []corev1.ContainerPort {
	return append(ports,
		corev1.ContainerPort{
			Name:          "amqp",
			ContainerPort: 5672,
		},
	)
}

func returnLabels(cr *rabbitmqv1alpha1.Rabbitmq) map[string]string {
	labels := map[string]string{
		"app": cr.Name,
	}
	return labels
}
