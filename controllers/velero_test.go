package controllers

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/common"
	"github.com/sirupsen/logrus"
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

const proxyEnvKey = "HTTP_PROXY"
const proxyEnvValue = "http://proxy.example.com:8080"

func TestDPAReconciler_buildVeleroDeployment(t *testing.T) {
	type fields struct {
		Client         client.Client
		Scheme         *runtime.Scheme
		Log            logr.Logger
		Context        context.Context
		NamespacedName types.NamespacedName
		EventRecorder  record.EventRecorder
	}

	trueVal := true
	tests := []struct {
		name                 string
		fields               fields
		veleroDeployment     *appsv1.Deployment
		dpa                  *oadpv1alpha1.DataProtectionApplication
		wantErr              bool
		wantVeleroDeployment *appsv1.Deployment
		clientObjects        []client.Object
		testProxy            bool
	}{
		{
			name: "DPA CR is nil",
			veleroDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-velero-deployment",
					Namespace: "test-ns",
				},
				Spec: appsv1.DeploymentSpec{
					Selector: veleroLabelSelector,
				},
			},
			dpa:     nil,
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
			dpa:                  &oadpv1alpha1.DataProtectionApplication{},
			wantErr:              true,
			wantVeleroDeployment: nil,
		},
		{
			name: "given valid DPA CR, appropriate Velero Deployment is built",
			veleroDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-velero-deployment",
					Namespace: "test-ns",
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app.kubernetes.io/name":       common.Velero,
							"app.kubernetes.io/instance":   "test-Velero-CR",
							"app.kubernetes.io/managed-by": common.OADPOperator,
							"app.kubernetes.io/component":  Server,
							"component":                    "velero",
							"deploy":                       "velero",
							oadpv1alpha1.OadpOperatorLabel: "True",
						},
					},
				},
			},
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
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
						"component":                    "velero",
						oadpv1alpha1.OadpOperatorLabel: "True",
					},
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "Deployment",
					APIVersion: appsv1.SchemeGroupVersion.String(),
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app.kubernetes.io/name":       common.Velero,
							"app.kubernetes.io/instance":   "test-Velero-CR",
							"app.kubernetes.io/managed-by": common.OADPOperator,
							"app.kubernetes.io/component":  Server,
							"component":                    "velero",
							"deploy":                       "velero",
							oadpv1alpha1.OadpOperatorLabel: "True",
						},
					},
					Replicas: pointer.Int32(1),
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app.kubernetes.io/name":       common.Velero,
								"app.kubernetes.io/instance":   "test-Velero-CR",
								"app.kubernetes.io/managed-by": common.OADPOperator,
								"app.kubernetes.io/component":  Server,
								"component":                    "velero",
								"deploy":                       "velero",
								oadpv1alpha1.OadpOperatorLabel: "True",
							},
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
			name: "given valid DPA CR with proxy env var, appropriate Velero Deployment is built",
			veleroDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-velero-deployment",
					Namespace: "test-ns",
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app.kubernetes.io/name":       common.Velero,
							"app.kubernetes.io/instance":   "test-Velero-CR",
							"app.kubernetes.io/managed-by": common.OADPOperator,
							"app.kubernetes.io/component":  Server,
							"component":                    "velero",
							"deploy":                       "velero",
							oadpv1alpha1.OadpOperatorLabel: "True",
						},
					},
				},
			},
			testProxy: true,
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
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
						"component":                    "velero",
						oadpv1alpha1.OadpOperatorLabel: "True",
					},
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "Deployment",
					APIVersion: appsv1.SchemeGroupVersion.String(),
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app.kubernetes.io/name":       common.Velero,
							"app.kubernetes.io/instance":   "test-Velero-CR",
							"app.kubernetes.io/managed-by": common.OADPOperator,
							"app.kubernetes.io/component":  Server,
							"component":                    "velero",
							"deploy":                       "velero",
							oadpv1alpha1.OadpOperatorLabel: "True",
						},
					},
					Replicas: pointer.Int32(1),
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app.kubernetes.io/name":       common.Velero,
								"app.kubernetes.io/instance":   "test-Velero-CR",
								"app.kubernetes.io/managed-by": common.OADPOperator,
								"app.kubernetes.io/component":  Server,
								"component":                    "velero",
								"deploy":                       "velero",
								oadpv1alpha1.OadpOperatorLabel: "True",
							},
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
											Name:  proxyEnvKey,
											Value: proxyEnvValue,
										},
										{
											Name:  strings.ToLower(proxyEnvKey),
											Value: proxyEnvValue,
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
			name: "given valid DPA CR with podConfig label, appropriate Velero Deployment has template labels",
			veleroDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-velero-deployment",
					Namespace: "test-ns",
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app.kubernetes.io/name":       common.Velero,
							"app.kubernetes.io/instance":   "test-Velero-CR",
							"app.kubernetes.io/managed-by": common.OADPOperator,
							"app.kubernetes.io/component":  Server,
							"component":                    "velero",
							"deploy":                       "velero",
							oadpv1alpha1.OadpOperatorLabel: "True",
						},
					},
				},
			},
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							PodConfig: &oadpv1alpha1.PodConfig{
								Labels: map[string]string{
									"thisIsVelero": "yes",
								},
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
						"component":                    "velero",
						oadpv1alpha1.OadpOperatorLabel: "True",
					},
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "Deployment",
					APIVersion: appsv1.SchemeGroupVersion.String(),
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app.kubernetes.io/name":       common.Velero,
							"app.kubernetes.io/instance":   "test-Velero-CR",
							"app.kubernetes.io/managed-by": common.OADPOperator,
							"app.kubernetes.io/component":  Server,
							"component":                    "velero",
							"deploy":                       "velero",
							oadpv1alpha1.OadpOperatorLabel: "True",
						},
					},
					Replicas: pointer.Int32(1),
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app.kubernetes.io/name":       common.Velero,
								"app.kubernetes.io/instance":   "test-Velero-CR",
								"app.kubernetes.io/managed-by": common.OADPOperator,
								"app.kubernetes.io/component":  Server,
								"component":                    "velero",
								"deploy":                       "velero",
								oadpv1alpha1.OadpOperatorLabel: "True",
								"thisIsVelero":                 "yes",
							},
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
			name: "given invalid DPA CR because invalid podConfig label, appropriate Velero Deployment is nil with error",
			veleroDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-velero-deployment",
					Namespace: "test-ns",
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app.kubernetes.io/name":       common.Velero,
							"app.kubernetes.io/instance":   "test-Velero-CR",
							"app.kubernetes.io/managed-by": common.OADPOperator,
							"app.kubernetes.io/component":  Server,
							"component":                    "velero",
							"deploy":                       "velero",
							oadpv1alpha1.OadpOperatorLabel: "True",
						},
					},
				},
			},
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							PodConfig: &oadpv1alpha1.PodConfig{
								Labels: map[string]string{
									"component": "restic",
								},
							},
						},
					},
				},
			},
			wantErr:              true,
			wantVeleroDeployment: nil,
		},
		{
			name: "given valid DPA CR and log level is defined correctly, log level is set",
			veleroDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-velero-deployment",
					Namespace: "test-ns",
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app.kubernetes.io/name":       common.Velero,
							"app.kubernetes.io/instance":   "test-Velero-CR",
							"app.kubernetes.io/managed-by": common.OADPOperator,
							"app.kubernetes.io/component":  Server,
							"component":                    "velero",
							"deploy":                       "velero",
							oadpv1alpha1.OadpOperatorLabel: "True",
						},
					},
				},
			},
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							LogLevel: logrus.InfoLevel.String(),
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
						"component":                    "velero",
						oadpv1alpha1.OadpOperatorLabel: "True",
					},
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "Deployment",
					APIVersion: appsv1.SchemeGroupVersion.String(),
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app.kubernetes.io/name":       common.Velero,
							"app.kubernetes.io/instance":   "test-Velero-CR",
							"app.kubernetes.io/managed-by": common.OADPOperator,
							"app.kubernetes.io/component":  Server,
							"component":                    "velero",
							"deploy":                       "velero",
							oadpv1alpha1.OadpOperatorLabel: "True",
						},
					},
					Replicas: pointer.Int32(1),
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app.kubernetes.io/name":       common.Velero,
								"app.kubernetes.io/instance":   "test-Velero-CR",
								"app.kubernetes.io/managed-by": common.OADPOperator,
								"app.kubernetes.io/component":  Server,
								"component":                    "velero",
								"deploy":                       "velero",
								oadpv1alpha1.OadpOperatorLabel: "True",
							},
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
										"--log-level",
										logrus.InfoLevel.String(),
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
			name: "given valid DPA CR and log level is defined incorrectly error is returned",
			veleroDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-velero-deployment",
					Namespace: "test-ns",
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app.kubernetes.io/name":       common.Velero,
							"app.kubernetes.io/instance":   "test-Velero-CR",
							"app.kubernetes.io/managed-by": common.OADPOperator,
							"app.kubernetes.io/component":  Server,
							"component":                    "velero",
							"deploy":                       "velero",
							oadpv1alpha1.OadpOperatorLabel: "True",
						},
					},
				},
			},
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							LogLevel: logrus.InfoLevel.String() + "typo",
						},
					},
				},
			},
			wantErr:              true,
			wantVeleroDeployment: nil,
		},
		{
			name: "given valid DPA CR, velero deployment resource customization",
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
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app.kubernetes.io/name":       common.Velero,
							"app.kubernetes.io/instance":   "test-Velero-CR",
							"app.kubernetes.io/managed-by": common.OADPOperator,
							"app.kubernetes.io/component":  Server,
							"component":                    "velero",
							"deploy":                       "velero",
							oadpv1alpha1.OadpOperatorLabel: "True",
						},
					},
				},
			},
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							PodConfig: &oadpv1alpha1.PodConfig{
								ResourceAllocations: corev1.ResourceRequirements{
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
						"component":                    "velero",
						oadpv1alpha1.OadpOperatorLabel: "True",
					},
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "Deployment",
					APIVersion: appsv1.SchemeGroupVersion.String(),
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app.kubernetes.io/name":       common.Velero,
							"app.kubernetes.io/instance":   "test-Velero-CR",
							"app.kubernetes.io/managed-by": common.OADPOperator,
							"app.kubernetes.io/component":  Server,
							"component":                    "velero",
							"deploy":                       "velero",
							oadpv1alpha1.OadpOperatorLabel: "True",
						},
					},
					Replicas: pointer.Int32(1),
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app.kubernetes.io/name":       common.Velero,
								"app.kubernetes.io/instance":   "test-Velero-CR",
								"app.kubernetes.io/managed-by": common.OADPOperator,
								"app.kubernetes.io/component":  Server,
								"component":                    "velero",
								"deploy":                       "velero",
								oadpv1alpha1.OadpOperatorLabel: "True",
							},
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
			name: "given valid DPA CR, velero deployment tolerations",
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
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app.kubernetes.io/name":       common.Velero,
							"app.kubernetes.io/instance":   "test-Velero-CR",
							"app.kubernetes.io/managed-by": common.OADPOperator,
							"app.kubernetes.io/component":  Server,
							oadpv1alpha1.OadpOperatorLabel: "True",
							"component":                    "velero",
							"deploy":                       "velero",
						},
					},
				},
			},
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							PodConfig: &oadpv1alpha1.PodConfig{
								ResourceAllocations: corev1.ResourceRequirements{
									Limits: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("2"),
										corev1.ResourceMemory: resource.MustParse("700Mi"),
									},
									Requests: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("1"),
										corev1.ResourceMemory: resource.MustParse("256Mi"),
									},
								},
								Tolerations: []corev1.Toleration{
									{
										Key:      "key1",
										Operator: "Equal",
										Value:    "value1",
										Effect:   "NoSchedule",
									},
								},
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
						"component":                    "velero",
					},
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "Deployment",
					APIVersion: appsv1.SchemeGroupVersion.String(),
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app.kubernetes.io/name":       common.Velero,
							"app.kubernetes.io/instance":   "test-Velero-CR",
							"app.kubernetes.io/managed-by": common.OADPOperator,
							"app.kubernetes.io/component":  Server,
							oadpv1alpha1.OadpOperatorLabel: "True",
							"component":                    "velero",
							"deploy":                       "velero",
						},
					},
					Replicas: pointer.Int32(1),
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app.kubernetes.io/name":       common.Velero,
								"app.kubernetes.io/instance":   "test-Velero-CR",
								"app.kubernetes.io/managed-by": common.OADPOperator,
								"app.kubernetes.io/component":  Server,
								oadpv1alpha1.OadpOperatorLabel: "True",
								"component":                    "velero",
								"deploy":                       "velero",
							},
							Annotations: map[string]string{
								"prometheus.io/scrape": "true",
								"prometheus.io/port":   "8085",
								"prometheus.io/path":   "/metrics",
							},
						},
						Spec: corev1.PodSpec{
							RestartPolicy:      corev1.RestartPolicyAlways,
							ServiceAccountName: common.Velero,
							Tolerations: []corev1.Toleration{
								{
									Key:      "key1",
									Operator: "Equal",
									Value:    "value1",
									Effect:   "NoSchedule",
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
			name: "given valid DPA CR, velero deployment nodeselector",
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
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app.kubernetes.io/name":       common.Velero,
							"app.kubernetes.io/instance":   "test-Velero-CR",
							"app.kubernetes.io/managed-by": common.OADPOperator,
							"app.kubernetes.io/component":  Server,
							oadpv1alpha1.OadpOperatorLabel: "True",
							"component":                    "velero",
							"deploy":                       "velero",
						},
					},
				},
			},
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							PodConfig: &oadpv1alpha1.PodConfig{
								ResourceAllocations: corev1.ResourceRequirements{
									Limits: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("2"),
										corev1.ResourceMemory: resource.MustParse("700Mi"),
									},
									Requests: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("1"),
										corev1.ResourceMemory: resource.MustParse("256Mi"),
									},
								},
								NodeSelector: map[string]string{
									"foo": "bar",
								},
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
						"component":                    "velero",
					},
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "Deployment",
					APIVersion: appsv1.SchemeGroupVersion.String(),
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app.kubernetes.io/name":       common.Velero,
							"app.kubernetes.io/instance":   "test-Velero-CR",
							"app.kubernetes.io/managed-by": common.OADPOperator,
							"app.kubernetes.io/component":  Server,
							oadpv1alpha1.OadpOperatorLabel: "True",
							"component":                    "velero",
							"deploy":                       "velero",
						},
					},
					Replicas: pointer.Int32(1),
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app.kubernetes.io/name":       common.Velero,
								"app.kubernetes.io/instance":   "test-Velero-CR",
								"app.kubernetes.io/managed-by": common.OADPOperator,
								"app.kubernetes.io/component":  Server,
								oadpv1alpha1.OadpOperatorLabel: "True",
								"component":                    "velero",
								"deploy":                       "velero",
							},
							Annotations: map[string]string{
								"prometheus.io/scrape": "true",
								"prometheus.io/port":   "8085",
								"prometheus.io/path":   "/metrics",
							},
						},
						Spec: corev1.PodSpec{
							RestartPolicy:      corev1.RestartPolicyAlways,
							ServiceAccountName: common.Velero,
							NodeSelector: map[string]string{
								"foo": "bar",
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
			name: "given valid DPA CR, appropriate velero deployment is build with aws plugin specific specs",
			veleroDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-velero-deployment",
					Namespace: "test-ns",
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app.kubernetes.io/name":       common.Velero,
							"app.kubernetes.io/instance":   "test-Velero-CR",
							"app.kubernetes.io/managed-by": common.OADPOperator,
							"app.kubernetes.io/component":  Server,
							"component":                    "velero",
							"deploy":                       "velero",
							oadpv1alpha1.OadpOperatorLabel: "True",
						},
					},
				},
			},
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
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
						"component":                    "velero",
						oadpv1alpha1.OadpOperatorLabel: "True",
					},
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "Deployment",
					APIVersion: appsv1.SchemeGroupVersion.String(),
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app.kubernetes.io/name":       common.Velero,
							"app.kubernetes.io/instance":   "test-Velero-CR",
							"app.kubernetes.io/managed-by": common.OADPOperator,
							"app.kubernetes.io/component":  Server,
							"component":                    "velero",
							"deploy":                       "velero",
							oadpv1alpha1.OadpOperatorLabel: "True",
						},
					},
					Replicas: pointer.Int32(1),
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app.kubernetes.io/name":       common.Velero,
								"app.kubernetes.io/instance":   "test-Velero-CR",
								"app.kubernetes.io/managed-by": common.OADPOperator,
								"app.kubernetes.io/component":  Server,
								"component":                    "velero",
								"deploy":                       "velero",
								oadpv1alpha1.OadpOperatorLabel: "True",
							},
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
			name: "given valid DPA CR, appropriate velero deployment is build with aws and kubevirt plugin specific specs",
			veleroDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-velero-deployment",
					Namespace: "test-ns",
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app.kubernetes.io/name":       common.Velero,
							"app.kubernetes.io/instance":   "test-Velero-CR",
							"app.kubernetes.io/managed-by": common.OADPOperator,
							"app.kubernetes.io/component":  Server,
							"component":                    "velero",
							"deploy":                       "velero",
							oadpv1alpha1.OadpOperatorLabel: "True",
						},
					},
				},
			},
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
								oadpv1alpha1.DefaultPluginKubeVirt,
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
						"component":                    "velero",
						oadpv1alpha1.OadpOperatorLabel: "True",
					},
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "Deployment",
					APIVersion: appsv1.SchemeGroupVersion.String(),
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app.kubernetes.io/name":       common.Velero,
							"app.kubernetes.io/instance":   "test-Velero-CR",
							"app.kubernetes.io/managed-by": common.OADPOperator,
							"app.kubernetes.io/component":  Server,
							"component":                    "velero",
							"deploy":                       "velero",
							oadpv1alpha1.OadpOperatorLabel: "True",
						},
					},
					Replicas: pointer.Int32(1),
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app.kubernetes.io/name":       common.Velero,
								"app.kubernetes.io/instance":   "test-Velero-CR",
								"app.kubernetes.io/managed-by": common.OADPOperator,
								"app.kubernetes.io/component":  Server,
								"component":                    "velero",
								"deploy":                       "velero",
								oadpv1alpha1.OadpOperatorLabel: "True",
							},
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
								{
									Image:                    common.KubeVirtPluginImage,
									Name:                     common.KubeVirtPlugin,
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
			name: "given valid DPA CR with annotations, appropriate velero deployment is build with aws plugin specific specs",
			veleroDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-velero-deployment",
					Namespace: "test-ns",
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app.kubernetes.io/name":       common.Velero,
							"app.kubernetes.io/instance":   "test-Velero-CR",
							"app.kubernetes.io/managed-by": common.OADPOperator,
							"app.kubernetes.io/component":  Server,
							"component":                    "velero",
							"deploy":                       "velero",
							oadpv1alpha1.OadpOperatorLabel: "True",
						},
					},
				},
			},
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
							},
						},
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
						"component":                    "velero",
						oadpv1alpha1.OadpOperatorLabel: "True",
					},
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "Deployment",
					APIVersion: appsv1.SchemeGroupVersion.String(),
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app.kubernetes.io/name":       common.Velero,
							"app.kubernetes.io/instance":   "test-Velero-CR",
							"app.kubernetes.io/managed-by": common.OADPOperator,
							"app.kubernetes.io/component":  Server,
							"component":                    "velero",
							"deploy":                       "velero",
							oadpv1alpha1.OadpOperatorLabel: "True",
						},
					},
					Replicas: pointer.Int32(1),
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app.kubernetes.io/name":       common.Velero,
								"app.kubernetes.io/instance":   "test-Velero-CR",
								"app.kubernetes.io/managed-by": common.OADPOperator,
								"app.kubernetes.io/component":  Server,
								"component":                    "velero",
								"deploy":                       "velero",
								oadpv1alpha1.OadpOperatorLabel: "True",
							},
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
			name: "given valid DPA CR with PodDNS Policy/Config, annotations, appropriate velero deployment is build with aws plugin specific specs",
			veleroDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-velero-deployment",
					Namespace: "test-ns",
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app.kubernetes.io/name":       common.Velero,
							"app.kubernetes.io/instance":   "test-Velero-CR",
							"app.kubernetes.io/managed-by": common.OADPOperator,
							"app.kubernetes.io/component":  Server,
							"component":                    "velero",
							"deploy":                       "velero",
							oadpv1alpha1.OadpOperatorLabel: "True",
						},
					},
				},
			},
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
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
						"component":                    "velero",
						oadpv1alpha1.OadpOperatorLabel: "True",
					},
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "Deployment",
					APIVersion: appsv1.SchemeGroupVersion.String(),
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app.kubernetes.io/name":       common.Velero,
							"app.kubernetes.io/instance":   "test-Velero-CR",
							"app.kubernetes.io/managed-by": common.OADPOperator,
							"app.kubernetes.io/component":  Server,
							"component":                    "velero",
							"deploy":                       "velero",
							oadpv1alpha1.OadpOperatorLabel: "True",
						},
					},
					Replicas: pointer.Int32(1),
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app.kubernetes.io/name":       common.Velero,
								"app.kubernetes.io/instance":   "test-Velero-CR",
								"app.kubernetes.io/managed-by": common.OADPOperator,
								"app.kubernetes.io/component":  Server,
								"component":                    "velero",
								"deploy":                       "velero",
								oadpv1alpha1.OadpOperatorLabel: "True",
							},
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
			name: "given valid Velero CR with with aws plugin from bucket",
			veleroDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-velero-deployment",
					Namespace: "test-ns",
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app.kubernetes.io/name":       common.Velero,
							"app.kubernetes.io/instance":   "test-Velero-CR",
							"app.kubernetes.io/managed-by": common.OADPOperator,
							"app.kubernetes.io/component":  Server,
							"component":                    "velero",
							"deploy":                       "velero",
							oadpv1alpha1.OadpOperatorLabel: "True",
						},
					},
				},
			},
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
							},
						},
					},
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							CloudStorage: &oadpv1alpha1.CloudStorageLocation{
								CloudStorageRef: corev1.LocalObjectReference{
									Name: "bucket-123",
								},
								Config: map[string]string{},
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cloud-credentials",
									},
									Key: "creds",
								},
								Default:          false,
								BackupSyncPeriod: &metav1.Duration{},
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
						"component":                    "velero",
						oadpv1alpha1.OadpOperatorLabel: "True",
					},
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "Deployment",
					APIVersion: appsv1.SchemeGroupVersion.String(),
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app.kubernetes.io/name":       common.Velero,
							"app.kubernetes.io/instance":   "test-Velero-CR",
							"app.kubernetes.io/managed-by": common.OADPOperator,
							"app.kubernetes.io/component":  Server,
							"component":                    "velero",
							"deploy":                       "velero",
							oadpv1alpha1.OadpOperatorLabel: "True",
						},
					},
					Replicas: pointer.Int32(1),
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app.kubernetes.io/name":       common.Velero,
								"app.kubernetes.io/instance":   "test-Velero-CR",
								"app.kubernetes.io/managed-by": common.OADPOperator,
								"app.kubernetes.io/component":  Server,
								"component":                    "velero",
								"deploy":                       "velero",
								oadpv1alpha1.OadpOperatorLabel: "True",
							},
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
											Name:      "bound-sa-token",
											MountPath: "/var/run/secrets/openshift/serviceaccount",
											ReadOnly:  true,
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
									Name: "bound-sa-token",
									VolumeSource: corev1.VolumeSource{
										Projected: &corev1.ProjectedVolumeSource{
											DefaultMode: pointer.Int32(420),
											Sources: []corev1.VolumeProjection{
												{
													ServiceAccountToken: &corev1.ServiceAccountTokenProjection{
														Audience:          "openshift",
														ExpirationSeconds: pointer.Int64(3600),
														Path:              "token",
													},
												},
											},
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
			clientObjects: []client.Object{
				&oadpv1alpha1.CloudStorage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "bucket-123",
						Namespace: "test-ns",
					},
					Spec: oadpv1alpha1.CloudStorageSpec{
						EnableSharedConfig: &trueVal,
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient, err := getFakeClientFromObjects(tt.clientObjects...)
			if err != nil {
				t.Errorf("error in creating fake client, likely programmer error")
			}
			r := DPAReconciler{
				Client: fakeClient,
			}
			if tt.testProxy {
				os.Setenv(proxyEnvKey, proxyEnvValue)
				defer os.Unsetenv(proxyEnvKey)
			}
			if err := r.buildVeleroDeployment(tt.veleroDeployment, tt.dpa); err != nil {
				if !tt.wantErr {
					t.Errorf("buildVeleroDeployment() error = %v, wantErr %v", err, tt.wantErr)
				}
				if tt.wantErr && tt.wantVeleroDeployment == nil {
					// if we expect an error and we got one, and wantVeleroDeployment is not defined, we don't need to compare further.
					t.Skip()
				}
			}
			if !reflect.DeepEqual(tt.wantVeleroDeployment, tt.veleroDeployment) {
				t.Errorf("expected velero deployment spec to be \n%#v, got \n%#v", tt.wantVeleroDeployment, tt.veleroDeployment)
			}
		})
	}
}

func TestDPAReconciler_getVeleroImage(t *testing.T) {
	tests := []struct {
		name       string
		DpaCR      *oadpv1alpha1.DataProtectionApplication
		pluginName string
		wantImage  string
		setEnvVars map[string]string
	}{
		{
			name: "given Velero image override, custom Velero image should be returned",
			DpaCR: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
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
			name: "given default DPA CR with no env var, default velero image should be returned",
			DpaCR: &oadpv1alpha1.DataProtectionApplication{
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
			name: "given default DPA CR with env var set, image should be built via env vars",
			DpaCR: &oadpv1alpha1.DataProtectionApplication{
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
			gotImage := getVeleroImage(tt.DpaCR)
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
		dpa     *oadpv1alpha1.DataProtectionApplication
		secret  *corev1.Secret
		wantErr bool
		want    bool
	}{

		{
			name: "given valid Velero default plugin, default secret gets mounted as volume mounts",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
							},
						},
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
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginOpenShift,
							},
						},
					},
				},
			},
			secret:  &corev1.Secret{},
			wantErr: false,
			want:    true,
		},
		{
			name: "given valid multiple Velero default plugins, default secrets gets mounted for each plugin if applicable",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
								oadpv1alpha1.DefaultPluginOpenShift,
							},
						},
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
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
							},
						},
					},
				},
			},
			secret:  &corev1.Secret{},
			wantErr: true,
			want:    false,
		},
	}
	for _, tt := range tests {
		fakeClient, err := getFakeClientFromObjects(tt.dpa, tt.secret)
		if err != nil {
			t.Errorf("error in creating fake client, likely programmer error")
		}
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

var allDefaultPluginsList = []oadpv1alpha1.DefaultPlugin{
	oadpv1alpha1.DefaultPluginAWS,
	oadpv1alpha1.DefaultPluginGCP,
	oadpv1alpha1.DefaultPluginMicrosoftAzure,
	oadpv1alpha1.DefaultPluginKubeVirt,
	oadpv1alpha1.DefaultPluginOpenShift,
	oadpv1alpha1.DefaultPluginCSI,
}

func TestDPAReconciler_noDefaultCredentials(t *testing.T) {
	type args struct {
		dpa oadpv1alpha1.DataProtectionApplication
	}
	tests := []struct {
		name                string
		args                args
		want                map[string]bool
		wantHasCloudStorage bool
		wantErr             bool
	}{
		{
			name: "dpa with all plugins but with noDefualtBackupLocation should not require default credentials",
			args: args{
				dpa: oadpv1alpha1.DataProtectionApplication{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-Velero-CR",
						Namespace: "test-ns",
					},
					Spec: oadpv1alpha1.DataProtectionApplicationSpec{
						Configuration: &oadpv1alpha1.ApplicationConfig{
							Velero: &oadpv1alpha1.VeleroConfig{
								DefaultPlugins:          allDefaultPluginsList,
								NoDefaultBackupLocation: true,
							},
						},
					},
				},
			},
			want: map[string]bool{
				"velero-plugin-for-aws":             false,
				"velero-plugin-for-gcp":             false,
				"velero-plugin-for-microsoft-azure": false,
			},
			wantHasCloudStorage: false,
			wantErr:             false,
		},
		{
			name: "dpa no default cloudprovider plugins should not require default credentials",
			args: args{
				dpa: oadpv1alpha1.DataProtectionApplication{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-Velero-CR",
						Namespace: "test-ns",
					},
					Spec: oadpv1alpha1.DataProtectionApplicationSpec{
						Configuration: &oadpv1alpha1.ApplicationConfig{
							Velero: &oadpv1alpha1.VeleroConfig{
								DefaultPlugins:          []oadpv1alpha1.DefaultPlugin{oadpv1alpha1.DefaultPluginOpenShift},
								NoDefaultBackupLocation: true,
							},
						},
					},
				},
			},
			want:                map[string]bool{},
			wantHasCloudStorage: false,
			wantErr:             false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient, err := getFakeClientFromObjects(&tt.args.dpa)
			if err != nil {
				t.Errorf("error in creating fake client, likely programmer error")
			}
			r := DPAReconciler{
				Client: fakeClient,
			}
			got, got1, err := r.noDefaultCredentials(tt.args.dpa)
			if (err != nil) != tt.wantErr {
				t.Errorf("DPAReconciler.noDefaultCredentials() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("DPAReconciler.noDefaultCredentials() got = \n%v, \nwant \n%v", got, tt.want)
			}
			if got1 != tt.wantHasCloudStorage {
				t.Errorf("DPAReconciler.noDefaultCredentials() got1 = %v, want %v", got1, tt.wantHasCloudStorage)
			}
		})
	}
}
