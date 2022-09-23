package credentials

import (
	"os"
	"testing"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCredentials_getPluginImage(t *testing.T) {
	tests := []struct {
		name       string
		dpa        *oadpv1alpha1.DataProtectionApplication
		pluginName string
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
			pluginName: common.VeleroPluginForAWS,
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
			pluginName: common.VeleroPluginForAWS,
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
			pluginName: common.VeleroPluginForAWS,
			wantImage:  "quay.io/konveyor/velero-plugin-for-aws:oadp-1.0",
			setEnvVars: map[string]string{
				"REGISTRY":               "quay.io",
				"PROJECT":                "konveyor",
				"VELERO_AWS_PLUGIN_REPO": "velero-plugin-for-aws",
				"VELERO_AWS_PLUGIN_TAG":  "oadp-1.0",
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
			pluginName: common.VeleroPluginForOpenshift,
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
			pluginName: common.VeleroPluginForOpenshift,
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
			pluginName: common.VeleroPluginForOpenshift,
			wantImage:  "quay.io/konveyor/openshift-velero-plugin:oadp-1.0",
			setEnvVars: map[string]string{
				"REGISTRY":                     "quay.io",
				"PROJECT":                      "konveyor",
				"VELERO_OPENSHIFT_PLUGIN_REPO": "openshift-velero-plugin",
				"VELERO_OPENSHIFT_PLUGIN_TAG":  "oadp-1.0",
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
			pluginName: common.VeleroPluginForGCP,
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
			pluginName: common.VeleroPluginForGCP,
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
			pluginName: common.VeleroPluginForGCP,
			wantImage:  "quay.io/konveyor/velero-plugin-for-gcp:oadp-1.0",
			setEnvVars: map[string]string{
				"REGISTRY":               "quay.io",
				"PROJECT":                "konveyor",
				"VELERO_GCP_PLUGIN_REPO": "velero-plugin-for-gcp",
				"VELERO_GCP_PLUGIN_TAG":  "oadp-1.0",
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
			pluginName: common.VeleroPluginForAzure,
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
			pluginName: common.VeleroPluginForAzure,
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
			pluginName: common.VeleroPluginForAzure,
			wantImage:  "quay.io/konveyor/velero-plugin-for-microsoft-azure:oadp-1.0",
			setEnvVars: map[string]string{
				"REGISTRY":                 "quay.io",
				"PROJECT":                  "konveyor",
				"VELERO_AZURE_PLUGIN_REPO": "velero-plugin-for-microsoft-azure",
				"VELERO_AZURE_PLUGIN_TAG":  "oadp-1.0",
			},
		},

		// CSI tests
		{
			name: "given csi plugin override, custom csi image should be returned",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginCSI,
							},
						},
					},
					UnsupportedOverrides: map[oadpv1alpha1.UnsupportedImageKey]string{
						oadpv1alpha1.CSIPluginImageKey: "test-image",
					},
				},
			},
			pluginName: common.VeleroPluginForCSI,
			wantImage:  "test-image",
			setEnvVars: make(map[string]string),
		},
		{
			name: "given default Velero CR with no env var, default csi image should be returned",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginCSI,
							},
						},
					},
				},
			},
			pluginName: common.VeleroPluginForCSI,
			wantImage:  common.CSIPluginImage,
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
								oadpv1alpha1.DefaultPluginCSI,
							},
						},
					},
				},
			},
			pluginName: common.VeleroPluginForCSI,
			wantImage:  "quay.io/konveyor/velero-plugin-for-csi:oadp-1.0",
			setEnvVars: map[string]string{
				"REGISTRY":               "quay.io",
				"PROJECT":                "konveyor",
				"VELERO_CSI_PLUGIN_REPO": "velero-plugin-for-csi",
				"VELERO_CSI_PLUGIN_TAG":  "oadp-1.0",
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
			pluginName: common.KubeVirtPlugin,
			wantImage:  "quay.io/konveyor/kubevirt-velero-plugin:v0.2.0",
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
			pluginName: common.KubeVirtPlugin,
			wantImage:  "quay.io/kubevirt/kubevirt-velero-plugin:latest",
			setEnvVars: map[string]string{
				"RELATED_IMAGE_KUBEVIRT_VELERO_PLUGIN": "quay.io/kubevirt/kubevirt-velero-plugin:latest",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for key, value := range tt.setEnvVars {
				os.Setenv(key, value)
				defer os.Unsetenv(key)
			}
			gotImage := getPluginImage(tt.pluginName, tt.dpa)
			if gotImage != tt.wantImage {
				t.Errorf("Expected plugin image %v did not match %v", tt.wantImage, gotImage)
			}
		})
	}
}
