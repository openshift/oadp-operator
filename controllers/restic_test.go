package controllers

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"testing"

	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"

	"github.com/go-logr/logr"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/common"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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
				TypeMeta: metav1.TypeMeta{
					Kind:       "DaemonSet",
					APIVersion: appsv1.SchemeGroupVersion.String(),
				},
				Spec: appsv1.DaemonSetSpec{
					UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
						Type: appsv1.RollingUpdateDaemonSetStrategyType,
					},
					Selector: resticLabelSelector,
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"component": common.Velero,
								"name":      common.Restic,
							},
						},
						Spec: corev1.PodSpec{
							NodeSelector:       velero.Spec.ResticNodeSelector,
							ServiceAccountName: common.Velero,
							SecurityContext: &corev1.PodSecurityContext{
								RunAsUser:          pointer.Int64(0),
								SupplementalGroups: velero.Spec.ResticSupplementalGroups,
							},
							Volumes: []corev1.Volume{
								// Cloud Provider volumes are dynamically added in the for loop below
								{
									Name: HostPods,
									VolumeSource: corev1.VolumeSource{
										HostPath: &corev1.HostPathVolumeSource{
											Path: resticPvHostPath,
										},
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
							},
							Tolerations: velero.Spec.ResticTolerations,
							Containers: []corev1.Container{
								{
									Name: common.Restic,
									SecurityContext: &corev1.SecurityContext{
										Privileged: pointer.Bool(true),
									},
									Image:           getVeleroImage(&velero),
									ImagePullPolicy: v1.PullAlways,
									Resources:       r.getVeleroResourceReqs(&velero), //setting default.
									Command: []string{
										"/velero",
									},
									Args: []string{
										"restic",
										"server",
									},
									VolumeMounts: []corev1.VolumeMount{
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
									Env: []corev1.EnvVar{
										{
											Name: "NODE_NAME",
											ValueFrom: &corev1.EnvVarSource{
												FieldRef: &corev1.ObjectFieldSelector{
													FieldPath: "spec.nodeName",
												},
											},
										},
										{
											Name: "VELERO_NAMESPACE",
											ValueFrom: &corev1.EnvVarSource{
												FieldRef: &corev1.ObjectFieldSelector{
													FieldPath: "metadata.namespace",
												},
											},
										},
										{
											Name:  "VELERO_SCRATCH_DIR",
											Value: "/scratch",
										},
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
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Valid velero and daemonset for aws as bsl",
			args: args{
				&oadpv1alpha1.Velero{
					Spec: oadpv1alpha1.VeleroSpec{
						DefaultVeleroPlugins: []oadpv1alpha1.DefaultPlugin{
							oadpv1alpha1.DefaultPluginAWS,
						},
					},
				}, &appsv1.DaemonSet{
					ObjectMeta: getResticObjectMeta(r),
				},
			},
			wantErr: false,
			want: &appsv1.DaemonSet{
				ObjectMeta: getResticObjectMeta(r),
				TypeMeta: metav1.TypeMeta{
					Kind:       "DaemonSet",
					APIVersion: appsv1.SchemeGroupVersion.String(),
				},
				Spec: appsv1.DaemonSetSpec{
					UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
						Type: appsv1.RollingUpdateDaemonSetStrategyType,
					},
					Selector: resticLabelSelector,
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"component": common.Velero,
								"name":      common.Restic,
							},
						},
						Spec: corev1.PodSpec{
							NodeSelector:       velero.Spec.ResticNodeSelector,
							ServiceAccountName: common.Velero,
							SecurityContext: &corev1.PodSecurityContext{
								RunAsUser:          pointer.Int64(0),
								SupplementalGroups: velero.Spec.ResticSupplementalGroups,
							},
							Volumes: []corev1.Volume{
								// Cloud Provider volumes are dynamically added in the for loop below
								{
									Name: "host-pods",
									VolumeSource: corev1.VolumeSource{
										HostPath: &corev1.HostPathVolumeSource{
											Path: resticPvHostPath,
										},
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
								{
									Name: "cloud-credentials",
									VolumeSource: corev1.VolumeSource{
										Secret: &corev1.SecretVolumeSource{
											SecretName: "cloud-credentials",
										},
									},
								},
							},
							Tolerations: velero.Spec.ResticTolerations,
							Containers: []corev1.Container{
								{
									Name: common.Restic,
									SecurityContext: &corev1.SecurityContext{
										Privileged: pointer.Bool(true),
									},
									Image:           getVeleroImage(&velero),
									ImagePullPolicy: corev1.PullAlways,
									Resources:       r.getVeleroResourceReqs(&velero), //setting default.
									Command: []string{
										"/velero",
									},
									Args: []string{
										"restic",
										"server",
									},
									VolumeMounts: []corev1.VolumeMount{
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
										{
											Name:      "cloud-credentials",
											MountPath: "/credentials",
										},
									},
									Env: []corev1.EnvVar{
										{
											Name: "NODE_NAME",
											ValueFrom: &corev1.EnvVarSource{
												FieldRef: &corev1.ObjectFieldSelector{
													FieldPath: "spec.nodeName",
												},
											},
										},
										{
											Name: "VELERO_NAMESPACE",
											ValueFrom: &corev1.EnvVarSource{
												FieldRef: &corev1.ObjectFieldSelector{
													FieldPath: "metadata.namespace",
												},
											},
										},
										{
											Name:  "VELERO_SCRATCH_DIR",
											Value: "/scratch",
										},
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
											Name:  common.AWSSharedCredentialsFileEnvKey,
											Value: "/credentials/cloud",
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Valid velero with annotation and daemonset for aws as bsl with default secret name",
			args: args{
				&oadpv1alpha1.Velero{
					Spec: oadpv1alpha1.VeleroSpec{
						DefaultVeleroPlugins: []oadpv1alpha1.DefaultPlugin{
							oadpv1alpha1.DefaultPluginAWS,
						},
						BackupStorageLocations: []velerov1.BackupStorageLocationSpec{
							{
								Provider: AWSProvider,
								StorageType: velerov1.StorageType{
									ObjectStorage: &velerov1.ObjectStorageLocation{
										Bucket: "aws-bucket",
									},
								},
								Config: map[string]string{
									Region:                "aws-region",
									S3URL:                 "https://sr-url-aws-domain.com",
									InsecureSkipTLSVerify: "false",
								},
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cloud-credentials",
									},
								},
							},
						},
						PodAnnotations: map[string]string{
							"test-annotation": "awesome annotation",
						},
					},
				}, &appsv1.DaemonSet{
					ObjectMeta: getResticObjectMeta(r),
				},
			},
			wantErr: false,
			want: &appsv1.DaemonSet{
				ObjectMeta: getResticObjectMeta(r),
				TypeMeta: metav1.TypeMeta{
					Kind:       "DaemonSet",
					APIVersion: appsv1.SchemeGroupVersion.String(),
				},
				Spec: appsv1.DaemonSetSpec{
					UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
						Type: appsv1.RollingUpdateDaemonSetStrategyType,
					},
					Selector: resticLabelSelector,
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"component": common.Velero,
								"name":      common.Restic,
							},
							Annotations: map[string]string{
								"test-annotation": "awesome annotation",
							},
						},
						Spec: corev1.PodSpec{
							NodeSelector:       velero.Spec.ResticNodeSelector,
							ServiceAccountName: common.Velero,
							SecurityContext: &corev1.PodSecurityContext{
								RunAsUser:          pointer.Int64(0),
								SupplementalGroups: velero.Spec.ResticSupplementalGroups,
							},
							Volumes: []corev1.Volume{
								// Cloud Provider volumes are dynamically added in the for loop below
								{
									Name: HostPods,
									VolumeSource: corev1.VolumeSource{
										HostPath: &corev1.HostPathVolumeSource{
											Path: resticPvHostPath,
										},
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
								{
									Name: "cloud-credentials",
									VolumeSource: corev1.VolumeSource{
										Secret: &corev1.SecretVolumeSource{
											SecretName: "cloud-credentials",
										},
									},
								},
							},
							Tolerations: velero.Spec.ResticTolerations,
							Containers: []corev1.Container{
								{
									Name: common.Restic,
									SecurityContext: &corev1.SecurityContext{
										Privileged: pointer.Bool(true),
									},
									Image:           getVeleroImage(&velero),
									ImagePullPolicy: corev1.PullAlways,
									Resources:       r.getVeleroResourceReqs(&velero), //setting default.
									Command: []string{
										"/velero",
									},
									Args: []string{
										"restic",
										"server",
									},
									VolumeMounts: []corev1.VolumeMount{
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
										{
											Name:      "cloud-credentials",
											MountPath: "/credentials",
										},
									},
									Env: []corev1.EnvVar{
										{
											Name: "NODE_NAME",
											ValueFrom: &corev1.EnvVarSource{
												FieldRef: &corev1.ObjectFieldSelector{
													FieldPath: "spec.nodeName",
												},
											},
										},
										{
											Name: "VELERO_NAMESPACE",
											ValueFrom: &corev1.EnvVarSource{
												FieldRef: &corev1.ObjectFieldSelector{
													FieldPath: "metadata.namespace",
												},
											},
										},
										{
											Name:  "VELERO_SCRATCH_DIR",
											Value: "/scratch",
										},
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
											Name:  common.AWSSharedCredentialsFileEnvKey,
											Value: "/credentials/cloud",
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Valid velero with DNS Policy/Config with annotation and daemonset for aws as bsl with default secret name",
			args: args{
				&oadpv1alpha1.Velero{
					Spec: oadpv1alpha1.VeleroSpec{
						DefaultVeleroPlugins: []oadpv1alpha1.DefaultPlugin{
							oadpv1alpha1.DefaultPluginAWS,
						},
						BackupStorageLocations: []velerov1.BackupStorageLocationSpec{
							{
								Provider: AWSProvider,
								StorageType: velerov1.StorageType{
									ObjectStorage: &velerov1.ObjectStorageLocation{
										Bucket: "aws-bucket",
									},
								},
								Config: map[string]string{
									Region:                "aws-region",
									S3URL:                 "https://sr-url-aws-domain.com",
									InsecureSkipTLSVerify: "false",
								},
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cloud-credentials",
									},
								},
							},
						},
						PodAnnotations: map[string]string{
							"test-annotation": "awesome annotation",
						},
						PodDnsPolicy: "None",
						PodDnsConfig: corev1.PodDNSConfig{
							Nameservers: []string{
								"1.1.1.1",
								"8.8.8.8",
							},
							Options: []corev1.PodDNSConfigOption{
								{
									Name:  "ndots",
									Value: pointer.String("2"),
								},
								{
									Name: "edns0",
								},
							},
						},
					},
				}, &appsv1.DaemonSet{
					ObjectMeta: getResticObjectMeta(r),
				},
			},
			wantErr: false,
			want: &appsv1.DaemonSet{
				ObjectMeta: getResticObjectMeta(r),
				TypeMeta: metav1.TypeMeta{
					Kind:       "DaemonSet",
					APIVersion: appsv1.SchemeGroupVersion.String(),
				},
				Spec: appsv1.DaemonSetSpec{
					UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
						Type: appsv1.RollingUpdateDaemonSetStrategyType,
					},
					Selector: resticLabelSelector,
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"component": common.Velero,
								"name":      common.Restic,
							},
							Annotations: map[string]string{
								"test-annotation": "awesome annotation",
							},
						},
						Spec: corev1.PodSpec{
							NodeSelector:       velero.Spec.ResticNodeSelector,
							ServiceAccountName: common.Velero,
							SecurityContext: &corev1.PodSecurityContext{
								RunAsUser:          pointer.Int64(0),
								SupplementalGroups: velero.Spec.ResticSupplementalGroups,
							},
							DNSPolicy: "None",
							DNSConfig: &corev1.PodDNSConfig{
								Nameservers: []string{
									"1.1.1.1",
									"8.8.8.8",
								},
								Options: []corev1.PodDNSConfigOption{
									{
										Name:  "ndots",
										Value: pointer.String("2"),
									},
									{
										Name: "edns0",
									},
								},
							},
							Volumes: []corev1.Volume{
								// Cloud Provider volumes are dynamically added in the for loop below
								{
									Name: HostPods,
									VolumeSource: corev1.VolumeSource{
										HostPath: &corev1.HostPathVolumeSource{
											Path: resticPvHostPath,
										},
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
								{
									Name: "cloud-credentials",
									VolumeSource: corev1.VolumeSource{
										Secret: &corev1.SecretVolumeSource{
											SecretName: "cloud-credentials",
										},
									},
								},
							},
							Tolerations: velero.Spec.ResticTolerations,
							Containers: []corev1.Container{
								{
									Name: common.Restic,
									SecurityContext: &corev1.SecurityContext{
										Privileged: pointer.Bool(true),
									},
									Image:           getVeleroImage(&velero),
									ImagePullPolicy: corev1.PullAlways,
									Resources:       r.getVeleroResourceReqs(&velero), //setting default.
									Command: []string{
										"/velero",
									},
									Args: []string{
										"restic",
										"server",
									},
									VolumeMounts: []corev1.VolumeMount{
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
										{
											Name:      "cloud-credentials",
											MountPath: "/credentials",
										},
									},
									Env: []corev1.EnvVar{
										{
											Name: "NODE_NAME",
											ValueFrom: &corev1.EnvVarSource{
												FieldRef: &corev1.ObjectFieldSelector{
													FieldPath: "spec.nodeName",
												},
											},
										},
										{
											Name: "VELERO_NAMESPACE",
											ValueFrom: &corev1.EnvVarSource{
												FieldRef: &corev1.ObjectFieldSelector{
													FieldPath: "metadata.namespace",
												},
											},
										},
										{
											Name:  "VELERO_SCRATCH_DIR",
											Value: "/scratch",
										},
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
											Name:  common.AWSSharedCredentialsFileEnvKey,
											Value: "/credentials/cloud",
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
				t.Errorf("VeleroReconciler.buildResticDaemonset() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVeleroReconciler_updateResticRestoreHelperCM(t *testing.T) {

	tests := []struct {
		name                      string
		resticRestoreHelperCM     *corev1.ConfigMap
		velero                    *oadpv1alpha1.Velero
		wantErr                   bool
		wantResticRestoreHelperCM *corev1.ConfigMap
	}{
		{
			name: "Given Velero CR instance, appropriate restic restore helper cm is created",
			resticRestoreHelperCM: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ResticRestoreHelperCM,
					Namespace: "test-ns",
				},
			},
			velero:  &oadpv1alpha1.Velero{},
			wantErr: false,
			wantResticRestoreHelperCM: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ResticRestoreHelperCM,
					Namespace: "test-ns",
					Labels: map[string]string{
						"velero.io/plugin-config":      "",
						"velero.io/restic":             "RestoreItemAction",
						oadpv1alpha1.OadpOperatorLabel: "True",
					},
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         oadpv1alpha1.SchemeBuilder.GroupVersion.String(),
							Kind:               "Velero",
							Name:               "",
							UID:                "",
							Controller:         pointer.BoolPtr(true),
							BlockOwnerDeletion: pointer.BoolPtr(true),
						},
					},
				},
				Data: map[string]string{
					"image": fmt.Sprintf("%v/%v/%v:%v", os.Getenv("REGISTRY"), os.Getenv("PROJECT"), os.Getenv("VELERO_RESTIC_RESTORE_HELPER_REPO"), os.Getenv("VELERO_RESTIC_RESTORE_HELPER_TAG")),
				},
			},
		},
	}
	for _, tt := range tests {
		fakeClient, err := getFakeClientFromObjects()
		if err != nil {
			t.Errorf("error in creating fake client, likely programmer error")
		}
		t.Run(tt.name, func(t *testing.T) {
			r := &VeleroReconciler{
				Client:  fakeClient,
				Scheme:  fakeClient.Scheme(),
				Log:     logr.Discard(),
				Context: newContextForTest(tt.name),
				NamespacedName: types.NamespacedName{
					Namespace: tt.velero.Namespace,
					Name:      tt.velero.Name,
				},
				EventRecorder: record.NewFakeRecorder(10),
			}
			if err := r.updateResticRestoreHelperCM(tt.resticRestoreHelperCM, tt.velero); (err != nil) != tt.wantErr {
				t.Errorf("updateResticRestoreHelperCM() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(tt.resticRestoreHelperCM, tt.wantResticRestoreHelperCM) {
				t.Errorf("updateResticRestoreHelperCM() got CM = %v, want CM %v", tt.resticRestoreHelperCM, tt.wantResticRestoreHelperCM)
			}
		})
	}
}
