package controllers

import (
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	"reflect"
	"testing"
)

func TestVeleroReconciler_buildRegistryDeployment(t *testing.T) {
	tests := []struct {
		name               string
		registryDeployment *appsv1.Deployment
		bsl                *velerov1.BackupStorageLocation
		wantErr            bool
	}{
		{
			name: "registry without owner reference as well as labels",
			registryDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-registry",
					Namespace: "test-ns",
				},
			},
			bsl: &velerov1.BackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-bsl",
					Namespace: "test-ns",
				},
			},
		},
		{
			name: "registry without owner reference but has labels",
			registryDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-registry",
					Namespace: "test-ns",
					Labels: map[string]string{
						"app.kubernetes.io/name":       OADPOperatorVelero,
						"app.kubernetes.io/instance":   "oadp-test-bsl-test-ns-registry",
						"app.kubernetes.io/managed-by": OADPOperator,
						"app.kubernetes.io/component":  Registry,
					},
				},
			},
			bsl: &velerov1.BackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-bsl",
					Namespace: "test-ns",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme, err := getSchemeForFakeClient()
			if err != nil {
				t.Errorf("error getting scheme for the test: %#v", err)
			}
			r := &VeleroReconciler{
				Scheme: scheme,
			}
			wantRegistryDeployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-registry",
					Namespace: "test-ns",
					Labels: map[string]string{
						"app.kubernetes.io/name":       OADPOperatorVelero,
						"app.kubernetes.io/instance":   "oadp-" + tt.bsl.Name + "-" + tt.bsl.Spec.Provider + "-registry",
						"app.kubernetes.io/managed-by": OADPOperator,
						"app.kubernetes.io/component":  Registry,
					},
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         "velero.io/v1",
							Kind:               "BackupStorageLocation",
							Name:               tt.bsl.Name,
							UID:                tt.bsl.UID,
							Controller:         pointer.BoolPtr(true),
							BlockOwnerDeletion: pointer.BoolPtr(true),
						},
					},
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: pointer.Int32(1),
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"component": "oadp-" + tt.bsl.Name + "-" + tt.bsl.Spec.Provider + "-registry",
							},
						},
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyAlways,
						},
					},
				},
			}

			err = r.buildRegistryDeployment(tt.registryDeployment, tt.bsl)
			if (err != nil) != tt.wantErr {
				t.Errorf("buildRegistryDeployment() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(wantRegistryDeployment.Labels, tt.registryDeployment.Labels) {
				t.Errorf("expected registry deployment labels to be %#v, got %#v", wantRegistryDeployment.Labels, tt.registryDeployment.Labels)
			}
			if !reflect.DeepEqual(wantRegistryDeployment.OwnerReferences, tt.registryDeployment.OwnerReferences) {
				t.Errorf("expected registry deployment owner references to be %#v, got %#v", wantRegistryDeployment.OwnerReferences, tt.registryDeployment.OwnerReferences)
			}
			if !reflect.DeepEqual(wantRegistryDeployment.Spec.Replicas, tt.registryDeployment.Spec.Replicas) {
				t.Errorf("expected registry deployment replicas to be %#v, got %#v", wantRegistryDeployment.Spec.Replicas, tt.registryDeployment.Spec.Replicas)
			}
		})
	}
}

func TestVeleroReconciler_buildRegistryContainer(t *testing.T) {
	tests := []struct {
		name                  string
		bsl                   *velerov1.BackupStorageLocation
		wantRegistryContainer *corev1.Container
	}{
		{
			name: "given bsl appropriate container is built or not",
			bsl: &velerov1.BackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-bsl",
					Namespace: "test-ns",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme, err := getSchemeForFakeClient()
			if err != nil {
				t.Errorf("error getting scheme for the test: %#v", err)
			}
			r := &VeleroReconciler{
				Scheme: scheme,
			}
			tt.wantRegistryContainer = &corev1.Container{
				Image: RegistryImage,
				Name:  "oadp-" + tt.bsl.Name + "-" + tt.bsl.Spec.Provider + "-registry" + "-container",
				Ports: []corev1.ContainerPort{
					{
						ContainerPort: 5000,
						Protocol:      corev1.ProtocolTCP,
					},
				},
				LivenessProbe: &corev1.Probe{
					Handler: corev1.Handler{
						HTTPGet: &corev1.HTTPGetAction{
							Path: "/v2/_catalog?n=5",
							Port: intstr.IntOrString{IntVal: 5000},
						},
					},
					PeriodSeconds:       5,
					TimeoutSeconds:      3,
					InitialDelaySeconds: 15,
				},
				ReadinessProbe: &corev1.Probe{
					Handler: corev1.Handler{
						HTTPGet: &corev1.HTTPGetAction{
							Path: "/v2/_catalog?n=5",
							Port: intstr.IntOrString{IntVal: 5000},
						},
					},
					PeriodSeconds:       5,
					TimeoutSeconds:      3,
					InitialDelaySeconds: 15,
				},
			}

			gotRegistryContainer := r.buildRegistryContainer(tt.bsl)

			if !reflect.DeepEqual(tt.wantRegistryContainer.Name, gotRegistryContainer[0].Name) {
				t.Errorf("expected registry container name to be %#v, got %#v", tt.wantRegistryContainer.Name, gotRegistryContainer[0].Name)
			}

			if !reflect.DeepEqual(tt.wantRegistryContainer.Ports, gotRegistryContainer[0].Ports) {
				t.Errorf("expected registry container ports to be %#v, got %#v", tt.wantRegistryContainer.Ports, gotRegistryContainer[0].Ports)
			}
			if !reflect.DeepEqual(tt.wantRegistryContainer.ReadinessProbe, gotRegistryContainer[0].ReadinessProbe) {
				t.Errorf("expected registry container readiness probe to be %#v, got %#v", tt.wantRegistryContainer.ReadinessProbe, gotRegistryContainer[0].ReadinessProbe)
			}
			if !reflect.DeepEqual(tt.wantRegistryContainer.LivenessProbe, gotRegistryContainer[0].LivenessProbe) {
				t.Errorf("expected registry container liveness probe to be %#v, got %#v", tt.wantRegistryContainer.LivenessProbe, gotRegistryContainer[0].LivenessProbe)
			}

		})
	}
}

var testAWSEnvVar = cloudProviderEnvVarMap["aws"]
var testAzureEnvVar = cloudProviderEnvVarMap["azure"]
var testGCPEnvVar = cloudProviderEnvVarMap["gcp"]

func TestVeleroReconciler_getAWSRegistryEnvVars(t *testing.T) {
	tests := []struct {
		name                        string
		bsl                         *velerov1.BackupStorageLocation
		wantRegistryContainerEnvVar []corev1.EnvVar
	}{
		{
			name: "given aws bsl, appropriate env var for the container are returned",
			bsl: &velerov1.BackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-bsl",
					Namespace: "test-ns",
				},
				Spec: velerov1.BackupStorageLocationSpec{
					Provider: AWSProvider,
					StorageType: velerov1.StorageType{
						ObjectStorage: &velerov1.ObjectStorageLocation{
							Bucket: "aws-bucket",
						},
					},
					Config: map[string]string{
						Region:                "aws-region",
						S3URL:                 "https://sr-url-aws-domain.com",
						RootDirectory:         "/velero-aws",
						InsecureSkipTLSVerify: "false",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme, err := getSchemeForFakeClient()
			if err != nil {
				t.Errorf("error getting scheme for the test: %#v", err)
			}
			r := &VeleroReconciler{
				Scheme: scheme,
			}
			tt.wantRegistryContainerEnvVar = []corev1.EnvVar{
				{
					Name:  RegistryStorageEnvVarKey,
					Value: S3,
				},
				{
					Name:  RegistryStorageS3AccesskeyEnvVarKey,
					Value: "",
				},
				{
					Name:  RegistryStorageS3BucketEnvVarKey,
					Value: "aws-bucket",
				},
				{
					Name:  RegistryStorageS3RegionEnvVarKey,
					Value: "aws-region",
				},
				{
					Name:  RegistryStorageS3SecretkeyEnvVarKey,
					Value: "",
				},
				{
					Name:  RegistryStorageS3RegionendpointEnvVarKey,
					Value: "https://sr-url-aws-domain.com",
				},
				{
					Name:  RegistryStorageS3RootdirectoryEnvVarKey,
					Value: "/velero-aws",
				},
				{
					Name:  RegistryStorageS3SkipverifyEnvVarKey,
					Value: "false",
				},
			}

			gotRegistryContainerEnvVar := r.getAWSRegistryEnvVars(tt.bsl, testAWSEnvVar)

			if !reflect.DeepEqual(tt.wantRegistryContainerEnvVar, gotRegistryContainerEnvVar) {
				t.Errorf("expected registry container env var to be %#v, got %#v", tt.wantRegistryContainerEnvVar, gotRegistryContainerEnvVar)
			}
		})
	}
}

func TestVeleroReconciler_getAzureRegistryEnvVars(t *testing.T) {
	tests := []struct {
		name                        string
		bsl                         *velerov1.BackupStorageLocation
		wantRegistryContainerEnvVar []corev1.EnvVar
	}{
		{
			name: "given azure bsl, appropriate env var for the container are returned",
			bsl: &velerov1.BackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-bsl",
					Namespace: "test-ns",
				},
				Spec: velerov1.BackupStorageLocationSpec{
					Provider: AzureProvider,
					StorageType: velerov1.StorageType{
						ObjectStorage: &velerov1.ObjectStorageLocation{
							Bucket: "azure-bucket",
						},
					},
					Config: map[string]string{
						StorageAccount: "velero-azure-account",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme, err := getSchemeForFakeClient()
			if err != nil {
				t.Errorf("error getting scheme for the test: %#v", err)
			}
			r := &VeleroReconciler{
				Scheme: scheme,
			}
			tt.wantRegistryContainerEnvVar = []corev1.EnvVar{
				{
					Name:  RegistryStorageEnvVarKey,
					Value: Azure,
				},
				{
					Name:  RegistryStorageAzureContainerEnvVarKey,
					Value: "azure-bucket",
				},
				{
					Name:  RegistryStorageAzureAccountnameEnvVarKey,
					Value: "velero-azure-account",
				},
				{
					Name:  RegistryStorageAzureAccountkeyEnvVarKey,
					Value: "",
				},
			}

			gotRegistryContainerEnvVar := r.getAzureRegistryEnvVars(tt.bsl, testAzureEnvVar)

			if !reflect.DeepEqual(tt.wantRegistryContainerEnvVar, gotRegistryContainerEnvVar) {
				t.Errorf("expected registry container env var to be %#v, got %#v", tt.wantRegistryContainerEnvVar, gotRegistryContainerEnvVar)
			}
		})
	}
}

func TestVeleroReconciler_getGCPRegistryEnvVars(t *testing.T) {
	tests := []struct {
		name                        string
		bsl                         *velerov1.BackupStorageLocation
		wantRegistryContainerEnvVar []corev1.EnvVar
	}{
		{
			name: "given gcp bsl, appropriate env var for the container are returned",
			bsl: &velerov1.BackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-bsl",
					Namespace: "test-ns",
				},
				Spec: velerov1.BackupStorageLocationSpec{
					Provider: GCPProvider,
					StorageType: velerov1.StorageType{
						ObjectStorage: &velerov1.ObjectStorageLocation{
							Bucket: "gcp-bucket",
						},
					},
					Config: map[string]string{
						RootDirectory: "/velero-gcp",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme, err := getSchemeForFakeClient()
			if err != nil {
				t.Errorf("error getting scheme for the test: %#v", err)
			}
			r := &VeleroReconciler{
				Scheme: scheme,
			}
			tt.wantRegistryContainerEnvVar = []corev1.EnvVar{
				{
					Name:  RegistryStorageEnvVarKey,
					Value: GCS,
				},
				{
					Name:  RegistryStorageGCSBucket,
					Value: "gcp-bucket",
				},
				{
					Name:  RegistryStorageGCSKeyfile,
					Value: "",
				},
				{
					Name:  RegistryStorageGCSRootdirectory,
					Value: "/velero-gcp",
				},
			}

			gotRegistryContainerEnvVar := r.getGCPRegistryEnvVars(tt.bsl, testGCPEnvVar)

			if !reflect.DeepEqual(tt.wantRegistryContainerEnvVar, gotRegistryContainerEnvVar) {
				t.Errorf("expected registry container env var to be %#v, got %#v", tt.wantRegistryContainerEnvVar, gotRegistryContainerEnvVar)
			}
		})
	}
}
