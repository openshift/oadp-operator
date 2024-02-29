package controllers

import (
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
)

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
	op, err := controllerutil.CreateOrPatch(r.Context, r.Client, &svc, func() error {
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
	svc.Spec.Selector = getDpaAppLabels(dpa)

	svc.Spec.Type = corev1.ServiceTypeClusterIP
	svc.Spec.Ports = []corev1.ServicePort{
		{
			Protocol: corev1.ProtocolTCP,
			Name:     "monitoring",
			Port:     int32(8085),
			TargetPort: intstr.IntOrString{
				IntVal: int32(8085),
			},
		},
	}

	svc.Labels = getDpaAppLabels(dpa)
	return nil
}
