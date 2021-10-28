package controllers

import (
	"fmt"
	"os"
	"reflect"

	"github.com/openshift/oadp-operator/pkg/credentials"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"

	//"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"
	security "github.com/openshift/api/security/v1"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/common"
	"github.com/vmware-tanzu/velero/pkg/install"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"

	//"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	Server   = "server"
	Registry = "Registry"
	//TODO: Check for default secret names
	VeleroAWSSecretName   = "cloud-credentials"
	VeleroAzureSecretName = "cloud-credentials-azure"
	VeleroGCPSecretName   = "cloud-credentials-gcp"
	enableCSIFeatureFlag  = "EnableCSI"
)

var (
	veleroLabelSelector = &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"component": common.Velero,
			"deploy":    common.Velero,
		},
	}
)

// TODO: Remove this function as it's no longer being used
func (r *VeleroReconciler) ReconcileVeleroServiceAccount(log logr.Logger) (bool, error) {
	velero := oadpv1alpha1.Velero{}
	if err := r.Get(r.Context, r.NamespacedName, &velero); err != nil {
		return false, err
	}
	veleroSa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      common.Velero,
			Namespace: velero.Namespace,
		},
	}
	op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, veleroSa, func() error {
		// Setting controller owner reference on the velero SA
		err := controllerutil.SetControllerReference(&velero, veleroSa, r.Scheme)
		if err != nil {
			return err
		}

		// update the SA template
		veleroSaUpdate, err := r.veleroServiceAccount(&velero)
		veleroSa = veleroSaUpdate
		return err
	})

	if err != nil {
		return false, err
	}

	//TODO: Review velero SA status and report errors and conditions

	if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
		// Trigger event to indicate velero SA was created or updated
		r.EventRecorder.Event(veleroSa,
			corev1.EventTypeNormal,
			"VeleroServiceAccountReconciled",
			fmt.Sprintf("performed %s on velero service account %s/%s", op, veleroSa.Namespace, veleroSa.Name),
		)
	}
	return true, nil
}

// TODO: Remove this function as it's no longer being used
//TODO: Temporary solution for Non-OLM Operator install
func (r *VeleroReconciler) ReconcileVeleroCRDs(log logr.Logger) (bool, error) {
	velero := oadpv1alpha1.Velero{}
	if err := r.Get(r.Context, r.NamespacedName, &velero); err != nil {
		return false, err
	}

	// check for Non-OLM install and proceed with Velero supporting CRD installation
	/*if velero.Spec.OlmManaged != nil && !*velero.Spec.OlmManaged {
		err := r.InstallVeleroCRDs(log)
		if err != nil {
			return false, err
		}
	}*/

	return true, nil
}

// TODO: Remove this function as it's no longer being used
func (r *VeleroReconciler) InstallVeleroCRDs(log logr.Logger) error {
	var err error
	// Install CRDs
	for _, unstructuredCrd := range install.AllCRDs("v1").Items {
		foundCrd := &v1.CustomResourceDefinition{}
		crd := &v1.CustomResourceDefinition{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredCrd.Object, crd); err != nil {
			return err
		}
		// Add Conversion to the spec, as this will be returned in the foundCrd
		crd.Spec.Conversion = &v1.CustomResourceConversion{
			Strategy: v1.NoneConverter,
		}
		if err = r.Client.Get(r.Context, types.NamespacedName{Name: crd.ObjectMeta.Name}, foundCrd); err != nil {
			if errors.IsNotFound(err) {
				// Didn't find CRD, we should create it.
				log.Info("Creating CRD", "CRD.Name", crd.ObjectMeta.Name)
				if err = r.Client.Create(r.Context, crd); err != nil {
					return err
				}
			} else {
				// Return other errors
				return err
			}
		} else {
			// CRD exists, check if it's updated.
			if !reflect.DeepEqual(foundCrd.Spec, crd.Spec) {
				// Specs aren't equal, update and fix.
				log.Info("Updating CRD", "CRD.Name", crd.ObjectMeta.Name, "foundCrd.Spec", foundCrd.Spec, "crd.Spec", crd.Spec)
				foundCrd.Spec = *crd.Spec.DeepCopy()
				if err = r.Client.Update(r.Context, foundCrd); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// TODO: Remove this function as it's no longer being used
func (r *VeleroReconciler) ReconcileVeleroClusterRoleBinding(log logr.Logger) (bool, error) {
	velero := oadpv1alpha1.Velero{}
	if err := r.Get(r.Context, r.NamespacedName, &velero); err != nil {
		return false, err
	}
	veleroCRB, err := r.veleroClusterRoleBinding(&velero)
	if err != nil {
		return false, err
	}
	op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, veleroCRB, func() error {
		// Setting controller owner reference on the velero CRB
		// TODO: HOW DO I DO THIS?? ALAY HALP PLZ
		/*err := controllerutil.SetControllerReference(&velero, veleroCRB, r.Scheme)
		if err != nil {
			return err
		}*/

		// update the CRB template
		veleroCRBUpdate, err := r.veleroClusterRoleBinding(&velero)
		veleroCRB = veleroCRBUpdate
		return err
	})

	if err != nil {
		return false, err
	}

	//TODO: Review velero CRB status and report errors and conditions

	if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
		// Trigger event to indicate velero SA was created or updated
		r.EventRecorder.Event(veleroCRB,
			corev1.EventTypeNormal,
			"VeleroClusterRoleBindingReconciled",
			fmt.Sprintf("performed %s on velero clusterrolebinding %s", op, veleroCRB.Name),
		)
	}
	return true, nil
}

func (r *VeleroReconciler) ReconcileVeleroSecurityContextConstraint(log logr.Logger) (bool, error) {
	velero := oadpv1alpha1.Velero{}
	if err := r.Get(r.Context, r.NamespacedName, &velero); err != nil {
		return false, err
	}
	sa := corev1.ServiceAccount{}
	nsName := types.NamespacedName{
		Namespace: velero.Namespace,
		Name:      common.Velero,
	}
	if err := r.Get(r.Context, nsName, &sa); err != nil {
		return false, err
	}

	veleroSCC := &security.SecurityContextConstraints{
		ObjectMeta: metav1.ObjectMeta{
			Name: "velero-privileged",
		},
	}
	op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, veleroSCC, func() error {
		// Setting controller owner reference on the velero SCC
		// TODO: HOW DO I DO THIS?? ALAY HALP PLZ
		/*err := controllerutil.SetControllerReference(&velero, veleroSCC, r.Scheme)
		if err != nil {
			return err
		}*/

		// update the SCC template
		return r.privilegedSecurityContextConstraints(veleroSCC, &velero, &sa)
	})

	if err != nil {
		return false, err
	}

	//TODO: Review velero SCC status and report errors and conditions

	if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
		// Trigger event to indicate velero SCC was created or updated
		r.EventRecorder.Event(veleroSCC,
			corev1.EventTypeNormal,
			"VeleroSecurityContextConstraintsReconciled",
			fmt.Sprintf("performed %s on velero scc %s", op, veleroSCC.Name),
		)
	}
	return true, nil
}

func (r *VeleroReconciler) ReconcileVeleroDeployment(log logr.Logger) (bool, error) {
	velero := oadpv1alpha1.Velero{}
	if err := r.Get(r.Context, r.NamespacedName, &velero); err != nil {
		return false, err
	}

	veleroDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      common.Velero,
			Namespace: velero.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, veleroDeployment, func() error {

		// Setting Deployment selector if a new object is created as it is immutable
		if veleroDeployment.ObjectMeta.CreationTimestamp.IsZero() {
			veleroDeployment.Spec.Selector = veleroLabelSelector
		}

		// Setting controller owner reference on the velero deployment
		err := controllerutil.SetControllerReference(&velero, veleroDeployment, r.Scheme)
		if err != nil {
			return err
		}
		// update the Deployment template
		return r.buildVeleroDeployment(veleroDeployment, &velero)
	})

	if err != nil {
		return false, err
	}

	//TODO: Review velero deployment status and report errors and conditions

	if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
		// Trigger event to indicate velero deployment was created or updated
		r.EventRecorder.Event(veleroDeployment,
			corev1.EventTypeNormal,
			"VeleroDeploymentReconciled",
			fmt.Sprintf("performed %s on velero deployment %s/%s", op, veleroDeployment.Namespace, veleroDeployment.Name),
		)
	}
	return true, nil
}

func (r *VeleroReconciler) veleroServiceAccount(velero *oadpv1alpha1.Velero) (*corev1.ServiceAccount, error) {
	annotations := make(map[string]string)
	sa := install.ServiceAccount(velero.Namespace, annotations)
	sa.Labels = r.getAppLabels(velero)
	return sa, nil
}

func (r *VeleroReconciler) veleroClusterRoleBinding(velero *oadpv1alpha1.Velero) (*rbacv1.ClusterRoleBinding, error) {
	crb := install.ClusterRoleBinding(velero.Namespace)
	crb.Labels = r.getAppLabels(velero)
	return crb, nil
}

func (r *VeleroReconciler) privilegedSecurityContextConstraints(scc *security.SecurityContextConstraints, velero *oadpv1alpha1.Velero, sa *corev1.ServiceAccount) error {
	// ObjectMeta set from prior step.

	scc.AllowHostDirVolumePlugin = true
	scc.AllowHostIPC = true
	scc.AllowHostNetwork = true
	scc.AllowHostPID = true
	scc.AllowHostPorts = true
	scc.AllowPrivilegeEscalation = pointer.BoolPtr(true)
	scc.AllowPrivilegedContainer = true
	scc.AllowedCapabilities = []corev1.Capability{
		security.AllowAllCapabilities,
	}
	scc.AllowedUnsafeSysctls = []string{
		"*",
	}
	scc.DefaultAddCapabilities = nil
	scc.FSGroup = security.FSGroupStrategyOptions{
		Type: security.FSGroupStrategyRunAsAny,
	}
	scc.Priority = nil
	scc.ReadOnlyRootFilesystem = false
	scc.RequiredDropCapabilities = nil
	scc.RunAsUser = security.RunAsUserStrategyOptions{
		Type: security.RunAsUserStrategyRunAsAny,
	}
	scc.SELinuxContext = security.SELinuxContextStrategyOptions{
		Type: security.SELinuxStrategyRunAsAny,
	}
	scc.SeccompProfiles = []string{
		"*",
	}
	scc.SupplementalGroups = security.SupplementalGroupsStrategyOptions{
		Type: security.SupplementalGroupsStrategyRunAsAny,
	}
	scc.Users = []string{
		"system:admin",
		fmt.Sprintf("system:serviceaccount:%s:%s", sa.Namespace, sa.Name),
	}
	scc.Volumes = []security.FSType{
		security.FSTypeAll,
	}

	return nil
}

// Build VELERO Deployment
func (r *VeleroReconciler) buildVeleroDeployment(veleroDeployment *appsv1.Deployment, velero *oadpv1alpha1.Velero) error {

	if velero == nil {
		return fmt.Errorf("velero CR cannot be nil")
	}
	if veleroDeployment == nil {
		return fmt.Errorf("velero deployment cannot be nil")
	}

	//check if CSI plugin is added in spec
	for _, plugin := range velero.Spec.DefaultVeleroPlugins {
		if plugin == oadpv1alpha1.DefaultPluginCSI {
			// CSI plugin is added so ensure that CSI feature flags is set
			velero.Spec.VeleroFeatureFlags = append(velero.Spec.VeleroFeatureFlags, enableCSIFeatureFlag)
			break
		}
	}
	r.ReconcileRestoreResourcesVersionPriority(velero)

	velero.Spec.VeleroFeatureFlags = removeDuplicateValues(velero.Spec.VeleroFeatureFlags)
	deploymentName := veleroDeployment.Name       //saves desired deployment name before install.Deployment overwrites them.
	ownerRefs := veleroDeployment.OwnerReferences // saves desired owner refs
	*veleroDeployment = *install.Deployment(veleroDeployment.Namespace,
		install.WithResources(r.getVeleroResourceReqs(velero)),
		install.WithImage(getVeleroImage(velero)),
		install.WithFeatures(velero.Spec.VeleroFeatureFlags),
		install.WithAnnotations(velero.Spec.PodAnnotations),
		// use WithSecret false even if we have secret because we use a different VolumeMounts and EnvVars
		// see: https://github.com/vmware-tanzu/velero/blob/ed5809b7fc22f3661eeef10bdcb63f0d74472b76/pkg/install/deployment.go#L223-L261
		// our secrets are appended to containers/volumeMounts in credentials.AppendPluginSpecificSpecs function
		install.WithSecret(false),
	)
	// adjust veleroDeployment from install
	veleroDeployment.Name = deploymentName //reapply saved deploymentName and owner refs
	veleroDeployment.OwnerReferences = ownerRefs
	return r.customizeVeleroDeployment(velero, veleroDeployment)
}

// remove duplicate entry in string slice
func removeDuplicateValues(slice []string) []string {
	if slice == nil {
		return nil
	}
	keys := make(map[string]bool)
	list := []string{}
	for _, entry := range slice {
		if _, found := keys[entry]; !found { //add entry to list if not found in keys already
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list // return the result through the passed in argument
}

func (r *VeleroReconciler) customizeVeleroDeployment(velero *oadpv1alpha1.Velero, veleroDeployment *appsv1.Deployment) error {
	veleroDeployment.Labels = r.getAppLabels(velero)
	veleroDeployment.Spec.Selector = veleroLabelSelector

	//TODO: add velero nodeselector, needs to be added to the VELERO CR first
	// Selector: veleroDeployment.Spec.Selector,
	veleroDeployment.Spec.Replicas = pointer.Int32(1)
	veleroDeployment.Spec.Template.Spec.Tolerations = velero.Spec.VeleroTolerations
	veleroDeployment.Spec.Template.Spec.Volumes = append(veleroDeployment.Spec.Template.Spec.Volumes,
		corev1.Volume{
			Name: "certs",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
	//add any default init containers here if needed eg: setup-certificate-secret
	// When you do this
	// - please set the ImagePullPolicy to Always, and
	// - please also update the test
	if veleroDeployment.Spec.Template.Spec.InitContainers == nil {
		veleroDeployment.Spec.Template.Spec.InitContainers = []corev1.Container{}
	}

	// attach DNS policy and config if enabled
	veleroDeployment.Spec.Template.Spec.DNSPolicy = velero.Spec.PodDnsPolicy
	if !reflect.DeepEqual(velero.Spec.PodDnsConfig, corev1.PodDNSConfig{}) {
		veleroDeployment.Spec.Template.Spec.DNSConfig = &velero.Spec.PodDnsConfig
	}

	var veleroContainer *corev1.Container
	for i, container := range veleroDeployment.Spec.Template.Spec.Containers {
		if container.Name == common.Velero {
			veleroContainer = &veleroDeployment.Spec.Template.Spec.Containers[i]
			break
		}
	}
	if err := r.customizeVeleroContainer(velero, veleroDeployment, veleroContainer); err != nil {
		return err
	}
	return credentials.AppendPluginSpecificSpecs(velero, veleroDeployment, veleroContainer)
}

func (r *VeleroReconciler) customizeVeleroContainer(velero *oadpv1alpha1.Velero, veleroDeployment *appsv1.Deployment, veleroContainer *corev1.Container) error {
	if veleroContainer == nil {
		return fmt.Errorf("could not find velero container in Deployment")
	}
	veleroContainer.ImagePullPolicy = corev1.PullAlways
	veleroContainer.VolumeMounts = append(veleroContainer.VolumeMounts,
		corev1.VolumeMount{
			Name:      "certs",
			MountPath: "/etc/ssl/certs",
		},
	)

	veleroContainer.Env = append(veleroContainer.Env,
		corev1.EnvVar{
			Name:  common.HTTPProxyEnvVar,
			Value: os.Getenv("HTTP_PROXY"),
		},
		corev1.EnvVar{
			Name:  common.HTTPSProxyEnvVar,
			Value: os.Getenv("HTTPS_PROXY"),
		},
		corev1.EnvVar{
			Name:  common.NoProxyEnvVar,
			Value: os.Getenv("NO_PROXY"),
		},
	)
	// Enable user to specify --restic-timeout (defaults to 1h)
	resticTimeout := "1h"
	if len(velero.Spec.ResticTimeout) > 0 {
		resticTimeout = velero.Spec.ResticTimeout
	}
	// Append restic timeout option manually. Not configurable via install package, missing from podTemplateConfig struct. See: https://github.com/vmware-tanzu/velero/blob/8d57215ded1aa91cdea2cf091d60e072ce3f340f/pkg/install/deployment.go#L34-L45
	veleroContainer.Args = append(veleroContainer.Args, fmt.Sprintf("--restic-timeout=%s", resticTimeout))

	return nil
}

func getVeleroImage(velero *oadpv1alpha1.Velero) string {
	if velero.Spec.UnsupportedOverrides[oadpv1alpha1.VeleroImageKey] != "" {
		return velero.Spec.UnsupportedOverrides[oadpv1alpha1.VeleroImageKey]
	}
	if os.Getenv("VELERO_REPO") == "" {
		return common.VeleroImage
	}
	return fmt.Sprintf("%v/%v/%v:%v", os.Getenv("REGISTRY"), os.Getenv("PROJECT"), os.Getenv("VELERO_REPO"), os.Getenv("VELERO_TAG"))
}

func (r *VeleroReconciler) getAppLabels(velero *oadpv1alpha1.Velero) map[string]string {
	labels := map[string]string{
		"app.kubernetes.io/name":       common.Velero,
		"app.kubernetes.io/instance":   velero.Name,
		"app.kubernetes.io/managed-by": common.OADPOperator,
		"app.kubernetes.io/component":  Server,
		oadpv1alpha1.OadpOperatorLabel: "True",
	}
	return labels
}

// Get VELERO Resource Requirements
func (r *VeleroReconciler) getVeleroResourceReqs(velero *oadpv1alpha1.Velero) corev1.ResourceRequirements {

	// Set default values
	ResourcesReqs := corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		},
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		},
	}

	if velero != nil {

		// Set custom limits and requests values if defined on VELERO Spec
		if velero.Spec.VeleroResourceAllocations.Requests != nil {
			ResourcesReqs.Requests[corev1.ResourceCPU] = resource.MustParse(velero.Spec.VeleroResourceAllocations.Requests.Cpu().String())
			ResourcesReqs.Requests[corev1.ResourceMemory] = resource.MustParse(velero.Spec.VeleroResourceAllocations.Requests.Memory().String())
		}

		if velero.Spec.VeleroResourceAllocations.Limits != nil {
			ResourcesReqs.Limits[corev1.ResourceCPU] = resource.MustParse(velero.Spec.VeleroResourceAllocations.Limits.Cpu().String())
			ResourcesReqs.Limits[corev1.ResourceMemory] = resource.MustParse(velero.Spec.VeleroResourceAllocations.Limits.Memory().String())
		}

	}

	return ResourcesReqs
}

// For later: Move this code into validator.go when more need for validation arises
// TODO: if multiple default plugins exist, ensure we validate all of them.
// Right now its sequential validation
func (r *VeleroReconciler) ValidateVeleroPlugins(log logr.Logger) (bool, error) {
	velero := oadpv1alpha1.Velero{}
	if err := r.Get(r.Context, r.NamespacedName, &velero); err != nil {
		return false, err
	}

	var defaultPlugin oadpv1alpha1.DefaultPlugin
	for _, plugin := range velero.Spec.DefaultVeleroPlugins {
		if pluginSpecificMap, ok := credentials.PluginSpecificFields[plugin]; ok && pluginSpecificMap.IsCloudProvider {
			secretName := pluginSpecificMap.SecretName
			_, err := r.getProviderSecret(secretName)
			if err != nil {
				r.Log.Info(fmt.Sprintf("error validating %s provider secret:  %s/%s", defaultPlugin, r.NamespacedName.Namespace, secretName))
				return false, err
			}
		}
	}
	return true, nil
}
