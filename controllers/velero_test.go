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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestVeleroReconciler_buildVeleroDeployment(t *testing.T) {
	type fields struct {
		Client         client.Client
		Scheme         *runtime.Scheme
		Log            logr.Logger
		Context        context.Context
		NamespacedName types.NamespacedName
		EventRecorder  record.EventRecorder
	}
	tests := []struct {
		name                 string
		fields               fields
		veleroDeployment     *appsv1.Deployment
		velero               *oadpv1alpha1.Velero
		wantErr              bool
		wantVeleroDeployment *appsv1.Deployment
	}{
		{
			name: "Velero CR is nil",
			veleroDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-velero-deployment",
					Namespace: "test-ns",
				},
				Spec: appsv1.DeploymentSpec{
					Selector: veleroLabelSelector,
				},
			},
			velero:  nil,
			wantErr: true,
			wantVeleroDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-velero-deployment",
					Namespace: "test-ns",
				},
				Spec: appsv1.DeploymentSpec{
					Selector: veleroLabelSelector,
				},
			},
		},

		{
			name:                 "Velero Deployment is nil",
			veleroDeployment:     nil,
			velero:               &oadpv1alpha1.Velero{},
			wantErr:              true,
			wantVeleroDeployment: nil,
		},
		{
			name: "given valid Velero CR, appropriate Velero Deployment is built",
			veleroDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-velero-deployment",
					Namespace: "test-ns",
				},
				Spec: appsv1.DeploymentSpec{
					Selector: veleroLabelSelector,
				},
			},
			velero: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
			},
			wantErr: false,
			wantVeleroDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-velero-deployment",
					Namespace: "test-ns",
					Labels: map[string]string{
						"app.kubernetes.io/name":       common.Velero,
						"app.kubernetes.io/instance":   "test-Velero-CR",
						"app.kubernetes.io/managed-by": common.OADPOperator,
						"app.kubernetes.io/component":  Server,
					},
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "Deployment",
					APIVersion: appsv1.SchemeGroupVersion.String(),
				},
				Spec: appsv1.DeploymentSpec{
					Selector: veleroLabelSelector,
					Replicas: pointer.Int32(1),
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: veleroLabelSelector.MatchLabels,
							Annotations: map[string]string{
								"prometheus.io/scrape": "true",
								"prometheus.io/port":   "8085",
								"prometheus.io/path":   "/metrics",
							},
						},
						Spec: corev1.PodSpec{
							RestartPolicy:      corev1.RestartPolicyAlways,
							ServiceAccountName: common.Velero,
							Containers: []corev1.Container{
								{
									Name:            common.Velero,
									Image:           common.VeleroImage,
									ImagePullPolicy: corev1.PullAlways,
									Ports: []corev1.ContainerPort{
										{
											Name:          "metrics",
											ContainerPort: 8085,
										},
									},
									Resources: corev1.ResourceRequirements{
										Limits: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("1"),
											corev1.ResourceMemory: resource.MustParse("512Mi"),
										},
										Requests: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("500m"),
											corev1.ResourceMemory: resource.MustParse("128Mi"),
										},
									},
									Command: []string{"/velero"},
									Args: []string{
										"server",
										"--restic-timeout=1h",
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "plugins",
											MountPath: "/plugins",
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
											Name:  common.VeleroScratchDirEnvKey,
											Value: "/scratch",
										},
										{
											Name: common.VeleroNamespaceEnvKey,
											ValueFrom: &corev1.EnvVarSource{
												FieldRef: &corev1.ObjectFieldSelector{
													FieldPath: "metadata.namespace",
												},
											},
										},
										{
											Name:  common.LDLibraryPathEnvKey,
											Value: "/plugins",
										},
										{
											Name:  common.HTTPProxyEnvVar,
											Value: os.Getenv("HTTP_PROXY"),
										},
										{
											Name:  common.HTTPSProxyEnvVar,
											Value: os.Getenv("HTTPS_PROXY"),
										},
										{
											Name:  common.NoProxyEnvVar,
											Value: os.Getenv("NO_PROXY"),
										},
									},
								},
							},
							Volumes: []corev1.Volume{
								{
									Name: "plugins",
									VolumeSource: corev1.VolumeSource{
										EmptyDir: &corev1.EmptyDirVolumeSource{},
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
							InitContainers: []corev1.Container{},
						},
					},
				},
			},
		},
		{
			name: "given valid Velero CR, velero deployment resource customization",
			veleroDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-velero-deployment",
					Namespace: "test-ns",
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "Deployment",
					APIVersion: appsv1.SchemeGroupVersion.String(),
				},
				Spec: appsv1.DeploymentSpec{
					Selector: veleroLabelSelector,
				},
			},
			velero: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					VeleroResourceAllocations: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("2"),
							corev1.ResourceMemory: resource.MustParse("700Mi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1"),
							corev1.ResourceMemory: resource.MustParse("256Mi"),
						},
					},
				},
			},
			wantErr: false,
			wantVeleroDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-velero-deployment",
					Namespace: "test-ns",
					Labels: map[string]string{
						"app.kubernetes.io/name":       common.Velero,
						"app.kubernetes.io/instance":   "test-Velero-CR",
						"app.kubernetes.io/managed-by": common.OADPOperator,
						"app.kubernetes.io/component":  Server,
					},
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "Deployment",
					APIVersion: appsv1.SchemeGroupVersion.String(),
				},
				Spec: appsv1.DeploymentSpec{
					Selector: veleroLabelSelector,
					Replicas: pointer.Int32(1),
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: veleroLabelSelector.MatchLabels,
							Annotations: map[string]string{
								"prometheus.io/scrape": "true",
								"prometheus.io/port":   "8085",
								"prometheus.io/path":   "/metrics",
							},
						},
						Spec: corev1.PodSpec{
							RestartPolicy:      corev1.RestartPolicyAlways,
							ServiceAccountName: common.Velero,
							Containers: []corev1.Container{
								{
									Name:            common.Velero,
									Image:           common.VeleroImage,
									ImagePullPolicy: corev1.PullAlways,
									Ports: []corev1.ContainerPort{
										{
											Name:          "metrics",
											ContainerPort: 8085,
										},
									},
									Resources: corev1.ResourceRequirements{
										Limits: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("2"),
											corev1.ResourceMemory: resource.MustParse("700Mi"),
										},
										Requests: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("1"),
											corev1.ResourceMemory: resource.MustParse("256Mi"),
										},
									},
									Command: []string{"/velero"},
									Args: []string{
										"server",
										"--restic-timeout=1h",
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "plugins",
											MountPath: "/plugins",
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
											Name:  common.VeleroScratchDirEnvKey,
											Value: "/scratch",
										},
										{
											Name: common.VeleroNamespaceEnvKey,
											ValueFrom: &corev1.EnvVarSource{
												FieldRef: &corev1.ObjectFieldSelector{
													FieldPath: "metadata.namespace",
												},
											},
										},
										{
											Name:  common.LDLibraryPathEnvKey,
											Value: "/plugins",
										},
										{
											Name:  common.HTTPProxyEnvVar,
											Value: os.Getenv("HTTP_PROXY"),
										},
										{
											Name:  common.HTTPSProxyEnvVar,
											Value: os.Getenv("HTTPS_PROXY"),
										},
										{
											Name:  common.NoProxyEnvVar,
											Value: os.Getenv("NO_PROXY"),
										},
									},
								},
							},
							Volumes: []corev1.Volume{
								{
									Name: "plugins",
									VolumeSource: corev1.VolumeSource{
										EmptyDir: &corev1.EmptyDirVolumeSource{},
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
							InitContainers: []corev1.Container{},
						},
					},
				},
			},
		},
		{
			name: "given valid Velero CR, appropriate velero deployment is build with aws plugin specific specs",
			veleroDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-velero-deployment",
					Namespace: "test-ns",
				},
				Spec: appsv1.DeploymentSpec{
					Selector: veleroLabelSelector,
				},
			},
			velero: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					DefaultVeleroPlugins: []oadpv1alpha1.DefaultPlugin{
						oadpv1alpha1.DefaultPluginAWS,
					},
				},
			},
			wantErr: false,
			wantVeleroDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-velero-deployment",
					Namespace: "test-ns",
					Labels: map[string]string{
						"app.kubernetes.io/name":       common.Velero,
						"app.kubernetes.io/instance":   "test-Velero-CR",
						"app.kubernetes.io/managed-by": common.OADPOperator,
						"app.kubernetes.io/component":  Server,
					},
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "Deployment",
					APIVersion: appsv1.SchemeGroupVersion.String(),
				},
				Spec: appsv1.DeploymentSpec{
					Selector: veleroLabelSelector,
					Replicas: pointer.Int32(1),
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: veleroLabelSelector.MatchLabels,
							Annotations: map[string]string{
								"prometheus.io/scrape": "true",
								"prometheus.io/port":   "8085",
								"prometheus.io/path":   "/metrics",
							},
						},
						Spec: corev1.PodSpec{
							RestartPolicy:      corev1.RestartPolicyAlways,
							ServiceAccountName: common.Velero,
							Containers: []corev1.Container{
								{
									Name:            common.Velero,
									Image:           common.VeleroImage,
									ImagePullPolicy: corev1.PullAlways,
									Ports: []corev1.ContainerPort{
										{
											Name:          "metrics",
											ContainerPort: 8085,
										},
									},
									Resources: corev1.ResourceRequirements{
										Limits: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("1"),
											corev1.ResourceMemory: resource.MustParse("512Mi"),
										},
										Requests: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("500m"),
											corev1.ResourceMemory: resource.MustParse("128Mi"),
										},
									},
									Command: []string{"/velero"},
									Args: []string{
										"server",
										"--restic-timeout=1h",
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "plugins",
											MountPath: "/plugins",
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
											Name:  common.VeleroScratchDirEnvKey,
											Value: "/scratch",
										},
										{
											Name: common.VeleroNamespaceEnvKey,
											ValueFrom: &corev1.EnvVarSource{
												FieldRef: &corev1.ObjectFieldSelector{
													FieldPath: "metadata.namespace",
												},
											},
										},
										{
											Name:  common.LDLibraryPathEnvKey,
											Value: "/plugins",
										},
										{
											Name:  common.HTTPProxyEnvVar,
											Value: os.Getenv("HTTP_PROXY"),
										},
										{
											Name:  common.HTTPSProxyEnvVar,
											Value: os.Getenv("HTTPS_PROXY"),
										},
										{
											Name:  common.NoProxyEnvVar,
											Value: os.Getenv("NO_PROXY"),
										},
										{
											Name:  common.AWSSharedCredentialsFileEnvKey,
											Value: "/credentials/cloud",
										},
									},
								},
							},
							Volumes: []corev1.Volume{
								{
									Name: "plugins",
									VolumeSource: corev1.VolumeSource{
										EmptyDir: &corev1.EmptyDirVolumeSource{},
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
							InitContainers: []corev1.Container{
								{
									Image:                    common.AWSPluginImage,
									Name:                     common.VeleroPluginForAWS,
									ImagePullPolicy:          corev1.PullAlways,
									Resources:                corev1.ResourceRequirements{},
									TerminationMessagePath:   "/dev/termination-log",
									TerminationMessagePolicy: "File",
									VolumeMounts: []corev1.VolumeMount{
										{
											MountPath: "/target",
											Name:      "plugins",
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
			name: "given valid Velero CR with annotations, appropriate velero deployment is build with aws plugin specific specs",
			veleroDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-velero-deployment",
					Namespace: "test-ns",
				},
				Spec: appsv1.DeploymentSpec{
					Selector: veleroLabelSelector,
				},
			},
			velero: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					DefaultVeleroPlugins: []oadpv1alpha1.DefaultPlugin{
						oadpv1alpha1.DefaultPluginAWS,
					},
					PodAnnotations: map[string]string{
						"test-annotation": "awesome annotation",
					},
				},
			},
			wantErr: false,
			wantVeleroDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-velero-deployment",
					Namespace: "test-ns",
					Labels: map[string]string{
						"app.kubernetes.io/name":       common.Velero,
						"app.kubernetes.io/instance":   "test-Velero-CR",
						"app.kubernetes.io/managed-by": common.OADPOperator,
						"app.kubernetes.io/component":  Server,
					},
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "Deployment",
					APIVersion: appsv1.SchemeGroupVersion.String(),
				},
				Spec: appsv1.DeploymentSpec{
					Selector: veleroLabelSelector,
					Replicas: pointer.Int32(1),
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: veleroLabelSelector.MatchLabels,
							Annotations: map[string]string{
								"prometheus.io/scrape": "true",
								"prometheus.io/port":   "8085",
								"prometheus.io/path":   "/metrics",
								"test-annotation":      "awesome annotation",
							},
						},
						Spec: corev1.PodSpec{
							RestartPolicy:      corev1.RestartPolicyAlways,
							ServiceAccountName: common.Velero,
							Containers: []corev1.Container{
								{
									Name:            common.Velero,
									Image:           common.VeleroImage,
									ImagePullPolicy: corev1.PullAlways,
									Ports: []corev1.ContainerPort{
										{
											Name:          "metrics",
											ContainerPort: 8085,
										},
									},
									Resources: corev1.ResourceRequirements{
										Limits: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("1"),
											corev1.ResourceMemory: resource.MustParse("512Mi"),
										},
										Requests: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("500m"),
											corev1.ResourceMemory: resource.MustParse("128Mi"),
										},
									},
									Command: []string{"/velero"},
									Args: []string{
										"server",
										"--restic-timeout=1h",
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "plugins",
											MountPath: "/plugins",
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
											Name:  common.VeleroScratchDirEnvKey,
											Value: "/scratch",
										},
										{
											Name: common.VeleroNamespaceEnvKey,
											ValueFrom: &corev1.EnvVarSource{
												FieldRef: &corev1.ObjectFieldSelector{
													FieldPath: "metadata.namespace",
												},
											},
										},
										{
											Name:  common.LDLibraryPathEnvKey,
											Value: "/plugins",
										},
										{
											Name:  common.HTTPProxyEnvVar,
											Value: os.Getenv("HTTP_PROXY"),
										},
										{
											Name:  common.HTTPSProxyEnvVar,
											Value: os.Getenv("HTTPS_PROXY"),
										},
										{
											Name:  common.NoProxyEnvVar,
											Value: os.Getenv("NO_PROXY"),
										},
										{
											Name:  common.AWSSharedCredentialsFileEnvKey,
											Value: "/credentials/cloud",
										},
									},
								},
							},
							Volumes: []corev1.Volume{
								{
									Name: "plugins",
									VolumeSource: corev1.VolumeSource{
										EmptyDir: &corev1.EmptyDirVolumeSource{},
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
							InitContainers: []corev1.Container{
								{
									Image:                    common.AWSPluginImage,
									Name:                     common.VeleroPluginForAWS,
									ImagePullPolicy:          corev1.PullAlways,
									Resources:                corev1.ResourceRequirements{},
									TerminationMessagePath:   "/dev/termination-log",
									TerminationMessagePolicy: "File",
									VolumeMounts: []corev1.VolumeMount{
										{
											MountPath: "/target",
											Name:      "plugins",
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
	r := &VeleroReconciler{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := r.buildVeleroDeployment(tt.veleroDeployment, tt.velero); (err != nil) != tt.wantErr {
				t.Errorf("buildVeleroDeployment() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(tt.wantVeleroDeployment, tt.veleroDeployment) {
				t.Errorf("expected registry deployment spec to be %#v, got %#v", tt.wantVeleroDeployment, tt.veleroDeployment)
			}
		})
	}
}

func TestVeleroReconciler_getVeleroImage(t *testing.T) {
	tests := []struct {
		name       string
		VeleroCR   *oadpv1alpha1.Velero
		pluginName string
		wantImage  string
		setEnvVars map[string]string
	}{
		{
			name: "given Velero image override, custom Velero image should be returned",
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					UnsupportedOverrides: map[oadpv1alpha1.UnsupportedImageKey]string{
						oadpv1alpha1.VeleroImageKey: "test-image",
					},
				},
			},
			pluginName: common.Velero,
			wantImage:  "test-image",
			setEnvVars: make(map[string]string),
		},
		{
			name: "given default Velero CR with no env var, default velero image should be returned",
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
			},
			pluginName: common.Velero,
			wantImage:  common.VeleroImage,
			setEnvVars: make(map[string]string),
		},
		{
			name: "given default Velero CR with env var set, image should be built via env vars",
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
			},
			pluginName: common.Velero,
			wantImage:  "quay.io/konveyor/velero:latest",
			setEnvVars: map[string]string{
				"REGISTRY":               "quay.io",
				"PROJECT":                "konveyor",
				"VELERO_AWS_PLUGIN_REPO": "velero",
				"VELERO_AWS_PLUGIN_TAG":  "latest",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for key, value := range tt.setEnvVars {
				os.Setenv(key, value)
				defer os.Unsetenv(key)
			}
			gotImage := getVeleroImage(tt.VeleroCR)
			if gotImage != tt.wantImage {
				t.Errorf("Expected plugin image %v did not match %v", tt.wantImage, gotImage)
			}
		})
	}
}
