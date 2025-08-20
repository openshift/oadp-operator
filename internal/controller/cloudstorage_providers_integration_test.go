/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
)

// TestCloudStorageRefIntegrationAWS tests end-to-end CloudStorageRef functionality for AWS
func TestCloudStorageRefIntegrationAWS(t *testing.T) {
	tests := []struct {
		name             string
		dpa              *oadpv1alpha1.DataProtectionApplication
		cloudStorage     *oadpv1alpha1.CloudStorage
		secret           *corev1.Secret
		expectBSLCreated bool
		expectedBSLName  string
		expectedProvider string
		expectedBucket   string
		expectedConfig   map[string]string
	}{
		{
			name: "AWS with STS credentials and custom config",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Name: "aws-location",
							CloudStorage: &oadpv1alpha1.CloudStorageLocation{
								CloudStorageRef: corev1.LocalObjectReference{
									Name: "aws-cs",
								},
								Config: map[string]string{
									"region":                "us-east-1",
									"checksumAlgorithm":     "CRC32",
									"enableSharedConfig":    "true",
									"s3ForcePathStyle":      "true",
									"s3Url":                 "https://s3.custom.endpoint",
									"insecureSkipTLSVerify": "true",
								},
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "aws-creds",
									},
									Key: "credentials",
								},
								Prefix:  "velero-backups",
								Default: true,
							},
						},
					},
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
							},
						},
					},
				},
			},
			cloudStorage: &oadpv1alpha1.CloudStorage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "aws-cs",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.CloudStorageSpec{
					Name:     "my-aws-backup-bucket",
					Provider: oadpv1alpha1.AWSBucketProvider,
					Region:   "us-west-2", // Should be overridden by DPA config
					CreationSecret: corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "aws-creds",
						},
						Key: "credentials",
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "aws-creds",
					Namespace: "test-ns",
				},
				Data: map[string][]byte{
					"credentials": []byte(`[default]
role_arn = arn:aws:iam::123456789012:role/velero-backup-role
web_identity_token_file = /var/run/secrets/openshift/serviceaccount/token`),
				},
			},
			expectBSLCreated: true,
			expectedBSLName:  "aws-location",
			expectedProvider: "aws",
			expectedBucket:   "my-aws-backup-bucket",
			expectedConfig: map[string]string{
				"region":                "us-east-1",
				"checksumAlgorithm":     "CRC32",
				"enableSharedConfig":    "true",
				"s3ForcePathStyle":      "true",
				"s3Url":                 "https://s3.custom.endpoint",
				"insecureSkipTLSVerify": "true",
			},
		},
		{
			name: "AWS with profile configuration",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Name: "aws-profile",
							CloudStorage: &oadpv1alpha1.CloudStorageLocation{
								CloudStorageRef: corev1.LocalObjectReference{
									Name: "aws-profile-cs",
								},
								Config: map[string]string{
									"region":  "eu-west-1",
									"profile": "backup-profile",
								},
								Prefix: "dev-backups",
							},
						},
					},
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
							},
						},
					},
				},
			},
			cloudStorage: &oadpv1alpha1.CloudStorage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "aws-profile-cs",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.CloudStorageSpec{
					Name:     "profile-backup-bucket",
					Provider: oadpv1alpha1.AWSBucketProvider,
					Region:   "eu-west-1",
				},
			},
			expectBSLCreated: true,
			expectedBSLName:  "aws-profile",
			expectedProvider: "aws",
			expectedBucket:   "profile-backup-bucket",
			expectedConfig: map[string]string{
				"region":  "eu-west-1",
				"profile": "backup-profile",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup objects
			objs := []client.Object{tt.dpa, tt.cloudStorage}
			if tt.secret != nil {
				objs = append(objs, tt.secret)
			}

			// Add namespace
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-ns",
				},
			}
			objs = append(objs, namespace)

			schemeForFakeClient, err := getSchemeForFakeClient()
			if err != nil {
				t.Error(err)
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(schemeForFakeClient).
				WithObjects(objs...).
				Build()

			r := &DataProtectionApplicationReconciler{
				Client:        fakeClient,
				Scheme:        schemeForFakeClient,
				Context:       context.Background(),
				Log:           log.Log,
				EventRecorder: record.NewFakeRecorder(10),
				NamespacedName: types.NamespacedName{
					Namespace: tt.dpa.Namespace,
					Name:      tt.dpa.Name,
				},
			}

			// Set the DPA for the reconciler
			r.dpa = tt.dpa

			// Create BSL from DPA spec
			_, err = r.ReconcileBackupStorageLocations(log.Log)
			assert.NoError(t, err)

			// Verify BSL was created
			if tt.expectBSLCreated {
				// List BSLs to verify creation
				bslList := velerov1.BackupStorageLocationList{}
				err = fakeClient.List(context.Background(), &bslList, client.InNamespace(tt.dpa.Namespace))
				assert.NoError(t, err)
				assert.Len(t, bslList.Items, 1)
				bsl := &bslList.Items[0]

				// Verify BSL properties
				assert.Equal(t, tt.expectedBSLName, bsl.Name)
				assert.Equal(t, tt.expectedProvider, bsl.Spec.Provider)
				assert.Equal(t, tt.expectedBucket, bsl.Spec.ObjectStorage.Bucket)
				assert.Equal(t, tt.expectedConfig, bsl.Spec.Config)

				// Verify credential reference if secret exists
				if tt.secret != nil {
					assert.NotNil(t, bsl.Spec.Credential)
					assert.Equal(t, tt.secret.Name, bsl.Spec.Credential.Name)
					assert.Equal(t, "credentials", bsl.Spec.Credential.Key)
				}
			}
		})
	}
}

// TestCloudStorageRefIntegrationGCP tests end-to-end CloudStorageRef functionality for GCP
func TestCloudStorageRefIntegrationGCP(t *testing.T) {
	tests := []struct {
		name             string
		dpa              *oadpv1alpha1.DataProtectionApplication
		cloudStorage     *oadpv1alpha1.CloudStorage
		secret           *corev1.Secret
		expectBSLCreated bool
		expectedBSLName  string
		expectedProvider string
		expectedBucket   string
		expectedConfig   map[string]string
	}{
		{
			name: "GCP with Workload Identity Federation",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Name: "gcp-location",
							CloudStorage: &oadpv1alpha1.CloudStorageLocation{
								CloudStorageRef: corev1.LocalObjectReference{
									Name: "gcp-cs",
								},
								Config: map[string]string{
									"project": "my-gcp-project",
								},
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "gcp-creds",
									},
									Key: "service_account.json",
								},
								Prefix:  "velero",
								Default: true,
							},
						},
					},
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginGCP,
							},
						},
					},
				},
			},
			cloudStorage: &oadpv1alpha1.CloudStorage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gcp-cs",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.CloudStorageSpec{
					Name:     "my-gcp-backup-bucket",
					Provider: oadpv1alpha1.GCPBucketProvider,
					Region:   "us-central1",
					CreationSecret: corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "gcp-creds",
						},
						Key: "service_account.json",
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gcp-creds",
					Namespace: "test-ns",
				},
				Data: map[string][]byte{
					"service_account.json": []byte(`{
  "type": "external_account",
  "audience": "//iam.googleapis.com/projects/123456789/locations/global/workloadIdentityPools/my-pool/providers/my-provider",
  "subject_token_type": "urn:ietf:params:oauth:token-type:jwt",
  "service_account_impersonation_url": "https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/velero@my-project.iam.gserviceaccount.com:generateAccessToken",
  "token_url": "https://sts.googleapis.com/v1/token",
  "credential_source": {
    "file": "/var/run/secrets/openshift/serviceaccount/token",
    "format": {
      "type": "text"
    }
  }
}`),
				},
			},
			expectBSLCreated: true,
			expectedBSLName:  "gcp-location",
			expectedProvider: "gcp",
			expectedBucket:   "my-gcp-backup-bucket",
			expectedConfig: map[string]string{
				"project": "my-gcp-project",
			},
		},
		{
			name: "GCP with service account key",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Name: "gcp-sa-key",
							CloudStorage: &oadpv1alpha1.CloudStorageLocation{
								CloudStorageRef: corev1.LocalObjectReference{
									Name: "gcp-sa-cs",
								},
								Config: map[string]string{
									"project":          "legacy-project",
									"snapshotLocation": "us-west1",
								},
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "gcp-sa-key",
									},
									Key: "service_account.json",
								},
								Prefix: "legacy-backups",
							},
						},
					},
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginGCP,
							},
						},
					},
				},
			},
			cloudStorage: &oadpv1alpha1.CloudStorage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gcp-sa-cs",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.CloudStorageSpec{
					Name:     "legacy-backup-bucket",
					Provider: oadpv1alpha1.GCPBucketProvider,
					Region:   "us-west1",
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gcp-sa-key",
					Namespace: "test-ns",
				},
				Data: map[string][]byte{
					"service_account.json": []byte(`{
  "type": "service_account",
  "project_id": "legacy-project",
  "private_key_id": "key-id",
  "private_key": "-----BEGIN RSA PRIVATE KEY-----\n...\n-----END RSA PRIVATE KEY-----\n",
  "client_email": "velero@legacy-project.iam.gserviceaccount.com",
  "client_id": "123456789",
  "auth_uri": "https://accounts.google.com/o/oauth2/auth",
  "token_uri": "https://oauth2.googleapis.com/token"
}`),
				},
			},
			expectBSLCreated: true,
			expectedBSLName:  "gcp-sa-key",
			expectedProvider: "gcp",
			expectedBucket:   "legacy-backup-bucket",
			expectedConfig: map[string]string{
				"project":          "legacy-project",
				"snapshotLocation": "us-west1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup objects
			objs := []client.Object{tt.dpa, tt.cloudStorage}
			if tt.secret != nil {
				objs = append(objs, tt.secret)
			}

			// Add namespace
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-ns",
				},
			}
			objs = append(objs, namespace)

			schemeForFakeClient, err := getSchemeForFakeClient()
			if err != nil {
				t.Error(err)
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(schemeForFakeClient).
				WithObjects(objs...).
				Build()

			r := &DataProtectionApplicationReconciler{
				Client:        fakeClient,
				Scheme:        schemeForFakeClient,
				Context:       context.Background(),
				Log:           log.Log,
				EventRecorder: record.NewFakeRecorder(10),
				NamespacedName: types.NamespacedName{
					Namespace: tt.dpa.Namespace,
					Name:      tt.dpa.Name,
				},
			}

			// Set the DPA for the reconciler
			r.dpa = tt.dpa

			// Create BSL from DPA spec
			_, err = r.ReconcileBackupStorageLocations(log.Log)
			assert.NoError(t, err)

			// Verify BSL was created
			if tt.expectBSLCreated {
				// List BSLs to verify creation
				bslList := velerov1.BackupStorageLocationList{}
				err = fakeClient.List(context.Background(), &bslList, client.InNamespace(tt.dpa.Namespace))
				assert.NoError(t, err)
				assert.Len(t, bslList.Items, 1)
				bsl := &bslList.Items[0]

				// Verify BSL properties
				assert.Equal(t, tt.expectedBSLName, bsl.Name)
				assert.Equal(t, tt.expectedProvider, bsl.Spec.Provider)
				assert.Equal(t, tt.expectedBucket, bsl.Spec.ObjectStorage.Bucket)
				assert.Equal(t, tt.expectedConfig, bsl.Spec.Config)

				// Verify credential reference
				if tt.secret != nil {
					assert.NotNil(t, bsl.Spec.Credential)
					assert.Equal(t, tt.secret.Name, bsl.Spec.Credential.Name)
					assert.Equal(t, "service_account.json", bsl.Spec.Credential.Key)
				}
			}
		})
	}
}

// TestCloudStorageRefIntegrationAzure tests end-to-end CloudStorageRef functionality for Azure
func TestCloudStorageRefIntegrationAzure(t *testing.T) {
	tests := []struct {
		name             string
		dpa              *oadpv1alpha1.DataProtectionApplication
		cloudStorage     *oadpv1alpha1.CloudStorage
		secret           *corev1.Secret
		expectBSLCreated bool
		expectedBSLName  string
		expectedProvider string
		expectedBucket   string
		expectedConfig   map[string]string
	}{
		{
			name: "Azure with Workload Identity",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Name: "azure-location",
							CloudStorage: &oadpv1alpha1.CloudStorageLocation{
								CloudStorageRef: corev1.LocalObjectReference{
									Name: "azure-cs",
								},
								Config: map[string]string{
									"resourceGroup":        "my-backup-rg",
									"storageAccount":       "mybackupstorageacct",
									"subscriptionId":       "12345678-1234-1234-1234-123456789012",
									"useAAD":               "true",
									"incrementalBackupVSC": "true",
								},
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "azure-creds",
									},
									Key: "azurekey",
								},
								Prefix:  "velero",
								Default: true,
							},
						},
					},
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginMicrosoftAzure,
							},
						},
					},
				},
			},
			cloudStorage: &oadpv1alpha1.CloudStorage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "azure-cs",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.CloudStorageSpec{
					Name:     "velerocontainer",
					Provider: oadpv1alpha1.AzureBucketProvider,
					CreationSecret: corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "azure-creds",
						},
						Key: "azurekey",
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "azure-creds",
					Namespace: "test-ns",
				},
				Data: map[string][]byte{
					"azurekey": []byte(`AZURE_CLIENT_ID=87654321-1234-1234-1234-210987654321
AZURE_TENANT_ID=12345678-1234-1234-1234-123456789012
AZURE_FEDERATED_TOKEN_FILE=/var/run/secrets/openshift/serviceaccount/token
AZURE_AUTHORITY_HOST=https://login.microsoftonline.com`),
				},
			},
			expectBSLCreated: true,
			expectedBSLName:  "azure-location",
			expectedProvider: "azure",
			expectedBucket:   "velerocontainer",
			expectedConfig: map[string]string{
				"resourceGroup":        "my-backup-rg",
				"storageAccount":       "mybackupstorageacct",
				"subscriptionId":       "12345678-1234-1234-1234-123456789012",
				"useAAD":               "true",
				"incrementalBackupVSC": "true",
			},
		},
		{
			name: "Azure with service principal",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Name: "azure-sp",
							CloudStorage: &oadpv1alpha1.CloudStorageLocation{
								CloudStorageRef: corev1.LocalObjectReference{
									Name: "azure-sp-cs",
								},
								Config: map[string]string{
									"resourceGroup":  "legacy-rg",
									"storageAccount": "legacystorageacct",
									"subscriptionId": "87654321-4321-4321-4321-210987654321",
								},
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "azure-sp-creds",
									},
									Key: "azurekey",
								},
								Prefix: "legacy-backups",
							},
						},
					},
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginMicrosoftAzure,
							},
						},
					},
				},
			},
			cloudStorage: &oadpv1alpha1.CloudStorage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "azure-sp-cs",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.CloudStorageSpec{
					Name:     "legacycontainer",
					Provider: oadpv1alpha1.AzureBucketProvider,
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "azure-sp-creds",
					Namespace: "test-ns",
				},
				Data: map[string][]byte{
					"azurekey": []byte(`AZURE_CLIENT_ID=11111111-2222-3333-4444-555555555555
AZURE_CLIENT_SECRET=super-secret-password
AZURE_TENANT_ID=87654321-4321-4321-4321-210987654321
AZURE_SUBSCRIPTION_ID=87654321-4321-4321-4321-210987654321`),
				},
			},
			expectBSLCreated: true,
			expectedBSLName:  "azure-sp",
			expectedProvider: "azure",
			expectedBucket:   "legacycontainer",
			expectedConfig: map[string]string{
				"resourceGroup":  "legacy-rg",
				"storageAccount": "legacystorageacct",
				"subscriptionId": "87654321-4321-4321-4321-210987654321",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup objects
			objs := []client.Object{tt.dpa, tt.cloudStorage}
			if tt.secret != nil {
				objs = append(objs, tt.secret)
			}

			// Add namespace
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-ns",
				},
			}
			objs = append(objs, namespace)

			schemeForFakeClient, err := getSchemeForFakeClient()
			if err != nil {
				t.Error(err)
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(schemeForFakeClient).
				WithObjects(objs...).
				Build()

			r := &DataProtectionApplicationReconciler{
				Client:        fakeClient,
				Scheme:        schemeForFakeClient,
				Context:       context.Background(),
				Log:           log.Log,
				EventRecorder: record.NewFakeRecorder(10),
				NamespacedName: types.NamespacedName{
					Namespace: tt.dpa.Namespace,
					Name:      tt.dpa.Name,
				},
			}

			// Set the DPA for the reconciler
			r.dpa = tt.dpa

			// Create BSL from DPA spec
			_, err = r.ReconcileBackupStorageLocations(log.Log)
			assert.NoError(t, err)

			// Verify BSL was created
			if tt.expectBSLCreated {
				// List BSLs to verify creation
				bslList := velerov1.BackupStorageLocationList{}
				err = fakeClient.List(context.Background(), &bslList, client.InNamespace(tt.dpa.Namespace))
				assert.NoError(t, err)
				assert.Len(t, bslList.Items, 1)
				bsl := &bslList.Items[0]

				// Verify BSL properties
				assert.Equal(t, tt.expectedBSLName, bsl.Name)
				assert.Equal(t, tt.expectedProvider, bsl.Spec.Provider)
				assert.Equal(t, tt.expectedBucket, bsl.Spec.ObjectStorage.Bucket)
				assert.Equal(t, tt.expectedConfig, bsl.Spec.Config)

				// Verify credential reference
				if tt.secret != nil {
					assert.NotNil(t, bsl.Spec.Credential)
					assert.Equal(t, tt.secret.Name, bsl.Spec.Credential.Name)
					assert.Equal(t, "azurekey", bsl.Spec.Credential.Key)
				}
			}
		})
	}
}

// TestCloudStorageRefUpdateScenarios tests CloudStorage update scenarios
func TestCloudStorageRefUpdateScenarios(t *testing.T) {
	tests := []struct {
		name                string
		initialCloudStorage *oadpv1alpha1.CloudStorage
		updatedCloudStorage *oadpv1alpha1.CloudStorage
		dpa                 *oadpv1alpha1.DataProtectionApplication
		expectBSLUpdate     bool
		expectedBucket      string
	}{
		{
			name: "CloudStorage bucket name update",
			initialCloudStorage: &oadpv1alpha1.CloudStorage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cs",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.CloudStorageSpec{
					Name:     "old-bucket",
					Provider: oadpv1alpha1.AWSBucketProvider,
					Region:   "us-east-1",
				},
			},
			updatedCloudStorage: &oadpv1alpha1.CloudStorage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cs",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.CloudStorageSpec{
					Name:     "new-bucket",
					Provider: oadpv1alpha1.AWSBucketProvider,
					Region:   "us-east-1",
				},
			},
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							CloudStorage: &oadpv1alpha1.CloudStorageLocation{
								CloudStorageRef: corev1.LocalObjectReference{
									Name: "test-cs",
								},
							},
						},
					},
				},
			},
			expectBSLUpdate: true,
			expectedBucket:  "new-bucket",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Initial setup
			objs := []client.Object{tt.dpa, tt.initialCloudStorage}
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-ns",
				},
			}
			objs = append(objs, namespace)

			schemeForFakeClient, err := getSchemeForFakeClient()
			if err != nil {
				t.Error(err)
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(schemeForFakeClient).
				WithObjects(objs...).
				Build()

			r := &DataProtectionApplicationReconciler{
				Client:        fakeClient,
				Scheme:        schemeForFakeClient,
				Context:       context.Background(),
				Log:           log.Log,
				EventRecorder: record.NewFakeRecorder(10),
				NamespacedName: types.NamespacedName{
					Namespace: tt.dpa.Namespace,
					Name:      tt.dpa.Name,
				},
			}

			// Set the DPA for the reconciler
			r.dpa = tt.dpa

			// Create initial BSL
			_, err = r.ReconcileBackupStorageLocations(log.Log)
			assert.NoError(t, err)

			// Get the current CloudStorage to ensure we have the latest resource version
			currentCS := &oadpv1alpha1.CloudStorage{}
			err = fakeClient.Get(context.Background(), client.ObjectKey{
				Namespace: tt.updatedCloudStorage.Namespace,
				Name:      tt.updatedCloudStorage.Name,
			}, currentCS)
			assert.NoError(t, err)

			// Update CloudStorage with the correct resource version
			tt.updatedCloudStorage.ResourceVersion = currentCS.ResourceVersion
			err = fakeClient.Update(context.Background(), tt.updatedCloudStorage)
			assert.NoError(t, err)

			// Rebuild BSL after CloudStorage update
			_, err = r.ReconcileBackupStorageLocations(log.Log)
			assert.NoError(t, err)

			// Verify BSL update
			if tt.expectBSLUpdate {
				// List BSLs to verify update
				bslList := velerov1.BackupStorageLocationList{}
				err = fakeClient.List(context.Background(), &bslList, client.InNamespace(tt.dpa.Namespace))
				assert.NoError(t, err)
				assert.Len(t, bslList.Items, 1)
				bsl := &bslList.Items[0]
				assert.Equal(t, tt.expectedBucket, bsl.Spec.ObjectStorage.Bucket)
			}
		})
	}
}

// TestCloudStorageRefDeletionScenarios tests CloudStorage deletion scenarios
func TestCloudStorageRefDeletionScenarios(t *testing.T) {
	ctx := context.Background()

	// Create test CloudStorage with finalizer
	cloudStorage := &oadpv1alpha1.CloudStorage{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-cs",
			Namespace:  "test-ns",
			Finalizers: []string{oadpFinalizerBucket},
		},
		Spec: oadpv1alpha1.CloudStorageSpec{
			Name:     "test-bucket",
			Provider: oadpv1alpha1.AWSBucketProvider,
		},
	}

	// Create DPA referencing the CloudStorage
	dpa := &oadpv1alpha1.DataProtectionApplication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dpa",
			Namespace: "test-ns",
		},
		Spec: oadpv1alpha1.DataProtectionApplicationSpec{
			BackupLocations: []oadpv1alpha1.BackupLocation{
				{
					CloudStorage: &oadpv1alpha1.CloudStorageLocation{
						CloudStorageRef: corev1.LocalObjectReference{
							Name: "test-cs",
						},
					},
				},
			},
		},
	}

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-ns",
		},
	}

	schemeForFakeClient, err := getSchemeForFakeClient()
	assert.NoError(t, err)

	fakeClient := fake.NewClientBuilder().
		WithScheme(schemeForFakeClient).
		WithObjects(namespace, cloudStorage, dpa).
		Build()

	// Try to delete CloudStorage
	err = fakeClient.Delete(ctx, cloudStorage)
	assert.NoError(t, err)

	// Verify CloudStorage still exists due to finalizer
	cs := &oadpv1alpha1.CloudStorage{}
	err = fakeClient.Get(ctx, types.NamespacedName{Name: "test-cs", Namespace: "test-ns"}, cs)
	assert.NoError(t, err)
	assert.NotNil(t, cs.DeletionTimestamp)

	// Add deletion annotation
	cs.Annotations = map[string]string{
		oadpCloudStorageDeleteAnnotation: "true",
	}
	err = fakeClient.Update(ctx, cs)
	assert.NoError(t, err)

	// In a real scenario, the controller would process the deletion and remove the finalizer
	// For this test, we'll simulate that by manually removing the finalizer
	cs.Finalizers = []string{}
	err = fakeClient.Update(ctx, cs)
	assert.NoError(t, err)

	// Now CloudStorage should be deleted
	err = fakeClient.Get(ctx, types.NamespacedName{Name: "test-cs", Namespace: "test-ns"}, cs)
	assert.Error(t, err)
	assert.True(t, client.IgnoreNotFound(err) == nil)
}

// TestCloudStorageRefProviderValidation tests provider-specific validation
func TestCloudStorageRefProviderValidation(t *testing.T) {
	tests := []struct {
		name          string
		cloudStorage  *oadpv1alpha1.CloudStorage
		bslSpec       oadpv1alpha1.BackupLocation
		expectError   bool
		errorContains string
	}{
		{
			name: "AWS CloudStorage with invalid region format",
			cloudStorage: &oadpv1alpha1.CloudStorage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "aws-cs",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.CloudStorageSpec{
					Name:     "test-bucket",
					Provider: oadpv1alpha1.AWSBucketProvider,
					Region:   "invalid region with spaces",
				},
			},
			bslSpec: oadpv1alpha1.BackupLocation{
				CloudStorage: &oadpv1alpha1.CloudStorageLocation{
					CloudStorageRef: corev1.LocalObjectReference{
						Name: "aws-cs",
					},
				},
			},
			expectError: false, // Region validation happens at provider level
		},
		{
			name: "GCP CloudStorage without project ID",
			cloudStorage: &oadpv1alpha1.CloudStorage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gcp-cs",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.CloudStorageSpec{
					Name:     "test-bucket",
					Provider: oadpv1alpha1.GCPBucketProvider,
				},
			},
			bslSpec: oadpv1alpha1.BackupLocation{
				CloudStorage: &oadpv1alpha1.CloudStorageLocation{
					CloudStorageRef: corev1.LocalObjectReference{
						Name: "gcp-cs",
					},
					// Missing project in config
				},
			},
			expectError: false, // Project can be auto-detected
		},
		{
			name: "Azure CloudStorage without storage account",
			cloudStorage: &oadpv1alpha1.CloudStorage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "azure-cs",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.CloudStorageSpec{
					Name:     "test-container",
					Provider: oadpv1alpha1.AzureBucketProvider,
				},
			},
			bslSpec: oadpv1alpha1.BackupLocation{
				CloudStorage: &oadpv1alpha1.CloudStorageLocation{
					CloudStorageRef: corev1.LocalObjectReference{
						Name: "azure-cs",
					},
					Config: map[string]string{
						"resourceGroup": "test-rg",
						// Missing storageAccount
					},
				},
			},
			expectError: false, // Storage account can be in env vars
		},
		{
			name: "Invalid provider type",
			cloudStorage: &oadpv1alpha1.CloudStorage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-cs",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.CloudStorageSpec{
					Name:     "test-bucket",
					Provider: "invalid-provider",
				},
			},
			bslSpec: oadpv1alpha1.BackupLocation{
				CloudStorage: &oadpv1alpha1.CloudStorageLocation{
					CloudStorageRef: corev1.LocalObjectReference{
						Name: "invalid-cs",
					},
				},
			},
			expectError:   true,
			errorContains: "unsupported CloudStorage provider",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-ns",
				},
			}

			schemeForFakeClient, err := getSchemeForFakeClient()
			assert.NoError(t, err)

			fakeClient := fake.NewClientBuilder().
				WithScheme(schemeForFakeClient).
				WithObjects(namespace, tt.cloudStorage).
				Build()

			r := &DataProtectionApplicationReconciler{
				Client:        fakeClient,
				Scheme:        schemeForFakeClient,
				Context:       context.Background(),
				Log:           log.Log,
				EventRecorder: record.NewFakeRecorder(10),
				NamespacedName: types.NamespacedName{
					Namespace: "test-ns",
					Name:      "test-dpa",
				},
			}

			// Attempt to populate BSL from CloudStorage
			err = r.populateBSLFromCloudStorage(&tt.bslSpec, "test-ns")

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestCloudStorageRefBackupSyncPeriod tests backup sync period configuration
func TestCloudStorageRefBackupSyncPeriod(t *testing.T) {
	oneHour := metav1.Duration{Duration: 1 * time.Hour}
	fiveMinutes := metav1.Duration{Duration: 5 * time.Minute}

	tests := []struct {
		name               string
		dpa                *oadpv1alpha1.DataProtectionApplication
		cloudStorage       *oadpv1alpha1.CloudStorage
		expectedSyncPeriod *metav1.Duration
	}{
		{
			name: "Default sync period (nil)",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							CloudStorage: &oadpv1alpha1.CloudStorageLocation{
								CloudStorageRef: corev1.LocalObjectReference{
									Name: "test-cs",
								},
							},
						},
					},
				},
			},
			cloudStorage: &oadpv1alpha1.CloudStorage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cs",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.CloudStorageSpec{
					Name:     "test-bucket",
					Provider: oadpv1alpha1.AWSBucketProvider,
				},
			},
			expectedSyncPeriod: nil,
		},
		{
			name: "Custom sync period - 1 hour",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							CloudStorage: &oadpv1alpha1.CloudStorageLocation{
								CloudStorageRef: corev1.LocalObjectReference{
									Name: "test-cs",
								},
								BackupSyncPeriod: &oneHour,
							},
						},
					},
				},
			},
			cloudStorage: &oadpv1alpha1.CloudStorage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cs",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.CloudStorageSpec{
					Name:     "test-bucket",
					Provider: oadpv1alpha1.AWSBucketProvider,
				},
			},
			expectedSyncPeriod: &oneHour,
		},
		{
			name: "Short sync period - 5 minutes",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							CloudStorage: &oadpv1alpha1.CloudStorageLocation{
								CloudStorageRef: corev1.LocalObjectReference{
									Name: "test-cs",
								},
								BackupSyncPeriod: &fiveMinutes,
							},
						},
					},
				},
			},
			cloudStorage: &oadpv1alpha1.CloudStorage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cs",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.CloudStorageSpec{
					Name:     "test-bucket",
					Provider: oadpv1alpha1.AWSBucketProvider,
				},
			},
			expectedSyncPeriod: &fiveMinutes,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-ns",
				},
			}

			schemeForFakeClient, err := getSchemeForFakeClient()
			assert.NoError(t, err)

			fakeClient := fake.NewClientBuilder().
				WithScheme(schemeForFakeClient).
				WithObjects(namespace, tt.dpa, tt.cloudStorage).
				Build()

			r := &DataProtectionApplicationReconciler{
				Client:        fakeClient,
				Scheme:        schemeForFakeClient,
				Context:       context.Background(),
				Log:           log.Log,
				EventRecorder: record.NewFakeRecorder(10),
				NamespacedName: types.NamespacedName{
					Namespace: tt.dpa.Namespace,
					Name:      tt.dpa.Name,
				},
			}

			// Set the DPA for the reconciler
			r.dpa = tt.dpa

			// Create BSL from DPA spec
			_, err = r.ReconcileBackupStorageLocations(log.Log)
			assert.NoError(t, err)

			// List BSLs to verify creation
			bslList := velerov1.BackupStorageLocationList{}
			err = fakeClient.List(context.Background(), &bslList, client.InNamespace(tt.dpa.Namespace))
			assert.NoError(t, err)
			assert.Len(t, bslList.Items, 1)

			bsl := &bslList.Items[0]

			// Verify backup sync period
			if tt.expectedSyncPeriod != nil {
				assert.NotNil(t, bsl.Spec.BackupSyncPeriod)
				assert.Equal(t, tt.expectedSyncPeriod.Duration, bsl.Spec.BackupSyncPeriod.Duration)
			} else {
				assert.Nil(t, bsl.Spec.BackupSyncPeriod)
			}
		})
	}
}
