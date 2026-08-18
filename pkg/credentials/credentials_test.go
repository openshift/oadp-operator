package credentials

import (
	"reflect"
	"testing"

	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/common"
)

func TestCredentials_getPluginImage(t *testing.T) {
	tests := []struct {
		name       string
		dpa        *oadpv1alpha1.DataProtectionApplication
		pluginName oadpv1alpha1.DefaultPlugin
		wantImage  string
		setEnvVars map[string]string
	}{
		// AWS tests
		{
			name: "given aws plugin override, custom aws image should be returned",
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
					UnsupportedOverrides: map[oadpv1alpha1.UnsupportedImageKey]string{
						oadpv1alpha1.AWSPluginImageKey: "test-image",
					},
				},
			},
			pluginName: oadpv1alpha1.DefaultPluginAWS,
			wantImage:  "test-image",
			setEnvVars: make(map[string]string),
		},
		{
			name: "given default Velero CR with no env var, default aws image should be returned",
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
			pluginName: oadpv1alpha1.DefaultPluginAWS,
			wantImage:  common.AWSPluginImage,
			setEnvVars: make(map[string]string),
		},
		{
			name: "given default Velero CR with env var set, image should be built via env vars",
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
			pluginName: oadpv1alpha1.DefaultPluginAWS,
			wantImage:  "quay.io/konveyor/velero-plugin-for-aws:latest",
			setEnvVars: map[string]string{
				"REGISTRY":               "quay.io",
				"PROJECT":                "konveyor",
				"VELERO_AWS_PLUGIN_REPO": "velero-plugin-for-aws",
				"VELERO_AWS_PLUGIN_TAG":  "latest",
			},
		},

		// Legacy AWS tests
		{
			name: "given legacy aws plugin override, custom aws image should be returned",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginLegacyAWS,
							},
						},
					},
					UnsupportedOverrides: map[oadpv1alpha1.UnsupportedImageKey]string{
						oadpv1alpha1.LegacyAWSPluginImageKey: "test-image",
					},
				},
			},
			pluginName: oadpv1alpha1.DefaultPluginLegacyAWS,
			wantImage:  "test-image",
			setEnvVars: make(map[string]string),
		},
		{
			name: "given default Velero CR with no env var, default legacy aws image should be returned",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginLegacyAWS,
							},
						},
					},
				},
			},
			pluginName: oadpv1alpha1.DefaultPluginLegacyAWS,
			wantImage:  common.LegacyAWSPluginImage,
			setEnvVars: make(map[string]string),
		},
		{
			name: "given default Velero CR with env var set, image should be built via env vars",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginLegacyAWS,
							},
						},
					},
				},
			},
			pluginName: oadpv1alpha1.DefaultPluginLegacyAWS,
			wantImage:  "quay.io/konveyor/velero-plugin-for-legacy-aws:latest",
			setEnvVars: map[string]string{
				"REGISTRY":                      "quay.io",
				"PROJECT":                       "konveyor",
				"VELERO_LEGACY_AWS_PLUGIN_REPO": "velero-plugin-for-legacy-aws",
				"VELERO_LEGACY_AWS_PLUGIN_TAG":  "latest",
			},
		},

		// OpenShift tests
		{
			name: "given openshift plugin override, custom openshift image should be returned",
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
					UnsupportedOverrides: map[oadpv1alpha1.UnsupportedImageKey]string{
						oadpv1alpha1.OpenShiftPluginImageKey: "test-image",
					},
				},
			},
			pluginName: oadpv1alpha1.DefaultPluginOpenShift,
			wantImage:  "test-image",
			setEnvVars: make(map[string]string),
		},
		{
			name: "given default Velero CR with no env var, default openshift image should be returned",
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
			pluginName: oadpv1alpha1.DefaultPluginOpenShift,
			wantImage:  common.OpenshiftPluginImage,
			setEnvVars: make(map[string]string),
		},
		{
			name: "given default Velero CR with env var set, image should be built via env vars",
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
			pluginName: oadpv1alpha1.DefaultPluginOpenShift,
			wantImage:  "quay.io/konveyor/openshift-velero-plugin:latest",
			setEnvVars: map[string]string{
				"REGISTRY":                     "quay.io",
				"PROJECT":                      "konveyor",
				"VELERO_OPENSHIFT_PLUGIN_REPO": "openshift-velero-plugin",
				"VELERO_OPENSHIFT_PLUGIN_TAG":  "latest",
			},
		},

		// GCP tests
		{
			name: "given gcp plugin override, custom gcp image should be returned",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginGCP,
							},
						},
					},
					UnsupportedOverrides: map[oadpv1alpha1.UnsupportedImageKey]string{
						oadpv1alpha1.GCPPluginImageKey: "test-image",
					},
				},
			},
			pluginName: oadpv1alpha1.DefaultPluginGCP,
			wantImage:  "test-image",
			setEnvVars: make(map[string]string),
		},
		{
			name: "given default Velero CR with no env var, default gcp image should be returned",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginGCP,
							},
						},
					},
				},
			},
			pluginName: oadpv1alpha1.DefaultPluginGCP,
			wantImage:  common.GCPPluginImage,
			setEnvVars: make(map[string]string),
		},
		{
			name: "given default Velero CR with env var set, image should be built via env vars",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginGCP,
							},
						},
					},
				},
			},
			pluginName: oadpv1alpha1.DefaultPluginGCP,
			wantImage:  "quay.io/konveyor/velero-plugin-for-gcp:latest",
			setEnvVars: map[string]string{
				"REGISTRY":               "quay.io",
				"PROJECT":                "konveyor",
				"VELERO_GCP_PLUGIN_REPO": "velero-plugin-for-gcp",
				"VELERO_GCP_PLUGIN_TAG":  "latest",
			},
		},

		// Azure tests
		{
			name: "given azure plugin override, custom azure image should be returned",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginMicrosoftAzure,
							},
						},
					},
					UnsupportedOverrides: map[oadpv1alpha1.UnsupportedImageKey]string{
						oadpv1alpha1.AzurePluginImageKey: "test-image",
					},
				},
			},
			pluginName: oadpv1alpha1.DefaultPluginMicrosoftAzure,
			wantImage:  "test-image",
			setEnvVars: make(map[string]string),
		},
		{
			name: "given default Velero CR with no env var, default azure image should be returned",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginMicrosoftAzure,
							},
						},
					},
				},
			},
			pluginName: oadpv1alpha1.DefaultPluginMicrosoftAzure,
			wantImage:  common.AzurePluginImage,
			setEnvVars: make(map[string]string),
		},
		{
			name: "given default Velero CR with env var set, image should be built via env vars",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginMicrosoftAzure,
							},
						},
					},
				},
			},
			pluginName: oadpv1alpha1.DefaultPluginMicrosoftAzure,
			wantImage:  "quay.io/konveyor/velero-plugin-for-microsoft-azure:latest",
			setEnvVars: map[string]string{
				"REGISTRY":                 "quay.io",
				"PROJECT":                  "konveyor",
				"VELERO_AZURE_PLUGIN_REPO": "velero-plugin-for-microsoft-azure",
				"VELERO_AZURE_PLUGIN_TAG":  "latest",
			},
		},
		// KubeVirt tests
		{
			name: "given default Velero CR without env var set, image should be built from default",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginKubeVirt,
							},
						},
					},
				},
			},
			pluginName: oadpv1alpha1.DefaultPluginKubeVirt,
			wantImage:  "quay.io/konveyor/kubevirt-velero-plugin:v0.7.0",
		},
		{
			name: "given default Velero CR with env var set, image should be built via env vars",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginKubeVirt,
							},
						},
					},
				},
			},
			pluginName: oadpv1alpha1.DefaultPluginKubeVirt,
			wantImage:  "quay.io/kubevirt/kubevirt-velero-plugin:latest",
			setEnvVars: map[string]string{
				"RELATED_IMAGE_KUBEVIRT_VELERO_PLUGIN": "quay.io/kubevirt/kubevirt-velero-plugin:latest",
			},
		},
		// Hypershift tests
		{
			name: "given default Velero CR with no env var, default hypershift image should be returned",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginHypershift,
							},
						},
					},
				},
			},
			pluginName: oadpv1alpha1.DefaultPluginHypershift,
			wantImage:  common.HypershiftPluginImage,
		},
		{
			name: "given hypershift plugin override, custom hypershift image should be returned",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginHypershift,
							},
						},
					},
				},
			},
			pluginName: oadpv1alpha1.DefaultPluginHypershift,
			wantImage:  "quay.io/redhat-user-workloads/ocp-art-tenant/oadp-hypershift-oadp-plugin-main:main",
			setEnvVars: map[string]string{
				"RELATED_IMAGE_HYPERSHIFT_VELERO_PLUGIN": "quay.io/redhat-user-workloads/ocp-art-tenant/oadp-hypershift-oadp-plugin-main:main",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for key, value := range tt.setEnvVars {
				t.Setenv(key, value)
			}
			gotImage := GetPluginImage(tt.pluginName, tt.dpa)
			if gotImage != tt.wantImage {
				t.Errorf("Expected plugin image %v did not match %v", tt.wantImage, gotImage)
			}
		})
	}
}

func TestGetSecretNameAndKey(t *testing.T) {
	tests := []struct {
		name           string
		bslSpec        *velerov1.BackupStorageLocationSpec
		wantSecretName string
		wantSecretKey  string
	}{
		{
			name: "no credential or credentialsFile, plugin defaults are returned",
			bslSpec: &velerov1.BackupStorageLocationSpec{
				Provider: "aws",
			},
			wantSecretName: "cloud-credentials",
			wantSecretKey:  "cloud",
		},
		{
			name: "only credentialsFile set, it is ignored and plugin defaults are returned",
			bslSpec: &velerov1.BackupStorageLocationSpec{
				Provider: "aws",
				Config: map[string]string{
					"credentialsFile": "custom-secret/custom-key",
				},
			},
			wantSecretName: "cloud-credentials",
			wantSecretKey:  "cloud",
		},
		{
			name: "credential set, it takes precedence over credentialsFile",
			bslSpec: &velerov1.BackupStorageLocationSpec{
				Provider: "aws",
				Credential: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "custom-secret"},
					Key:                  "custom-key",
				},
				Config: map[string]string{
					"credentialsFile": "some-other-secret/some-other-key",
				},
			},
			wantSecretName: "custom-secret",
			wantSecretKey:  "custom-key",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotKey := GetSecretNameAndKey(tt.bslSpec, oadpv1alpha1.DefaultPlugin(tt.bslSpec.Provider))
			if gotName != tt.wantSecretName {
				t.Errorf("expected secret name %v, got %v", tt.wantSecretName, gotName)
			}
			if gotKey != tt.wantSecretKey {
				t.Errorf("expected secret key %v, got %v", tt.wantSecretKey, gotKey)
			}
		})
	}
}

func TestAppendCloudProviderVolumes_CredentialsFile(t *testing.T) {
	tests := []struct {
		name            string
		backupLocations []oadpv1alpha1.BackupLocation
		wantVolumeNames []string
	}{
		{
			name: "given spec.credential set on BSL, a volume is mounted for it",
			backupLocations: []oadpv1alpha1.BackupLocation{
				{
					Velero: &velerov1.BackupStorageLocationSpec{
						Credential: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: "custom-secret"},
							Key:                  "cloud",
						},
					},
				},
			},
			wantVolumeNames: []string{"custom-secret"},
		},
		{
			name: "given only config.credentialsFile set on BSL, no volume is mounted for it",
			backupLocations: []oadpv1alpha1.BackupLocation{
				{
					Velero: &velerov1.BackupStorageLocationSpec{
						Config: map[string]string{
							"credentialsFile": "some-secret/cloud",
						},
					},
				},
			},
			wantVolumeNames: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dpa := &oadpv1alpha1.DataProtectionApplication{
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
					BackupLocations: tt.backupLocations,
				},
			}
			ds := &appsv1.DaemonSet{}

			AppendCloudProviderVolumes(dpa, ds, map[string]bool{})

			var gotVolumeNames []string
			for _, v := range ds.Spec.Template.Spec.Volumes {
				gotVolumeNames = append(gotVolumeNames, v.Name)
			}
			if !reflect.DeepEqual(tt.wantVolumeNames, gotVolumeNames) {
				t.Errorf("expected volumes %#v, got %#v", tt.wantVolumeNames, gotVolumeNames)
			}
		})
	}
}
