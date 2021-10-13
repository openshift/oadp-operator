package controllers

import (
	"context"
	"fmt"
	"os"
	"reflect"

	"github.com/openshift/oadp-operator/pkg/common"
	"github.com/vmware-tanzu/velero/pkg/install"

	"github.com/go-logr/logr"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/credentials"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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

func getResticObjectMeta(r *VeleroReconciler) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      Restic,
		Namespace: r.NamespacedName.Namespace,
		Labels: map[string]string{
			"component": "velero",
		},
	}
}

func (r *VeleroReconciler) ReconcileResticDaemonset(log logr.Logger) (bool, error) {
	velero := oadpv1alpha1.Velero{}
	if err := r.Get(r.Context, r.NamespacedName, &velero); err != nil {
		return false, err
	}

	// Define "static" portion of daemonset
	ds := &appsv1.DaemonSet{
		ObjectMeta: getResticObjectMeta(r),
	}
	if velero.Spec.EnableRestic != nil && !*velero.Spec.EnableRestic {
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
		// If velero Spec enableRestic exists and is false, attempt to delete.
		deleteOptionPropagationForeground := metav1.DeletePropagationForeground
		if err := r.Delete(deleteContext, ds, &client.DeleteOptions{PropagationPolicy: &deleteOptionPropagationForeground}); err != nil {
			// TODO: Come back and fix event recording to be consistent
			r.EventRecorder.Event(ds, corev1.EventTypeNormal, "DeleteDaemonSetFailed", "Got DaemonSet to delete but could not delete err:"+err.Error())
			return false, err
		}
		r.EventRecorder.Event(ds, corev1.EventTypeNormal, "DeletedDaemonSet", "DaemonSet deleted")

		return true, nil
	}

	op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, ds, func() error {
		// Deployment selector is immutable so we set this value only if
		// a new object is going to be created
		if ds.ObjectMeta.CreationTimestamp.IsZero() {
			ds.Spec.Selector = resticLabelSelector
		}

		if err := controllerutil.SetControllerReference(&velero, ds, r.Scheme); err != nil {
			return err
		}

		if _, err := r.buildResticDaemonset(&velero, ds); err != nil {
			return err
		}
		return nil
	})

	if err != nil {
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
func (r *VeleroReconciler) buildResticDaemonset(velero *oadpv1alpha1.Velero, ds *appsv1.DaemonSet) (*appsv1.DaemonSet, error) {
	if velero == nil {
		return nil, fmt.Errorf("velero cannot be nil")
	}
	if ds == nil {
		return nil, fmt.Errorf("ds cannot be nil")
	}

	resticDaemonSetName := ds.Name
	ownerRefs := ds.OwnerReferences

	*ds = *install.DaemonSet(ds.Namespace,
		install.WithResources(r.getVeleroResourceReqs(velero)),
		install.WithImage(getVeleroImage(velero)),
		install.WithAnnotations(velero.Spec.PodAnnotations),
		install.WithSecret(false))

	ds.Name = resticDaemonSetName
	ds.OwnerReferences = ownerRefs
	return r.customizeResticDaemonset(velero, ds)
}

func (r *VeleroReconciler) customizeResticDaemonset(velero *oadpv1alpha1.Velero, ds *appsv1.DaemonSet) (*appsv1.DaemonSet, error) {

	// customize specs
	ds.Spec.Selector = resticLabelSelector
	ds.Spec.UpdateStrategy = appsv1.DaemonSetUpdateStrategy{
		Type: appsv1.RollingUpdateDaemonSetStrategyType,
	}

	// customize template specs
	ds.Spec.Template.Spec.NodeSelector = velero.Spec.ResticNodeSelector

	ds.Spec.Template.Spec.SecurityContext = &corev1.PodSecurityContext{
		RunAsUser:          pointer.Int64(0),
		SupplementalGroups: velero.Spec.ResticSupplementalGroups,
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

	ds.Spec.Template.Spec.Tolerations = velero.Spec.VeleroTolerations

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

		// append env vars to the restic container
		resticContainer.Env = append(resticContainer.Env,
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

		resticContainer.SecurityContext = &corev1.SecurityContext{
			Privileged: pointer.Bool(true),
		}

		resticContainer.ImagePullPolicy = corev1.PullAlways
	}

	// attach DNS policy and config if enabled
	ds.Spec.Template.Spec.DNSPolicy = velero.Spec.PodDnsPolicy
	if !reflect.DeepEqual(velero.Spec.PodDnsConfig, corev1.PodDNSConfig{}) {
		ds.Spec.Template.Spec.DNSConfig = &velero.Spec.PodDnsConfig
	}

	if err := credentials.AppendCloudProviderVolumes(velero, ds); err != nil {
		return nil, err
	}
	return ds, nil
}

func (r *VeleroReconciler) ReconcileResticRestoreHelperConfig(log logr.Logger) (bool, error) {
	velero := oadpv1alpha1.Velero{}
	if err := r.Get(r.Context, r.NamespacedName, &velero); err != nil {
		return false, err
	}

	resticRestoreHelperCM := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ResticRestoreHelperCM,
			Namespace: r.NamespacedName.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, &resticRestoreHelperCM, func() error {

		// update the Config Map
		err := r.updateResticRestoreHelperCM(&resticRestoreHelperCM, &velero)
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

func (r *VeleroReconciler) updateResticRestoreHelperCM(resticRestoreHelperCM *corev1.ConfigMap, velero *oadpv1alpha1.Velero) error {

	// Setting controller owner reference on the restic restore helper CM
	err := controllerutil.SetControllerReference(velero, resticRestoreHelperCM, r.Scheme)
	if err != nil {
		return err
	}

	resticRestoreHelperCM.Labels = map[string]string{
		"velero.io/plugin-config":      "",
		"velero.io/restic":             "RestoreItemAction",
		oadpv1alpha1.OadpOperatorLabel: "True",
	}

	resticRestoreHelperCM.Data = map[string]string{
		"image": fmt.Sprintf("%v/%v/%v:%v", os.Getenv("REGISTRY"), os.Getenv("PROJECT"), os.Getenv("VELERO_RESTIC_RESTORE_HELPER_REPO"), os.Getenv("VELERO_RESTIC_RESTORE_HELPER_TAG")),
	}

	return nil
}
