package controllers

import (
	"fmt"
	"github.com/go-logr/logr"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	monitor "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *VeleroReconciler) ReconcileServiceMonitor(log logr.Logger) (bool, error) {

	velero := oadpv1alpha1.Velero{}
	if err := r.Get(r.Context, r.NamespacedName, &velero); err != nil {
		return false, err
	}

	serviceMonitor := &monitor.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "oadp-operator",
			Namespace: r.NamespacedName.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, serviceMonitor, func() error {

		if serviceMonitor.ObjectMeta.CreationTimestamp.IsZero() {
			serviceMonitor.Spec.Selector = metav1.LabelSelector{
				MatchLabels: map[string]string{
					oadpv1alpha1.OadpOperatorLabel: "true",
					"app":                          "oadp-operator",
				},
			}
		}

		// update service monitor
		return r.buildServiceMonitor(serviceMonitor, &velero)
	})

	if err != nil {
		return false, err
	}

	//TODO: Review service monitor status and report errors and conditions

	if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
		// Trigger event to indicate service monitor was created or updated
		r.EventRecorder.Event(serviceMonitor,
			corev1.EventTypeNormal,
			"ServiceMonitorReconciled",
			fmt.Sprintf("performed %s on service monitor %s/%s", op, serviceMonitor.Namespace, serviceMonitor.Name),
		)
	}
	return true, nil
}

func (r *VeleroReconciler) buildServiceMonitor(serviceMonitor *monitor.ServiceMonitor, velero *oadpv1alpha1.Velero) error {

	if velero == nil {
		return fmt.Errorf("velero CR cannot be nil")
	}

	if serviceMonitor == nil {
		return fmt.Errorf("service monitor cannot be nil")
	}

	// Setting controller owner reference on the service monitor
	err := controllerutil.SetControllerReference(velero, serviceMonitor, r.Scheme)
	if err != nil {
		return err
	}

	serviceMonitor.Spec.Selector = metav1.LabelSelector{
		MatchLabels: map[string]string{
			oadpv1alpha1.OadpOperatorLabel: "true",
			"app":                          "oadp-operator",
		},
	}

	serviceMonitor.Labels = map[string]string{
		oadpv1alpha1.OadpOperatorLabel: "true",
		"app":                          "oadp-operator",
	}

	serviceMonitor.Spec.Endpoints = []monitor.Endpoint{
		{
			Interval: "30s",
			Port:     "metrics",
		},
	}

	serviceMonitor.Spec.JobLabel = "app"

	serviceMonitor.Spec.NamespaceSelector = monitor.NamespaceSelector{
		MatchNames: []string{
			velero.Namespace,
		},
	}

	return nil
}

func (r *VeleroReconciler) ReconcileVeleroServiceMonitor(log logr.Logger) (bool, error) {

	velero := oadpv1alpha1.Velero{}
	if err := r.Get(r.Context, r.NamespacedName, &velero); err != nil {
		return false, err
	}

	serviceMonitor := &monitor.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "openshift-adp-velero-monitor",
			Namespace: r.NamespacedName.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, serviceMonitor, func() error {

		if serviceMonitor.ObjectMeta.CreationTimestamp.IsZero() {
			serviceMonitor.Spec.Selector = metav1.LabelSelector{
				MatchLabels: map[string]string{
					"k8s-app":                         "openshift-adp",
				},
			}
		}

		// update service monitor
		return r.buildVeleroServiceMonitor(serviceMonitor, &velero)
	})

	if err != nil {
		return false, err
	}

	//TODO: Review service monitor status and report errors and conditions

	if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
		// Trigger event to indicate service monitor was created or updated
		r.EventRecorder.Event(serviceMonitor,
			corev1.EventTypeNormal,
			"VeleroServiceMonitorReconciled",
			fmt.Sprintf("performed %s on velero service monitor %s/%s", op, serviceMonitor.Namespace, serviceMonitor.Name),
		)
	}
	return true, nil
}

func (r *VeleroReconciler) buildVeleroServiceMonitor(serviceMonitor *monitor.ServiceMonitor, velero *oadpv1alpha1.Velero) error {

	if velero == nil {
		return fmt.Errorf("velero CR cannot be nil")
	}

	if serviceMonitor == nil {
		return fmt.Errorf("service monitor cannot be nil")
	}

	// Setting controller owner reference on the service monitor
	err := controllerutil.SetControllerReference(velero, serviceMonitor, r.Scheme)
	if err != nil {
		return err
	}

	serviceMonitor.Spec.Selector = metav1.LabelSelector{
		MatchLabels: map[string]string{
			"k8s-app":                          "openshift-adp",
		},
	}

	serviceMonitor.Labels = map[string]string{
		"k8s-app":                          "openshift-adp-velero-monitor",
	}

	serviceMonitor.Spec.Endpoints = []monitor.Endpoint{
		{
			Interval: "30s",
			Port:     "monitoring",
		},
	}

	//serviceMonitor.Spec.JobLabel = "app"

	serviceMonitor.Spec.NamespaceSelector = monitor.NamespaceSelector{
		MatchNames: []string{
			velero.Namespace,
		},
	}

	return nil
}

func (r *VeleroReconciler) ReconcileMetricsSVC(log logr.Logger) (bool, error) {
	velero := oadpv1alpha1.Velero{}
	if err := r.Get(r.Context, r.NamespacedName, &velero); err != nil {
		return false, err
	}

	svc := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "oadp-operator-metrics",
			Namespace: r.NamespacedName.Namespace,
		},
	}

	// Create SVC
	op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, &svc, func() error {
		// TODO: check for svc status condition errors and respond here
		err := r.updateMetricsSVC(&svc, &velero)

		return err
	})
	if err != nil {
		return false, err
	}
	if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
		// Trigger event to indicate SVC was created or updated
		r.EventRecorder.Event(&svc,
			corev1.EventTypeNormal,
			"MetricsServiceReconciled",
			fmt.Sprintf("performed %s on service %s/%s", op, svc.Namespace, svc.Name),
		)
	}

	return true, nil
}

func (r *VeleroReconciler) updateMetricsSVC(svc *corev1.Service, velero *oadpv1alpha1.Velero) error {
	// Setting controller owner reference on the metrics svc
	err := controllerutil.SetControllerReference(velero, svc, r.Scheme)
	if err != nil {
		return err
	}

	// when updating the spec fields we update each field individually
	// to get around the immutable fields
	svc.Spec.Selector = map[string]string{
		oadpv1alpha1.OadpOperatorLabel: "true",
		"control-plane":                "controller-manager",
	}

	svc.Spec.Type = corev1.ServiceTypeClusterIP
	svc.Spec.Ports = []corev1.ServicePort{
		{
			Name:     "metrics",
			Port:     int32(2112),
			Protocol: corev1.ProtocolTCP,
			TargetPort: intstr.IntOrString{
				IntVal: int32(2112),
			},
		},
	}

	svc.Spec.ClusterIP = "None"

	svc.Spec.SessionAffinity = "None"

	svc.Labels = map[string]string{
		oadpv1alpha1.OadpOperatorLabel: "true",
		"app":                          "oadp-operator",
	}
	return nil
}

func (r *VeleroReconciler) ReconcileVeleroMetricsSVC(log logr.Logger) (bool, error) {
	velero := oadpv1alpha1.Velero{}
	if err := r.Get(r.Context, r.NamespacedName, &velero); err != nil {
		return false, err
	}

	svc := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.NamespacedName.Name,
			Namespace: r.NamespacedName.Namespace,
		},
	}

	// Create SVC
	op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, &svc, func() error {
		// TODO: check for svc status condition errors and respond here
		err := r.updateVeleroMetricsSVC(&svc, &velero)

		return err
	})
	if err != nil {
		return false, err
	}
	if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
		// Trigger event to indicate SVC was created or updated
		r.EventRecorder.Event(&svc,
			corev1.EventTypeNormal,
			"VeleroMetricsServiceReconciled",
			fmt.Sprintf("performed %s on velero metrics service %s/%s", op, svc.Namespace, svc.Name),
		)
	}

	return true, nil
}

func (r *VeleroReconciler) updateVeleroMetricsSVC(svc *corev1.Service, velero *oadpv1alpha1.Velero) error {
	// Setting controller owner reference on the metrics svc
	err := controllerutil.SetControllerReference(velero, svc, r.Scheme)
	if err != nil {
		return err
	}

	// when updating the spec fields we update each field individually
	// to get around the immutable fields
	svc.Spec.Selector = map[string]string{
		"k8s-app":                "openshift-adp",
	}

	svc.Spec.Type = corev1.ServiceTypeClusterIP
	svc.Spec.Ports = []corev1.ServicePort{
		{
			Name:     "monitoring",
			Port:     int32(8085),
			TargetPort: intstr.IntOrString{
				IntVal: int32(8085),
			},
		},
	}

	svc.Labels = map[string]string{
		"k8s-app":                          "openshift-adp",
	}
	return nil
}

func (r *VeleroReconciler) ReconcileMetricsRole(log logr.Logger) (bool, error) {
	velero := oadpv1alpha1.Velero{}
	if err := r.Get(r.Context, r.NamespacedName, &velero); err != nil {
		return false, err
	}

	role := rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "prometheus-k8s",
			Namespace: r.NamespacedName.Namespace,
		},
	}

	// Create Role
	op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, &role, func() error {
		// TODO: check for role status condition errors and respond here
		err := r.updateMetricsRole(&role, &velero)

		return err
	})
	if err != nil {
		return false, err
	}
	if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
		// Trigger event to indicate role was created or updated
		r.EventRecorder.Event(&role,
			corev1.EventTypeNormal,
			"MetricsRoleReconciled",
			fmt.Sprintf("performed %s on role %s/%s", op, role.Namespace, role.Name),
		)
	}

	return true, nil
}

func (r *VeleroReconciler) updateMetricsRole(role *rbacv1.Role, velero *oadpv1alpha1.Velero) error {
	// Setting controller owner reference on the metrics role
	err := controllerutil.SetControllerReference(velero, role, r.Scheme)
	if err != nil {
		return err
	}

	role.Rules = []rbacv1.PolicyRule{
		{
			APIGroups: []string{
				"",
			},
			Resources: []string{
				"services",
				"endpoints",
				"pods",
			},
			Verbs: []string{
				"get",
				"list",
				"watch",
			},
		},
	}

	return nil
}

func (r *VeleroReconciler) ReconcileMetricsRoleBinding(log logr.Logger) (bool, error) {
	velero := oadpv1alpha1.Velero{}
	if err := r.Get(r.Context, r.NamespacedName, &velero); err != nil {
		return false, err
	}

	roleBinding := rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "prometheus-k8s",
			Namespace: r.NamespacedName.Namespace,
		},
	}

	// Create RoleBinding
	op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, &roleBinding, func() error {
		// TODO: check for roleBinding status condition errors and respond here
		err := r.updateMetricsRoleBinding(&roleBinding, &velero)

		return err
	})
	if err != nil {
		return false, err
	}
	if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
		// Trigger event to indicate roleBinding was created or updated
		r.EventRecorder.Event(&roleBinding,
			corev1.EventTypeNormal,
			"MetricsRoleBindingReconciled",
			fmt.Sprintf("performed %s on roleBinding %s/%s", op, roleBinding.Namespace, roleBinding.Name),
		)
	}

	return true, nil
}

func (r *VeleroReconciler) updateMetricsRoleBinding(roleBinding *rbacv1.RoleBinding, velero *oadpv1alpha1.Velero) error {
	// Setting controller owner reference on the metrics roleBinding
	err := controllerutil.SetControllerReference(velero, roleBinding, r.Scheme)
	if err != nil {
		return err
	}

	roleBinding.RoleRef = rbacv1.RoleRef{
		APIGroup: "rbac.authorization.k8s.io",
		Kind:     "Role",
		Name:     "prometheus-k8s",
	}

	roleBinding.Subjects = []rbacv1.Subject{
		{
			Kind:      "ServiceAccount",
			Name:      "prometheus-k8s",
			Namespace: "openshift-monitoring",
		},
	}

	return nil
}
