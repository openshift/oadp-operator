package controllers

import (
	"context"
	"fmt"
	"os"
	"reflect"

	"github.com/go-logr/logr"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/operator-framework/operator-lib/proxy"
	"github.com/vmware-tanzu/velero/pkg/install"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/common"
	"github.com/openshift/oadp-operator/pkg/credentials"
)

const (
	ResticRestoreHelperCM   = "restic-restore-action-config"
	FsRestoreHelperCM       = "fs-restore-action-config"
	HostPods                = "host-pods"
	HostPlugins             = "host-plugins"
	Cluster                 = "cluster"
	IBMCloudPlatform        = "IBMCloud"
	GenericPVHostPath       = "/var/lib/kubelet/pods"
	IBMCloudPVHostPath      = "/var/data/kubelet/pods"
	GenericPluginsHostPath  = "/var/lib/kubelet/plugins"
	IBMCloudPluginsHostPath = "/var/data/kubelet/plugins"
	ResticPVHostPathEnvVar  = "RESTIC_PV_HOSTPATH"
	FSPVHostPathEnvVar      = "FS_PV_HOSTPATH"
	PluginsHostPathEnvVar   = "PLUGINS_HOSTPATH"
)

var (
	// v1.MountPropagationHostToContainer is a const. Const cannot be pointed to.
	// we need to declare mountPropagationToHostContainer so that we have an address to point to
	// for ds.Spec.Template.Spec.Volumes[].Containers[].VolumeMounts[].MountPropagation
	mountPropagationToHostContainer = corev1.MountPropagationHostToContainer
	nodeAgentMatchLabels            = map[string]string{
		"component": common.Velero,
		"name":      common.NodeAgent,
	}
	nodeAgentLabelSelector = &metav1.LabelSelector{
		MatchLabels: nodeAgentMatchLabels,
	}
)

// getFsPvHostPath returns the host path for persistent volumes based on the platform type.
func getFsPvHostPath(platformType string) string {
	// Check if environment variables are set for host paths
	if envFs := os.Getenv(FSPVHostPathEnvVar); envFs != "" {
		return envFs
	}

	if env := os.Getenv(ResticPVHostPathEnvVar); env != "" {
		return env
	}

	// Return platform-specific host paths
	switch platformType {
	case IBMCloudPlatform:
		return IBMCloudPVHostPath
	default:
		return GenericPVHostPath
	}
}

// getPluginsHostPath returns the host path for persistent volumes based on the platform type.
func getPluginsHostPath(platformType string) string {
	// Check if environment var is set for host plugins
	if env := os.Getenv(PluginsHostPathEnvVar); env != "" {
		return env
	}

	// Return platform-specific host paths
	switch platformType {
	case IBMCloudPlatform:
		return IBMCloudPluginsHostPath
	default:
		return GenericPluginsHostPath
	}
}

func getNodeAgentObjectMeta(r *DPAReconciler) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      common.NodeAgent,
		Namespace: r.NamespacedName.Namespace,
		Labels:    nodeAgentMatchLabels,
	}
}

func (r *DPAReconciler) ReconcileNodeAgentDaemonset(log logr.Logger) (bool, error) {
	dpa := r.dpa
	var deleteDaemonSet bool = true
	// Define "static" portion of daemonset
	ds := &appsv1.DaemonSet{
		ObjectMeta: getNodeAgentObjectMeta(r),
	}

	if dpa.Spec.Configuration.Restic != nil {
		// V(-1) corresponds to the warn level
		var deprecationMsg string = "(Deprecation Warning) Use nodeAgent instead of restic, which is deprecated and will be removed in the future"
		log.V(-1).Info(deprecationMsg)
		r.EventRecorder.Event(dpa, corev1.EventTypeWarning, "DeprecationResticConfig", deprecationMsg)
	}

	if dpa.Spec.Configuration.Restic != nil && dpa.Spec.Configuration.Restic.Enable != nil && *dpa.Spec.Configuration.Restic.Enable {
		deleteDaemonSet = false
	} else if dpa.Spec.Configuration.NodeAgent != nil && dpa.Spec.Configuration.NodeAgent.Enable != nil && *dpa.Spec.Configuration.NodeAgent.Enable {
		deleteDaemonSet = false
	}

	if deleteDaemonSet {
		deleteContext := context.Background()
		if err := r.Get(deleteContext, types.NamespacedName{
			Name:      ds.Name,
			Namespace: r.NamespacedName.Namespace,
		}, ds); err != nil {
			if errors.IsNotFound(err) {
				return true, nil
			}
			return false, err
		}
		// no errors means there already is an existing DaeMonset.
		// TODO: Check if NodeAgent is in use, a backup is running, so don't blindly delete NodeAgent.
		// If dpa.Spec.Configuration.NodeAgent enable exists and is false, attempt to delete.
		deleteOptionPropagationForeground := metav1.DeletePropagationForeground
		if err := r.Delete(deleteContext, ds, &client.DeleteOptions{PropagationPolicy: &deleteOptionPropagationForeground}); err != nil {
			// TODO: Come back and fix event recording to be consistent
			r.EventRecorder.Event(ds, corev1.EventTypeNormal, "DeleteDaemonSetFailed", "Got DaemonSet to delete but could not delete err:"+err.Error())
			return false, err
		}
		r.EventRecorder.Event(ds, corev1.EventTypeNormal, "DeletedDaemonSet", "DaemonSet deleted")

		return true, nil
	}

	op, err := controllerutil.CreateOrPatch(r.Context, r.Client, ds, func() error {
		// Deployment selector is immutable so we set this value only if
		// a new object is going to be created
		if ds.ObjectMeta.CreationTimestamp.IsZero() {
			if ds.Spec.Selector == nil {
				ds.Spec.Selector = &metav1.LabelSelector{}
			}
			var err error
			if ds.Spec.Selector == nil {
				ds.Spec.Selector = &metav1.LabelSelector{
					MatchLabels: make(map[string]string),
				}
			}
			if ds.Spec.Selector.MatchLabels == nil {
				ds.Spec.Selector.MatchLabels = make(map[string]string)
			}
			ds.Spec.Selector.MatchLabels, err = common.AppendUniqueKeyTOfTMaps(ds.Spec.Selector.MatchLabels, nodeAgentLabelSelector.MatchLabels)
			if err != nil {
				return fmt.Errorf("failed to append labels to selector: %s", err)
			}
		}

		if _, err := r.buildNodeAgentDaemonset(ds); err != nil {
			return err
		}
		if err := controllerutil.SetControllerReference(dpa, ds, r.Scheme); err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		if errors.IsInvalid(err) {
			cause, isStatusCause := errors.StatusCause(err, metav1.CauseTypeFieldValueInvalid)
			if isStatusCause && cause.Field == "spec.selector" {
				// recreate deployment
				// TODO: check for in-progress backup/restore to wait for it to finish
				log.Info("Found immutable selector from previous daemonset, recreating NodeAgent daemonset")
				err := r.Delete(r.Context, ds)
				if err != nil {
					return false, err
				}
				return r.ReconcileNodeAgentDaemonset(log)
			}
		}
		return false, err
	}

	if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
		// Trigger event to indicate NodeAgent was created or updated
		r.EventRecorder.Event(ds,
			corev1.EventTypeNormal,
			"NodeAgentDaemonsetReconciled",
			fmt.Sprintf("performed %s on NodeAgent deployment %s/%s", op, ds.Namespace, ds.Name),
		)
	}

	return true, nil
}

/**
 * This function builds NodeAgent Daemonset. It calls /pkg/credentials function AppendCloudProviderVolumes
 * args: velero - the velero object pointer
 * 		 ds		- pointer to daemonset with objectMeta defined
 * returns: (pointer to daemonset, nil) if successful
 */
func (r *DPAReconciler) buildNodeAgentDaemonset(ds *appsv1.DaemonSet) (*appsv1.DaemonSet, error) {
	dpa := r.dpa
	if dpa == nil {
		return nil, fmt.Errorf("dpa cannot be nil")
	}
	if ds == nil {
		return nil, fmt.Errorf("ds cannot be nil")
	}

	var nodeAgentResourceReqs corev1.ResourceRequirements

	// get resource requirements for nodeAgent ds
	// ignoring err here as it is checked in validator.go
	if dpa.Spec.Configuration.Restic != nil {
		nodeAgentResourceReqs, _ = getResticResourceReqs(dpa)
	} else if dpa.Spec.Configuration.NodeAgent != nil {
		nodeAgentResourceReqs, _ = getNodeAgentResourceReqs(dpa)
	} else {
		return nil, fmt.Errorf("NodeAgent or Restic configuration cannot be nil")
	}

	installDs := install.DaemonSet(ds.Namespace,
		install.WithResources(nodeAgentResourceReqs),
		install.WithImage(getVeleroImage(dpa)),
		install.WithAnnotations(dpa.Spec.PodAnnotations),
		install.WithSecret(false),
		install.WithServiceAccountName(common.Velero),
	)
	// Update Items in ObjectMeta
	dsName := ds.Name
	ds.TypeMeta = installDs.TypeMeta
	var err error
	ds.Labels, err = common.AppendUniqueKeyTOfTMaps(ds.Labels, installDs.Labels)
	if err != nil {
		return nil, fmt.Errorf("NodeAgent daemonset label: %s", err)
	}
	// Update Spec
	ds.Spec = installDs.Spec
	ds.Name = dsName

	return r.customizeNodeAgentDaemonset(ds)
}

func (r *DPAReconciler) customizeNodeAgentDaemonset(ds *appsv1.DaemonSet) (*appsv1.DaemonSet, error) {
	dpa := r.dpa
	if dpa.Spec.Configuration == nil || (dpa.Spec.Configuration.Restic == nil && dpa.Spec.Configuration.NodeAgent == nil) {
		// if restic and nodeAgent are not configured, therefore not enabled, return early.
		return nil, nil
	}

	var useResticConf bool = true

	if dpa.Spec.Configuration.NodeAgent != nil {
		useResticConf = false
	}

	// add custom pod labels
	var err error
	if useResticConf {
		if dpa.Spec.Configuration.Restic.PodConfig != nil && dpa.Spec.Configuration.Restic.PodConfig.Labels != nil {
			ds.Spec.Template.Labels, err = common.AppendUniqueKeyTOfTMaps(ds.Spec.Template.Labels, dpa.Spec.Configuration.Restic.PodConfig.Labels)
		}
	} else if dpa.Spec.Configuration.NodeAgent.PodConfig != nil && dpa.Spec.Configuration.NodeAgent.PodConfig.Labels != nil {
		ds.Spec.Template.Labels, err = common.AppendUniqueKeyTOfTMaps(ds.Spec.Template.Labels, dpa.Spec.Configuration.NodeAgent.PodConfig.Labels)
	}
	if err != nil {
		return nil, fmt.Errorf("NodeAgent daemonset template custom label: %s", err)
	}

	// customize specs
	ds.Spec.Selector = nodeAgentLabelSelector
	ds.Spec.UpdateStrategy = appsv1.DaemonSetUpdateStrategy{
		Type: appsv1.RollingUpdateDaemonSetStrategyType,
	}

	// customize template specs
	if useResticConf {
		ds.Spec.Template.Spec.SecurityContext = &corev1.PodSecurityContext{
			RunAsUser:          pointer.Int64(0),
			SupplementalGroups: dpa.Spec.Configuration.Restic.SupplementalGroups,
		}
	} else {
		ds.Spec.Template.Spec.SecurityContext = &corev1.PodSecurityContext{
			RunAsUser:          pointer.Int64(0),
			SupplementalGroups: dpa.Spec.Configuration.NodeAgent.SupplementalGroups,
		}
	}

	// append certs volume
	ds.Spec.Template.Spec.Volumes = append(ds.Spec.Template.Spec.Volumes,
		corev1.Volume{
			Name: "certs",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})

	// check platform type
	platformType, err := r.getPlatformType()
	if err != nil {
		return nil, fmt.Errorf("error checking platform type: %s", err)
	}
	// update nodeAgent host PV path
	for i, vol := range ds.Spec.Template.Spec.Volumes {
		if vol.Name == HostPods {
			ds.Spec.Template.Spec.Volumes[i].HostPath.Path = getFsPvHostPath(platformType)
		}
	}

	// update nodeAgent plugins host path
	for i, vol := range ds.Spec.Template.Spec.Volumes {
		if vol.Name == HostPlugins {
			ds.Spec.Template.Spec.Volumes[i].HostPath.Path = getPluginsHostPath(platformType)
		}
	}

	// Update with any pod config values
	if useResticConf {
		if dpa.Spec.Configuration.Restic.PodConfig != nil {
			ds.Spec.Template.Spec.Tolerations = dpa.Spec.Configuration.Restic.PodConfig.Tolerations
			ds.Spec.Template.Spec.NodeSelector = dpa.Spec.Configuration.Restic.PodConfig.NodeSelector
		}
	} else if dpa.Spec.Configuration.NodeAgent.PodConfig != nil {
		ds.Spec.Template.Spec.Tolerations = dpa.Spec.Configuration.NodeAgent.PodConfig.Tolerations
		ds.Spec.Template.Spec.NodeSelector = dpa.Spec.Configuration.NodeAgent.PodConfig.NodeSelector
	}

	// fetch nodeAgent container in order to customize it
	var nodeAgentContainer *corev1.Container
	for i, container := range ds.Spec.Template.Spec.Containers {
		if container.Name == common.NodeAgent {
			nodeAgentContainer = &ds.Spec.Template.Spec.Containers[i]

			// append certs volume mount
			nodeAgentContainer.VolumeMounts = append(nodeAgentContainer.VolumeMounts, corev1.VolumeMount{
				Name:      "certs",
				MountPath: "/etc/ssl/certs",
			})

			// update nodeAgent plugins volume mount host path
			for v, volumeMount := range nodeAgentContainer.VolumeMounts {
				if volumeMount.Name == HostPlugins {
					nodeAgentContainer.VolumeMounts[v].MountPath = getPluginsHostPath(platformType)
				}
			}
			// append PodConfig envs to nodeAgent container
			if useResticConf {
				if dpa.Spec.Configuration.Restic.PodConfig != nil && dpa.Spec.Configuration.Restic.PodConfig.Env != nil {
					nodeAgentContainer.Env = common.AppendUniqueEnvVars(nodeAgentContainer.Env, dpa.Spec.Configuration.Restic.PodConfig.Env)
				}
			} else if dpa.Spec.Configuration.NodeAgent.PodConfig != nil && dpa.Spec.Configuration.NodeAgent.PodConfig.Env != nil {
				nodeAgentContainer.Env = common.AppendUniqueEnvVars(nodeAgentContainer.Env, dpa.Spec.Configuration.NodeAgent.PodConfig.Env)
			}

			// append env vars to the nodeAgent container
			nodeAgentContainer.Env = common.AppendUniqueEnvVars(nodeAgentContainer.Env, proxy.ReadProxyVarsFromEnv())

			nodeAgentContainer.SecurityContext = &corev1.SecurityContext{
				Privileged: pointer.Bool(true),
			}

			imagePullPolicy, err := common.GetImagePullPolicy(dpa.Spec.ImagePullPolicy, getVeleroImage(dpa))
			if err != nil {
				r.Log.Error(err, "imagePullPolicy regex failed")
			}

			nodeAgentContainer.ImagePullPolicy = imagePullPolicy
			setContainerDefaults(nodeAgentContainer)

			if configMapName, ok := dpa.Annotations[common.UnsupportedNodeAgentServerArgsAnnotation]; ok {
				if configMapName != "" {
					unsupportedServerArgsCM := corev1.ConfigMap{}
					if err := r.Get(r.Context, types.NamespacedName{Namespace: dpa.Namespace, Name: configMapName}, &unsupportedServerArgsCM); err != nil {
						return nil, err
					}
					if err := common.ApplyUnsupportedServerArgsOverride(nodeAgentContainer, unsupportedServerArgsCM, common.NodeAgent); err != nil {
						return nil, err
					}
				}
			}

			break
		}
	}

	// attach DNS policy and config if enabled
	ds.Spec.Template.Spec.DNSPolicy = dpa.Spec.PodDnsPolicy
	if !reflect.DeepEqual(dpa.Spec.PodDnsConfig, corev1.PodDNSConfig{}) {
		ds.Spec.Template.Spec.DNSConfig = &dpa.Spec.PodDnsConfig
	}

	providerNeedsDefaultCreds, hasCloudStorage, err := r.noDefaultCredentials()
	if err != nil {
		return nil, err
	}

	if err := credentials.AppendCloudProviderVolumes(dpa, ds, providerNeedsDefaultCreds, hasCloudStorage); err != nil {
		return nil, err
	}
	setPodTemplateSpecDefaults(&ds.Spec.Template)
	if ds.Spec.UpdateStrategy.Type == appsv1.RollingUpdateDaemonSetStrategyType {
		ds.Spec.UpdateStrategy.RollingUpdate = &appsv1.RollingUpdateDaemonSet{
			MaxUnavailable: &intstr.IntOrString{
				Type:   intstr.Int,
				IntVal: 1,
			},
			MaxSurge: &intstr.IntOrString{
				Type:   intstr.Int,
				IntVal: 0,
			},
		}
	}
	if ds.Spec.RevisionHistoryLimit == nil {
		ds.Spec.RevisionHistoryLimit = pointer.Int32(10)
	}

	return ds, nil
}

func (r *DPAReconciler) ReconcileFsRestoreHelperConfig(log logr.Logger) (bool, error) {
	fsRestoreHelperCM := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      FsRestoreHelperCM,
			Namespace: r.NamespacedName.Namespace,
		},
	}

	// Delete renamed CM restic-restore-action-config
	// Velero uses labels to identify the CM. For consistency we have the
	// same name as upstream, whch is `fs-restore-action-config`
	resticRestoreHelperCM := corev1.ConfigMap{}
	if err := r.Get(r.Context, types.NamespacedName{Namespace: r.NamespacedName.Namespace, Name: ResticRestoreHelperCM}, &resticRestoreHelperCM); err == nil {
		r.Log.Info("Deleting deprecated ConfigMap restic-restore-action-config.")
		if err := r.Delete(r.Context, &resticRestoreHelperCM); err != nil {
			return false, err
		}
	}

	op, err := controllerutil.CreateOrPatch(r.Context, r.Client, &fsRestoreHelperCM, func() error {

		// update the Config Map
		err := r.updateFsRestoreHelperCM(&fsRestoreHelperCM)
		return err
	})

	if err != nil {
		return false, err
	}

	//TODO: Review FS Restore Helper CM status and report errors and conditions

	if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
		// Trigger event to indicate FS Restore Helper CM was created or updated
		r.EventRecorder.Event(&fsRestoreHelperCM,
			corev1.EventTypeNormal,
			"ReconcileFsRestoreHelperConfigReconciled",
			fmt.Sprintf("performed %s on FS restore Helper config map %s/%s", op, fsRestoreHelperCM.Namespace, fsRestoreHelperCM.Name),
		)
	}
	return true, nil
}

func (r *DPAReconciler) updateFsRestoreHelperCM(fsRestoreHelperCM *corev1.ConfigMap) error {

	// Setting controller owner reference on the FS restore helper CM
	err := controllerutil.SetControllerReference(r.dpa, fsRestoreHelperCM, r.Scheme)
	if err != nil {
		return err
	}

	fsRestoreHelperCM.Labels = map[string]string{
		"velero.io/plugin-config":      "",
		"velero.io/pod-volume-restore": "RestoreItemAction",
		oadpv1alpha1.OadpOperatorLabel: "True",
	}

	fsRestoreHelperCM.Data = map[string]string{
		"image": os.Getenv("RELATED_IMAGE_VELERO_RESTORE_HELPER"),
	}

	return nil
}

// getPlatformType fetches the cluster infrastructure object and returns the platform type.
func (r *DPAReconciler) getPlatformType() (string, error) {
	infra := &configv1.Infrastructure{}
	key := types.NamespacedName{Name: Cluster}
	if err := r.Get(r.Context, key, infra); err != nil {
		return "", err
	}

	if platformStatus := infra.Status.PlatformStatus; platformStatus != nil {
		if platformType := platformStatus.Type; platformType != "" {
			return string(platformType), nil
		}
	}
	return "", nil
}
