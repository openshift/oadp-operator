package controller

import (
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	oadpResourceQuotaName             = "oadp-resource-quota"
	oadpResourceQuotaPolicyAnnotation = "oadp.openshift.io/quota-policy"
	oadpResourceQuotaPolicyValue      = "create-if-absent"
)

// defaultOADPResourceQuotaHard is a generous mid-size ceiling for the OADP
// namespace. It intentionally omits limits.* because default Velero and
// node-agent pods set requests only; tracking limits in the quota can reject
// those pods at admission.
func defaultOADPResourceQuotaHard() corev1.ResourceList {
	return corev1.ResourceList{
		corev1.ResourcePods:           resource.MustParse("200"),
		corev1.ResourceRequestsCPU:    resource.MustParse("50"),
		corev1.ResourceRequestsMemory: resource.MustParse("64Gi"),
	}
}

// ReconcileResourceQuota ensures ResourceQuota/oadp-resource-quota exists in
// the DPA namespace. Create-if-absent: never updates an existing quota's spec.
func (r *DataProtectionApplicationReconciler) ReconcileResourceQuota(log logr.Logger) (bool, error) {
	log = log.WithValues("resourcequota", oadpResourceQuotaName)

	ns := r.dpa.Namespace
	existing := &corev1.ResourceQuota{}
	err := r.Get(r.Context, types.NamespacedName{Name: oadpResourceQuotaName, Namespace: ns}, existing)
	if err == nil {
		log.V(1).Info("ResourceQuota already exists; leaving unchanged")
		return true, nil
	}
	if !apierrors.IsNotFound(err) {
		return false, err
	}

	quota := &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      oadpResourceQuotaName,
			Namespace: ns,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "oadp-operator",
				"app.kubernetes.io/managed-by": "oadp-operator",
			},
			Annotations: map[string]string{
				oadpResourceQuotaPolicyAnnotation: oadpResourceQuotaPolicyValue,
			},
		},
		Spec: corev1.ResourceQuotaSpec{
			Hard: defaultOADPResourceQuotaHard(),
		},
	}

	log.Info("Creating default ResourceQuota")
	if err := r.Create(r.Context, quota); err != nil {
		// Another reconcile may have created it concurrently.
		if apierrors.IsAlreadyExists(err) {
			return true, nil
		}
		return false, err
	}
	return true, nil
}
