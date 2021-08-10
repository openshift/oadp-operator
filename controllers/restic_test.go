package controllers

import (
	"context"
	"os"
	"reflect"
	"testing"

	"github.com/go-logr/logr"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/common"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestVeleroReconciler_ReconcileResticDaemonset(t *testing.T) {
	type fields struct {
		Client         client.Client
		Scheme         *runtime.Scheme
		Log            logr.Logger
		Context        context.Context
		NamespacedName types.NamespacedName
		EventRecorder  record.EventRecorder
	}
	type args struct {
		log logr.Logger
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    bool
		wantErr bool
	}{
		//TODO: Add tests
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &VeleroReconciler{
				Client:         tt.fields.Client,
				Scheme:         tt.fields.Scheme,
				Log:            tt.fields.Log,
				Context:        tt.fields.Context,
				NamespacedName: tt.fields.NamespacedName,
				EventRecorder:  tt.fields.EventRecorder,
			}
			got, err := r.ReconcileResticDaemonset(tt.args.log)
			if (err != nil) != tt.wantErr {
				t.Errorf("VeleroReconciler.ReconcileResticDaemonset() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("VeleroReconciler.ReconcileResticDaemonset() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVeleroReconciler_buildResticDaemonset(t *testing.T) {
	type fields struct {
		Client         client.Client
		Scheme         *runtime.Scheme
		Log            logr.Logger
		Context        context.Context
		NamespacedName types.NamespacedName
		EventRecorder  record.EventRecorder
	}
	type args struct {
		velero *oadpv1alpha1.Velero
		ds     *appsv1.DaemonSet
	}
	r := &VeleroReconciler{}
	velero := oadpv1alpha1.Velero{}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *appsv1.DaemonSet
		wantErr bool
	}{
		{
			name:   "velero is nil",
			fields: fields{NamespacedName: types.NamespacedName{Namespace: "velero"}},
			args: args{
				nil, &appsv1.DaemonSet{},
			},
			wantErr: true,
			want:    nil,
		},
		{
			name: "DaemonSet is nil",
			args: args{
				&oadpv1alpha1.Velero{}, nil,
			},
			wantErr: true,
			want:    nil,
		},
		{
			name: "Valid velero and daemonset",
			args: args{
				&oadpv1alpha1.Velero{}, &appsv1.DaemonSet{
					ObjectMeta: getResticObjectMeta(r),
				},
			},
			wantErr: false,
			want: &appsv1.DaemonSet{
				ObjectMeta: getResticObjectMeta(r),
				Spec: appsv1.DaemonSetSpec{
					UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
						Type: appsv1.RollingUpdateDaemonSetStrategyType,
					},
					Template: v1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"component": Restic,
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
									Resources:       r.getVeleroResourceReqs(&velero), //setting default.
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
											Name:  "VELERO_SCRATCH_DIR",
											Value: "/scratch",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &VeleroReconciler{
				Client:         tt.fields.Client,
				Scheme:         tt.fields.Scheme,
				Log:            tt.fields.Log,
				Context:        tt.fields.Context,
				NamespacedName: tt.fields.NamespacedName,
				EventRecorder:  tt.fields.EventRecorder,
			}
			got, err := r.buildResticDaemonset(tt.args.velero, tt.args.ds)
			if (err != nil) != tt.wantErr {
				t.Errorf("VeleroReconciler.buildResticDaemonset() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("VeleroReconciler.buildResticDaemonset() = %v, want %v", got, tt.want)
			}
		})
	}
}
