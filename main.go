/*
Copyright 2021.

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

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	consolev1 "github.com/openshift/api/console/v1"
	routev1 "github.com/openshift/api/route/v1"
	security "github.com/openshift/api/security/v1"
	"github.com/openshift/oadp-operator/pkg/common"
	monitor "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	client "sigs.k8s.io/controller-runtime/pkg/client"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/openshift/oadp-operator/controllers"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	//+kubebuilder:scaffold:imports
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/leaderelection"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

const (
	// WebIdentityTokenPath mount present on operator CSV
	WebIdentityTokenPath = "/var/run/secrets/openshift/serviceaccount/token"

	// CloudCredentials API constants
	CloudCredentialGroupVersion = "cloudcredential.openshift.io/v1"
	CloudCredentialsCRDName     = "credentialsrequests"

	// Pod security admission (PSA) labels
	psaLabelPrefix = "pod-security.kubernetes.io/"
	enforceLabel   = psaLabelPrefix + "enforce"
	auditLabel     = psaLabelPrefix + "audit"
	warnLabel      = psaLabelPrefix + "warn"

	privileged = "privileged"

	// CRD availability wait configuration for IBU (Image-Based Upgrade) scenarios.
	// During IBU on SNO clusters, the node reboots into a new OS image and OpenShift
	// CRDs may not be immediately available while the API server initializes.
	// See: https://issues.redhat.com/browse/OADP-7419
	crdWaitInterval = 10 * time.Second
	crdWaitTimeout  = 10 * time.Minute
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(oadpv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	kubeconf := ctrl.GetConfigOrDie()

	// Get LeaderElection configs
	leConfig := leaderelection.GetLeaderElectionConfig(kubeconf, enableLeaderElection)

	watchNamespace, err := getWatchNamespace()
	if err != nil {
		setupLog.Error(err, "unable to get WatchNamespace, "+
			"the manager will watch and manage resources in all namespaces")
	}

	clientset, err := kubernetes.NewForConfig(kubeconf)
	if err != nil {
		setupLog.Error(err, "problem getting client")
		os.Exit(1)
	}

	// setting privileged pod security labels to operator ns
	err = addPodSecurityPrivilegedLabels(watchNamespace, clientset)
	if err != nil {
		setupLog.Error(err, "error setting privileged pod security labels to operator namespace")
		os.Exit(1)
	}

	// Wait for required OpenShift CRDs before starting the manager.
	// During IBU (Image-Based Upgrade) on SNO clusters, the node reboots and external
	// OpenShift operators may not have registered their CRDs yet. Without this wait,
	// the controller crashes trying to watch Route/SCC resources that don't exist.
	// See: https://issues.redhat.com/browse/OADP-7419
	if err := waitForRequiredCRDs(context.Background(), kubeconf); err != nil {
		setupLog.Error(err, "failed waiting for required CRDs")
		os.Exit(1)
	}

	// check if this is standardized STS workflow via OLM and CCO
	if common.CCOWorkflow() {
		setupLog.Info("AWS Role ARN specified by the user, following standardized STS workflow")
		// ROLEARN env var is set via operator subscription
		roleARN := os.Getenv("ROLEARN")
		setupLog.Info("getting role ARN", "role ARN =", roleARN)

		// check if cred request API exists in the cluster before creating a cred request
		setupLog.Info("Checking if credentialsrequest CRD exists in the cluster")
		credReqCRDExists, err := DoesCRDExist(CloudCredentialGroupVersion, CloudCredentialsCRDName, kubeconf)
		if err != nil {
			setupLog.Error(err, "problem checking the existence of CredentialRequests CRD")
			os.Exit(1)
		}

		if credReqCRDExists {
			// create cred request
			setupLog.Info(fmt.Sprintf("Creating credentials request for role: %s, and WebIdentityTokenPath: %s", roleARN, WebIdentityTokenPath))
			if err := CreateOrUpdateCredRequest(roleARN, WebIdentityTokenPath, watchNamespace, kubeconf); err != nil {
				if !errors.IsAlreadyExists(err) {
					setupLog.Error(err, "unable to create credRequest")
					os.Exit(1)
				}
			}
		}
	}

	mgr, err := ctrl.NewManager(kubeconf, ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		WebhookServer: &webhook.DefaultServer{
			Options: webhook.Options{
				Port: 9443,
			},
		},
		HealthProbeBindAddress: probeAddr,
		// Enable leader election only if requested via flag AND not disabled by config.
		// On SNO clusters, leConfig.Disable is set to true to avoid leader election
		// overhead and delays during IBU. See: https://issues.redhat.com/browse/OADP-7419
		LeaderElection:   enableLeaderElection && !leConfig.Disable,
		LeaseDuration:    &leConfig.LeaseDuration.Duration,
		RenewDeadline:    &leConfig.RenewDeadline.Duration,
		RetryPeriod:      &leConfig.RetryPeriod.Duration,
		LeaderElectionID: "oadp.openshift.io",
		// LeaderElectionReleaseOnCancel ensures the leader lease is released when the
		// controller crashes or shuts down. Without this, on SNO clusters with 270s lease
		// duration, a crashed controller would block the new instance from acquiring
		// leadership for ~4.5 minutes. See: https://issues.redhat.com/browse/OADP-7419
		LeaderElectionReleaseOnCancel: true,
		Cache: cache.Options{
			DefaultNamespaces: map[string]cache.Config{
				watchNamespace: {},
			},
		},
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Setup scheme for OCP resources
	if err := monitor.AddToScheme(mgr.GetScheme()); err != nil {
		setupLog.Error(err, "unable to add OpenShift monitoring APIs to scheme")
		os.Exit(1)
	}

	if err := security.AddToScheme(mgr.GetScheme()); err != nil {
		setupLog.Error(err, "unable to add OpenShift security APIs to scheme")
		os.Exit(1)
	}

	if err := routev1.AddToScheme(mgr.GetScheme()); err != nil {
		setupLog.Error(err, "unable to add OpenShift route API to scheme")
		os.Exit(1)
	}

	if err := velerov1.AddToScheme(mgr.GetScheme()); err != nil {
		setupLog.Error(err, "unable to add Velero APIs to scheme")
		os.Exit(1)
	}

	if err := appsv1.AddToScheme(mgr.GetScheme()); err != nil {
		setupLog.Error(err, "unable to add Kubernetes APIs to scheme")
		os.Exit(1)
	}

	if err := v1.AddToScheme(mgr.GetScheme()); err != nil {
		setupLog.Error(err, "unable to add Kubernetes API extensions to scheme")
		os.Exit(1)
	}

	if err := configv1.AddToScheme(mgr.GetScheme()); err != nil {
		setupLog.Error(err, "unable to add OpenShift configuration API to scheme")
		os.Exit(1)
	}

	if err := consolev1.AddToScheme(mgr.GetScheme()); err != nil {
		setupLog.Error(err, "unable to add OpenShift console API to scheme")
		os.Exit(1)
	}

	if err = (&controllers.DPAReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		EventRecorder: mgr.GetEventRecorderFor("DPA-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DataProtectionApplication")
		os.Exit(1)
	}

	if err = (&controllers.BucketReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		EventRecorder: mgr.GetEventRecorderFor("bucket-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Bucket")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	// Add CLI download setup runnable.
	// Skip when the ConsoleCLIDownload CRD is absent (clusters without Console capability, e.g. SNO).
	if available, err := controllers.IsConsoleCRDAvailable(mgr.GetRESTMapper(), setupLog); !available {
		if err != nil {
			setupLog.Error(err, "unable to check ConsoleCLIDownload CRD availability, skipping CLI download setup")
		}
	} else {
		if err := mgr.Add(&controllers.CLIDownloadSetup{
			Client:            mgr.GetClient(),
			Namespace:         watchNamespace,
			OperatorName:      "openshift-adp-controller-manager",
			OperatorNamespace: watchNamespace,
		}); err != nil {
			setupLog.Error(err, "unable to add CLI download setup")
			os.Exit(1)
		}
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

// getWatchNamespace returns the Namespace the operator should be watching for changes
func getWatchNamespace() (string, error) {
	// WatchNamespaceEnvVar is the constant for env variable WATCH_NAMESPACE
	// which specifies the Namespace to watch.
	// An empty value means the operator is running with cluster scope.
	var watchNamespaceEnvVar = "WATCH_NAMESPACE"

	ns, found := os.LookupEnv(watchNamespaceEnvVar)
	if !found {
		return "", fmt.Errorf("%s must be set", watchNamespaceEnvVar)
	}
	return ns, nil
}

// setting Pod security admission (PSA) labels to privileged in OADP operator namespace
func addPodSecurityPrivilegedLabels(watchNamespaceName string, clientset kubernetes.Interface) error {
	setupLog.Info("patching operator namespace with Pod security admission (PSA) labels to privileged")

	if len(watchNamespaceName) == 0 {
		return fmt.Errorf("cannot patch operator namespace with PSA labels to privileged, watchNamespaceName is empty")
	}

	nsPatch, err := json.Marshal(map[string]interface{}{
		"metadata": map[string]interface{}{
			"labels": map[string]string{
				enforceLabel: privileged,
				auditLabel:   privileged,
				warnLabel:    privileged,
			},
		},
	})
	if err != nil {
		setupLog.Error(err, "problem marshalling patches")
		return err
	}
	_, err = clientset.CoreV1().Namespaces().Patch(context.TODO(), watchNamespaceName, types.StrategicMergePatchType, nsPatch, metav1.PatchOptions{})
	if err != nil {
		setupLog.Error(err, "problem patching operator namespace with PSA labels to privileged")
		return err
	}
	return nil
}

func DoesCRDExist(CRDGroupVersion, CRDName string, kubeconf *rest.Config) (bool, error) {
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(kubeconf)
	if err != nil {
		return false, err
	}

	resources, err := discoveryClient.ServerPreferredResources()
	if err != nil {
		return false, err
	}
	discoveryResult := false
	for _, resource := range resources {
		if resource.GroupVersion == CRDGroupVersion {
			for _, crd := range resource.APIResources {
				if crd.Name == CRDName {
					discoveryResult = true
					break
				}
			}
		}
	}
	return discoveryResult, nil

}

// CreateCredRequest WITP : WebIdentityTokenPath
func CreateOrUpdateCredRequest(roleARN string, WITP string, secretNS string, kubeconf *rest.Config) error {
	clientInstance, err := client.New(kubeconf, client.Options{})
	if err != nil {
		setupLog.Error(err, "unable to create client")
	}

	// Extra deps were getting added and existing ones were getting upgraded when the CloudCredentials API was imported
	// This caused updates to go.mod and started resulting in operator build failures due to incompatibility with the existing velero deps
	// Hence for now going via the unstructured route
	credRequest := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cloudcredential.openshift.io/v1",
			"kind":       "CredentialsRequest",
			"metadata": map[string]interface{}{
				"name":      "oadp-aws-credentials-request",
				"namespace": "openshift-cloud-credential-operator",
			},
			"spec": map[string]interface{}{
				"secretRef": map[string]interface{}{
					"name":      "cloud-credentials",
					"namespace": secretNS,
				},
				"serviceAccountNames": []interface{}{
					"openshift-adp-controller-manager",
				},
				"providerSpec": map[string]interface{}{
					"apiVersion": "cloudcredential.openshift.io/v1",
					"kind":       "AWSProviderSpec",
					"statementEntries": []interface{}{
						map[string]interface{}{
							"effect": "Allow",
							"action": []interface{}{
								"s3:*",
							},
							"resource": "arn:aws:s3:*:*:*",
						},
					},
					"stsIAMRoleARN": roleARN,
				},
				"cloudTokenPath": WITP,
			},
		},
	}
	verb := "created"
	if err := clientInstance.Create(context.Background(), credRequest); err != nil {
		if errors.IsAlreadyExists(err) {
			verb = "updated"
			setupLog.Info("CredentialsRequest already exists, updating")
			fromCluster := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "cloudcredential.openshift.io/v1",
					"kind":       "CredentialsRequest",
				},
			}
			err = clientInstance.Get(context.Background(), types.NamespacedName{Name: "oadp-aws-credentials-request", Namespace: "openshift-cloud-credential-operator"}, fromCluster)
			if err != nil {
				setupLog.Error(err, "unable to get existing credentials request resource")
				return err
			}
			// update spec
			fromCluster.Object["spec"] = credRequest.Object["spec"]
			if err := clientInstance.Update(context.Background(), fromCluster); err != nil {
				setupLog.Error(err, fmt.Sprintf("unable to update credentials request resource, %v, %+v", err, fromCluster.Object))
				return err
			}
		} else {
			setupLog.Error(err, "unable to create credentials request resource")
			return err
		}
	}
	setupLog.Info("Custom resource credentialsrequest " + verb + " successfully")
	return nil
}

// requiredCRD represents a CRD that must be available before the controller can start.
type requiredCRD struct {
	groupVersion string
	resourceName string
}

// getRequiredCRDs returns the list of CRDs that must be available before the OADP
// controller can start.
//
// Why only Route and SecurityContextConstraints?
//
// The DPA controller watches several resource types via Owns() calls:
//   - Core K8s resources (Deployment, Service, ConfigMap, etc.) - always available
//   - OADP CRDs (DPA, CloudStorage) - installed by OLM before operator starts
//   - Velero CRDs (BSL, VSL) - part of OADP CSV bundle, OLM guarantees availability
//   - OpenShift CRDs (Route, SCC) - registered by OTHER OpenShift operators
//
// Route and SCC are the only CRDs that are:
//  1. Required by the DPA controller (via Owns() in SetupWithManager)
//  2. NOT installed by OADP itself
//  3. Registered by external OpenShift operators (openshift-apiserver, authentication-operator)
//
// During Image-Based Upgrade (IBU) on SNO clusters, the node reboots and these
// external operators may not have registered their CRDs yet when OADP starts.
// This causes cache sync failures and controller crashes.
//
// See: https://issues.redhat.com/browse/OADP-7419
func getRequiredCRDs() []requiredCRD {
	return []requiredCRD{
		{groupVersion: "route.openshift.io/v1", resourceName: "routes"},
		{groupVersion: "security.openshift.io/v1", resourceName: "securitycontextconstraints"},
	}
}

// waitForRequiredCRDs waits for external OpenShift CRDs to be registered before
// starting the controller.
//
// Context: During Image-Based Upgrade (IBU) on Single Node OpenShift (SNO) clusters,
// the node reboots into a new OS image. The kube-apiserver starts before OpenShift
// operators have registered their CRDs (Route, SecurityContextConstraints).
//
// Without this wait, the controller attempts to create informers for these CRDs,
// fails with "no matches for kind", and crashes after the cache sync timeout (2 min).
// Combined with the SNO leader lease duration (270s), this causes an ~8 minute delay
// before the DPA can reconcile.
//
// By explicitly waiting for CRDs via the discovery API, we avoid the crash entirely
// and allow the controller to start as soon as the CRDs are available.
//
// See: https://issues.redhat.com/browse/OADP-7419
func waitForRequiredCRDs(ctx context.Context, kubeconf *rest.Config) error {
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(kubeconf)
	if err != nil {
		return fmt.Errorf("failed to create discovery client: %w", err)
	}
	return waitForRequiredCRDsWithClient(ctx, discoveryClient)
}

// waitForRequiredCRDsWithClient is the internal implementation that accepts a
// discovery client directly. This allows for easier unit testing with mock clients.
func waitForRequiredCRDsWithClient(ctx context.Context, discoveryClient discovery.DiscoveryInterface) error {
	requiredCRDs := getRequiredCRDs()

	setupLog.Info("Waiting for required OpenShift CRDs to be available",
		"crds", requiredCRDs,
		"timeout", crdWaitTimeout.String())

	// Use a shorter interval for testing (context deadline will control actual timeout)
	interval := crdWaitInterval
	timeout := crdWaitTimeout

	// If context has a deadline shorter than our default timeout, use that
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining < timeout {
			timeout = remaining
		}
		// Use shorter interval for short timeouts (testing)
		if remaining < 1*time.Second {
			interval = 10 * time.Millisecond
		}
	}

	return wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
		for _, crd := range requiredCRDs {
			if !isCRDAvailable(discoveryClient, crd.groupVersion, crd.resourceName) {
				setupLog.Info("Waiting for CRD to be registered",
					"groupVersion", crd.groupVersion,
					"resource", crd.resourceName)
				return false, nil
			}
		}
		setupLog.Info("All required CRDs are available, proceeding with controller startup")
		return true, nil
	})
}

// isCRDAvailable checks if a specific CRD is registered with the API server
// by querying the discovery API for the given group version and resource name.
// Returns true if the CRD is available, false otherwise (including on errors).
func isCRDAvailable(discoveryClient discovery.DiscoveryInterface, groupVersion, resourceName string) bool {
	resourceList, err := discoveryClient.ServerResourcesForGroupVersion(groupVersion)
	if err != nil {
		// If the group version doesn't exist, the CRD is not available
		return false
	}

	for _, resource := range resourceList.APIResources {
		if resource.Name == resourceName {
			return true
		}
	}
	return false
}
