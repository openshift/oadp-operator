package controller

import (
	"fmt"
	"os"
	"reflect"
	"strconv"

	"github.com/go-logr/logr"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/common"
	"github.com/sirupsen/logrus"
	"golang.org/x/exp/maps"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serror "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	vmFileRestoreObjectName          = "oadp-vm-file-restore-controller-manager"
	vmFileRestoreControlPlaneKey     = "control-plane"
	vmFileRestoreControlPlaneValue   = "oadp-vm-file-restore-controller"
	vmfrDpaResourceVersionAnnotation = oadpv1alpha1.OadpOperatorLabel + "-vmfr-dpa-resource-version"
)

var (
	vmFileRestoreControlPlaneLabel = map[string]string{
		vmFileRestoreControlPlaneKey: vmFileRestoreControlPlaneValue,
	}
	vmFileRestoreDeploymentLabels = map[string]string{
		"app.kubernetes.io/component":  "manager",
		"app.kubernetes.io/created-by": common.OADPOperator,
		"app.kubernetes.io/instance":   vmFileRestoreObjectName,
		"app.kubernetes.io/managed-by": "kustomize",
		"app.kubernetes.io/name":       "deployment",
		"app.kubernetes.io/part-of":    common.OADPOperator,
	}

	vmfrDpaResourceVersion                                         = ""
	previousVMFileRestoreConfiguration *oadpv1alpha1.VMFileRestore = nil
)

// ReconcileVMFileRestoreController manages the VM file restore controller deployment
func (r *DataProtectionApplicationReconciler) ReconcileVMFileRestoreController(log logr.Logger) (bool, error) {
	vmFileRestoreDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vmFileRestoreObjectName,
			Namespace: r.NamespacedName.Namespace,
		},
	}

	// Delete (possible) previously deployment
	if !r.checkVMFileRestoreEnabled() {
		if err := r.Get(
			r.Context,
			types.NamespacedName{
				Name:      vmFileRestoreDeployment.Name,
				Namespace: vmFileRestoreDeployment.Namespace,
			},
			vmFileRestoreDeployment,
		); err != nil {
			if k8serror.IsNotFound(err) {
				return true, nil
			}
			return false, err
		}

		if err := r.Delete(
			r.Context,
			vmFileRestoreDeployment,
			&client.DeleteOptions{PropagationPolicy: ptr.To(metav1.DeletePropagationForeground)},
		); err != nil {
			r.EventRecorder.Event(
				vmFileRestoreDeployment,
				corev1.EventTypeWarning,
				"VMFileRestoreDeploymentDeleteFailed",
				fmt.Sprintf("Could not delete VM file restore controller deployment %s/%s: %s", vmFileRestoreDeployment.Namespace, vmFileRestoreDeployment.Name, err),
			)
			return false, err
		}
		r.EventRecorder.Event(
			vmFileRestoreDeployment,
			corev1.EventTypeNormal,
			"VMFileRestoreDeploymentDeleteSucceed",
			fmt.Sprintf("VM file restore controller deployment %s/%s deleted", vmFileRestoreDeployment.Namespace, vmFileRestoreDeployment.Name),
		)
		return true, nil
	}

	operation, err := controllerutil.CreateOrUpdate(
		r.Context,
		r.Client,
		vmFileRestoreDeployment,
		func() error {
			err := r.buildVMFileRestoreDeployment(vmFileRestoreDeployment)
			if err != nil {
				return err
			}

			// Setting controller owner reference on the VM file restore controller deployment
			return controllerutil.SetControllerReference(r.dpa, vmFileRestoreDeployment, r.Scheme)
		},
	)
	if err != nil {
		return false, err
	}

	if operation != controllerutil.OperationResultNone {
		r.EventRecorder.Event(
			vmFileRestoreDeployment,
			corev1.EventTypeNormal,
			"VMFileRestoreDeploymentReconciled",
			fmt.Sprintf("VM file restore controller deployment %s/%s %s", vmFileRestoreDeployment.Namespace, vmFileRestoreDeployment.Name, operation),
		)
	}
	return true, nil
}

func (r *DataProtectionApplicationReconciler) buildVMFileRestoreDeployment(deploymentObject *appsv1.Deployment) error {
	vmFileRestoreControllerImage := r.getVMFileRestoreControllerImage()
	imagePullPolicy, err := common.GetImagePullPolicy(r.dpa.Spec.ImagePullPolicy, vmFileRestoreControllerImage)
	if err != nil {
		r.Log.Error(err, "imagePullPolicy regex failed")
	}
	ensureVMFileRestoreRequiredLabels(deploymentObject)
	err = ensureVMFileRestoreRequiredSpecs(deploymentObject, r.dpa, vmFileRestoreControllerImage, imagePullPolicy, r)
	if err != nil {
		return err
	}
	return nil
}

func ensureVMFileRestoreRequiredLabels(deploymentObject *appsv1.Deployment) {
	maps.Copy(vmFileRestoreDeploymentLabels, vmFileRestoreControlPlaneLabel)
	deploymentObjectLabels := deploymentObject.GetLabels()
	if deploymentObjectLabels == nil {
		deploymentObject.SetLabels(vmFileRestoreDeploymentLabels)
	} else {
		for key, value := range vmFileRestoreDeploymentLabels {
			deploymentObjectLabels[key] = value
		}
		deploymentObject.SetLabels(deploymentObjectLabels)
	}
}

func ensureVMFileRestoreRequiredSpecs(
	deploymentObject *appsv1.Deployment,
	dpa *oadpv1alpha1.DataProtectionApplication,
	image string,
	imagePullPolicy corev1.PullPolicy,
	r *DataProtectionApplicationReconciler,
) error {
	// Build environment variables
	envVars := []corev1.EnvVar{
		{
			Name:  "WATCH_NAMESPACE",
			Value: deploymentObject.Namespace,
		},
		{
			Name:  "VMFR_ACCESS_IMAGE",
			Value: r.getVMFileRestoreAccessImage(),
		},
		{
			Name:  "VMFR_SSH_IMAGE",
			Value: r.getVMFileRestoreSSHImage(),
		},
		{
			Name:  "VMFR_BROWSER_IMAGE",
			Value: r.getVMFileRestoreBrowserImage(),
		},
	}

	// Add log level if configured
	if dpa.Spec.Configuration != nil && dpa.Spec.Configuration.Velero != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name: common.LogLevelEnvVar,
			Value: func() string {
				level, err := logrus.ParseLevel(dpa.Spec.Configuration.Velero.LogLevel)
				if err != nil {
					return ""
				}
				return strconv.FormatUint(uint64(level), 10)
			}(),
		})
	}

	// Add log format if configured
	if len(dpa.Spec.LogFormat) > 0 {
		envVars = append(envVars, corev1.EnvVar{
			Name:  common.LogFormatEnvVar,
			Value: string(dpa.Spec.LogFormat),
		})
	}

	// Track DPA resource version for change detection
	if len(vmfrDpaResourceVersion) == 0 ||
		!reflect.DeepEqual(dpa.Spec.VMFileRestore, previousVMFileRestoreConfiguration) {
		vmfrDpaResourceVersion = dpa.GetResourceVersion()
		previousVMFileRestoreConfiguration = dpa.Spec.VMFileRestore
	}

	podAnnotations := map[string]string{
		vmfrDpaResourceVersionAnnotation: vmfrDpaResourceVersion,
	}

	// Get resource requirements
	resources := r.getVMFileRestoreResources()

	// Set deployment spec
	deploymentObject.Spec.Replicas = ptr.To(int32(1))
	deploymentObject.Spec.Selector = &metav1.LabelSelector{
		MatchLabels: vmFileRestoreControlPlaneLabel,
	}

	// Set template labels
	templateObjectLabels := deploymentObject.Spec.Template.GetLabels()
	if templateObjectLabels == nil {
		deploymentObject.Spec.Template.SetLabels(vmFileRestoreControlPlaneLabel)
	} else {
		templateObjectLabels[vmFileRestoreControlPlaneKey] = vmFileRestoreControlPlaneLabel[vmFileRestoreControlPlaneKey]
		deploymentObject.Spec.Template.SetLabels(templateObjectLabels)
	}

	// Set template annotations
	templateObjectAnnotations := deploymentObject.Spec.Template.GetAnnotations()
	if templateObjectAnnotations == nil {
		deploymentObject.Spec.Template.SetAnnotations(podAnnotations)
	} else {
		templateObjectAnnotations[vmfrDpaResourceVersionAnnotation] = podAnnotations[vmfrDpaResourceVersionAnnotation]
		deploymentObject.Spec.Template.SetAnnotations(templateObjectAnnotations)
	}

	// Set pod security context only if not already set (to avoid reconciliation loops)
	if deploymentObject.Spec.Template.Spec.SecurityContext == nil {
		deploymentObject.Spec.Template.Spec.SecurityContext = &corev1.PodSecurityContext{
			RunAsNonRoot: ptr.To(true),
			SeccompProfile: &corev1.SeccompProfile{
				Type: corev1.SeccompProfileTypeRuntimeDefault,
			},
		}
	}

	// Build container spec
	vmFileRestoreContainerFound := false
	containerSpec := corev1.Container{
		Name:            "manager",
		Image:           image,
		ImagePullPolicy: imagePullPolicy,
		Command:         []string{"/manager"},
		Args: []string{
			"--leader-elect",
			"--health-probe-bind-address=:8081",
			"--metrics-bind-address=:8443",
			"--metrics-secure=true",
		},
		Env: envVars,
		Ports: []corev1.ContainerPort{
			{
				Name:          "https",
				ContainerPort: 8443,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Resources: resources,
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: ptr.To(false),
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
			ReadOnlyRootFilesystem: ptr.To(true),
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path:   "/healthz",
					Port:   intstr.FromInt(8081),
					Scheme: corev1.URISchemeHTTP,
				},
			},
			InitialDelaySeconds: 15,
			PeriodSeconds:       20,
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path:   "/readyz",
					Port:   intstr.FromInt(8081),
					Scheme: corev1.URISchemeHTTP,
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       10,
		},
		TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
	}

	if len(deploymentObject.Spec.Template.Spec.Containers) == 0 {
		deploymentObject.Spec.Template.Spec.Containers = []corev1.Container{containerSpec}
		vmFileRestoreContainerFound = true
	} else {
		for index, container := range deploymentObject.Spec.Template.Spec.Containers {
			if container.Name == "manager" {
				// Update only dynamic fields that can change based on DPA configuration
				// Static fields (Command, Args, Ports, SecurityContext, Probes) are left as-is
				vmFileRestoreContainer := &deploymentObject.Spec.Template.Spec.Containers[index]
				vmFileRestoreContainer.Image = image
				vmFileRestoreContainer.ImagePullPolicy = imagePullPolicy
				vmFileRestoreContainer.Env = envVars
				vmFileRestoreContainer.Resources = resources
				vmFileRestoreContainer.TerminationMessagePolicy = corev1.TerminationMessageFallbackToLogsOnError
				vmFileRestoreContainerFound = true
				break
			}
		}
	}

	if !vmFileRestoreContainerFound {
		return fmt.Errorf("could not find VM file restore container in Deployment")
	}

	deploymentObject.Spec.Template.Spec.RestartPolicy = corev1.RestartPolicyAlways
	deploymentObject.Spec.Template.Spec.ServiceAccountName = vmFileRestoreObjectName
	return nil
}

func (r *DataProtectionApplicationReconciler) checkVMFileRestoreEnabled() bool {
	if r.dpa.Spec.VMFileRestore != nil && r.dpa.Spec.VMFileRestore.Enable != nil {
		return *r.dpa.Spec.VMFileRestore.Enable
	}
	return false
}

func (r *DataProtectionApplicationReconciler) getVMFileRestoreControllerImage() string {
	dpa := r.dpa
	unsupportedOverride := dpa.Spec.UnsupportedOverrides[oadpv1alpha1.VMFileRestoreControllerImageKey]
	if unsupportedOverride != "" {
		return unsupportedOverride
	}

	environmentVariable := os.Getenv("RELATED_IMAGE_VM_FILE_RESTORE_CONTROLLER")
	if environmentVariable != "" {
		return environmentVariable
	}

	return "quay.io/konveyor/oadp-vm-file-restore:latest"
}

func (r *DataProtectionApplicationReconciler) getVMFileRestoreAccessImage() string {
	dpa := r.dpa
	unsupportedOverride := dpa.Spec.UnsupportedOverrides[oadpv1alpha1.VMFileRestoreAccessImageKey]
	if unsupportedOverride != "" {
		return unsupportedOverride
	}

	environmentVariable := os.Getenv("RELATED_IMAGE_VM_FILE_RESTORE_ACCESS")
	if environmentVariable != "" {
		return environmentVariable
	}

	return "quay.io/konveyor/oadp-vmfr-access:latest"
}

func (r *DataProtectionApplicationReconciler) getVMFileRestoreSSHImage() string {
	dpa := r.dpa
	unsupportedOverride := dpa.Spec.UnsupportedOverrides[oadpv1alpha1.VMFileRestoreSSHImageKey]
	if unsupportedOverride != "" {
		return unsupportedOverride
	}

	environmentVariable := os.Getenv("RELATED_IMAGE_VM_FILE_RESTORE_SSH")
	if environmentVariable != "" {
		return environmentVariable
	}

	return "quay.io/konveyor/oadp-vmfr-access-sshd:latest"
}

func (r *DataProtectionApplicationReconciler) getVMFileRestoreBrowserImage() string {
	dpa := r.dpa
	unsupportedOverride := dpa.Spec.UnsupportedOverrides[oadpv1alpha1.VMFileRestoreBrowserImageKey]
	if unsupportedOverride != "" {
		return unsupportedOverride
	}

	environmentVariable := os.Getenv("RELATED_IMAGE_VM_FILE_RESTORE_BROWSER")
	if environmentVariable != "" {
		return environmentVariable
	}

	return "quay.io/konveyor/oadp-vmfr-access-filebrowser:latest"
}

func (r *DataProtectionApplicationReconciler) getVMFileRestoreResources() corev1.ResourceRequirements {
	dpa := r.dpa

	// If custom resources specified in DPA, use them
	if dpa.Spec.VMFileRestore != nil && dpa.Spec.VMFileRestore.Resources != nil {
		return *dpa.Spec.VMFileRestore.Resources
	}

	// Default resource requirements (matching upstream oadp-vm-file-restore)
	return corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		},
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("10m"),
			corev1.ResourceMemory: resource.MustParse("64Mi"),
		},
	}
}
