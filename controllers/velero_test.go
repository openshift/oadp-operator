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
						oadpv1alpha1.OadpOperatorLabel: "True",
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
						oadpv1alpha1.OadpOperatorLabel: "True",
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
						oadpv1alpha1.OadpOperatorLabel: "True",
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
						oadpv1alpha1.OadpOperatorLabel: "True",
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
		{
			name: "given valid Velero CR with PodDNS Policy/Config, annotations, appropriate velero deployment is build with aws plugin specific specs",
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
						oadpv1alpha1.OadpOperatorLabel: "True",
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
							DNSPolicy:          "None",
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
				"REGISTRY":    "quay.io",
				"PROJECT":     "konveyor",
				"VELERO_REPO": "velero",
				"VELERO_TAG":  "latest",
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
func Test_removeDuplicateValues(t *testing.T) {
	type args struct {
		slice []string
	}
	tests := []struct {
		name string
		args args
		want []string
	}{
		{
			name: "nill slice",
			args: args{slice: nil},
			want: nil,
		},
		{
			name: "empty slice",
			args: args{slice: []string{}},
			want: []string{},
		},
		{
			name: "one item in slice",
			args: args{slice: []string{"yo"}},
			want: []string{"yo"},
		},
		{
			name: "duplicate item in slice",
			args: args{slice: []string{"ice", "ice", "baby"}},
			want: []string{"ice", "baby"},
		},
		{
			name: "maintain order of first appearance in slice",
			args: args{slice: []string{"ice", "ice", "baby", "ice"}},
			want: []string{"ice", "baby"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := removeDuplicateValues(tt.args.slice); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("removeDuplicateValues() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_validateVeleroPlugins(t *testing.T) {
	tests := []struct {
		name    string
		velero  *oadpv1alpha1.Velero
		secret  *corev1.Secret
		wantErr bool
		want    bool
	}{

		{
			name: "given valid Velero default plugin, default secret gets mounted as volume mounts",
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
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials",
					Namespace: "test-ns",
				},
			},
			wantErr: false,
			want:    true,
		},
		{
			name: "given valid Velero default plugin that is not a cloud provider, no secrets get mounted",
			velero: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					DefaultVeleroPlugins: []oadpv1alpha1.DefaultPlugin{
						oadpv1alpha1.DefaultPluginOpenShift,
					},
				},
			},
			secret:  &corev1.Secret{},
			wantErr: false,
			want:    true,
		},
		{
			name: "given valid multiple Velero default plugins, default secrets gets mounted for each plugin if applicable",
			velero: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					DefaultVeleroPlugins: []oadpv1alpha1.DefaultPlugin{
						oadpv1alpha1.DefaultPluginAWS,
						oadpv1alpha1.DefaultPluginOpenShift,
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials",
					Namespace: "test-ns",
				},
			},
			wantErr: false,
			want:    true,
		},
		{
			name: "given invalid Velero secret, the validplugin check fails",
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
			secret:  &corev1.Secret{},
			wantErr: true,
			want:    false,
		},
	}
	for _, tt := range tests {
		fakeClient, err := getFakeClientFromObjects(tt.velero, tt.secret)
		if err != nil {
			t.Errorf("error in creating fake client, likely programmer error")
		}
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
		t.Run(tt.name, func(t *testing.T) {
			result, err := r.ValidateVeleroPlugins(r.Log)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateVeleroPlugins() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(result, tt.want) {
				t.Errorf("ValidateVeleroPlugins() = %v, want %v", result, tt.want)
			}
		})
	}
}
