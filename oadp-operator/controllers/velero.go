package controllers

import (
	"fmt"
	"github.com/openshift/oadp-operator/pkg/credentials"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"os"
	"reflect"

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
)

// Environment Vars keys
const (
	LDLibraryPathEnvKey              = "LD_LIBRARY_PATH"
	VeleroNamespaceEnvKey            = "VELERO_NAMESPACE"
	VeleroScratchDirEnvKey           = "VELERO_SCRATCH_DIR"
	AWSSharedCredentialsFileEnvKey   = "AWS_SHARED_CREDENTIALS_FILE"
	AzureSharedCredentialsFileEnvKey = "AZURE_SHARED_CREDENTIALS_FILE"
	GCPSharedCredentialsFileEnvKey   = "GCP_SHARED_CREDENTIALS_FILE"
	HTTPProxyEnvVar                  = "HTTP_PROXY"
	HTTPSProxyEnvVar                 = "HTTPS_PROXY"
	NoProxyEnvVar                    = "NO_PROXY"
)

//TODO: Add Image customization options
// Images
const (
	VeleroImage          = "quay.io/konveyor/velero:latest"
	OpenshiftPluginImage = "quay.io/konveyor/openshift-velero-plugins:latest"
	AWSPluginImage       = "quay.io/konveyor/velero-plugin-for-aws:latest"
	AzurePluginImage     = "quay.io/konveyor/velero-plugin-for-microsoft-azure:latest"
	GCPPluginImage       = "quay.io/konveyor/velero-plugin-for-gcp:latest"
	CSIPluginImage       = "quay.io/konveyor/velero-plugin-for-csi:latest"
	RegistryImage        = "quay.io/konveyor/registry:latest"
)

// Plugin names
const (
	VeleroPluginForAWS       = "velero-plugin-for-aws"
	VeleroPluginForAzure     = "velero-plugin-for-microsoft-azure"
	VeleroPluginForGCP       = "velero-plugin-for-gcp"
	VeleroPluginForCSI       = "velero-plugin-for-csi"
	VeleroPluginForOpenshift = "openshift-velero-plugin"
)

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

//TODO: Temporary solution for Non-OLM Operator install
func (r *VeleroReconciler) ReconcileVeleroCRDs(log logr.Logger) (bool, error) {
	velero := oadpv1alpha1.Velero{}
	if err := r.Get(r.Context, r.NamespacedName, &velero); err != nil {
		return false, err
	}

	// check for Non-OLM install and proceed with Velero supporting CRD installation
	if velero.Spec.OlmManaged != nil && velero.Spec.OlmManaged == pointer.Bool(false) {
		err := r.InstallVeleroCRDs(log)
		if err != nil {
			return false, err
		}
	}

	return true, nil
}

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
			veleroDeployment.Spec.Selector = &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"component": common.Velero,
				},
			}
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
	scc = &security.SecurityContextConstraints{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "velero-privileged",
			Labels: r.getAppLabels(velero),
		},
		AllowHostDirVolumePlugin: true,
		AllowHostIPC:             true,
		AllowHostNetwork:         true,
		AllowHostPID:             true,
		AllowHostPorts:           true,
		AllowPrivilegeEscalation: pointer.BoolPtr(true),
		AllowPrivilegedContainer: true,
		AllowedCapabilities: []corev1.Capability{
			security.AllowAllCapabilities,
		},
		AllowedUnsafeSysctls: []string{
			"*",
		},
		DefaultAddCapabilities: nil,
		FSGroup: security.FSGroupStrategyOptions{
			Type: security.FSGroupStrategyRunAsAny,
		},
		Priority:                 nil,
		ReadOnlyRootFilesystem:   false,
		RequiredDropCapabilities: nil,
		RunAsUser: security.RunAsUserStrategyOptions{
			Type: security.RunAsUserStrategyRunAsAny,
		},
		SELinuxContext: security.SELinuxContextStrategyOptions{
			Type: security.SELinuxStrategyRunAsAny,
		},
		SeccompProfiles: []string{
			"*",
		},
		SupplementalGroups: security.SupplementalGroupsStrategyOptions{
			Type: security.SupplementalGroupsStrategyRunAsAny,
		},
		Users: []string{
			"system:admin",
			fmt.Sprintf("system:serviceaccount:%s:%s", sa.Namespace, sa.Name),
		},
		Volumes: []security.FSType{
			security.FSTypeAll,
		},
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

	// get default values
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "plugins",
			MountPath: "/plugins",
		},
		{
			Name:      "scratch",
			MountPath: "/scratch",
		},
		{
			Name:      "certs",
			MountPath: "/etc/ssl/certs",
		},
	}

	envVars := []corev1.EnvVar{
		{
			Name:  LDLibraryPathEnvKey,
			Value: "/plugins",
		},
		{
			Name:  VeleroNamespaceEnvKey,
			Value: velero.Namespace,
		},
		{
			Name:  VeleroScratchDirEnvKey,
			Value: "/scratch",
		},
		{
			Name:  HTTPProxyEnvVar,
			Value: os.Getenv("HTTP_PROXY"),
		},
		{
			Name:  HTTPSProxyEnvVar,
			Value: os.Getenv("HTTPS_PROXY"),
		},
		{
			Name:  NoProxyEnvVar,
			Value: os.Getenv("NO_PROXY"),
		},
	}

	volumes := []corev1.Volume{
		{
			Name: "plugins",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: "scratch",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: "certs",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}

	//add any default init containers here if needed eg: setup-certificate-secret
	initContainers := []corev1.Container{}

	// assign spec values
	veleroDeployment.Labels = r.getAppLabels(velero)

	veleroDeployment.Spec = appsv1.DeploymentSpec{
		//TODO: add velero nodeselector, needs to be added to the VELERO CR first
		Selector: veleroDeployment.Spec.Selector,
		Replicas: pointer.Int32(1),
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"component": common.Velero,
				},
				Annotations: map[string]string{
					"prometheus.io/scrape": "true",
					"prometheus.io/port":   "8085",
					"prometheus.io/path":   "/metrics",
				},
			},
			Spec: corev1.PodSpec{
				RestartPolicy:      corev1.RestartPolicyAlways,
				ServiceAccountName: common.Velero,
				Tolerations:        velero.Spec.VeleroTolerations,
				Containers: []corev1.Container{
					{
						Name:            common.Velero,
						Image:           getVeleroImage(),
						ImagePullPolicy: corev1.PullAlways,
						Ports: []corev1.ContainerPort{
							{
								Name:          "metrics",
								ContainerPort: 8085,
							},
						},
						Resources: r.getVeleroResourceReqs(velero),
						Command:   []string{"/velero"},
						//TODO: Parametrize restic timeout, Features flag as well as VELERO debug flag
						Args:         []string{"server", "--restic-timeout", "1h"},
						VolumeMounts: volumeMounts,
						Env:          envVars,
					},
				},
				Volumes:        volumes,
				InitContainers: initContainers,
			},
		},
	}

	err := credentials.AppendPluginSpecficSpecs(velero, veleroDeployment)

	if err != nil {
		return err
	}

	return nil
}

func getVeleroImage() string {
	return fmt.Sprintf("%v/%v/%v:%v", os.Getenv("REGISTRY"), os.Getenv("PROJECT"), os.Getenv("VELERO_REPO"), os.Getenv("VELERO_TAG"))
}

func (r *VeleroReconciler) getAppLabels(velero *oadpv1alpha1.Velero) map[string]string {
	labels := map[string]string{
		"app.kubernetes.io/name":       common.Velero,
		"app.kubernetes.io/instance":   velero.Name,
		"app.kubernetes.io/managed-by": common.OADPOperator,
		"app.kubernetes.io/component":  Server,
	}
	return labels
}

// Get VELERO Resource Requirements
func (r *VeleroReconciler) getVeleroResourceReqs(velero *oadpv1alpha1.Velero) corev1.ResourceRequirements {

	ResourcesReqs := corev1.ResourceRequirements{}
	ResourceReqsLimits := corev1.ResourceList{}
	ResourceReqsRequests := corev1.ResourceList{}

	if velero != nil {

		// Set custom limits and requests values if defined on VELERO Spec
		if velero.Spec.VeleroResourceAllocations.Requests != nil {
			ResourceReqsRequests[corev1.ResourceCPU] = resource.MustParse(velero.Spec.VeleroResourceAllocations.Requests.Cpu().String())
			ResourceReqsRequests[corev1.ResourceMemory] = resource.MustParse(velero.Spec.VeleroResourceAllocations.Requests.Memory().String())
		}

		if velero.Spec.VeleroResourceAllocations.Limits != nil {
			ResourceReqsLimits[corev1.ResourceCPU] = resource.MustParse(velero.Spec.VeleroResourceAllocations.Limits.Cpu().String())
			ResourceReqsLimits[corev1.ResourceMemory] = resource.MustParse(velero.Spec.VeleroResourceAllocations.Limits.Memory().String())
		}
		ResourcesReqs.Requests = ResourceReqsRequests
		ResourcesReqs.Limits = ResourceReqsLimits

		return ResourcesReqs

	}

	// Set default values
	ResourcesReqs = corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		},
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		},
	}
	return ResourcesReqs
}
