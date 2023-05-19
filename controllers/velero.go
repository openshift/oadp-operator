package controllers

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/google/go-cmp/cmp"
	"github.com/openshift/oadp-operator/pkg/credentials"
	"github.com/operator-framework/operator-lib/proxy"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/intstr"

	//"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"
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
	Server = "server"
	//TODO: Check for default secret names
	VeleroAWSSecretName   = "cloud-credentials"
	VeleroAzureSecretName = "cloud-credentials-azure"
	VeleroGCPSecretName   = "cloud-credentials-gcp"
	enableCSIFeatureFlag  = "EnableCSI"
	veleroIOPrefix        = "velero.io/"
)

var (
	veleroLabelSelector = &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"k8s-app":   "openshift-adp",
			"component": common.Velero,
			"deploy":    common.Velero,
		},
	}
	oadpAppLabel = map[string]string{
		"app.kubernetes.io/name":       common.Velero,
		"app.kubernetes.io/managed-by": common.OADPOperator,
		"app.kubernetes.io/component":  Server,
		oadpv1alpha1.OadpOperatorLabel: "True",
	}
)

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
	var orig *appsv1.Deployment // for debugging purposes
	op, err := controllerutil.CreateOrPatch(r.Context, r.Client, veleroDeployment, func() error {
		if debugMode {
			orig = veleroDeployment.DeepCopy() // for debugging purposes
		}
		// Setting Deployment selector if a new object is created as it is immutable
		if veleroDeployment.ObjectMeta.CreationTimestamp.IsZero() {
			veleroDeployment.Spec.Selector = &metav1.LabelSelector{
				MatchLabels: getDpaAppLabels(&dpa),
			}
		}

		// update the Deployment template
		err := r.buildVeleroDeployment(veleroDeployment, &dpa)
		if err != nil {
			return err
		}

		// Setting controller owner reference on the velero deployment
		return controllerutil.SetControllerReference(&dpa, veleroDeployment, r.Scheme)
	})
	if debugMode && op != controllerutil.OperationResultNone { // for debugging purposes
		fmt.Printf("DEBUG: There was a diff which resulted in an operation on Velero Deployment: %s\n", cmp.Diff(orig, veleroDeployment))
	}

	if err != nil {
		if errors.IsInvalid(err) {
			cause, isStatusCause := errors.StatusCause(err, metav1.CauseTypeFieldValueInvalid)
			if isStatusCause && cause.Field == "spec.selector" {
				// recreate deployment
				// TODO: check for in-progress backup/restore to wait for it to finish
				log.Info("Found immutable selector from previous deployment, recreating Velero Deployment")
				err := r.Delete(r.Context, veleroDeployment)
				if err != nil {
					return false, err
				}
				return r.ReconcileVeleroDeployment(log)
			}
		}

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
	sa.Labels = getDpaAppLabels(dpa)
	return sa, nil
}

func (r *DPAReconciler) veleroClusterRoleBinding(dpa *oadpv1alpha1.DataProtectionApplication) (*rbacv1.ClusterRoleBinding, error) {
	crb := install.ClusterRoleBinding(dpa.Namespace)
	crb.Labels = getDpaAppLabels(dpa)
	return crb, nil
}

// Build VELERO Deployment
func (r *DPAReconciler) buildVeleroDeployment(veleroDeployment *appsv1.Deployment, dpa *oadpv1alpha1.DataProtectionApplication) error {

	if dpa == nil {
		return fmt.Errorf("DPA CR cannot be nil")
	}
	if veleroDeployment == nil {
		return fmt.Errorf("velero deployment cannot be nil")
	}
	// Auto corrects DPA
	dpa.AutoCorrect()

	_, err := r.ReconcileRestoreResourcesVersionPriority(dpa)
	if err != nil {
		return fmt.Errorf("error creating configmap for restore resource version priority:" + err.Error())
	}
	// get resource requirements for velero deployment
	// ignoring err here as it is checked in validator.go
	veleroResourceReqs, _ := r.getVeleroResourceReqs(dpa)
	podAnnotations, err := common.AppendUniqueKeyTOfTMaps(dpa.Spec.PodAnnotations, veleroDeployment.Annotations)
	if err != nil {
		return fmt.Errorf("error appending pod annotations: %v", err)
	}
	installDeployment := install.Deployment(veleroDeployment.Namespace,
		install.WithResources(veleroResourceReqs),
		install.WithImage(getVeleroImage(dpa)),
		install.WithAnnotations(podAnnotations),
		install.WithFeatures(dpa.Spec.Configuration.Velero.FeatureFlags),
		// last label overrides previous ones
		install.WithLabels(veleroDeployment.Labels),
		// use WithSecret false even if we have secret because we use a different VolumeMounts and EnvVars
		// see: https://github.com/vmware-tanzu/velero/blob/ed5809b7fc22f3661eeef10bdcb63f0d74472b76/pkg/install/deployment.go#L223-L261
		// our secrets are appended to containers/volumeMounts in credentials.AppendPluginSpecificSpecs function
		install.WithSecret(false),
		install.WithServiceAccountName(common.Velero),
	)
	veleroDeploymentName := veleroDeployment.Name
	veleroDeployment.TypeMeta = installDeployment.TypeMeta
	veleroDeployment.Spec = installDeployment.Spec
	veleroDeployment.Name = veleroDeploymentName
	labels, err := common.AppendUniqueKeyTOfTMaps(veleroDeployment.Labels, installDeployment.Labels)
	if err != nil {
		return fmt.Errorf("velero deployment label: %v", err)
	}
	veleroDeployment.Labels = labels
	annotations, err := common.AppendUniqueKeyTOfTMaps(veleroDeployment.Annotations, installDeployment.Annotations)
	veleroDeployment.Annotations = annotations
	return r.customizeVeleroDeployment(dpa, veleroDeployment)
}

func (r *DPAReconciler) customizeVeleroDeployment(dpa *oadpv1alpha1.DataProtectionApplication, veleroDeployment *appsv1.Deployment) error {
	//append dpa labels
	var err error
	veleroDeployment.Labels, err = common.AppendUniqueKeyTOfTMaps(veleroDeployment.Labels, getDpaAppLabels(dpa))
	if err != nil {
		return fmt.Errorf("velero deployment label: %v", err)
	}
	if veleroDeployment.Spec.Selector == nil {
		veleroDeployment.Spec.Selector = &metav1.LabelSelector{
			MatchLabels: make(map[string]string),
		}
	}
	if veleroDeployment.Spec.Selector.MatchLabels == nil {
		veleroDeployment.Spec.Selector.MatchLabels = make(map[string]string)
	}
	veleroDeployment.Spec.Selector.MatchLabels, err = common.AppendUniqueKeyTOfTMaps(veleroDeployment.Spec.Selector.MatchLabels, veleroDeployment.Labels, getDpaAppLabels(dpa))
	if err != nil {
		return fmt.Errorf("velero deployment selector label: %v", err)
	}
	veleroDeployment.Spec.Template.Labels, err = common.AppendUniqueKeyTOfTMaps(veleroDeployment.Spec.Template.Labels, veleroDeployment.Labels)
	if err != nil {
		return fmt.Errorf("velero deployment template label: %v", err)
	}
	// add custom pod labels
	if dpa.Spec.Configuration.Velero != nil && dpa.Spec.Configuration.Velero.PodConfig != nil && dpa.Spec.Configuration.Velero.PodConfig.Labels != nil {
		veleroDeployment.Spec.Template.Labels, err = common.AppendUniqueKeyTOfTMaps(veleroDeployment.Spec.Template.Labels, dpa.Spec.Configuration.Velero.PodConfig.Labels)
		if err != nil {
			return fmt.Errorf("velero deployment template custom label: %v", err)
		}
	}

	isSTSNeeded := r.isSTSTokenNeeded(dpa.Spec.BackupLocations, dpa.Namespace)

	// Selector: veleroDeployment.Spec.Selector,
	veleroDeployment.Spec.Replicas = pointer.Int32(1)
	if dpa.Spec.Configuration.Velero.PodConfig != nil {
		veleroDeployment.Spec.Template.Spec.Tolerations = dpa.Spec.Configuration.Velero.PodConfig.Tolerations
		veleroDeployment.Spec.Template.Spec.NodeSelector = dpa.Spec.Configuration.Velero.PodConfig.NodeSelector
	}
	veleroDeployment.Spec.Template.Spec.Volumes = append(veleroDeployment.Spec.Template.Spec.Volumes,
		corev1.Volume{
			Name: "certs",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})

	if isSTSNeeded {
		expirationSeconds := int64(3600)
		veleroDeployment.Spec.Template.Spec.Volumes = append(veleroDeployment.Spec.Template.Spec.Volumes,
			corev1.Volume{
				Name: "bound-sa-token",
				VolumeSource: corev1.VolumeSource{
					Projected: &corev1.ProjectedVolumeSource{
						DefaultMode: common.DefaultModePtr(),
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

	// if metrics address is set, change annotation and ports
	var prometheusPort *int
	if dpa.Spec.Configuration.Velero.Args != nil &&
		dpa.Spec.Configuration.Velero.Args.MetricsAddress != "" {
		address := strings.Split(dpa.Spec.Configuration.Velero.Args.MetricsAddress, ":")
		if len(address) == 2 {
			veleroDeployment.Spec.Template.Annotations["prometheus.io/port"] = address[1]
			if prometheusPort == nil {
				prometheusPort = new(int)
			}
			*prometheusPort, err = strconv.Atoi(address[1])
			if err != nil {
				return fmt.Errorf("error parsing metrics address port: %v", err)
			}
		}
	}

	var veleroContainer *corev1.Container
	for _, container := range veleroDeployment.Spec.Template.Spec.Containers {
		if container.Name == common.Velero {
			veleroContainer = &veleroDeployment.Spec.Template.Spec.Containers[0]
			break
		}
	}
	if err := r.customizeVeleroContainer(dpa, veleroDeployment, veleroContainer, isSTSNeeded, prometheusPort); err != nil {
		return err
	}

	providerNeedsDefaultCreds, hasCloudStorage, err := r.noDefaultCredentials(*dpa)
	if err != nil {
		return err
	}

	if dpa.Spec.Configuration.Velero.LogLevel != "" {
		logLevel, err := logrus.ParseLevel(dpa.Spec.Configuration.Velero.LogLevel)
		if err != nil {
			return fmt.Errorf("invalid log level %s, use: %s", dpa.Spec.Configuration.Velero.LogLevel, "trace, debug, info, warning, error, fatal, or panic")
		}
		veleroContainer.Args = append(veleroContainer.Args, "--log-level", logLevel.String())
	}

	// Setting async operations server parameter ItemOperationSyncFrequency
	if dpa.Spec.Configuration.Velero.ItemOperationSyncFrequency != "" {
		ItemOperationSyncFrequencyString := dpa.Spec.Configuration.Velero.ItemOperationSyncFrequency
		if err != nil {
			return err
		}
		veleroContainer.Args = append(veleroContainer.Args, fmt.Sprintf("--item-operation-sync-frequency=%v", ItemOperationSyncFrequencyString))
	}

	// Setting async operations server parameter DefaultItemOperationTimeout
	if dpa.Spec.Configuration.Velero.DefaultItemOperationTimeout != "" {
		DefaultItemOperationTimeoutString := dpa.Spec.Configuration.Velero.DefaultItemOperationTimeout
		if err != nil {
			return err
		}
		veleroContainer.Args = append(veleroContainer.Args, fmt.Sprintf("--default-item-operation-timeout=%v", DefaultItemOperationTimeoutString))
	}

	if dpa.Spec.Configuration.Velero.ResourceTimeout != "" {
		resourceTimeoutString := dpa.Spec.Configuration.Velero.ResourceTimeout
		if err != nil {
			return err
		}
		veleroContainer.Args = append(veleroContainer.Args, fmt.Sprintf("--resource-timeout=%v", resourceTimeoutString))
	}

	// Set defaults to avoid update events
	if veleroDeployment.Spec.Strategy.Type == "" {
		veleroDeployment.Spec.Strategy.Type = appsv1.RollingUpdateDeploymentStrategyType
	}
	if veleroDeployment.Spec.Strategy.RollingUpdate == nil {
		veleroDeployment.Spec.Strategy.RollingUpdate = &appsv1.RollingUpdateDeployment{
			MaxUnavailable: &intstr.IntOrString{Type: intstr.String, StrVal: "25%"},
			MaxSurge:       &intstr.IntOrString{Type: intstr.String, StrVal: "25%"},
		}
	}
	if veleroDeployment.Spec.RevisionHistoryLimit == nil {
		veleroDeployment.Spec.RevisionHistoryLimit = pointer.Int32(10)
	}
	if veleroDeployment.Spec.ProgressDeadlineSeconds == nil {
		veleroDeployment.Spec.ProgressDeadlineSeconds = pointer.Int32(600)
	}
	setPodTemplateSpecDefaults(&veleroDeployment.Spec.Template)
	return credentials.AppendPluginSpecificSpecs(dpa, veleroDeployment, veleroContainer, providerNeedsDefaultCreds, hasCloudStorage)
}

func (r *DPAReconciler) customizeVeleroContainer(dpa *oadpv1alpha1.DataProtectionApplication, veleroDeployment *appsv1.Deployment, veleroContainer *corev1.Container, isSTSNeeded bool, prometheusPort *int) error {
	if veleroContainer == nil {
		return fmt.Errorf("could not find velero container in Deployment")
	}
	if prometheusPort != nil {
		for i := range veleroContainer.Ports {
			if veleroContainer.Ports[i].Name == "metrics" {
				veleroContainer.Ports[i].ContainerPort = int32(*prometheusPort)
				break
			}
		}
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
	// append velero PodConfig envs to container
	if dpa.Spec.Configuration != nil && dpa.Spec.Configuration.Velero != nil && dpa.Spec.Configuration.Velero.PodConfig != nil && dpa.Spec.Configuration.Velero.PodConfig.Env != nil {
		veleroContainer.Env = common.AppendUniqueEnvVars(veleroContainer.Env, dpa.Spec.Configuration.Velero.PodConfig.Env)
	}
	// Append proxy settings to the container from environment variables
	veleroContainer.Env = common.AppendUniqueEnvVars(veleroContainer.Env, proxy.ReadProxyVarsFromEnv())
	if dpa.BackupImages() {
		veleroContainer.Env = common.AppendUniqueEnvVars(veleroContainer.Env, []corev1.EnvVar{{
			Name:  "OPENSHIFT_IMAGESTREAM_BACKUP",
			Value: "true",
		}})
	}

	// Check if data-mover is enabled and set the env var so that the csi data-mover code path is triggred
	if r.checkIfDataMoverIsEnabled(dpa) {
		veleroContainer.Env = common.AppendUniqueEnvVars(veleroContainer.Env, []corev1.EnvVar{{
			Name:  "VOLUME_SNAPSHOT_MOVER",
			Value: "true",
		}})

		if len(dpa.Spec.Features.DataMover.Timeout) > 0 {
			veleroContainer.Env = common.AppendUniqueEnvVars(veleroContainer.Env, []corev1.EnvVar{{
				Name:  "DATAMOVER_TIMEOUT",
				Value: dpa.Spec.Features.DataMover.Timeout,
			}})
		}
	}

	// Enable user to specify --fs-backup-timeout (defaults to 1h)
	fsBackupTimeout := "1h"
	if dpa.Spec.Configuration.Restic != nil && len(dpa.Spec.Configuration.Restic.Timeout) > 0 {
		fsBackupTimeout = dpa.Spec.Configuration.Restic.Timeout
	}
	// Append restic timeout option manually. Not configurable via install package, missing from podTemplateConfig struct. See: https://github.com/vmware-tanzu/velero/blob/8d57215ded1aa91cdea2cf091d60e072ce3f340f/pkg/install/deployment.go#L34-L45
	veleroContainer.Args = append(veleroContainer.Args, fmt.Sprintf("--fs-backup-timeout=%s", fsBackupTimeout))

	setContainerDefaults(veleroContainer)
	// if server args is set, override the default server args
	if dpa.Spec.Configuration.Velero.Args != nil {
		var err error
		veleroContainer.Args, err = dpa.Spec.Configuration.Velero.Args.StringArr(
			dpa.Spec.Configuration.Velero.FeatureFlags,
			dpa.Spec.Configuration.Velero.LogLevel)
		if err != nil {
			return err
		}
	}
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
	if os.Getenv("RELATED_IMAGE_VELERO") == "" {
		return common.VeleroImage
	}
	return os.Getenv("RELATED_IMAGE_VELERO")
}

func getDpaAppLabels(dpa *oadpv1alpha1.DataProtectionApplication) map[string]string {
	//append dpa name
	if dpa != nil {
		return getAppLabels(dpa.Name)
	}
	return nil
}

func getAppLabels(instanceName string) map[string]string {
	labels := make(map[string]string)
	//copy base labels
	for k, v := range oadpAppLabel {
		labels[k] = v
	}
	//append instance name
	if instanceName != "" {
		labels["app.kubernetes.io/instance"] = instanceName
	}
	return labels
}

// Get Velero Resource Requirements
func (r *DPAReconciler) getVeleroResourceReqs(dpa *oadpv1alpha1.DataProtectionApplication) (corev1.ResourceRequirements, error) {

	// Set default values
	ResourcesReqs := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		},
	}

	if dpa != nil && dpa.Spec.Configuration != nil && dpa.Spec.Configuration.Velero != nil && dpa.Spec.Configuration.Velero.PodConfig != nil {
		// Set custom limits and requests values if defined on VELERO Spec
		if dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Requests.Cpu() != nil && dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Requests.Cpu().Value() != 0 {
			parsedQuantity, err := resource.ParseQuantity(dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Requests.Cpu().String())
			ResourcesReqs.Requests[corev1.ResourceCPU] = parsedQuantity
			if err != nil {
				return ResourcesReqs, err
			}
		}

		if dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Requests.Memory() != nil && dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Requests.Memory().Value() != 0 {
			parsedQuantity, err := resource.ParseQuantity(dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Requests.Memory().String())
			ResourcesReqs.Requests[corev1.ResourceMemory] = parsedQuantity
			if err != nil {
				return ResourcesReqs, err
			}
		}

		if dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Limits.Cpu() != nil && dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Limits.Cpu().Value() != 0 {
			if ResourcesReqs.Limits == nil {
				ResourcesReqs.Limits = corev1.ResourceList{}
			}
			parsedQuantity, err := resource.ParseQuantity(dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Limits.Cpu().String())
			ResourcesReqs.Limits[corev1.ResourceCPU] = parsedQuantity
			if err != nil {
				return ResourcesReqs, err
			}
		}

		if dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Limits.Memory() != nil && dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Limits.Memory().Value() != 0 {
			if ResourcesReqs.Limits == nil {
				ResourcesReqs.Limits = corev1.ResourceList{}
			}
			parsedQuantiy, err := resource.ParseQuantity(dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Limits.Memory().String())
			ResourcesReqs.Limits[corev1.ResourceMemory] = parsedQuantiy
			if err != nil {
				return ResourcesReqs, err
			}
		}

	}

	return ResourcesReqs, nil
}

// Get Restic Resource Requirements
func getResticResourceReqs(dpa *oadpv1alpha1.DataProtectionApplication) (corev1.ResourceRequirements, error) {

	// Set default values
	ResourcesReqs := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		},
	}

	if dpa != nil && dpa.Spec.Configuration != nil && dpa.Spec.Configuration.Restic != nil && dpa.Spec.Configuration.Restic.PodConfig != nil {
		// Set custom limits and requests values if defined on Restic Spec
		if dpa.Spec.Configuration.Restic.PodConfig.ResourceAllocations.Requests.Cpu() != nil && dpa.Spec.Configuration.Restic.PodConfig.ResourceAllocations.Requests.Cpu().Value() != 0 {
			parsedQuantity, err := resource.ParseQuantity(dpa.Spec.Configuration.Restic.PodConfig.ResourceAllocations.Requests.Cpu().String())
			ResourcesReqs.Requests[corev1.ResourceCPU] = parsedQuantity
			if err != nil {
				return ResourcesReqs, err
			}
		}

		if dpa.Spec.Configuration.Restic.PodConfig.ResourceAllocations.Requests.Memory() != nil && dpa.Spec.Configuration.Restic.PodConfig.ResourceAllocations.Requests.Memory().Value() != 0 {
			parsedQuantity, err := resource.ParseQuantity(dpa.Spec.Configuration.Restic.PodConfig.ResourceAllocations.Requests.Memory().String())
			ResourcesReqs.Requests[corev1.ResourceMemory] = parsedQuantity
			if err != nil {
				return ResourcesReqs, err
			}
		}

		if dpa.Spec.Configuration.Restic.PodConfig.ResourceAllocations.Limits.Cpu() != nil && dpa.Spec.Configuration.Restic.PodConfig.ResourceAllocations.Limits.Cpu().Value() != 0 {
			if ResourcesReqs.Limits == nil {
				ResourcesReqs.Limits = corev1.ResourceList{}
			}
			parsedQuantity, err := resource.ParseQuantity(dpa.Spec.Configuration.Restic.PodConfig.ResourceAllocations.Limits.Cpu().String())
			ResourcesReqs.Limits[corev1.ResourceCPU] = parsedQuantity
			if err != nil {
				return ResourcesReqs, err
			}
		}

		if dpa.Spec.Configuration.Restic.PodConfig.ResourceAllocations.Limits.Memory() != nil && dpa.Spec.Configuration.Restic.PodConfig.ResourceAllocations.Limits.Memory().Value() != 0 {
			if ResourcesReqs.Limits == nil {
				ResourcesReqs.Limits = corev1.ResourceList{}
			}
			parsedQuantiy, err := resource.ParseQuantity(dpa.Spec.Configuration.Restic.PodConfig.ResourceAllocations.Limits.Memory().String())
			ResourcesReqs.Limits[corev1.ResourceMemory] = parsedQuantiy
			if err != nil {
				return ResourcesReqs, err
			}
		}

	}

	return ResourcesReqs, nil
}

// noDefaultCredentials determines if a provider needs the default credentials.
// This returns a map of providers found to if they need a default credential,
// a boolean if Cloud Storage backup storage location was used and an error if any occured.
func (r DPAReconciler) noDefaultCredentials(dpa oadpv1alpha1.DataProtectionApplication) (map[string]bool, bool, error) {
	providerNeedsDefaultCreds := map[string]bool{}
	hasCloudStorage := false
	if dpa.Spec.Configuration.Velero.NoDefaultBackupLocation {
		needDefaultCred := false

		if dpa.Spec.UnsupportedOverrides[oadpv1alpha1.OperatorTypeKey] == oadpv1alpha1.OperatorTypeMTC {
			// MTC requires default credentials
			needDefaultCred = true
		}
		// go through cloudprovider plugins and mark providerNeedsDefaultCreds to false
		for _, provider := range dpa.Spec.Configuration.Velero.DefaultPlugins {
			if psf, ok := credentials.PluginSpecificFields[provider]; ok && psf.IsCloudProvider {
				providerNeedsDefaultCreds[psf.PluginName] = needDefaultCred
			}
		}
	} else {
		for _, bsl := range dpa.Spec.BackupLocations {
			if bsl.Velero != nil && bsl.Velero.Credential == nil {
				bslProvider := strings.TrimPrefix(bsl.Velero.Provider, veleroIOPrefix)
				providerNeedsDefaultCreds[bslProvider] = true
			}
			if bsl.Velero != nil && bsl.Velero.Credential != nil {
				bslProvider := strings.TrimPrefix(bsl.Velero.Provider, veleroIOPrefix)
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
	}
	for _, vsl := range dpa.Spec.SnapshotLocations {
		if vsl.Velero != nil {
			// To handle the case where we want to manually hand the credentials for a cloud storage created
			// Bucket credentials via configuration. Only AWS is supported
			provider := strings.TrimPrefix(vsl.Velero.Provider, veleroIOPrefix)
			if vsl.Velero.Credential != nil || provider == string(oadpv1alpha1.AWSBucketProvider) && hasCloudStorage {
				providerNeedsDefaultCreds[provider] = false
			} else {
				providerNeedsDefaultCreds[provider] = true
			}
		}
	}

	return providerNeedsDefaultCreds, hasCloudStorage, nil

}
