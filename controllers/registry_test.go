package controllers

import (
	"github.com/go-logr/logr"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	//"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"testing"

	"github.com/openshift/oadp-operator/pkg/common"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"
)

func getSchemeForFakeClientForRegistry() (*runtime.Scheme, error) {
	err := oadpv1alpha1.AddToScheme(scheme.Scheme)
	if err != nil {
		return nil, err
	}

	err = velerov1.AddToScheme(scheme.Scheme)
	if err != nil {
		return nil, err
	}

	return scheme.Scheme, nil
}

func getFakeClientFromObjectsForRegistry(objs ...client.Object) (client.WithWatch, error) {
	schemeForFakeClient, err := getSchemeForFakeClientForRegistry()
	if err != nil {
		return nil, err
	}

	return fake.NewClientBuilder().WithScheme(schemeForFakeClient).WithObjects(objs...).Build(), nil
}

const (
	testAccessKey       = "someAccessKey"
	testSecretAccessKey = "someSecretAccessKey"
)

var (
	secretData = map[string][]byte{
		"cloud": []byte("[default]" + "\n" +
			"aws_access_key_id=" + testAccessKey + "\n" +
			"aws_secret_access_key=" + testSecretAccessKey),
	}
)

func TestVeleroReconciler_buildRegistryDeployment(t *testing.T) {
	tests := []struct {
		name               string
		registryDeployment *appsv1.Deployment
		bsl                *velerov1.BackupStorageLocation
		secret             *corev1.Secret
		wantErr            bool
	}{
		{
			name: "given a valid bsl get appropriate registry deployment",
			registryDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-registry",
					Namespace: "test-ns",
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"component": "oadp-" + "test-bsl" + "-" + "aws" + "-registry",
						},
					},
				},
			},
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
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials",
					Namespace: "test-ns",
				},
				Data: secretData,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient, err := getFakeClientFromObjectsForRegistry(tt.secret, tt.registryDeployment, tt.bsl)
			if err != nil {
				t.Errorf("error in creating fake client, likely programmer error")
			}
			r := &VeleroReconciler{
				Client:  fakeClient,
				Scheme:  fakeClient.Scheme(),
				Log:     logr.Discard(),
				Context: newContextForTest(tt.name),
				NamespacedName: types.NamespacedName{
					Namespace: tt.registryDeployment.Namespace,
					Name:      tt.registryDeployment.Name,
				},
				EventRecorder: record.NewFakeRecorder(10),
			}
			wantRegistryDeployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-registry",
					Namespace: "test-ns",
					Labels: map[string]string{
						"app.kubernetes.io/name":       common.OADPOperatorVelero,
						"app.kubernetes.io/instance":   "oadp-" + tt.bsl.Name + "-" + tt.bsl.Spec.Provider + "-registry",
						"app.kubernetes.io/managed-by": common.OADPOperator,
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
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"component": "oadp-" + tt.bsl.Name + "-" + tt.bsl.Spec.Provider + "-registry",
						},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"component": "oadp-" + tt.bsl.Name + "-" + tt.bsl.Spec.Provider + "-registry",
							},
						},
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyAlways,
							Containers: []corev1.Container{
								{
									Image: RegistryImage,
									Name:  "oadp-" + tt.bsl.Name + "-" + tt.bsl.Spec.Provider + "-registry" + "-container",
									Ports: []corev1.ContainerPort{
										{
											ContainerPort: 5000,
											Protocol:      corev1.ProtocolTCP,
										},
									},
									Env: []corev1.EnvVar{
										{
											Name:  RegistryStorageEnvVarKey,
											Value: S3,
										},
										{
											Name:  RegistryStorageS3AccesskeyEnvVarKey,
											Value: testAccessKey,
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
											Value: testSecretAccessKey,
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
								},
							},
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
			if !reflect.DeepEqual(wantRegistryDeployment.Spec, tt.registryDeployment.Spec) {
				t.Errorf("expected registry deployment spec to be %#v, got %#v", wantRegistryDeployment, tt.registryDeployment)
			}
		})
	}
}

func TestVeleroReconciler_buildRegistryContainer(t *testing.T) {
	tests := []struct {
		name                  string
		bsl                   *velerov1.BackupStorageLocation
		wantRegistryContainer *corev1.Container
		wantErr               bool
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

			gotRegistryContainer, gotErr := r.buildRegistryContainer(tt.bsl)

			if (gotErr != nil) != tt.wantErr {
				t.Errorf("ValidateBackupStorageLocations() gotErr = %v, wantErr %v", gotErr, tt.wantErr)
				return
			}

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
		secret                      *corev1.Secret
		wantErr                     bool
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
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials",
					Namespace: "test-ns",
				},
				Data: secretData,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient, err := getFakeClientFromObjectsForRegistry(tt.secret, tt.bsl)
			if err != nil {
				t.Errorf("error in creating fake client, likely programmer error")
			}
			r := &VeleroReconciler{
				Client:  fakeClient,
				Scheme:  fakeClient.Scheme(),
				Log:     logr.Discard(),
				Context: newContextForTest(tt.name),
				NamespacedName: types.NamespacedName{
					Namespace: tt.bsl.Namespace,
					Name:      tt.bsl.Name,
				},
				EventRecorder: record.NewFakeRecorder(10),
			}
			tt.wantRegistryContainerEnvVar = []corev1.EnvVar{
				{
					Name:  RegistryStorageEnvVarKey,
					Value: S3,
				},
				{
					Name:  RegistryStorageS3AccesskeyEnvVarKey,
					Value: testAccessKey,
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
					Value: testSecretAccessKey,
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

			gotRegistryContainerEnvVar, gotErr := r.getAWSRegistryEnvVars(tt.bsl, testAWSEnvVar)

			if (gotErr != nil) != tt.wantErr {
				t.Errorf("ValidateBackupStorageLocations() gotErr = %v, wantErr %v", gotErr, tt.wantErr)
				return
			}

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
