package controllers

import (
	"context"
	"reflect"
	"testing"

	"github.com/go-logr/logr"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
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
	secretDataWithEqualInSecret = map[string][]byte{
		"cloud": []byte(
			"\n[" + testBslProfile + "]\n" +
				"aws_access_key_id=" + testBslAccessKey + "\n" +
				"aws_secret_access_key=" + testBslSecretAccessKey + "=" + testBslSecretAccessKey +
				"\n[default]" + "\n" +
				"aws_access_key_id=" + testAccessKey + "\n" +
				"aws_secret_access_key=" + testSecretAccessKey + "=" + testSecretAccessKey +
				"\n[test-profile]\n" +
				"aws_access_key_id=" + testAccessKey + "\n" +
				"aws_secret_access_key=" + testSecretAccessKey + "=" + testSecretAccessKey,
		),
	}
	secretDataWithCarriageReturnInSecret = map[string][]byte{
		"cloud": []byte(
			"\n[" + testBslProfile + "]\r\n" +
				"aws_access_key_id=" + testBslAccessKey + "\n" +
				"aws_secret_access_key=" + testBslSecretAccessKey + "=" + testBslSecretAccessKey +
				"\n[default]" + "\n" +
				"aws_access_key_id=" + testAccessKey + "\n" +
				"aws_secret_access_key=" + testSecretAccessKey + "=" + testSecretAccessKey +
				"\r\n[test-profile]\n" +
				"aws_access_key_id=" + testAccessKey + "\r\n" +
				"aws_secret_access_key=" + testSecretAccessKey + "=" + testSecretAccessKey,
		),
	}
	secretDataWithMixedQuotesAndSpacesInSecret = map[string][]byte{
		"cloud": []byte(
			"\n[" + testBslProfile + "]\n" +
				"aws_access_key_id =" + testBslAccessKey + "\n" +
				" aws_secret_access_key=" + "\" " + testBslSecretAccessKey + "\"" +
				"\n[default]" + "\n" +
				" aws_access_key_id= " + testAccessKey + "\n" +
				"aws_secret_access_key =" + "'" + testSecretAccessKey + " '" +
				"\n[test-profile]\n" +
				"aws_access_key_id =" + testAccessKey + "\n" +
				"aws_secret_access_key=" + "\" " + testSecretAccessKey + "\"",
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

var testAWSEnvVar = cloudProviderEnvVarMap["aws"]
var testAzureEnvVar = cloudProviderEnvVarMap["azure"]
var testGCPEnvVar = cloudProviderEnvVarMap["gcp"]

func TestDPAReconciler_getSecretNameAndKey(t *testing.T) {
	tests := []struct {
		name           string
		bsl            *oadpv1alpha1.BackupLocation
		secret         *corev1.Secret
		wantProfile    string
		wantSecretName string
		wantSecretKey  string
	}{
		{
			name: "given provider secret, appropriate secret name and key are returned",
			bsl: &oadpv1alpha1.BackupLocation{
				Velero: &velerov1.BackupStorageLocationSpec{
					Provider: AWSProvider,
					Credential: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "cloud-credentials-aws",
						},
						Key: "cloud",
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
					Name:      "cloud-credentials-aws",
					Namespace: "test-ns",
				},
				Data: secretData,
			},

			wantProfile: "aws-provider",
		},
		{
			name: "given no provider secret, appropriate secret name and key are returned",
			bsl: &oadpv1alpha1.BackupLocation{
				Velero: &velerov1.BackupStorageLocationSpec{
					Provider: AWSProvider,
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

			wantProfile: "aws-provider-no-cred",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient, err := getFakeClientFromObjects(tt.secret)
			if err != nil {
				t.Errorf("error in creating fake client, likely programmer error")
			}
			r := &DPAReconciler{
				Client:        fakeClient,
				Scheme:        fakeClient.Scheme(),
				Log:           logr.Discard(),
				Context:       newContextForTest(tt.name),
				EventRecorder: record.NewFakeRecorder(10),
			}

			if tt.wantProfile == "aws-provider" {
				tt.wantSecretKey = "cloud"
				tt.wantSecretName = "cloud-credentials-aws"
			}
			if tt.wantProfile == "aws-provider-no-cred" {
				tt.wantSecretKey = "cloud"
				tt.wantSecretName = "cloud-credentials"
			}

			gotName, gotKey, _ := r.getSecretNameAndKey(tt.bsl.Velero.Config, tt.bsl.Velero.Credential, oadpv1alpha1.DefaultPlugin(tt.bsl.Velero.Provider))

			if !reflect.DeepEqual(tt.wantSecretName, gotName) {
				t.Errorf("expected secret name to be %#v, got %#v", tt.wantSecretName, gotName)
			}
			if !reflect.DeepEqual(tt.wantSecretKey, gotKey) {
				t.Errorf("expected secret key to be %#v, got %#v", tt.wantSecretKey, gotKey)
			}
		})
	}
}

func TestDPAReconciler_getSecretNameAndKeyFromCloudStorage(t *testing.T) {
	tests := []struct {
		name           string
		bsl            *oadpv1alpha1.BackupLocation
		secret         *corev1.Secret
		wantProfile    string
		wantSecretName string
		wantSecretKey  string
	}{
		{
			name: "given cloud storage secret, appropriate secret name and key are returned",
			bsl: &oadpv1alpha1.BackupLocation{
				CloudStorage: &oadpv1alpha1.CloudStorageLocation{
					Credential: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "cloud-credentials-aws",
						},
						Key: "cloud",
					},
					CloudStorageRef: corev1.LocalObjectReference{
						Name: "example",
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials-aws",
					Namespace: "test-ns",
				},
				Data: secretData,
			},

			wantProfile: "aws-cloud-cred",
		},
		{
			name: "given no cloud storage secret, appropriate secret name and key are returned",
			bsl: &oadpv1alpha1.BackupLocation{
				CloudStorage: &oadpv1alpha1.CloudStorageLocation{
					CloudStorageRef: corev1.LocalObjectReference{
						Name: "example",
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "",
					Namespace: "test-ns",
				},
				Data: secretData,
			},

			wantProfile: "aws-cloud-no-cred",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient, err := getFakeClientFromObjects(tt.secret)
			if err != nil {
				t.Errorf("error in creating fake client, likely programmer error")
			}
			r := &DPAReconciler{
				Client:        fakeClient,
				Scheme:        fakeClient.Scheme(),
				Log:           logr.Discard(),
				Context:       newContextForTest(tt.name),
				EventRecorder: record.NewFakeRecorder(10),
			}

			if tt.wantProfile == "aws-cloud-cred" {
				tt.wantSecretKey = "cloud"
				tt.wantSecretName = "cloud-credentials-aws"
			}
			if tt.wantProfile == "aws-cloud-no-cred" {
				tt.wantSecretKey = ""
				tt.wantSecretName = ""
			}

			gotName, gotKey, _ := r.getSecretNameAndKeyFromCloudStorage(tt.bsl.CloudStorage)

			if !reflect.DeepEqual(tt.wantSecretName, gotName) {
				t.Errorf("expected secret name to be %#v, got %#v", tt.wantSecretName, gotName)
			}
			if !reflect.DeepEqual(tt.wantSecretKey, gotKey) {
				t.Errorf("expected secret key to be %#v, got %#v", tt.wantSecretKey, gotKey)
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
				t.Errorf("expected aws registry secret to be %#v, got %#v", tt.registrySecret.Data, wantRegistrySecret.Data)
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
				t.Errorf("populateAzureRegistrySecret() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(tt.registrySecret.Data, wantRegistrySecret.Data) {
				t.Errorf("expected azure registry secret to be %#v, got %#v", tt.registrySecret, wantRegistrySecret.Data)
			}
		})
	}
}

func Test_replaceCarriageReturn(t *testing.T) {
	type args struct {
		data   map[string][]byte
		logger logr.Logger
	}
	tests := []struct {
		name string
		args args
		want map[string][]byte
	}{
		{
			name: "Given a map with carriage return, carriage return is replaced with new line",
			args: args{
				data: map[string][]byte{
					"test": []byte("test\r\n"),
				},
				logger: logr.FromContextOrDiscard(context.TODO()),
			},
			want: map[string][]byte{
				"test": []byte("test\n"),
			},
		},
		{
			name: "Given secret data with carriage return, carriage return is replaced with new line",
			args: args{
				data:   secretDataWithCarriageReturnInSecret,
				logger: logr.FromContextOrDiscard(context.TODO()),
			},
			want: secretDataWithEqualInSecret,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := replaceCarriageReturn(tt.args.data, tt.args.logger); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("replaceCarriageReturn() = %v, want %v", got, tt.want)
			}
		})
	}
}
