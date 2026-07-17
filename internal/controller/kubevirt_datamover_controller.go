package controller

import (
	"fmt"
	"os"
	"strconv"

	"github.com/go-logr/logr"
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

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/common"
)

const (
	kubevirtDatamoverObjectName        = "oadp-kubevirt-datamover-controller-manager"
	kubevirtDatamoverControlPlaneKey   = "control-plane"
	kubevirtDatamoverControlPlaneValue = "oadp-kubevirt-datamover-controller"
	kdmDpaResourceVersionAnnotation    = oadpv1alpha1.OadpOperatorLabel + "-kdm-dpa-resource-version"
)

var (
	kubevirtDatamoverControlPlaneLabel = map[string]string{
		kubevirtDatamoverControlPlaneKey: kubevirtDatamoverControlPlaneValue,
	}
	kubevirtDatamoverDeploymentLabels = map[string]string{
		"app.kubernetes.io/component":  "manager",
		"app.kubernetes.io/created-by": common.OADPOperator,
		"app.kubernetes.io/instance":   kubevirtDatamoverObjectName,
		"app.kubernetes.io/managed-by": "kustomize",
		"app.kubernetes.io/name":       "deployment",
		"app.kubernetes.io/part-of":    common.OADPOperator,
	}

	kdmDpaResourceVersion            = ""
	previousKubevirtDatamoverEnabled = false
)

// ReconcileKubevirtDatamoverController manages the kubevirt-datamover controller deployment
func (r *DataProtectionApplicationReconciler) ReconcileKubevirtDatamoverController(log logr.Logger) (bool, error) {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubevirtDatamoverObjectName,
			Namespace: r.NamespacedName.Namespace,
		},
	}

	// Delete deployment if plugin not enabled
	if !r.checkKubevirtDatamoverEnabled() {
		if err := r.Get(r.Context, types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, deployment); err != nil {
			if k8serror.IsNotFound(err) {
				return true, nil
			}
			return false, err
		}
		if err := r.Delete(r.Context, deployment, &client.DeleteOptions{PropagationPolicy: ptr.To(metav1.DeletePropagationForeground)}); err != nil {
			r.EventRecorder.Event(deployment, corev1.EventTypeWarning, "KubevirtDatamoverDeploymentDeleteFailed",
				fmt.Sprintf("Could not delete kubevirt-datamover controller deployment %s/%s: %s", deployment.Namespace, deployment.Name, err))
			return false, err
		}
		r.EventRecorder.Event(deployment, corev1.EventTypeNormal, "KubevirtDatamoverDeploymentDeleteSucceed",
			fmt.Sprintf("Kubevirt-datamover controller deployment %s/%s deleted", deployment.Namespace, deployment.Name))
		return true, nil
	}

	// Create or update deployment
	operation, err := controllerutil.CreateOrUpdate(r.Context, r.Client, deployment, func() error {
		if err := r.buildKubevirtDatamoverDeployment(deployment); err != nil {
			return err
		}
		return controllerutil.SetControllerReference(r.dpa, deployment, r.Scheme)
	})
	if err != nil {
		return false, err
	}
	if operation != controllerutil.OperationResultNone {
		r.EventRecorder.Event(deployment, corev1.EventTypeNormal, "KubevirtDatamoverDeploymentReconciled",
			fmt.Sprintf("Kubevirt-datamover controller deployment %s/%s %s", deployment.Namespace, deployment.Name, operation))
	}
	return true, nil
}

func (r *DataProtectionApplicationReconciler) buildKubevirtDatamoverDeployment(deploymentObject *appsv1.Deployment) error {
	kubevirtDatamoverControllerImage := r.getKubevirtDatamoverControllerImage()
	imagePullPolicy, err := common.GetImagePullPolicy(r.dpa.Spec.ImagePullPolicy, kubevirtDatamoverControllerImage)
	if err != nil {
		r.Log.Error(err, "imagePullPolicy regex failed")
	}
	ensureKubevirtDatamoverRequiredLabels(deploymentObject)
	err = ensureKubevirtDatamoverRequiredSpecs(deploymentObject, r.dpa, kubevirtDatamoverControllerImage, imagePullPolicy, r)
	if err != nil {
		return err
	}

	// Apply user-provided resource labels and annotations
	// Note: NOT applied to Spec.Selector.MatchLabels as those are immutable after creation
	deploymentObject.Labels, deploymentObject.Annotations = applyResourceLabels(r.dpa, deploymentObject.Labels, deploymentObject.Annotations)
	deploymentObject.Spec.Template.Labels, deploymentObject.Spec.Template.Annotations = applyResourceLabels(r.dpa, deploymentObject.Spec.Template.Labels, deploymentObject.Spec.Template.Annotations)

	// Re-assert selector labels to ensure template labels match selector
	// This prevents user resourceLabels from overriding selector-critical labels
	if deploymentObject.Spec.Selector != nil && deploymentObject.Spec.Selector.MatchLabels != nil {
		if deploymentObject.Spec.Template.Labels == nil {
			deploymentObject.Spec.Template.Labels = make(map[string]string)
		}
		for k, v := range deploymentObject.Spec.Selector.MatchLabels {
			deploymentObject.Spec.Template.Labels[k] = v
		}
	}

	deploymentObject.Annotations = applyResourceAnnotations(r.dpa, deploymentObject.Annotations)
	deploymentObject.Spec.Template.Annotations = applyResourceAnnotations(r.dpa, deploymentObject.Spec.Template.Annotations)

	return nil
}

func ensureKubevirtDatamoverRequiredLabels(deploymentObject *appsv1.Deployment) {
	maps.Copy(kubevirtDatamoverDeploymentLabels, kubevirtDatamoverControlPlaneLabel)
	deploymentObjectLabels := deploymentObject.GetLabels()
	if deploymentObjectLabels == nil {
		deploymentObject.SetLabels(kubevirtDatamoverDeploymentLabels)
	} else {
		for key, value := range kubevirtDatamoverDeploymentLabels {
			deploymentObjectLabels[key] = value
		}
		deploymentObject.SetLabels(deploymentObjectLabels)
	}
}

func ensureKubevirtDatamoverRequiredSpecs(
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
			Name:  "DATAMOVER_IMAGE",
			Value: image,
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
	currentKubevirtDatamoverEnabled := r.checkKubevirtDatamoverEnabled()
	if len(kdmDpaResourceVersion) == 0 ||
		currentKubevirtDatamoverEnabled != previousKubevirtDatamoverEnabled {
		kdmDpaResourceVersion = dpa.GetResourceVersion()
		previousKubevirtDatamoverEnabled = currentKubevirtDatamoverEnabled
	}

	podAnnotations := map[string]string{
		kdmDpaResourceVersionAnnotation: kdmDpaResourceVersion,
	}

	// Get resource requirements
	resources := r.getKubevirtDatamoverResources()

	// Set deployment spec
	deploymentObject.Spec.Replicas = ptr.To(int32(1))
	deploymentObject.Spec.Selector = &metav1.LabelSelector{
		MatchLabels: kubevirtDatamoverControlPlaneLabel,
	}

	// Set template labels
	templateObjectLabels := deploymentObject.Spec.Template.GetLabels()
	if templateObjectLabels == nil {
		deploymentObject.Spec.Template.SetLabels(kubevirtDatamoverControlPlaneLabel)
	} else {
		templateObjectLabels[kubevirtDatamoverControlPlaneKey] = kubevirtDatamoverControlPlaneLabel[kubevirtDatamoverControlPlaneKey]
		deploymentObject.Spec.Template.SetLabels(templateObjectLabels)
	}

	// Set template annotations
	templateObjectAnnotations := deploymentObject.Spec.Template.GetAnnotations()
	if templateObjectAnnotations == nil {
		deploymentObject.Spec.Template.SetAnnotations(podAnnotations)
	} else {
		templateObjectAnnotations[kdmDpaResourceVersionAnnotation] = podAnnotations[kdmDpaResourceVersionAnnotation]
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

	// Build container args
	args := []string{
		"--leader-elect",
		"--health-probe-bind-address=:8081",
		"--metrics-bind-address=:8443",
		"--metrics-secure=true",
	}
	if dpa.Spec.Configuration != nil && dpa.Spec.Configuration.KubevirtDatamover != nil {
		if dpa.Spec.Configuration.KubevirtDatamover.MaxIncrementalBackups != nil {
			args = append(args, fmt.Sprintf("--max-incremental-backups=%d",
				*dpa.Spec.Configuration.KubevirtDatamover.MaxIncrementalBackups))
		}
	}

	// Build container spec
	kubevirtDatamoverContainerFound := false
	containerSpec := corev1.Container{
		Name:            "manager",
		Image:           image,
		ImagePullPolicy: imagePullPolicy,
		Command:         []string{"/manager"},
		Args:            args,
		Env:             envVars,
		Ports: []corev1.ContainerPort{
			{
				Name:          "https",
				ContainerPort: 8443,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Resources: resources,
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "tmp",
				MountPath: "/tmp",
			},
		},
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
		kubevirtDatamoverContainerFound = true
	} else {
		for index, container := range deploymentObject.Spec.Template.Spec.Containers {
			if container.Name == "manager" {
				// Update dynamic fields that can change based on DPA configuration
				// Static fields (Command, Ports, SecurityContext, Probes) are left as-is
				kubevirtDatamoverContainer := &deploymentObject.Spec.Template.Spec.Containers[index]
				kubevirtDatamoverContainer.Image = image
				kubevirtDatamoverContainer.ImagePullPolicy = imagePullPolicy
				kubevirtDatamoverContainer.Args = args
				kubevirtDatamoverContainer.Env = envVars
				kubevirtDatamoverContainer.Resources = resources
				kubevirtDatamoverContainer.TerminationMessagePolicy = corev1.TerminationMessageFallbackToLogsOnError
				kubevirtDatamoverContainerFound = true
				break
			}
		}
	}

	if !kubevirtDatamoverContainerFound {
		return fmt.Errorf("could not find kubevirt-datamover container in Deployment")
	}

	deploymentObject.Spec.Template.Spec.Volumes = []corev1.Volume{
		{
			Name: "tmp",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}
	deploymentObject.Spec.Template.Spec.RestartPolicy = corev1.RestartPolicyAlways
	deploymentObject.Spec.Template.Spec.ServiceAccountName = kubevirtDatamoverObjectName
	return nil
}

func (r *DataProtectionApplicationReconciler) checkKubevirtDatamoverEnabled() bool {
	if r.dpa.Spec.Configuration == nil || r.dpa.Spec.Configuration.Velero == nil {
		return false
	}
	if r.dpa.Spec.Configuration.Velero.DefaultPlugins != nil {
		for _, plugin := range r.dpa.Spec.Configuration.Velero.DefaultPlugins {
			if plugin == oadpv1alpha1.DefaultPluginKubeVirtDataMover {
				return true
			}
		}
	}
	return false
}

func (r *DataProtectionApplicationReconciler) getKubevirtDatamoverControllerImage() string {
	// Priority 1: UnsupportedOverrides (controller-specific key)
	if unsupportedOverride := r.dpa.Spec.UnsupportedOverrides[oadpv1alpha1.KubeVirtDatamoverControllerImageKey]; unsupportedOverride != "" {
		return unsupportedOverride
	}
	// Priority 2: Environment variable
	if envVar := os.Getenv("RELATED_IMAGE_KUBEVIRT_DATAMOVER_CONTROLLER"); envVar != "" {
		return envVar
	}
	// Priority 3: Default
	return "quay.io/konveyor/kubevirt-datamover-controller:oadp-1.6"
}

func (r *DataProtectionApplicationReconciler) getKubevirtDatamoverResources() corev1.ResourceRequirements {
	// Conservative defaults matching vmfilerestore pattern
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
