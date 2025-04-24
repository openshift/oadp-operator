package lib

import (
	volumesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	openshiftappsv1 "github.com/openshift/api/apps/v1"
	openshiftbuildv1 "github.com/openshift/api/build/v1"
	openshiftconfigv1 "github.com/openshift/api/config/v1"
	openshiftroutev1 "github.com/openshift/api/route/v1"
	openshiftsecurityv1 "github.com/openshift/api/security/v1"
	openshifttemplatev1 "github.com/openshift/api/template/v1"
	hypershiftv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
)

var (
	Scheme = apiruntime.NewScheme()
)

func init() {
	_ = oadpv1alpha1.AddToScheme(Scheme)
	_ = velerov1.AddToScheme(Scheme)
	_ = openshiftappsv1.AddToScheme(Scheme)
	_ = openshiftbuildv1.AddToScheme(Scheme)
	_ = openshiftsecurityv1.AddToScheme(Scheme)
	_ = openshifttemplatev1.AddToScheme(Scheme)
	_ = openshiftroutev1.AddToScheme(Scheme)
	_ = corev1.AddToScheme(Scheme)
	_ = volumesnapshotv1.AddToScheme(Scheme)
	_ = operatorsv1alpha1.AddToScheme(Scheme)
	_ = operatorsv1.AddToScheme(Scheme)
	_ = hypershiftv1.AddToScheme(Scheme)
	_ = appsv1.AddToScheme(Scheme)
	_ = openshiftconfigv1.AddToScheme(Scheme)
}
