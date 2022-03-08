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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestDPAReconciler_ReconcileResticDaemonset(t *testing.T) {
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
			r := &DPAReconciler{
				Client:         tt.fields.Client,
				Scheme:         tt.fields.Scheme,
				Log:            tt.fields.Log,
				Context:        tt.fields.Context,
				NamespacedName: tt.fields.NamespacedName,
				EventRecorder:  tt.fields.EventRecorder,
			}
			got, err := r.ReconcileResticDaemonset(tt.args.log)
			if (err != nil) != tt.wantErr {
				t.Errorf("DPAReconciler.ReconcileResticDaemonset() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("DPAReconciler.ReconcileResticDaemonset() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDPAReconciler_buildResticDaemonset(t *testing.T) {
	type fields struct {
		Client         client.Client
		Scheme         *runtime.Scheme
		Log            logr.Logger
		Context        context.Context
		NamespacedName types.NamespacedName
		EventRecorder  record.EventRecorder
	}
	type args struct {
		dpa *oadpv1alpha1.DataProtectionApplication
		ds  *appsv1.DaemonSet
	}
	r := &DPAReconciler{}
	dpa := oadpv1alpha1.DataProtectionApplication{
		Spec: oadpv1alpha1.DataProtectionApplicationSpec{
			Configuration: &oadpv1alpha1.ApplicationConfig{
				Restic: &oadpv1alpha1.ResticConfig{
					PodConfig: &oadpv1alpha1.PodConfig{},
				},
			},
		},
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *appsv1.DaemonSet
		wantErr bool
	}{
		{
			name:   "dpa is nil",
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
				&oadpv1alpha1.DataProtectionApplication{}, nil,
			},
			wantErr: true,
			want:    nil,
		},
		{
			name: "Valid velero and daemonset",
			args: args{
				&oadpv1alpha1.DataProtectionApplication{
					Spec: oadpv1alpha1.DataProtectionApplicationSpec{
						Configuration: &oadpv1alpha1.ApplicationConfig{
							Restic: &oadpv1alpha1.ResticConfig{
								PodConfig: &oadpv1alpha1.PodConfig{},
							},
							Velero: &oadpv1alpha1.VeleroConfig{
								PodConfig: &oadpv1alpha1.PodConfig{},
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
					Template: v1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"component": common.Velero,
								"name":      common.Restic,
							},
						},
						Spec: v1.PodSpec{
							NodeSelector:       dpa.Spec.Configuration.Restic.PodConfig.NodeSelector,
							ServiceAccountName: common.Velero,
							SecurityContext: &v1.PodSecurityContext{
								RunAsUser:          pointer.Int64(0),
								SupplementalGroups: dpa.Spec.Configuration.Restic.SupplementalGroups,
							},
							Volumes: []v1.Volume{
								// Cloud Provider volumes are dynamically added in the for loop below
								{
									Name: HostPods,
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
							Tolerations: dpa.Spec.Configuration.Restic.PodConfig.Tolerations,
							Containers: []v1.Container{
								{
									Name: common.Restic,
									SecurityContext: &v1.SecurityContext{
										Privileged: pointer.Bool(true),
									},
									Image:           getVeleroImage(&dpa),
									ImagePullPolicy: v1.PullAlways,
									Resources:       r.getResticResourceReqs(&dpa), //setting default.
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
											Name: "NODE_NAME",
											ValueFrom: &v1.EnvVarSource{
												FieldRef: &v1.ObjectFieldSelector{
													FieldPath: "spec.nodeName",
												},
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
		{
			name: "Valid velero and daemonset for aws as bsl",
			args: args{
				&oadpv1alpha1.DataProtectionApplication{
					Spec: oadpv1alpha1.DataProtectionApplicationSpec{
						Configuration: &oadpv1alpha1.ApplicationConfig{
							Restic: &oadpv1alpha1.ResticConfig{
								PodConfig: &oadpv1alpha1.PodConfig{},
							},
							Velero: &oadpv1alpha1.VeleroConfig{
								PodConfig: &oadpv1alpha1.PodConfig{},
								DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
									oadpv1alpha1.DefaultPluginAWS,
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
					Template: v1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"component": common.Velero,
								"name":      common.Restic,
							},
						},
						Spec: v1.PodSpec{
							NodeSelector:       dpa.Spec.Configuration.Restic.PodConfig.NodeSelector,
							ServiceAccountName: common.Velero,
							SecurityContext: &v1.PodSecurityContext{
								RunAsUser:          pointer.Int64(0),
								SupplementalGroups: dpa.Spec.Configuration.Restic.SupplementalGroups,
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
								{
									Name: "cloud-credentials",
									VolumeSource: v1.VolumeSource{
										Secret: &v1.SecretVolumeSource{
											SecretName: "cloud-credentials",
										},
									},
								},
							},
							Tolerations: dpa.Spec.Configuration.Restic.PodConfig.Tolerations,
							Containers: []v1.Container{
								{
									Name: common.Restic,
									SecurityContext: &v1.SecurityContext{
										Privileged: pointer.Bool(true),
									},
									Image:           getVeleroImage(&dpa),
									ImagePullPolicy: v1.PullAlways,
									Resources:       r.getResticResourceReqs(&dpa), //setting default.
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
										{
											Name:      "cloud-credentials",
											MountPath: "/credentials",
										},
									},
									Env: []v1.EnvVar{
										{
											Name: "NODE_NAME",
											ValueFrom: &v1.EnvVarSource{
												FieldRef: &v1.ObjectFieldSelector{
													FieldPath: "spec.nodeName",
												},
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
				&oadpv1alpha1.DataProtectionApplication{
					Spec: oadpv1alpha1.DataProtectionApplicationSpec{
						Configuration: &oadpv1alpha1.ApplicationConfig{
							Velero: &oadpv1alpha1.VeleroConfig{
								DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
									oadpv1alpha1.DefaultPluginAWS,
								},
							},
							Restic: &oadpv1alpha1.ResticConfig{},
						},
						BackupLocations: []oadpv1alpha1.BackupLocation{
							{
								Velero: &velerov1.BackupStorageLocationSpec{
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
									Credential: &v1.SecretKeySelector{
										LocalObjectReference: v1.LocalObjectReference{
											Name: "cloud-credentials",
										},
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
					Template: v1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"component": common.Velero,
								"name":      common.Restic,
							},
							Annotations: map[string]string{
								"test-annotation": "awesome annotation",
							},
						},
						Spec: v1.PodSpec{
							NodeSelector:       dpa.Spec.Configuration.Restic.PodConfig.NodeSelector,
							ServiceAccountName: common.Velero,
							SecurityContext: &v1.PodSecurityContext{
								RunAsUser:          pointer.Int64(0),
								SupplementalGroups: dpa.Spec.Configuration.Restic.SupplementalGroups,
							},
							Volumes: []v1.Volume{
								// Cloud Provider volumes are dynamically added in the for loop below
								{
									Name: HostPods,
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
							Tolerations: dpa.Spec.Configuration.Restic.PodConfig.Tolerations,
							Containers: []v1.Container{
								{
									Name: common.Restic,
									SecurityContext: &v1.SecurityContext{
										Privileged: pointer.Bool(true),
									},
									Image:           getVeleroImage(&dpa),
									ImagePullPolicy: v1.PullAlways,
									Resources:       r.getResticResourceReqs(&dpa), //setting default.
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
											Name: "NODE_NAME",
											ValueFrom: &v1.EnvVarSource{
												FieldRef: &v1.ObjectFieldSelector{
													FieldPath: "spec.nodeName",
												},
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
		{
			name: "Valid velero with DNS Policy/Config with annotation and daemonset for aws as bsl with default secret name not specified",
			args: args{
				&oadpv1alpha1.DataProtectionApplication{
					Spec: oadpv1alpha1.DataProtectionApplicationSpec{
						Configuration: &oadpv1alpha1.ApplicationConfig{
							Velero: &oadpv1alpha1.VeleroConfig{
								DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
									oadpv1alpha1.DefaultPluginAWS,
								},
							},
							Restic: &oadpv1alpha1.ResticConfig{},
						},
						BackupLocations: []oadpv1alpha1.BackupLocation{
							{
								Velero: &velerov1.BackupStorageLocationSpec{
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
								},
							},
						},
						PodAnnotations: map[string]string{
							"test-annotation": "awesome annotation",
						},
						PodDnsPolicy: "None",
						PodDnsConfig: v1.PodDNSConfig{
							Nameservers: []string{
								"1.1.1.1",
								"8.8.8.8",
							},
							Options: []v1.PodDNSConfigOption{
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
					Template: v1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"component": common.Velero,
								"name":      common.Restic,
							},
							Annotations: map[string]string{
								"test-annotation": "awesome annotation",
							},
						},
						Spec: v1.PodSpec{
							NodeSelector:       dpa.Spec.Configuration.Restic.PodConfig.NodeSelector,
							ServiceAccountName: common.Velero,
							SecurityContext: &v1.PodSecurityContext{
								RunAsUser:          pointer.Int64(0),
								SupplementalGroups: dpa.Spec.Configuration.Restic.SupplementalGroups,
							},
							DNSPolicy: "None",
							DNSConfig: &v1.PodDNSConfig{
								Nameservers: []string{
									"1.1.1.1",
									"8.8.8.8",
								},
								Options: []v1.PodDNSConfigOption{
									{
										Name:  "ndots",
										Value: pointer.String("2"),
									},
									{
										Name: "edns0",
									},
								},
							},
							Volumes: []v1.Volume{
								// Cloud Provider volumes are dynamically added in the for loop below
								{
									Name: HostPods,
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
								{
									Name: "cloud-credentials",
									VolumeSource: v1.VolumeSource{
										Secret: &v1.SecretVolumeSource{
											SecretName: "cloud-credentials",
										},
									},
								},
							},
							Tolerations: dpa.Spec.Configuration.Restic.PodConfig.Tolerations,
							Containers: []v1.Container{
								{
									Name: common.Restic,
									SecurityContext: &v1.SecurityContext{
										Privileged: pointer.Bool(true),
									},
									Image:           getVeleroImage(&dpa),
									ImagePullPolicy: v1.PullAlways,
									Resources:       r.getResticResourceReqs(&dpa), //setting default.
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
										{
											Name:      "cloud-credentials",
											MountPath: "/credentials",
										},
									},
									Env: []v1.EnvVar{
										{
											Name: "NODE_NAME",
											ValueFrom: &v1.EnvVarSource{
												FieldRef: &v1.ObjectFieldSelector{
													FieldPath: "spec.nodeName",
												},
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
			r := &DPAReconciler{
				Client:         tt.fields.Client,
				Scheme:         tt.fields.Scheme,
				Log:            tt.fields.Log,
				Context:        tt.fields.Context,
				NamespacedName: tt.fields.NamespacedName,
				EventRecorder:  tt.fields.EventRecorder,
			}
			got, err := r.buildResticDaemonset(tt.args.dpa, tt.args.ds)
			if (err != nil) != tt.wantErr {
				t.Errorf("DPAReconciler.buildResticDaemonset() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("DPAReconciler.buildResticDaemonset() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDPAReconciler_updateResticRestoreHelperCM(t *testing.T) {

	tests := []struct {
		name                      string
		resticRestoreHelperCM     *v1.ConfigMap
		dpa                       *oadpv1alpha1.DataProtectionApplication
		wantErr                   bool
		wantResticRestoreHelperCM *v1.ConfigMap
	}{
		{
			name: "Given DPA CR instance, appropriate restic restore helper cm is created",
			resticRestoreHelperCM: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ResticRestoreHelperCM,
					Namespace: "test-ns",
				},
			},
			dpa:     &oadpv1alpha1.DataProtectionApplication{},
			wantErr: false,
			wantResticRestoreHelperCM: &v1.ConfigMap{
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
							Kind:               "DataProtectionApplication",
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
			r := &DPAReconciler{
				Client:  fakeClient,
				Scheme:  fakeClient.Scheme(),
				Log:     logr.Discard(),
				Context: newContextForTest(tt.name),
				NamespacedName: types.NamespacedName{
					Namespace: tt.dpa.Namespace,
					Name:      tt.dpa.Name,
				},
				EventRecorder: record.NewFakeRecorder(10),
			}
			if err := r.updateResticRestoreHelperCM(tt.resticRestoreHelperCM, tt.dpa); (err != nil) != tt.wantErr {
				t.Errorf("updateResticRestoreHelperCM() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(tt.resticRestoreHelperCM, tt.wantResticRestoreHelperCM) {
				t.Errorf("updateResticRestoreHelperCM() got CM = %v, want CM %v", tt.resticRestoreHelperCM, tt.wantResticRestoreHelperCM)
			}
		})
	}
}
