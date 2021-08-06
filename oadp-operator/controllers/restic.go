package controllers

import (
	"fmt"
	"os"

	"github.com/go-logr/logr"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/common"
	"github.com/openshift/oadp-operator/pkg/credentials"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	Restic = "restic"
)

var (
	resticPvHostPath string = getResticPvHostPath()

	// v1.MountPropagationHostToContainer is a const. Const cannot be pointed to.
	// we need to declare mountPropagationToHostContainer so that we have an address to point to
	// for ds.Spec.Template.Spec.Volumes[].Containers[].VolumeMounts[].MountPropagation
	mountPropagationToHostContainer = v1.MountPropagationHostToContainer
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

		if err := r.Get(r.Context, r.NamespacedName, ds); err == nil {
			// no errors means there already is an existing DaeMonset.
			// TODO: Check if restic is in use, a backup is running, so don't blindly delete restic.
			// If velero Spec enableRestic exists and is false, attempt to delete.
			if err := r.Delete(r.Context, ds); err != nil {
				return false, err
			}
		}
		return true, nil
	}

	op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, ds, func() error {
		// Deployment selector is immutable so we set this value only if
		// a new object is going to be created
		if ds.ObjectMeta.CreationTimestamp.IsZero() {
			ds.Spec.Selector = &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"component": Restic,
				},
			}
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
			v1.EventTypeNormal,
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
	ds.Spec = appsv1.DaemonSetSpec{
		UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
			Type: appsv1.RollingUpdateDaemonSetStrategyType,
		},
		Template: v1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"name": Restic,
				},
			},
			Spec: v1.PodSpec{
				NodeSelector:       velero.Spec.ResticNodeSelector,
				ServiceAccountName: common.Velero,
				SecurityContext: &v1.PodSecurityContext{
					RunAsUser:          pointer.Int64(0),
					SupplementalGroups: velero.Spec.ResticSupplementalGroups,
				},
				Volumes: []v1.Volume{
					// Cloud Provider volumes are dynamically added in the for loop below
					{
						Name: "host-pods",
						VolumeSource: v1.VolumeSource{
							HostPath: &v1.HostPathVolumeSource{
								Path: resticPvHostPath,
							},
						},
					},
					{
						Name: "scratch",
						VolumeSource: v1.VolumeSource{
							EmptyDir: &v1.EmptyDirVolumeSource{},
						},
					},
					{
						Name: "certs",
						VolumeSource: v1.VolumeSource{
							EmptyDir: &v1.EmptyDirVolumeSource{},
						},
					},
				},
				Tolerations: velero.Spec.ResticTolerations,
				Containers: []v1.Container{
					{
						Name: common.Velero,
						SecurityContext: &v1.SecurityContext{
							Privileged: pointer.Bool(true),
						},
						Image:           getResticImage(),
						ImagePullPolicy: v1.PullAlways,
						Resources:       r.getVeleroResourceReqs(velero), //setting default.
						Command: []string{
							"/velero",
						},
						Args: []string{
							"restic",
							"server",
						},
						VolumeMounts: []v1.VolumeMount{
							{
								Name:             "host-pods",
								MountPath:        "/host_pods",
								MountPropagation: &mountPropagationToHostContainer,
							},
							{
								Name:      "scratch",
								MountPath: "/scratch",
							},
							{
								Name:      "certs",
								MountPath: "/etc/ssl/certs",
							},
						},
						Env: []v1.EnvVar{
							{
								Name:  "HTTP_PROXY",
								Value: os.Getenv("HTTP_PROXY"),
							},
							{
								Name:  "HTTPS_PROXY",
								Value: os.Getenv("HTTPS_PROXY"),
							},
							{
								Name:  "NO_PROXY",
								Value: os.Getenv("NO_PROXY"),
							},
							{
								Name: "NODE_NAME",
								ValueFrom: &v1.EnvVarSource{
									FieldRef: &v1.ObjectFieldSelector{
										FieldPath: "spec.nodeName",
									},
								},
							},
							{
								Name: "POD_NAME",
								ValueFrom: &v1.EnvVarSource{
									FieldRef: &v1.ObjectFieldSelector{
										FieldPath: "metadata.name"},
								},
							},
							{
								Name: "VELERO_NAMESPACE",
								ValueFrom: &v1.EnvVarSource{
									FieldRef: &v1.ObjectFieldSelector{
										FieldPath: "metadata.namespace",
									},
								},
							},
							{
								Name: "VELERO_SCRATCH_DIR",
								ValueFrom: &v1.EnvVarSource{
									FieldRef: &v1.ObjectFieldSelector{
										FieldPath: "/scratch",
									},
								},
							},
						},
					},
				},
			},
		},
	}
	if err := credentials.AppendCloudProviderVolumes(velero, ds); err != nil {
		return nil, err
	}
	return ds, nil
}

func getResticImage() string {
	return fmt.Sprintf("%v/%v/%v:%v", os.Getenv("REGISTRY"), os.Getenv("PROJECT"), os.Getenv("VELERO_REPO"), os.Getenv("VELERO_TAG"))
}
