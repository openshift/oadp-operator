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
	"github.com/openshift/oadp-operator/pkg/credentials/stsflow"
)

const (
	kubevirtDatamoverObjectName        = "oadp-kubevirt-datamover-controller-manager"
	kubevirtDatamoverControlPlaneKey   = "control-plane"
	kubevirtDatamoverControlPlaneValue = "oadp-kubevirt-datamover-controller"
	kdmDpaResourceVersionAnnotation    = oadpv1alpha1.OadpOperatorLabel + "-kdm-dpa-resource-version"

	// Probe tuning. /healthz and /readyz are healthz.Ping, so a failed probe
	// means the process could not answer HTTP in time (for example while
	// stalled near its memory limit). With a 5s timeout and 5 consecutive
	// failures the liveness probe (period 20s) restarts the container about
	// 85s after the first failed probe, instead of about 40s with the
	// Kubernetes defaults (1s timeout, 3 failures).
	kubevirtDatamoverProbeTimeoutSeconds   = 5
	kubevirtDatamoverProbeFailureThreshold = 5

	kubevirtDatamoverResourceAllocationsPath = "spec.configuration.kubevirtDatamover.podConfig.resourceAllocations"
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

	// Default kubevirt-datamover-controller container resources. The
	// controller caches DataUploads, VirtualMachineBackups, PVCs and pods, so
	// its memory grows with the number of VMs and the size of the cluster.
	// There is no CPU limit so CPU throttling cannot slow down the health
	// probes.
	kubevirtDatamoverDefaultResourceRequirements = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("100m"),
			corev1.ResourceMemory: resource.MustParse("256Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("1Gi"),
		},
	}

	// Resources that can be set in kubevirtDatamover.podConfig.resourceAllocations
	// (the ones getResourceListFrom applies), in the order they are validated.
	kubevirtDatamoverResourceNames = []corev1.ResourceName{
		corev1.ResourceCPU,
		corev1.ResourceMemory,
		corev1.ResourceEphemeralStorage,
	}
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
	// Get resource requirements
	resources, err := getKubevirtDatamoverResourceReqs(dpa)
	if err != nil {
		return err
	}

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

	// The Go runtime does not read the container memory limit, so set a soft
	// limit below it to make the GC work harder before the container runs out
	// of memory.
	if goMemLimit, ok := kubevirtDatamoverGoMemLimit(resources.Limits); ok {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "GOMEMLIMIT",
			Value: goMemLimit,
		})
	}

	// Add Azure workload identity environment variables if configured
	var envFrom []corev1.EnvFromSource
	azureClientID := os.Getenv(stsflow.ClientIDEnvKey)
	if azureClientID != "" && os.Getenv(stsflow.TenantIDEnvKey) != "" && os.Getenv(stsflow.SubscriptionIDEnvKey) != "" {
		envFrom = append(envFrom, corev1.EnvFromSource{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: stsflow.AzureWorkloadIdentitySecretName,
				},
			},
		})
		r.Log.Info("Added Azure workload identity secret reference to KubeVirt DataMover container")
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
	// The controller reads its log settings from controller-runtime's zap
	// flags, not from the LOG_LEVEL and LOG_FORMAT environment variables.
	args = append(args, kubevirtDatamoverLogArgs(dpa)...)
	if dpa.Spec.Configuration != nil && dpa.Spec.Configuration.KubevirtDatamover != nil {
		if dpa.Spec.Configuration.KubevirtDatamover.MaxIncrementalBackups != nil {
			args = append(args, fmt.Sprintf("--max-incremental-backups=%d",
				*dpa.Spec.Configuration.KubevirtDatamover.MaxIncrementalBackups))
		}
		if dpa.Spec.Configuration.KubevirtDatamover.MaxConcurrentDataMovers != nil {
			args = append(args, fmt.Sprintf("--max-concurrent-data-movers=%d",
				*dpa.Spec.Configuration.KubevirtDatamover.MaxConcurrentDataMovers))
		}
		if dpa.Spec.Configuration.KubevirtDatamover.StaleDataUploadThreshold != nil {
			args = append(args, fmt.Sprintf("--stale-dataupload-threshold=%s",
				dpa.Spec.Configuration.KubevirtDatamover.StaleDataUploadThreshold.Duration.String()))
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
		EnvFrom:         envFrom,
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
			{
				Name:      "bound-sa-token",
				MountPath: "/var/run/secrets/openshift/serviceaccount",
				ReadOnly:  true,
			},
		},
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: ptr.To(false),
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
			ReadOnlyRootFilesystem: ptr.To(true),
		},
		LivenessProbe:            kubevirtDatamoverProbe("/healthz", 15, 20),
		ReadinessProbe:           kubevirtDatamoverProbe("/readyz", 5, 10),
		TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
	}

	if len(deploymentObject.Spec.Template.Spec.Containers) == 0 {
		deploymentObject.Spec.Template.Spec.Containers = []corev1.Container{containerSpec}
		kubevirtDatamoverContainerFound = true
	} else {
		for index, container := range deploymentObject.Spec.Template.Spec.Containers {
			if container.Name == "manager" {
				// Update dynamic fields that can change based on DPA configuration.
				// Probes are also re-applied so existing deployments pick up probe
				// changes on upgrade; every field the API server defaults is set
				// explicitly, so this does not cause an update on every reconcile.
				// Static fields (Command, Ports, SecurityContext) are left as-is
				kubevirtDatamoverContainer := &deploymentObject.Spec.Template.Spec.Containers[index]
				kubevirtDatamoverContainer.Image = image
				kubevirtDatamoverContainer.ImagePullPolicy = imagePullPolicy
				kubevirtDatamoverContainer.Args = args
				kubevirtDatamoverContainer.Env = envVars
				kubevirtDatamoverContainer.EnvFrom = envFrom
				kubevirtDatamoverContainer.Resources = resources
				kubevirtDatamoverContainer.LivenessProbe = containerSpec.LivenessProbe
				kubevirtDatamoverContainer.ReadinessProbe = containerSpec.ReadinessProbe
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
		{
			Name: "bound-sa-token",
			VolumeSource: corev1.VolumeSource{
				Projected: &corev1.ProjectedVolumeSource{
					// Set explicitly to the API server default so the
					// deployment does not differ from the stored object on
					// every reconcile.
					DefaultMode: ptr.To(common.DefaultProjectedPermission),
					Sources: []corev1.VolumeProjection{
						{
							ServiceAccountToken: &corev1.ServiceAccountTokenProjection{
								Audience:          "openshift",
								ExpirationSeconds: ptr.To(int64(3600)),
								Path:              "token",
							},
						},
					},
				},
			},
		},
	}
	deploymentObject.Spec.Template.Spec.RestartPolicy = corev1.RestartPolicyAlways
	deploymentObject.Spec.Template.Spec.ServiceAccountName = "velero"
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
	return "quay.io/konveyor/kubevirt-datamover-controller:latest"
}

// kubevirtDatamoverProbe returns an HTTP probe against the health probe port.
// Every field the API server defaults is set explicitly so the probe compares
// equal to the stored object and re-applying it on update is a no-op.
func kubevirtDatamoverProbe(path string, initialDelaySeconds, periodSeconds int32) *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path:   path,
				Port:   intstr.FromInt(8081),
				Scheme: corev1.URISchemeHTTP,
			},
		},
		InitialDelaySeconds: initialDelaySeconds,
		PeriodSeconds:       periodSeconds,
		TimeoutSeconds:      kubevirtDatamoverProbeTimeoutSeconds,
		SuccessThreshold:    1,
		FailureThreshold:    kubevirtDatamoverProbeFailureThreshold,
	}
}

// getKubevirtDatamoverResourceReqs returns the kubevirt-datamover-controller
// container resources: the defaults, with each cpu, memory or
// ephemeral-storage request or limit set in
// spec.configuration.kubevirtDatamover.podConfig.resourceAllocations
// replacing the matching default.
//
// When a value from the DPA conflicts with a default (for example a memory
// request above the default memory limit), the default is adjusted to match
// the DPA value. A request above a limit when both come from the DPA is an
// error.
func getKubevirtDatamoverResourceReqs(dpa *oadpv1alpha1.DataProtectionApplication) (corev1.ResourceRequirements, error) {
	resources := *kubevirtDatamoverDefaultResourceRequirements.DeepCopy()
	if dpa.Spec.Configuration == nil || dpa.Spec.Configuration.KubevirtDatamover == nil ||
		dpa.Spec.Configuration.KubevirtDatamover.PodConfig == nil {
		return resources, nil
	}
	allocations := dpa.Spec.Configuration.KubevirtDatamover.PodConfig.ResourceAllocations

	// getResourceListFrom skips zero values but passes negative ones through.
	for _, name := range kubevirtDatamoverResourceNames {
		if quantity, ok := allocations.Requests[name]; ok && quantity.Sign() < 0 {
			return resources, fmt.Errorf("%s: requests.%s must not be negative, got %s",
				kubevirtDatamoverResourceAllocationsPath, name, quantity.String())
		}
		if quantity, ok := allocations.Limits[name]; ok && quantity.Sign() < 0 {
			return resources, fmt.Errorf("%s: limits.%s must not be negative, got %s",
				kubevirtDatamoverResourceAllocationsPath, name, quantity.String())
		}
	}

	if allocations.Requests != nil {
		requests, err := getResourceListFrom(allocations.Requests, resources.Requests)
		if err != nil {
			return resources, err
		}
		resources.Requests = *requests
	}
	if allocations.Limits != nil {
		limits, err := getResourceListFrom(allocations.Limits, resources.Limits)
		if err != nil {
			return resources, err
		}
		resources.Limits = *limits
	}

	for _, name := range kubevirtDatamoverResourceNames {
		request, hasRequest := resources.Requests[name]
		limit, hasLimit := resources.Limits[name]
		if !hasRequest || !hasLimit || request.Cmp(limit) <= 0 {
			continue
		}
		requestSet := isResourceSet(allocations.Requests, name)
		limitSet := isResourceSet(allocations.Limits, name)
		switch {
		case requestSet && limitSet:
			return resources, fmt.Errorf("%s: requests.%s (%s) must be less than or equal to limits.%s (%s)",
				kubevirtDatamoverResourceAllocationsPath, name, request.String(), name, limit.String())
		case requestSet:
			resources.Limits[name] = request.DeepCopy()
		case limitSet:
			resources.Requests[name] = limit.DeepCopy()
		}
	}

	return resources, nil
}

// isResourceSet reports whether list has a non-zero value for name, matching
// the values getResourceListFrom applies.
func isResourceSet(list corev1.ResourceList, name corev1.ResourceName) bool {
	quantity, ok := list[name]
	return ok && !quantity.IsZero()
}

// kubevirtDatamoverGoMemLimit returns the GOMEMLIMIT value for the given
// container limits: 90% of the memory limit, in bytes. It returns false when
// there is no memory limit.
func kubevirtDatamoverGoMemLimit(limits corev1.ResourceList) (string, bool) {
	memoryLimit, ok := limits[corev1.ResourceMemory]
	if !ok {
		return "", false
	}
	// Divide first so large limits cannot overflow int64.
	goMemLimit := memoryLimit.Value() / 10 * 9
	if goMemLimit <= 0 {
		return "", false
	}
	// GOMEMLIMIT takes a byte count or a B/KiB/MiB/GiB/TiB suffix, not
	// Kubernetes quantity suffixes such as Mi, so always use plain bytes.
	return strconv.FormatInt(goMemLimit, 10), true
}

// kubevirtDatamoverLogArgs returns the controller-runtime zap flags for the
// DPA log settings.
func kubevirtDatamoverLogArgs(dpa *oadpv1alpha1.DataProtectionApplication) []string {
	logLevel := ""
	if dpa.Spec.Configuration != nil && dpa.Spec.Configuration.Velero != nil {
		logLevel = dpa.Spec.Configuration.Velero.LogLevel
	}
	return []string{
		"--zap-log-level=" + kubevirtDatamoverZapLogLevel(logLevel),
		"--zap-encoder=" + kubevirtDatamoverZapEncoder(dpa.Spec.LogFormat),
	}
}

// kubevirtDatamoverZapLogLevel maps a velero.logLevel value (a logrus level)
// to a --zap-log-level value. The flag only accepts debug, info, error, panic
// or a positive verbosity, and the controller exits on startup for anything
// else, so unset and unknown values map to info.
// The controller logs its warnings at info and its fatal errors at error, so
// warning maps to info and fatal and panic map to error instead of hiding
// those messages.
func kubevirtDatamoverZapLogLevel(logLevel string) string {
	level, err := logrus.ParseLevel(logLevel)
	if err != nil {
		return "info"
	}
	switch level {
	case logrus.TraceLevel:
		// V(2), one step more verbose than debug (V(1)).
		return "2"
	case logrus.DebugLevel:
		return "debug"
	case logrus.InfoLevel, logrus.WarnLevel:
		return "info"
	default:
		return "error"
	}
}

// kubevirtDatamoverZapEncoder maps the DPA logFormat to a --zap-encoder value.
func kubevirtDatamoverZapEncoder(logFormat oadpv1alpha1.LogFormat) string {
	if logFormat == oadpv1alpha1.LogFormatJSON {
		return "json"
	}
	return "console"
}
