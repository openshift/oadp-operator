package controllers

import (
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/openshift/oadp-operator/pkg/credentials"
	"github.com/operator-framework/operator-lib/proxy"
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
	"sigs.k8s.io/controller-runtime/pkg/client"
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
			"k8s-app":   "openshift-adp",
			"component": common.Velero,
			"deploy":    common.Velero,
		},
	}
)

// TODO: Remove this function as it's no longer being used
func (r *DPAReconciler) ReconcileVeleroServiceAccount(log logr.Logger) (bool, error) {
	dpa := oadpv1alpha1.DataProtectionApplication{}
	if err := r.Get(r.Context, r.NamespacedName, &dpa); err != nil {
		return false, err
	}
	veleroSa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      common.Velero,
			Namespace: dpa.Namespace,
		},
	}
	op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, veleroSa, func() error {
		// Setting controller owner reference on the velero SA
		err := controllerutil.SetControllerReference(&dpa, veleroSa, r.Scheme)
		if err != nil {
			return err
		}

		// update the SA template
		veleroSaUpdate, err := r.veleroServiceAccount(&dpa)
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
func (r *DPAReconciler) ReconcileVeleroCRDs(log logr.Logger) (bool, error) {
	dpa := oadpv1alpha1.DataProtectionApplication{}
	if err := r.Get(r.Context, r.NamespacedName, &dpa); err != nil {
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
func (r *DPAReconciler) InstallVeleroCRDs(log logr.Logger) error {
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
func (r *DPAReconciler) ReconcileVeleroClusterRoleBinding(log logr.Logger) (bool, error) {
	dpa := oadpv1alpha1.DataProtectionApplication{}
	if err := r.Get(r.Context, r.NamespacedName, &dpa); err != nil {
		return false, err
	}
	veleroCRB, err := r.veleroClusterRoleBinding(&dpa)
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
		veleroCRBUpdate, err := r.veleroClusterRoleBinding(&dpa)
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

func (r *DPAReconciler) ReconcileVeleroSecurityContextConstraint(log logr.Logger) (bool, error) {
	dpa := oadpv1alpha1.DataProtectionApplication{}
	if err := r.Get(r.Context, r.NamespacedName, &dpa); err != nil {
		return false, err
	}
	sa := corev1.ServiceAccount{}
	nsName := types.NamespacedName{
		Namespace: dpa.Namespace,
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
		return r.privilegedSecurityContextConstraints(veleroSCC, &dpa, &sa)
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

func (r *DPAReconciler) ReconcileVeleroDeployment(log logr.Logger) (bool, error) {
	dpa := oadpv1alpha1.DataProtectionApplication{}
	if err := r.Get(r.Context, r.NamespacedName, &dpa); err != nil {
		return false, err
	}

	veleroDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      common.Velero,
			Namespace: dpa.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, veleroDeployment, func() error {

		// Setting Deployment selector if a new object is created as it is immutable
		if veleroDeployment.ObjectMeta.CreationTimestamp.IsZero() {
			veleroDeployment.Spec.Selector = &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name":       common.Velero,
					"app.kubernetes.io/instance":   dpa.Name,
					"app.kubernetes.io/managed-by": common.OADPOperator,
					"app.kubernetes.io/component":  Server,
					oadpv1alpha1.OadpOperatorLabel: "True",
				},
			}
		}

		// Setting controller owner reference on the velero deployment
		err := controllerutil.SetControllerReference(&dpa, veleroDeployment, r.Scheme)
		if err != nil {
			return err
		}
		// update the Deployment template
		return r.buildVeleroDeployment(veleroDeployment, &dpa)
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

func (r *DPAReconciler) veleroServiceAccount(dpa *oadpv1alpha1.DataProtectionApplication) (*corev1.ServiceAccount, error) {
	annotations := make(map[string]string)
	sa := install.ServiceAccount(dpa.Namespace, annotations)
	sa.Labels = r.getAppLabels(dpa)
	return sa, nil
}

func (r *DPAReconciler) veleroClusterRoleBinding(dpa *oadpv1alpha1.DataProtectionApplication) (*rbacv1.ClusterRoleBinding, error) {
	crb := install.ClusterRoleBinding(dpa.Namespace)
	crb.Labels = r.getAppLabels(dpa)
	return crb, nil
}

func (r *DPAReconciler) privilegedSecurityContextConstraints(scc *security.SecurityContextConstraints, dpa *oadpv1alpha1.DataProtectionApplication, sa *corev1.ServiceAccount) error {
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
func (r *DPAReconciler) buildVeleroDeployment(veleroDeployment *appsv1.Deployment, dpa *oadpv1alpha1.DataProtectionApplication) error {

	if dpa == nil {
		return fmt.Errorf("DPA CR cannot be nil")
	}
	if veleroDeployment == nil {
		return fmt.Errorf("velero deployment cannot be nil")
	}

	//check if CSI plugin is added in spec
	for _, plugin := range dpa.Spec.Configuration.Velero.DefaultPlugins {
		if plugin == oadpv1alpha1.DefaultPluginCSI {
			// CSI plugin is added so ensure that CSI feature flags is set
			dpa.Spec.Configuration.Velero.FeatureFlags = append(dpa.Spec.Configuration.Velero.FeatureFlags, enableCSIFeatureFlag)
			break
		}
	}
	r.ReconcileRestoreResourcesVersionPriority(dpa)

	dpa.Spec.Configuration.Velero.FeatureFlags = removeDuplicateValues(dpa.Spec.Configuration.Velero.FeatureFlags)
	deploymentName := veleroDeployment.Name       //saves desired deployment name before install.Deployment overwrites them.
	ownerRefs := veleroDeployment.OwnerReferences // saves desired owner refs
	*veleroDeployment = *install.Deployment(veleroDeployment.Namespace,
		install.WithResources(r.getVeleroResourceReqs(dpa)),
		install.WithImage(getVeleroImage(dpa)),
		install.WithFeatures(dpa.Spec.Configuration.Velero.FeatureFlags),
		install.WithAnnotations(dpa.Spec.PodAnnotations),
		// use WithSecret false even if we have secret because we use a different VolumeMounts and EnvVars
		// see: https://github.com/vmware-tanzu/velero/blob/ed5809b7fc22f3661eeef10bdcb63f0d74472b76/pkg/install/deployment.go#L223-L261
		// our secrets are appended to containers/volumeMounts in credentials.AppendPluginSpecificSpecs function
		install.WithSecret(false),
	)
	// adjust veleroDeployment from install
	veleroDeployment.Name = deploymentName //reapply saved deploymentName and owner refs
	veleroDeployment.OwnerReferences = ownerRefs
	return r.customizeVeleroDeployment(dpa, veleroDeployment)
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

func (r *DPAReconciler) customizeVeleroDeployment(dpa *oadpv1alpha1.DataProtectionApplication, veleroDeployment *appsv1.Deployment) error {
	veleroDeployment.Labels = r.getAppLabels(dpa)
	veleroDeployment.Spec.Selector = &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"app.kubernetes.io/name":       common.Velero,
			"app.kubernetes.io/instance":   dpa.Name,
			"app.kubernetes.io/managed-by": common.OADPOperator,
			"app.kubernetes.io/component":  Server,
			oadpv1alpha1.OadpOperatorLabel: "True",
		},
	}
	veleroDeployment.Spec.Template.Labels = map[string]string{
		"app.kubernetes.io/name":       common.Velero,
		"app.kubernetes.io/instance":   dpa.Name,
		"app.kubernetes.io/managed-by": common.OADPOperator,
		"app.kubernetes.io/component":  Server,
		oadpv1alpha1.OadpOperatorLabel: "True",
	}

	isSTSNeeded := r.isSTSTokenNeeded(dpa.Spec.BackupLocations, dpa.Namespace)

	//TODO: add velero nodeselector, needs to be added to the VELERO CR first
	// Selector: veleroDeployment.Spec.Selector,
	veleroDeployment.Spec.Replicas = pointer.Int32(1)
	if dpa.Spec.Configuration.Velero.PodConfig != nil {
		veleroDeployment.Spec.Template.Spec.Tolerations = dpa.Spec.Configuration.Velero.PodConfig.Tolerations
	}
	veleroDeployment.Spec.Template.Spec.Volumes = append(veleroDeployment.Spec.Template.Spec.Volumes,
		corev1.Volume{
			Name: "certs",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})

	if isSTSNeeded {
		defaultMode := int32(420)
		expirationSeconds := int64(3600)
		veleroDeployment.Spec.Template.Spec.Volumes = append(veleroDeployment.Spec.Template.Spec.Volumes,
			corev1.Volume{
				Name: "bound-sa-token",
				VolumeSource: corev1.VolumeSource{
					Projected: &corev1.ProjectedVolumeSource{
						DefaultMode: &defaultMode,
						Sources: []corev1.VolumeProjection{
							{
								ServiceAccountToken: &corev1.ServiceAccountTokenProjection{
									Audience:          "openshift",
									ExpirationSeconds: &expirationSeconds,
									Path:              "token",
								},
							},
						},
					},
				},
			},
		)
	}
	//add any default init containers here if needed eg: setup-certificate-secret
	// When you do this
	// - please set the ImagePullPolicy to Always, and
	// - please also update the test
	if veleroDeployment.Spec.Template.Spec.InitContainers == nil {
		veleroDeployment.Spec.Template.Spec.InitContainers = []corev1.Container{}
	}

	// attach DNS policy and config if enabled
	veleroDeployment.Spec.Template.Spec.DNSPolicy = dpa.Spec.PodDnsPolicy
	if !reflect.DeepEqual(dpa.Spec.PodDnsConfig, corev1.PodDNSConfig{}) {
		veleroDeployment.Spec.Template.Spec.DNSConfig = &dpa.Spec.PodDnsConfig
	}

	var veleroContainer *corev1.Container
	for i, container := range veleroDeployment.Spec.Template.Spec.Containers {
		if container.Name == common.Velero {
			veleroContainer = &veleroDeployment.Spec.Template.Spec.Containers[i]
			break
		}
	}
	if err := r.customizeVeleroContainer(dpa, veleroDeployment, veleroContainer, isSTSNeeded); err != nil {
		return err
	}

	providerNeedsDefaultCreds, hasCloudStorage, err := r.noDefaultCredentials(*dpa)
	if err != nil {
		return err
	}
	return credentials.AppendPluginSpecificSpecs(dpa, veleroDeployment, veleroContainer, providerNeedsDefaultCreds, hasCloudStorage)
}

func (r *DPAReconciler) customizeVeleroContainer(dpa *oadpv1alpha1.DataProtectionApplication, veleroDeployment *appsv1.Deployment, veleroContainer *corev1.Container, isSTSNeeded bool) error {
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

	if isSTSNeeded {
		veleroContainer.VolumeMounts = append(veleroContainer.VolumeMounts,
			corev1.VolumeMount{
				Name:      "bound-sa-token",
				MountPath: "/var/run/secrets/openshift/serviceaccount",
				ReadOnly:  true,
			})
	}
	// Append proxy settings to the container from environment variables
	veleroContainer.Env = append(veleroContainer.Env, proxy.ReadProxyVarsFromEnv()...)

	// Enable user to specify --restic-timeout (defaults to 1h)
	resticTimeout := "1h"
	if dpa.Spec.Configuration.Restic != nil && len(dpa.Spec.Configuration.Restic.Timeout) > 0 {
		resticTimeout = dpa.Spec.Configuration.Restic.Timeout
	}
	// Append restic timeout option manually. Not configurable via install package, missing from podTemplateConfig struct. See: https://github.com/vmware-tanzu/velero/blob/8d57215ded1aa91cdea2cf091d60e072ce3f340f/pkg/install/deployment.go#L34-L45
	veleroContainer.Args = append(veleroContainer.Args, fmt.Sprintf("--restic-timeout=%s", resticTimeout))

	return nil
}

func (r *DPAReconciler) isSTSTokenNeeded(bsls []oadpv1alpha1.BackupLocation, ns string) bool {

	for _, bsl := range bsls {
		if bsl.CloudStorage != nil {
			bucket := &oadpv1alpha1.CloudStorage{}
			err := r.Get(r.Context, client.ObjectKey{
				Name:      bsl.CloudStorage.CloudStorageRef.Name,
				Namespace: ns,
			}, bucket)
			if err != nil {
				//log
				return false
			}
			if bucket.Spec.EnableSharedConfig != nil && *bucket.Spec.EnableSharedConfig {
				return true
			}
		}
	}

	return false
}

func getVeleroImage(dpa *oadpv1alpha1.DataProtectionApplication) string {
	if dpa.Spec.UnsupportedOverrides[oadpv1alpha1.VeleroImageKey] != "" {
		return dpa.Spec.UnsupportedOverrides[oadpv1alpha1.VeleroImageKey]
	}
	if os.Getenv("VELERO_REPO") == "" {
		return common.VeleroImage
	}
	return fmt.Sprintf("%v/%v/%v:%v", os.Getenv("REGISTRY"), os.Getenv("PROJECT"), os.Getenv("VELERO_REPO"), os.Getenv("VELERO_TAG"))
}

func (r *DPAReconciler) getAppLabels(dpa *oadpv1alpha1.DataProtectionApplication) map[string]string {
	labels := map[string]string{
		"app.kubernetes.io/name":       common.Velero,
		"app.kubernetes.io/instance":   dpa.Name,
		"app.kubernetes.io/managed-by": common.OADPOperator,
		"app.kubernetes.io/component":  Server,
		oadpv1alpha1.OadpOperatorLabel: "True",
	}
	return labels
}

// Get Velero Resource Requirements
func (r *DPAReconciler) getVeleroResourceReqs(dpa *oadpv1alpha1.DataProtectionApplication) corev1.ResourceRequirements {

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

	if dpa != nil && dpa.Spec.Configuration.Velero.PodConfig != nil {
		// Set custom limits and requests values if defined on VELERO Spec
		if dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Requests != nil {
			ResourcesReqs.Requests[corev1.ResourceCPU] = resource.MustParse(dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Requests.Cpu().String())
			ResourcesReqs.Requests[corev1.ResourceMemory] = resource.MustParse(dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Requests.Memory().String())
		}

		if dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Limits != nil {
			ResourcesReqs.Limits[corev1.ResourceCPU] = resource.MustParse(dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Limits.Cpu().String())
			ResourcesReqs.Limits[corev1.ResourceMemory] = resource.MustParse(dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Limits.Memory().String())
		}

	}

	return ResourcesReqs
}

// Get Restic Resource Requirements
func (r *DPAReconciler) getResticResourceReqs(dpa *oadpv1alpha1.DataProtectionApplication) corev1.ResourceRequirements {

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

	if dpa != nil && dpa.Spec.Configuration != nil && dpa.Spec.Configuration.Restic != nil && dpa.Spec.Configuration.Restic.PodConfig != nil {
		// Set custom limits and requests values if defined on VELERO Spec
		if dpa.Spec.Configuration.Restic.PodConfig.ResourceAllocations.Requests != nil {
			ResourcesReqs.Requests[corev1.ResourceCPU] = resource.MustParse(dpa.Spec.Configuration.Restic.PodConfig.ResourceAllocations.Requests.Cpu().String())
			ResourcesReqs.Requests[corev1.ResourceMemory] = resource.MustParse(dpa.Spec.Configuration.Restic.PodConfig.ResourceAllocations.Requests.Memory().String())
		}

		if dpa.Spec.Configuration.Restic.PodConfig.ResourceAllocations.Limits != nil {
			ResourcesReqs.Limits[corev1.ResourceCPU] = resource.MustParse(dpa.Spec.Configuration.Restic.PodConfig.ResourceAllocations.Limits.Cpu().String())
			ResourcesReqs.Limits[corev1.ResourceMemory] = resource.MustParse(dpa.Spec.Configuration.Restic.PodConfig.ResourceAllocations.Limits.Memory().String())
		}

	}

	return ResourcesReqs
}

// noDefaultCredentials determines if a provider needs the default credentials.
// This returns a map of providers found to if they need a default credential,
// a boolean if Cloud Storage backup storage location was used and an error if any occured.
func (r DPAReconciler) noDefaultCredentials(dpa oadpv1alpha1.DataProtectionApplication) (map[string]bool, bool, error) {
	providerNeedsDefaultCreds := map[string]bool{}
	hasCloudStorage := false

	for _, bsl := range dpa.Spec.BackupLocations {
		if bsl.Velero != nil && bsl.Velero.Credential == nil {
			bslProvider := strings.TrimPrefix(bsl.Velero.Provider, "velero.io/")
			providerNeedsDefaultCreds[bslProvider] = true
		}
		if bsl.Velero != nil && bsl.Velero.Credential != nil {
			bslProvider := strings.TrimPrefix(bsl.Velero.Provider, "velero.io/")
			if found := providerNeedsDefaultCreds[bslProvider]; !found {
				providerNeedsDefaultCreds[bslProvider] = false
			}
		}
		if bsl.CloudStorage != nil {
			if bsl.CloudStorage.Credential == nil {
				cloudStorage := oadpv1alpha1.CloudStorage{}
				err := r.Get(r.Context, types.NamespacedName{Name: bsl.CloudStorage.CloudStorageRef.Name, Namespace: dpa.Namespace}, &cloudStorage)
				if err != nil {
					return nil, false, err
				}
				providerNeedsDefaultCreds[string(cloudStorage.Spec.Provider)] = true
			} else {
				hasCloudStorage = true
			}
		}
	}

	for _, vsl := range dpa.Spec.SnapshotLocations {
		if vsl.Velero != nil {
			// To handle the case where we want to manually hand the credentials for a cloud storage created
			// Bucket credentials via configuration. Only AWS is supported
			provider := strings.TrimPrefix(vsl.Velero.Provider, "velero.io")
			if provider == string(oadpv1alpha1.AWSBucketProvider) && hasCloudStorage {
				providerNeedsDefaultCreds[provider] = false
			} else {
				providerNeedsDefaultCreds[provider] = true
			}
		}
	}

	return providerNeedsDefaultCreds, hasCloudStorage, nil

}
