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
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	snapshotv1api "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	configv1 "github.com/openshift/api/config/v1"
	routev1 "github.com/openshift/api/route/v1"
	security "github.com/openshift/api/security/v1"
	monitor "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/internal/controller"

	//+kubebuilder:scaffold:imports
	"github.com/openshift/oadp-operator/pkg/common"
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
	var secureMetrics bool
	var enableHTTP2 bool
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", false,
		"If set the metrics endpoint is served securely")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
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

	// check if this is standardized STS workflow via OLM and CCO
	if err := common.STSStandardizedFlow(); err != nil {
				// create cred request
				// setupLog.Info(fmt.Sprintf("Creating AWS credentials request for role: %s, and WebIdentityTokenPath: %s", roleARN, WebIdentityTokenPath))
				// if err := CreateOrUpdateSTSAWSSecret(roleARN, WebIdentityTokenPath, watchNamespace, kubeconf); err != nil {
				// 	if !errors.IsAlreadyExists(err) {
				// 		setupLog.Error(err, "unable to create AWS credRequest")
				// 		os.Exit(1)
				// 	}
				// }

				// create cred request
				// setupLog.Info(fmt.Sprintf("Creating GCP credentials request for audience: %s, service account email: %s, and WebIdentityTokenPath: %s",
				// 	audience, serviceAccountEmail, gcpIdentityTokenFile))
				// if err := CreateOrUpdateGCPCredRequest(audience, serviceAccountEmail, gcpIdentityTokenFile, watchNamespace, kubeconf); err != nil {
				// 	if !errors.IsAlreadyExists(err) {
				// 		setupLog.Error(err, "unable to create GCP credRequest")
				// 		os.Exit(1)
				// 	}
				// }
	}

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancelation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	tlsOpts := []func(*tls.Config){}
	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	webhookServer := webhook.NewServer(webhook.Options{
		TLSOpts: tlsOpts,
	})

	mgr, err := ctrl.NewManager(kubeconf, ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress:   metricsAddr,
			SecureServing: secureMetrics,
			TLSOpts:       tlsOpts,
		},
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaseDuration:          &leConfig.LeaseDuration.Duration,
		RenewDeadline:          &leConfig.RenewDeadline.Duration,
		RetryPeriod:            &leConfig.RetryPeriod.Duration,
		LeaderElectionID:       "oadp.openshift.io",
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

	if err := snapshotv1api.AddToScheme(mgr.GetScheme()); err != nil {
		setupLog.Error(err, "unable to add snapshot API to scheme")
	}

	uncachedClientScheme := runtime.NewScheme()
	utilruntime.Must(oadpv1alpha1.AddToScheme(uncachedClientScheme))
	utilruntime.Must(appsv1.AddToScheme(uncachedClientScheme))
	utilruntime.Must(snapshotv1api.AddToScheme(uncachedClientScheme))
	uncachedClient, err := client.New(kubeconf, client.Options{
		Scheme: uncachedClientScheme,
	})
	if err != nil {
		setupLog.Error(err, "unable to create Kubernetes client")
		os.Exit(1)
	}

	if err = (&controller.DataProtectionApplicationReconciler{
		Client:            mgr.GetClient(),
		Scheme:            mgr.GetScheme(),
		EventRecorder:     mgr.GetEventRecorderFor("DPA-controller"),
		ClusterWideClient: uncachedClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DataProtectionApplication")
		os.Exit(1)
	}

	if err = (&controller.CloudStorageReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		EventRecorder: mgr.GetEventRecorderFor("CloudStorage-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "CloudStorage")
		os.Exit(1)
	}

	if err = (&controller.DataProtectionTestReconciler{
		Client:            mgr.GetClient(),
		Scheme:            mgr.GetScheme(),
		EventRecorder:     mgr.GetEventRecorderFor("DPT-controller"),
		ClusterWideClient: uncachedClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DataProtectionTest")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

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
// WaitForSecret is a function that takes a Kubernetes client, a namespace, and a v1 "k8s.io/api/core/v1" name as arguments
// It waits until the secret object with the given name exists in the given namespace
// It returns the secret object or an error if the timeout is exceeded
func WaitForSecret(client kubernetes.Interface, namespace, name string) (*corev1.Secret, error) {
	// set a timeout of 10 minutes
	timeout := time.After(10 * time.Minute)

	// set a polling interval of 10 seconds
	ticker := time.NewTicker(10 * time.Second)

	// loop until the timeout or the secret is found
	for {
		select {
		case <-timeout:
			// timeout is exceeded, return an error
			return nil, fmt.Errorf("timed out waiting for secret %s in namespace %s. Please follow the manual path to create a Secret", name, namespace)
		case <-ticker.C:
			// polling interval is reached, try to get the secret
			secret, err := client.CoreV1().Secrets(namespace).Get(context.Background(), name, metav1.GetOptions{})
			if err != nil {
				if errors.IsNotFound(err) {
					// secret does not exist yet, continue waiting
					continue
				} else {
					// some other error occurred, return it
					return nil, err
				}
			} else {
				// secret is found, return it
				return secret, nil
			}
		}
	}
}

// GetGCPCredentialsFromSecret waits for the cloud-credentials Secret
// to be created by CCO and reads the service_account.json field
func GetGCPCredentialsFromSecret(clientset kubernetes.Interface, namespace string) (string, error) {
	// Wait for the Secret to be created by CCO
	secret, err := WaitForSecret(clientset, namespace, "cloud-credentials")
	if err != nil {
		return "", fmt.Errorf("error waiting for GCP credentials Secret: %v", err)
	}

	// Read the service_account.json field from the Secret
	serviceAccountJSON, ok := secret.Data["service_account.json"]
	if !ok {
		return "", fmt.Errorf("cloud-credentials Secret does not contain service_account.json field")
	}

	return string(serviceAccountJSON), nil
}

func CreateOrUpdateGCPCredRequest(audience string, serviceAccountEmail string, cloudTokenPath string, secretNS string, kubeconf *rest.Config) error {
	clientInstance, err := client.New(kubeconf, client.Options{})
	if err != nil {
		setupLog.Error(err, "unable to create client")
		return err
	}

	// Create a clientset to use for waiting on the secret
	clientset, err := kubernetes.NewForConfig(kubeconf)
	if err != nil {
		setupLog.Error(err, "unable to create clientset")
		return err
	}

	credRequest := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cloudcredential.openshift.io/v1",
			"kind":       "CredentialsRequest",
			"metadata": map[string]interface{}{
				"name":      "oadp-gcp-credentials-request",
				"namespace": "openshift-cloud-credential-operator",
			},
			"spec": map[string]interface{}{
				"secretRef": map[string]interface{}{
					"name":      "cloud-credentials",
					"namespace": secretNS,
				},
				"serviceAccountNames": []interface{}{
					common.OADPOperatorServiceAccount,
				},
				"providerSpec": map[string]interface{}{
					"apiVersion":          "cloudcredential.openshift.io/v1",
					"kind":                "GCPProviderSpec",
					"audience":            audience,
					"serviceAccountEmail": serviceAccountEmail,
				},
				"cloudTokenPath": cloudTokenPath,
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
			err = clientInstance.Get(context.Background(), types.NamespacedName{Name: "oadp-gcp-credentials-request", Namespace: "openshift-cloud-credential-operator"}, fromCluster)
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

	// Wait for the Secret to be created by CCO
	setupLog.Info("Waiting for cloud-credentials Secret to be created by Cloud Credential Operator")
	_, err = WaitForSecret(clientset, secretNS, "cloud-credentials")
	if err != nil {
		setupLog.Error(err, "error waiting for AWS credentials Secret")
		return err
	}
	setupLog.Info("cloud-credentials Secret is now available")

	return nil
}

func CreateOrUpdateSTSAWSSecret(roleARN string, WITP string, secretNS string, kubeconf *rest.Config) error {
	clientInstance, err := client.New(kubeconf, client.Options{})
	if err != nil {
		setupLog.Error(err, "unable to create client")
		return err
	}

	// Create a clientset to use for waiting on the secret
	clientset, err := kubernetes.NewForConfig(kubeconf)
	if err != nil {
		setupLog.Error(err, "unable to create clientset")
		return err
	}

	// For an STS cluster
	// replicate what https://github.com/openshift/cloud-credential-operator/blob/6a880b473554aee4d1f3cd125048fef2bca6a04d/pkg/aws/actuator/actuator.go#L405-L441
	// does
	awsSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cloud-credentials",
			Namespace: secretNS,
		},
		StringData: map[string]string{
			// TODO: region from BSL
			"credentials": fmt.Sprintf(`[default]
sts_regional_endpoints = regional
role_arn = %s
web_identity_token_file = %s`, roleARN, WITP),
		},
	}
	verb := "created"
	if err := clientInstance.Create(context.Background(), &awsSecret); err != nil {
		if errors.IsAlreadyExists(err) {
			verb = "updated"
			setupLog.Info("Secret already exists, updating")
			fromCluster := corev1.Secret{}
			err = clientInstance.Get(context.Background(), types.NamespacedName{Name: awsSecret.Name, Namespace: awsSecret.Namespace}, &fromCluster)
			if err != nil {
				setupLog.Error(err, "unable to get existing credentials request resource")
				return err
			}
			// update StringData
			updatedFromCluster := fromCluster.DeepCopy()
			updatedFromCluster.StringData = awsSecret.StringData
			if err := clientInstance.Patch(context.Background(), updatedFromCluster, client.MergeFrom(&fromCluster)); err != nil {
				setupLog.Error(err, fmt.Sprintf("unable to update secret resource, %v, %+v", err, fromCluster))
				return err
			}
		} else {
			setupLog.Error(err, "unable to create secret resource")
			return err
		}
	}
	setupLog.Info("Custom resource credentialsrequest " + verb + " successfully")

	// Wait for the Secret to be created by CCO
	setupLog.Info("Waiting for cloud-credentials Secret to be created by Cloud Credential Operator")
	_, err = WaitForSecret(clientset, secretNS, "cloud-credentials")
	if err != nil {
		setupLog.Error(err, "error waiting for AWS credentials Secret")
		return err
	}
	setupLog.Info("cloud-credentials Secret is now available")

	return nil
}
