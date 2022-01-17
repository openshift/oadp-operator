package controllers

import (
	"fmt"
	"github.com/go-logr/logr"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/common"
	monitor "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *DPAReconciler) ReconcileVeleroServiceMonitor(log logr.Logger) (bool, error) {

	dpa := oadpv1alpha1.DataProtectionApplication{}
	if err := r.Get(r.Context, r.NamespacedName, &dpa); err != nil {
		return false, err
	}

	serviceMonitor := &monitor.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "openshift-adp-velero-metrics-sm",
			Namespace: r.NamespacedName.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, serviceMonitor, func() error {

		if serviceMonitor.ObjectMeta.CreationTimestamp.IsZero() {
			serviceMonitor.Spec.Selector = metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name":       common.Velero,
					"app.kubernetes.io/instance":   dpa.Name,
					"app.kubernetes.io/managed-by": common.OADPOperator,
					"app.kubernetes.io/component":  Server,
					oadpv1alpha1.OadpOperatorLabel: "True",
				},
			}
		}

		// update service monitor
		return r.buildVeleroServiceMonitor(serviceMonitor, &dpa)
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
			fmt.Sprintf("performed %s on dpa service monitor %s/%s", op, serviceMonitor.Namespace, serviceMonitor.Name),
		)
	}
	return true, nil
}

func (r *DPAReconciler) buildVeleroServiceMonitor(serviceMonitor *monitor.ServiceMonitor, dpa *oadpv1alpha1.DataProtectionApplication) error {

	if dpa == nil {
		return fmt.Errorf("dpa CR cannot be nil")
	}

	if serviceMonitor == nil {
		return fmt.Errorf("service monitor cannot be nil")
	}

	// Setting controller owner reference on the service monitor
	err := controllerutil.SetControllerReference(dpa, serviceMonitor, r.Scheme)
	if err != nil {
		return err
	}

	serviceMonitor.Spec.Selector = metav1.LabelSelector{
		MatchLabels: map[string]string{
			"app.kubernetes.io/name":       common.Velero,
			"app.kubernetes.io/instance":   dpa.Name,
			"app.kubernetes.io/managed-by": common.OADPOperator,
			"app.kubernetes.io/component":  Server,
			oadpv1alpha1.OadpOperatorLabel: "True",
		},
	}

	serviceMonitor.Labels = map[string]string{
		"app.kubernetes.io/name":       common.Velero,
		"app.kubernetes.io/instance":   dpa.Name,
		"app.kubernetes.io/managed-by": common.OADPOperator,
		"app.kubernetes.io/component":  Server,
		oadpv1alpha1.OadpOperatorLabel: "True",
	}

	serviceMonitor.Spec.Endpoints = []monitor.Endpoint{
		{
			Interval: "30s",
			Port:     "monitoring",
			MetricRelabelConfigs: []*monitor.RelabelConfig{
				{
					Action: "keep",
					Regex:  ("velero_backup_total|velero_restore_total"),
					SourceLabels: []string{
						"__name__",
					},
				},
			},
		},
	}

	//serviceMonitor.Spec.JobLabel = "app"

	serviceMonitor.Spec.NamespaceSelector = monitor.NamespaceSelector{
		MatchNames: []string{
			dpa.Namespace,
		},
	}

	return nil
}

func (r *DPAReconciler) ReconcileVeleroMetricsSVC(log logr.Logger) (bool, error) {
	dpa := oadpv1alpha1.DataProtectionApplication{}
	if err := r.Get(r.Context, r.NamespacedName, &dpa); err != nil {
		return false, err
	}

	svc := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "openshift-adp-velero-metrics-svc",
			Namespace: r.NamespacedName.Namespace,
		},
	}

	// Create SVC
	op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, &svc, func() error {
		// TODO: check for svc status condition errors and respond here
		err := r.updateVeleroMetricsSVC(&svc, &dpa)

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
			fmt.Sprintf("performed %s on dpa metrics service %s/%s", op, svc.Namespace, svc.Name),
		)
	}

	return true, nil
}

func (r *DPAReconciler) updateVeleroMetricsSVC(svc *corev1.Service, dpa *oadpv1alpha1.DataProtectionApplication) error {
	// Setting controller owner reference on the metrics svc
	err := controllerutil.SetControllerReference(dpa, svc, r.Scheme)
	if err != nil {
		return err
	}

	// when updating the spec fields we update each field individually
	// to get around the immutable fields
	svc.Spec.Selector = map[string]string{
		"app.kubernetes.io/name":       common.Velero,
		"app.kubernetes.io/instance":   dpa.Name,
		"app.kubernetes.io/managed-by": common.OADPOperator,
		"app.kubernetes.io/component":  Server,
		oadpv1alpha1.OadpOperatorLabel: "True",
	}

	svc.Spec.Type = corev1.ServiceTypeClusterIP
	svc.Spec.Ports = []corev1.ServicePort{
		{
			Name: "monitoring",
			Port: int32(8085),
			TargetPort: intstr.IntOrString{
				IntVal: int32(8085),
			},
		},
	}

	svc.Labels = map[string]string{
		"app.kubernetes.io/name":       common.Velero,
		"app.kubernetes.io/instance":   dpa.Name,
		"app.kubernetes.io/managed-by": common.OADPOperator,
		"app.kubernetes.io/component":  Server,
		oadpv1alpha1.OadpOperatorLabel: "True",
	}
	return nil
}
