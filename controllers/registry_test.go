package controllers

import (
	"github.com/go-logr/logr"
	routev1 "github.com/openshift/api/route/v1"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	//"k8s.io/apimachinery/pkg/types"
	"reflect"
	"testing"

	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

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
	testProfile            = "someProfile"
	testAccessKey          = "someAccessKey"
	testSecretAccessKey    = "someSecretAccessKey"
	testStoragekey         = "someStorageKey"
	testCloudName          = "someCloudName"
	testBslProfile         = "bslProfile"
	testBslAccessKey       = "bslAccessKey"
	testBslSecretAccessKey = "bslSecretAccessKey"
	testSubscriptionID     = "someSubscriptionID"
	testTenantID           = "someTenantID"
	testClientID           = "someClientID"
	testClientSecret       = "someClientSecret"
	testResourceGroup      = "someResourceGroup"
)

var (
	secretData = map[string][]byte{
		"cloud": []byte(
			"\n[" + testBslProfile + "]\n" +
				"aws_access_key_id=" + testBslAccessKey + "\n" +
				"aws_secret_access_key=" + testBslSecretAccessKey +
				"\n[default]" + "\n" +
				"aws_access_key_id=" + testAccessKey + "\n" +
				"aws_secret_access_key=" + testSecretAccessKey +
				"\n[test-profile]\n" +
				"aws_access_key_id=" + testAccessKey + "\n" +
				"aws_secret_access_key=" + testSecretAccessKey,
		),
	}
	awsSecretDataWithMissingProfile = map[string][]byte{
		"cloud": []byte(
			"[default]" + "\n" +
				"aws_access_key_id=" + testAccessKey + "\n" +
				"aws_secret_access_key=" + testSecretAccessKey +
				"\n[test-profile]\n" +
				"aws_access_key_id=" + testAccessKey + "\n" +
				"aws_secret_access_key=" + testSecretAccessKey,
		),
	}
	secretAzureData = map[string][]byte{
		"cloud": []byte("[default]" + "\n" +
			"AZURE_STORAGE_ACCOUNT_ACCESS_KEY=" + testStoragekey + "\n" +
			"AZURE_CLOUD_NAME=" + testCloudName),
	}
	secretAzureServicePrincipalData = map[string][]byte{
		"cloud": []byte("[default]" + "\n" +
			"AZURE_STORAGE_ACCOUNT_ACCESS_KEY=" + testStoragekey + "\n" +
			"AZURE_CLOUD_NAME=" + testCloudName + "\n" +
			"AZURE_SUBSCRIPTION_ID=" + testSubscriptionID + "\n" +
			"AZURE_TENANT_ID=" + testTenantID + "\n" +
			"AZURE_CLIENT_ID=" + testClientID + "\n" +
			"AZURE_CLIENT_SECRET=" + testClientSecret + "\n" +
			"AZURE_RESOURCE_GROUP=" + testResourceGroup),
	}
	awsRegistrySecretData = map[string][]byte{
		"access_key": []byte(testBslAccessKey),
		"secret_key": []byte(testBslSecretAccessKey),
	}
	azureRegistrySecretData = map[string][]byte{
		"client_id_key":       []byte(""),
		"client_secret_key":   []byte(""),
		"resource_group_key":  []byte(""),
		"storage_account_key": []byte(testStoragekey),
		"subscription_id_key": []byte(""),
		"tenant_id_key":       []byte(""),
	}
	azureRegistrySPSecretData = map[string][]byte{
		"client_id_key":       []byte(testClientID),
		"client_secret_key":   []byte(testClientSecret),
		"resource_group_key":  []byte(testResourceGroup),
		"storage_account_key": []byte(testStoragekey),
		"subscription_id_key": []byte(testSubscriptionID),
		"tenant_id_key":       []byte(testTenantID),
	}
)

func TestDPAReconciler_buildRegistryDeployment(t *testing.T) {
	tests := []struct {
		name               string
		registryDeployment *appsv1.Deployment
		bsl                *velerov1.BackupStorageLocation
		secret             *corev1.Secret
		registrySecret     *corev1.Secret
		dpa                *oadpv1alpha1.DataProtectionApplication
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
			registrySecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "oadp-test-bsl-aws-registry-secret",
					Namespace: "test-ns",
				},
				Data: awsRegistrySecretData,
			},
			dpa: &oadpv1alpha1.DataProtectionApplication{},
		},
		{
			name: "given a valid bsl with velero annotation get appropriate registry deployment",
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
			registrySecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "oadp-test-bsl-aws-registry-secret",
					Namespace: "test-ns",
				},
				Data: awsRegistrySecretData,
			},
			dpa: &oadpv1alpha1.DataProtectionApplication{
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					PodAnnotations: map[string]string{
						"test-annotation": "awesome-annotation",
					},
				},
			},
		},
		{
			name: "given a valid bsl with velero annotation and DNS Policy/Config get appropriate registry deployment",
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
			registrySecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "oadp-test-bsl-aws-registry-secret",
					Namespace: "test-ns",
				},
				Data: awsRegistrySecretData,
			},
			dpa: &oadpv1alpha1.DataProtectionApplication{
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					PodAnnotations: map[string]string{
						"test-annotation": "awesome-annotation",
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
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient, err := getFakeClientFromObjectsForRegistry(tt.secret, tt.registryDeployment, tt.bsl, tt.registrySecret)
			if err != nil {
				t.Errorf("error in creating fake client, likely programmer error")
			}
			r := &DPAReconciler{
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
						"app.kubernetes.io/name":             common.OADPOperatorVelero,
						"app.kubernetes.io/instance":         "oadp-" + tt.bsl.Name + "-" + tt.bsl.Spec.Provider + "-registry",
						"app.kubernetes.io/managed-by":       common.OADPOperator,
						"app.kubernetes.io/component":        Registry,
						oadpv1alpha1.OadpOperatorLabel:       "True",
						oadpv1alpha1.RegistryDeploymentLabel: "True",
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
							Annotations: tt.dpa.Spec.PodAnnotations,
						},
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyAlways,
							DNSPolicy:     tt.dpa.Spec.PodDnsPolicy,
							DNSConfig:     &tt.dpa.Spec.PodDnsConfig,
							Containers: []corev1.Container{
								{
									Image: getRegistryImage(tt.dpa),
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
											Name: RegistryStorageS3AccesskeyEnvVarKey,
											ValueFrom: &corev1.EnvVarSource{
												SecretKeyRef: &corev1.SecretKeySelector{
													LocalObjectReference: corev1.LocalObjectReference{Name: "oadp-" + tt.bsl.Name + "-" + tt.bsl.Spec.Provider + "-registry-secret"},
													Key:                  "access_key",
												},
											},
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
											Name: RegistryStorageS3SecretkeyEnvVarKey,
											ValueFrom: &corev1.EnvVarSource{
												SecretKeyRef: &corev1.SecretKeySelector{
													LocalObjectReference: corev1.LocalObjectReference{Name: "oadp-" + tt.bsl.Name + "-" + tt.bsl.Spec.Provider + "-registry-secret"},
													Key:                  "secret_key",
												},
											},
										},
										{
											Name:  RegistryStorageS3RegionendpointEnvVarKey,
											Value: "https://sr-url-aws-domain.com",
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
			if reflect.DeepEqual(tt.dpa.Spec.PodDnsConfig, corev1.PodDNSConfig{}) {
				wantRegistryDeployment.Spec.Template.Spec.DNSConfig = nil
			}

			err = r.buildRegistryDeployment(tt.registryDeployment, tt.bsl, tt.dpa)
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

func TestDPAReconciler_buildRegistryContainer(t *testing.T) {
	tests := []struct {
		name                  string
		bsl                   *velerov1.BackupStorageLocation
		dpa                   *oadpv1alpha1.DataProtectionApplication
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
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "Velero-test-CR",
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
			r := &DPAReconciler{
				Scheme: scheme,
			}
			tt.wantRegistryContainer = &corev1.Container{
				Image: getRegistryImage(tt.dpa),
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

			gotRegistryContainer, gotErr := r.buildRegistryContainer(tt.bsl, tt.dpa)

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

func TestDPAReconciler_getAWSRegistryEnvVars(t *testing.T) {
	tests := []struct {
		name                        string
		bsl                         *velerov1.BackupStorageLocation
		wantRegistryContainerEnvVar []corev1.EnvVar
		wantProfile                 string
		secret                      *corev1.Secret
		registrySecret              *corev1.Secret
		wantErr                     bool
		matchProfile                bool
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
						InsecureSkipTLSVerify: "false",
						Profile:               "test-profile",
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
			registrySecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "oadp-test-bsl-aws-registry-secret",
					Namespace: "test-ns",
				},
				Data: awsRegistrySecretData,
			},
			wantProfile:  "test-profile",
			matchProfile: true,
		}, {
			name: "given aws profile in bsl, appropriate env var for the container are returned",
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
						InsecureSkipTLSVerify: "false",
						Profile:               testBslProfile,
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
			registrySecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "oadp-test-bsl-aws-registry-secret",
					Namespace: "test-ns",
				},
				Data: awsRegistrySecretData,
			},
			wantProfile:  testBslProfile,
			matchProfile: true,
		}, {
			name: "given missing aws profile in bsl, env var should not match",
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
						InsecureSkipTLSVerify: "false",
						Profile:               testBslProfile,
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials",
					Namespace: "test-ns",
				},
				Data: awsSecretDataWithMissingProfile,
			},
			registrySecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "oadp-test-bsl-aws-registry-secret",
					Namespace: "test-ns",
				},
				Data: awsRegistrySecretData,
			},
			wantProfile:  testBslProfile,
			matchProfile: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient, err := getFakeClientFromObjectsForRegistry(tt.secret, tt.bsl, tt.registrySecret)
			if err != nil {
				t.Errorf("error in creating fake client, likely programmer error")
			}
			r := &DPAReconciler{
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
					Name: RegistryStorageS3AccesskeyEnvVarKey,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: "oadp-" + tt.bsl.Name + "-" + tt.bsl.Spec.Provider + "-registry-secret"},
							Key:                  "access_key",
						},
					},
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
					Name: RegistryStorageS3SecretkeyEnvVarKey,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: "oadp-" + tt.bsl.Name + "-" + tt.bsl.Spec.Provider + "-registry-secret"},
							Key:                  "secret_key",
						},
					},
				},
				{
					Name:  RegistryStorageS3RegionendpointEnvVarKey,
					Value: "https://sr-url-aws-domain.com",
				},
				{
					Name:  RegistryStorageS3SkipverifyEnvVarKey,
					Value: "false",
				},
			}
			if tt.wantProfile == testBslProfile {
				tt.wantRegistryContainerEnvVar = []corev1.EnvVar{
					{
						Name:  RegistryStorageEnvVarKey,
						Value: S3,
					},
					{
						Name: RegistryStorageS3AccesskeyEnvVarKey,
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{Name: "oadp-" + tt.bsl.Name + "-" + tt.bsl.Spec.Provider + "-registry-secret"},
								Key:                  "access_key",
							},
						},
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
						Name: RegistryStorageS3SecretkeyEnvVarKey,
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{Name: "oadp-" + tt.bsl.Name + "-" + tt.bsl.Spec.Provider + "-registry-secret"},
								Key:                  "secret_key",
							},
						},
					},
					{
						Name:  RegistryStorageS3RegionendpointEnvVarKey,
						Value: "https://sr-url-aws-domain.com",
					},
					{
						Name:  RegistryStorageS3SkipverifyEnvVarKey,
						Value: "false",
					},
				}
			}

			gotRegistryContainerEnvVar, gotErr := r.getAWSRegistryEnvVars(tt.bsl, testAWSEnvVar)

			if tt.matchProfile && (gotErr != nil) != tt.wantErr {
				t.Errorf("ValidateBackupStorageLocations() gotErr = %v, wantErr %v", gotErr, tt.wantErr)
				return
			}

			if tt.matchProfile && !reflect.DeepEqual(tt.wantRegistryContainerEnvVar, gotRegistryContainerEnvVar) {
				t.Errorf("expected registry container env var to be %#v, got %#v", tt.wantRegistryContainerEnvVar, gotRegistryContainerEnvVar)
			}
		})
	}
}

func TestDPAReconciler_getAzureRegistryEnvVars(t *testing.T) {
	tests := []struct {
		name                        string
		bsl                         *velerov1.BackupStorageLocation
		wantRegistryContainerEnvVar []corev1.EnvVar
		secret                      *corev1.Secret
		registrySecret              *corev1.Secret
		wantErr                     bool
		wantProfile                 string
		matchProfile                bool
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
						StorageAccount:                           "velero-azure-account",
						ResourceGroup:                            testResourceGroup,
						RegistryStorageAzureAccountnameEnvVarKey: "velero-azure-account",
						"storageAccountKeyEnvVar":                "AZURE_STORAGE_ACCOUNT_ACCESS_KEY",
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials-azure",
					Namespace: "test-ns",
				},
				Data: secretAzureData,
			},
			registrySecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "oadp-test-bsl-azure-registry-secret",
					Namespace: "test-ns",
				},
				Data: azureRegistrySecretData,
			},
			wantProfile:  "test-profile",
			matchProfile: true,
		},
		{
			name: "given azure bsl & SP credentials, appropriate env var for the container are returned",
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
						StorageAccount:   "velero-azure-account",
						ResourceGroup:    testResourceGroup,
						"subscriptionId": testSubscriptionID,
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials-azure",
					Namespace: "test-ns",
				},
				Data: secretAzureServicePrincipalData,
			},
			registrySecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "oadp-test-bsl-azure-registry-secret",
					Namespace: "test-ns",
				},
				Data: azureRegistrySPSecretData,
			},
			wantProfile:  "test-sp-profile",
			matchProfile: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient, err := getFakeClientFromObjectsForRegistry(tt.secret, tt.bsl, tt.registrySecret)
			if err != nil {
				t.Errorf("error in creating fake client, likely programmer error")
			}
			r := &DPAReconciler{
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
					Name: RegistryStorageAzureAccountkeyEnvVarKey,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: "oadp-" + tt.bsl.Name + "-" + tt.bsl.Spec.Provider + "-registry-secret"},
							Key:                  "storage_account_key",
						},
					},
				},
				{
					Name:  RegistryStorageAzureAADEndpointEnvVarKey,
					Value: "",
				},
				{
					Name: RegistryStorageAzureSPNClientIDEnvVarKey,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: "oadp-" + tt.bsl.Name + "-" + tt.bsl.Spec.Provider + "-registry-secret"},
							Key:                  "client_id_key",
						},
					},
				},
				{
					Name: RegistryStorageAzureSPNClientSecretEnvVarKey,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: "oadp-" + tt.bsl.Name + "-" + tt.bsl.Spec.Provider + "-registry-secret"},
							Key:                  "client_secret_key",
						},
					},
				},
				{
					Name: RegistryStorageAzureSPNTenantIDEnvVarKey,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: "oadp-" + tt.bsl.Name + "-" + tt.bsl.Spec.Provider + "-registry-secret"},
							Key:                  "tenant_id_key",
						},
					},
				},
			}
			if tt.wantProfile == "test-sp-profile" {
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
						Name: RegistryStorageAzureAccountkeyEnvVarKey,
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{Name: "oadp-" + tt.bsl.Name + "-" + tt.bsl.Spec.Provider + "-registry-secret"},
								Key:                  "storage_account_key",
							},
						},
					},
					{
						Name:  RegistryStorageAzureAADEndpointEnvVarKey,
						Value: "",
					},
					{
						Name: RegistryStorageAzureSPNClientIDEnvVarKey,
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{Name: "oadp-" + tt.bsl.Name + "-" + tt.bsl.Spec.Provider + "-registry-secret"},
								Key:                  "client_id_key",
							},
						},
					},
					{
						Name: RegistryStorageAzureSPNClientSecretEnvVarKey,
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{Name: "oadp-" + tt.bsl.Name + "-" + tt.bsl.Spec.Provider + "-registry-secret"},
								Key:                  "client_secret_key",
							},
						},
					},
					{
						Name: RegistryStorageAzureSPNTenantIDEnvVarKey,
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{Name: "oadp-" + tt.bsl.Name + "-" + tt.bsl.Spec.Provider + "-registry-secret"},
								Key:                  "tenant_id_key",
							},
						},
					},
				}
			}

			gotRegistryContainerEnvVar, gotErr := r.getAzureRegistryEnvVars(tt.bsl, testAzureEnvVar)

			if tt.matchProfile && (gotErr != nil) != tt.wantErr {
				t.Errorf("ValidateBackupStorageLocations() gotErr = %v, wantErr %v", gotErr, tt.wantErr)
				return
			}

			if tt.matchProfile && !reflect.DeepEqual(tt.wantRegistryContainerEnvVar, gotRegistryContainerEnvVar) {
				t.Errorf("expected registry container env var to be %#v, got %#v", tt.wantRegistryContainerEnvVar, gotRegistryContainerEnvVar)
			}
		})
	}
}

func TestDPAReconciler_getGCPRegistryEnvVars(t *testing.T) {
	tests := []struct {
		name                        string
		bsl                         *velerov1.BackupStorageLocation
		wantRegistryContainerEnvVar []corev1.EnvVar
		secret                      *corev1.Secret
		wantErr                     bool
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
					Config: map[string]string{},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials-gcp",
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
			r := &DPAReconciler{
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
					Value: GCS,
				},
				{
					Name:  RegistryStorageGCSBucket,
					Value: "gcp-bucket",
				},
				{
					Name:  RegistryStorageGCSKeyfile,
					Value: "/credentials-gcp/cloud",
				},
			}

			gotRegistryContainerEnvVar, gotErr := r.getGCPRegistryEnvVars(tt.bsl, testGCPEnvVar)

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

func TestDPAReconciler_updateRegistrySVC(t *testing.T) {
	tests := []struct {
		name    string
		svc     *corev1.Service
		bsl     *velerov1.BackupStorageLocation
		dpa     *oadpv1alpha1.DataProtectionApplication
		wantErr bool
	}{
		{
			name:    "Given Velero CR and BSL and SVC instance, appropriate registry svc gets updated",
			wantErr: false,
			bsl: &velerov1.BackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-bsl",
					Namespace: "test-ns",
				},
				Spec: velerov1.BackupStorageLocationSpec{
					Provider: "test-provider",
				},
			},
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "oadp-" + "test-bsl" + "-" + "test-provider" + "-registry-svc",
					Namespace: "test-ns",
				},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{
						"component": "oadp-" + "test-bsl" + "-" + "test-provider" + "-registry",
					},
					Type: corev1.ServiceTypeClusterIP,
					Ports: []corev1.ServicePort{
						{
							Name: "5000-tcp",
							Port: int32(5000),
							TargetPort: intstr.IntOrString{
								IntVal: int32(5000),
							},
							Protocol: corev1.ProtocolTCP,
						},
					},
				},
			},
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "Velero-test-CR",
					Namespace: "test-ns",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient, err := getFakeClientFromObjects(tt.dpa)
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
			wantSVC := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "oadp-" + "test-bsl" + "-" + "test-provider" + "-registry-svc",
					Namespace: "test-ns",
					Labels: map[string]string{
						"component":                    "oadp-" + "test-bsl" + "-" + "test-provider" + "-registry",
						oadpv1alpha1.OadpOperatorLabel: "True",
					},
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion:         oadpv1alpha1.SchemeBuilder.GroupVersion.String(),
						Kind:               "DataProtectionApplication",
						Name:               tt.dpa.Name,
						UID:                tt.dpa.UID,
						Controller:         pointer.BoolPtr(true),
						BlockOwnerDeletion: pointer.BoolPtr(true),
					}},
				},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{
						"component": "oadp-" + "test-bsl" + "-" + "test-provider" + "-registry",
					},
					Ports: []corev1.ServicePort{
						{
							Name: "5000-tcp",
							Port: int32(5000),
							TargetPort: intstr.IntOrString{
								IntVal: int32(5000),
							},
							Protocol: corev1.ProtocolTCP,
						},
					},
					Type: corev1.ServiceTypeClusterIP,
				},
			}
			if err := r.updateRegistrySVC(tt.svc, tt.bsl, tt.dpa); (err != nil) != tt.wantErr {
				t.Errorf("updateRegistrySVC() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(tt.svc, wantSVC) {
				t.Errorf("expected bsl labels to be %#v, got %#v", tt.svc, wantSVC)
			}
		})
	}
}

func TestDPAReconciler_updateRegistryRoute(t *testing.T) {
	tests := []struct {
		name    string
		route   *routev1.Route
		bsl     *velerov1.BackupStorageLocation
		dpa     *oadpv1alpha1.DataProtectionApplication
		wantErr bool
	}{
		{
			name:    "Given DPA CR and BSL and SVC instance, appropriate registry route gets updated",
			wantErr: false,
			bsl: &velerov1.BackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-bsl",
					Namespace: "test-ns",
				},
				Spec: velerov1.BackupStorageLocationSpec{
					Provider: "test-provider",
				},
			},
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "Velero-test-CR",
					Namespace: "test-ns",
				},
			},
			route: &routev1.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "oadp-" + "test-bsl" + "-" + "test-provider" + "-registry-route",
					Namespace: "test-ns",
				},
				Spec: routev1.RouteSpec{
					To: routev1.RouteTargetReference{
						Kind: "Service",
						Name: "oadp-" + "test-bsl" + "-" + "test-provider" + "-registry-svc",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient, err := getFakeClientFromObjects()
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
			wantRoute := &routev1.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "oadp-" + "test-bsl" + "-" + "test-provider" + "-registry-route",
					Namespace: "test-ns",
					Labels: map[string]string{
						"component":                    "oadp-" + "test-bsl" + "-" + "test-provider" + "-registry",
						"service":                      "oadp-" + "test-bsl" + "-" + "test-provider" + "-registry-svc",
						"track":                        "registry-routes",
						oadpv1alpha1.OadpOperatorLabel: "True",
					},
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion:         oadpv1alpha1.SchemeBuilder.GroupVersion.String(),
						Kind:               "DataProtectionApplication",
						Name:               tt.dpa.Name,
						UID:                tt.dpa.UID,
						Controller:         pointer.BoolPtr(true),
						BlockOwnerDeletion: pointer.BoolPtr(true),
					}},
				},
				Spec: routev1.RouteSpec{
					To: routev1.RouteTargetReference{
						Kind: "Service",
						Name: "oadp-" + "test-bsl" + "-" + "test-provider" + "-registry-svc",
					},
				},
			}
			if err := r.updateRegistryRoute(tt.route, tt.bsl, tt.dpa); (err != nil) != tt.wantErr {
				t.Errorf("updateRegistryRoute() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(tt.route, wantRoute) {
				t.Errorf("expected bsl labels to be %#v, got %#v", tt.route, wantRoute)
			}
		})
	}
}

func TestDPAReconciler_updateRegistryRouteCM(t *testing.T) {
	tests := []struct {
		name            string
		registryRouteCM *corev1.ConfigMap
		bsl             *velerov1.BackupStorageLocation
		dpa             *oadpv1alpha1.DataProtectionApplication
		wantErr         bool
	}{
		{
			name:    "Given Velero CR and bsl instance, appropriate registry cm is updated",
			wantErr: false,
			bsl: &velerov1.BackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-bsl",
					Namespace: "test-ns",
				},
				Spec: velerov1.BackupStorageLocationSpec{
					Provider: "test-provider",
				},
			},
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "Velero-test-CR",
					Namespace: "test-ns",
				},
			},
			registryRouteCM: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "oadp-registry-config",
					Namespace: "test-ns",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient, err := getFakeClientFromObjects()
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
			wantRegitryRouteCM := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "oadp-registry-config",
					Namespace: "test-ns",
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion:         oadpv1alpha1.SchemeBuilder.GroupVersion.String(),
						Kind:               "DataProtectionApplication",
						Name:               tt.dpa.Name,
						UID:                tt.dpa.UID,
						Controller:         pointer.BoolPtr(true),
						BlockOwnerDeletion: pointer.BoolPtr(true),
					}},
				},
				Data: map[string]string{
					"test-bsl": "oadp-" + "test-bsl" + "-" + "test-provider" + "-registry-route",
				},
			}
			if err := r.updateRegistryConfigMap(tt.registryRouteCM, tt.bsl, tt.dpa); (err != nil) != tt.wantErr {
				t.Errorf("updateRegistryConfigMap() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(tt.registryRouteCM, wantRegitryRouteCM) {
				t.Errorf("expected registry CM to be %#v, got %#v", tt.registryRouteCM, wantRegitryRouteCM)
			}
		})
	}
}

func TestDPAReconciler_populateAWSRegistrySecret(t *testing.T) {

	tests := []struct {
		name           string
		bsl            *velerov1.BackupStorageLocation
		registrySecret *corev1.Secret
		awsSecret      *corev1.Secret
		dpa            *oadpv1alpha1.DataProtectionApplication
		wantErr        bool
	}{
		{
			name:    "Given Velero CR and bsl instance, appropriate registry secret is updated for aws case",
			wantErr: false,
			bsl: &velerov1.BackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-bsl",
					Namespace: "test-ns",
				},
				Spec: velerov1.BackupStorageLocationSpec{
					StorageType: velerov1.StorageType{
						ObjectStorage: &velerov1.ObjectStorageLocation{
							Bucket: "aws-bucket",
						},
					},
					Config: map[string]string{
						Region:                "aws-region",
						S3URL:                 "https://sr-url-aws-domain.com",
						InsecureSkipTLSVerify: "false",
						Profile:               testBslProfile,
					},
				},
			},
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "Velero-test-CR",
					Namespace: "test-ns",
				},
			},
			awsSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials",
					Namespace: "test-ns",
				},
				Data: secretData,
			},
			registrySecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "oadp-test-bsl-aws-registry-secret",
					Namespace: "test-ns",
					Labels: map[string]string{
						oadpv1alpha1.OadpOperatorLabel: "True",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient, err := getFakeClientFromObjects(tt.awsSecret, tt.dpa)
			if err != nil {
				t.Errorf("error in creating fake client, likely programmer error")
			}
			r := &DPAReconciler{
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
			wantRegistrySecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "oadp-" + tt.bsl.Name + "-" + tt.bsl.Spec.Provider + "-registry-secret",
					Namespace: r.NamespacedName.Namespace,
					Labels: map[string]string{
						oadpv1alpha1.OadpOperatorLabel: "True",
					},
				},
				Data: awsRegistrySecretData,
			}
			if err := r.populateAWSRegistrySecret(tt.bsl, tt.registrySecret); (err != nil) != tt.wantErr {
				t.Errorf("populateAWSRegistrySecret() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(tt.registrySecret.Data, wantRegistrySecret.Data) {
				t.Errorf("expected bsl labels to be %#v, got %#v", tt.registrySecret.Data, wantRegistrySecret.Data)
			}
		})
	}
}

func TestDPAReconciler_populateAzureRegistrySecret(t *testing.T) {
	tests := []struct {
		name           string
		bsl            *velerov1.BackupStorageLocation
		registrySecret *corev1.Secret
		azureSecret    *corev1.Secret
		dpa            *oadpv1alpha1.DataProtectionApplication
		wantErr        bool
	}{
		{
			name:    "Given Velero CR and bsl instance, appropriate registry secret is updated for azure case",
			wantErr: false,
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
						StorageAccount:                           "velero-azure-account",
						ResourceGroup:                            testResourceGroup,
						RegistryStorageAzureAccountnameEnvVarKey: "velero-azure-account",
						"storageAccountKeyEnvVar":                "AZURE_STORAGE_ACCOUNT_ACCESS_KEY",
					},
				},
			},
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "Velero-test-CR",
					Namespace: "test-ns",
				},
			},
			azureSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials-azure",
					Namespace: "test-ns",
				},
				Data: secretAzureData,
			},
			registrySecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "oadp-test-bsl-azure-registry-secret",
					Namespace: "test-ns",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient, err := getFakeClientFromObjects(tt.azureSecret, tt.dpa)
			if err != nil {
				t.Errorf("error in creating fake client, likely programmer error")
			}
			r := &DPAReconciler{
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
			wantRegistrySecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "oadp-" + tt.bsl.Name + "-" + tt.bsl.Spec.Provider + "-registry-secret",
					Namespace: r.NamespacedName.Namespace,
					Labels: map[string]string{
						oadpv1alpha1.OadpOperatorLabel: "True",
					},
				},
				Data: azureRegistrySecretData,
			}
			if err := r.populateAzureRegistrySecret(tt.bsl, tt.registrySecret); (err != nil) != tt.wantErr {
				t.Errorf("populateAWSRegistrySecret() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(tt.registrySecret.Data, wantRegistrySecret.Data) {
				t.Errorf("expected bsl labels to be %#v, got %#v", tt.registrySecret, wantRegistrySecret.Data)
			}
		})
	}
}
