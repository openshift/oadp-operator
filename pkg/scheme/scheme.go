package scheme

import (
	"os"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/go-logr/logr"
	monitor "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.

	_ "k8s.io/client-go/plugin/pkg/client/auth"

	routev1 "github.com/openshift/api/route/v1"
	security "github.com/openshift/api/security/v1"
	"github.com/openshift/oadp-operator/api/v1alpha1"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)
func AddToScheme(scheme *runtime.Scheme, setupLog logr.Logger) {
		// Setup scheme for OCP resources
	if err := monitor.AddToScheme(scheme); err != nil {
		setupLog.Error(err, "unable to add OpenShift monitoring APIs to scheme")
		os.Exit(1)
	}

	if err := security.AddToScheme(scheme); err != nil {
		setupLog.Error(err, "unable to add OpenShift security APIs to scheme")
		os.Exit(1)
	}

	if err := routev1.AddToScheme(scheme); err != nil {
		setupLog.Error(err, "unable to add OpenShift route API to scheme")
		os.Exit(1)
	}

	if err := velerov1.AddToScheme(scheme); err != nil {
		setupLog.Error(err, "unable to add Velero APIs to scheme")
		os.Exit(1)
	}

	if err := appsv1.AddToScheme(scheme); err != nil {
		setupLog.Error(err, "unable to add Kubernetes APIs to scheme")
		os.Exit(1)
	}

	if err := v1.AddToScheme(scheme); err != nil {
		setupLog.Error(err, "unable to add Kubernetes API extensions to scheme")
		os.Exit(1)
	}

	if err := v1alpha1.AddToScheme(scheme); err != nil {
		setupLog.Error(err, "unable to add Kubernetes API extensions to scheme")
		os.Exit(1)
	}
}