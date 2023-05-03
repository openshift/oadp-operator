package controllers

import (
	"context"
	"fmt"
	"os"
	"reflect"

	"github.com/openshift/oadp-operator/pkg/common"
	"github.com/operator-framework/operator-lib/proxy"
	"github.com/vmware-tanzu/velero/pkg/install"

	"github.com/go-logr/logr"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/credentials"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	Restic                = "restic"
	ResticRestoreHelperCM = "restic-restore-action-config"
	HostPods              = "host-pods"
)

var (
	resticPvHostPath string = getResticPvHostPath()

	// v1.MountPropagationHostToContainer is a const. Const cannot be pointed to.
	// we need to declare mountPropagationToHostContainer so that we have an address to point to
	// for ds.Spec.Template.Spec.Volumes[].Containers[].VolumeMounts[].MountPropagation
	mountPropagationToHostContainer = corev1.MountPropagationHostToContainer
	resticLabelSelector             = &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"component": common.Velero,
			"name":      common.Restic,
		},
	}
)

func getResticPvHostPath() string {
	env := os.Getenv("RESTIC_PV_HOSTPATH")
	if env == "" {
		return "/var/lib/kubelet/pods"
	}
	return env
}

func getResticObjectMeta(r *DPAReconciler) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      Restic,
		Namespace: r.NamespacedName.Namespace,
		Labels: map[string]string{
			"component": "velero",
		},
	}
}

func (r *DPAReconciler) ReconcileResticDaemonset(log logr.Logger) (bool, error) {
	dpa := oadpv1alpha1.DataProtectionApplication{}
	if err := r.Get(r.Context, r.NamespacedName, &dpa); err != nil {
		return false, err
	}

	// Define "static" portion of daemonset
	ds := &appsv1.DaemonSet{
		ObjectMeta: getResticObjectMeta(r),
	}
	if dpa.Spec.Configuration.Restic == nil || dpa.Spec.Configuration.Restic != nil && (dpa.Spec.Configuration.Restic.Enable == nil || !*dpa.Spec.Configuration.Restic.Enable) {
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
		// TODO: Check if restic is in use, a backup is running, so don't blindly delete restic.
		// If dpa.Spec enableRestic exists and is false, attempt to delete.
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
			ds.Spec.Selector.MatchLabels, err = common.AppendUniqueKeyTOfTMaps(ds.Spec.Selector.MatchLabels, resticLabelSelector.MatchLabels)
			if err != nil {
				return fmt.Errorf("failed to append labels to selector: %s", err)
			}
		}

		if _, err := r.buildResticDaemonset(&dpa, ds); err != nil {
			return err
		}
		if err := controllerutil.SetControllerReference(&dpa, ds, r.Scheme); err != nil {
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
				log.Info("Found immutable selector from previous daemonset, recreating restic daemonset")
				err := r.Delete(r.Context, ds)
				if err != nil {
					return false, err
				}
				return r.ReconcileResticDaemonset(log)
			}
		}
		return false, err
	}

	if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
		// Trigger event to indicate restic was created or updated
		r.EventRecorder.Event(ds,
			corev1.EventTypeNormal,
			"ResticDaemonsetReconciled",
			fmt.Sprintf("performed %s on restic deployment %s/%s", op, ds.Namespace, ds.Name),
		)
	}

	return true, nil
}

/**
 * This function builds restic Daemonset. It calls /pkg/credentials function AppendCloudProviderVolumes
 * args: velero - the velero object pointer
 * 		 ds		- pointer to daemonset with objectMeta defined
 * returns: (pointer to daemonset, nil) if successful
 */
func (r *DPAReconciler) buildResticDaemonset(dpa *oadpv1alpha1.DataProtectionApplication, ds *appsv1.DaemonSet) (*appsv1.DaemonSet, error) {
	if dpa == nil {
		return nil, fmt.Errorf("dpa cannot be nil")
	}
	if ds == nil {
		return nil, fmt.Errorf("ds cannot be nil")
	}

	// get resource requirements for restic ds
	// ignoring err here as it is checked in validator.go
	resticResourceReqs, _ := getResticResourceReqs(dpa)

	installDs := install.DaemonSet(ds.Namespace,
		install.WithResources(resticResourceReqs),
		install.WithImage(getVeleroImage(dpa)),
		install.WithAnnotations(dpa.Spec.PodAnnotations),
		install.WithSecret(false))
	// Update Items in ObjectMeta
	dsName := ds.Name
	ds.TypeMeta = installDs.TypeMeta
	var err error
	ds.Labels, err = common.AppendUniqueKeyTOfTMaps(ds.Labels, installDs.Labels)
	if err != nil {
		return nil, fmt.Errorf("restic daemonset label: %s", err)
	}
	// Update Spec
	ds.Spec = installDs.Spec
	ds.Name = dsName

	return r.customizeResticDaemonset(dpa, ds)
}

func (r *DPAReconciler) customizeResticDaemonset(dpa *oadpv1alpha1.DataProtectionApplication, ds *appsv1.DaemonSet) (*appsv1.DaemonSet, error) {
	if dpa.Spec.Configuration.Restic == nil {
		// if restic is not configured, therefore not enabled, return early.
		return nil, nil
	}
	// add custom pod labels
	if dpa.Spec.Configuration.Restic.PodConfig != nil && dpa.Spec.Configuration.Restic.PodConfig.Labels != nil {
		var err error
		ds.Spec.Template.Labels, err = common.AppendUniqueKeyTOfTMaps(ds.Spec.Template.Labels, dpa.Spec.Configuration.Restic.PodConfig.Labels)
		if err != nil {
			return nil, fmt.Errorf("restic daemonset template custom label: %s", err)
		}
	}
	// customize specs
	ds.Spec.Selector = resticLabelSelector
	ds.Spec.UpdateStrategy = appsv1.DaemonSetUpdateStrategy{
		Type: appsv1.RollingUpdateDaemonSetStrategyType,
	}

	// customize template specs
	ds.Spec.Template.Spec.SecurityContext = &corev1.PodSecurityContext{
		RunAsUser:          pointer.Int64(0),
		SupplementalGroups: dpa.Spec.Configuration.Restic.SupplementalGroups,
	}

	// append certs volume
	ds.Spec.Template.Spec.Volumes = append(ds.Spec.Template.Spec.Volumes,
		corev1.Volume{
			Name: "certs",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})

	// update restic host PV path
	for i, vol := range ds.Spec.Template.Spec.Volumes {
		if vol.Name == HostPods {
			ds.Spec.Template.Spec.Volumes[i].HostPath.Path = getResticPvHostPath()
		}
	}

	// Update with any pod config values
	if dpa.Spec.Configuration.Restic.PodConfig != nil {
		ds.Spec.Template.Spec.Tolerations = dpa.Spec.Configuration.Restic.PodConfig.Tolerations
		ds.Spec.Template.Spec.NodeSelector = dpa.Spec.Configuration.Restic.PodConfig.NodeSelector
	}

	// fetch restic container in order to customize it
	var resticContainer *corev1.Container
	for i, container := range ds.Spec.Template.Spec.Containers {
		if container.Name == Restic {
			resticContainer = &ds.Spec.Template.Spec.Containers[i]
			break
		}
	}

	if resticContainer != nil {
		// append certs volume mount
		resticContainer.VolumeMounts = append(resticContainer.VolumeMounts, corev1.VolumeMount{
			Name:      "certs",
			MountPath: "/etc/ssl/certs",
		})
		// append restic PodConfig envs to container
		if dpa.Spec.Configuration != nil && dpa.Spec.Configuration.Restic != nil && dpa.Spec.Configuration.Restic.PodConfig != nil && dpa.Spec.Configuration.Restic.PodConfig.Env != nil {
			resticContainer.Env = common.AppendUniqueEnvVars(resticContainer.Env, dpa.Spec.Configuration.Restic.PodConfig.Env)
		}
		// append env vars to the restic container
		resticContainer.Env = common.AppendUniqueEnvVars(resticContainer.Env, proxy.ReadProxyVarsFromEnv())

		resticContainer.SecurityContext = &corev1.SecurityContext{
			Privileged: pointer.Bool(true),
		}

		resticContainer.ImagePullPolicy = corev1.PullAlways
		setContainerDefaults(resticContainer)
	}

	// attach DNS policy and config if enabled
	ds.Spec.Template.Spec.DNSPolicy = dpa.Spec.PodDnsPolicy
	if !reflect.DeepEqual(dpa.Spec.PodDnsConfig, corev1.PodDNSConfig{}) {
		ds.Spec.Template.Spec.DNSConfig = &dpa.Spec.PodDnsConfig
	}

	providerNeedsDefaultCreds, hasCloudStorage, err := r.noDefaultCredentials(*dpa)
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

func (r *DPAReconciler) ReconcileResticRestoreHelperConfig(log logr.Logger) (bool, error) {
	dpa := oadpv1alpha1.DataProtectionApplication{}
	if err := r.Get(r.Context, r.NamespacedName, &dpa); err != nil {
		return false, err
	}

	resticRestoreHelperCM := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ResticRestoreHelperCM,
			Namespace: r.NamespacedName.Namespace,
		},
	}

	op, err := controllerutil.CreateOrPatch(r.Context, r.Client, &resticRestoreHelperCM, func() error {

		// update the Config Map
		err := r.updateResticRestoreHelperCM(&resticRestoreHelperCM, &dpa)
		return err
	})

	if err != nil {
		return false, err
	}

	//TODO: Review Restic Restore Helper CM status and report errors and conditions

	if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
		// Trigger event to indicate Restic Restore Helper CM was created or updated
		r.EventRecorder.Event(&resticRestoreHelperCM,
			corev1.EventTypeNormal,
			"ReconcileResticRestoreHelperConfigReconciled",
			fmt.Sprintf("performed %s on restic restore Helper config map %s/%s", op, resticRestoreHelperCM.Namespace, resticRestoreHelperCM.Name),
		)
	}
	return true, nil
}

func (r *DPAReconciler) updateResticRestoreHelperCM(resticRestoreHelperCM *corev1.ConfigMap, dpa *oadpv1alpha1.DataProtectionApplication) error {

	// Setting controller owner reference on the restic restore helper CM
	err := controllerutil.SetControllerReference(dpa, resticRestoreHelperCM, r.Scheme)
	if err != nil {
		return err
	}

	resticRestoreHelperCM.Labels = map[string]string{
		"velero.io/plugin-config":      "",
		"velero.io/restic":             "RestoreItemAction",
		oadpv1alpha1.OadpOperatorLabel: "True",
	}

	resticRestoreHelperCM.Data = map[string]string{
		"image": os.Getenv("RELATED_IMAGE_VELERO_RESTIC_RESTORE_HELPER"),
	}

	return nil
}
