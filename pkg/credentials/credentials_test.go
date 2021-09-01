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
		VeleroCR   *oadpv1alpha1.Velero
		pluginName string
		wantImage  string
		setEnvVars map[string]string
	}{
		// AWS tests
		{
			name: "given aws plugin override, custom aws image should be returned",
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					DefaultVeleroPlugins: []oadpv1alpha1.DefaultPlugin{
						oadpv1alpha1.DefaultPluginAWS,
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
			VeleroCR: &oadpv1alpha1.Velero{
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
			pluginName: common.VeleroPluginForAWS,
			wantImage:  common.AWSPluginImage,
			setEnvVars: make(map[string]string),
		},
		{
			name: "given default Velero CR with env var set, image should be built via env vars",
			VeleroCR: &oadpv1alpha1.Velero{
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
			pluginName: common.VeleroPluginForAWS,
			wantImage:  "quay.io/konveyor/velero-plugin-for-aws:latest",
			setEnvVars: map[string]string{
				"REGISTRY":               "quay.io",
				"PROJECT":                "konveyor",
				"VELERO_AWS_PLUGIN_REPO": "velero-plugin-for-aws",
				"VELERO_AWS_PLUGIN_TAG":  "latest",
			},
		},

		// OpenShift tests
		{
			name: "given openshift plugin override, custom openshift image should be returned",
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					DefaultVeleroPlugins: []oadpv1alpha1.DefaultPlugin{
						oadpv1alpha1.DefaultPluginOpenShift,
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
			VeleroCR: &oadpv1alpha1.Velero{
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
			pluginName: common.VeleroPluginForOpenshift,
			wantImage:  common.OpenshiftPluginImage,
			setEnvVars: make(map[string]string),
		},
		{
			name: "given default Velero CR with env var set, image should be built via env vars",
			VeleroCR: &oadpv1alpha1.Velero{
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
			pluginName: common.VeleroPluginForOpenshift,
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
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					DefaultVeleroPlugins: []oadpv1alpha1.DefaultPlugin{
						oadpv1alpha1.DefaultPluginGCP,
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
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					DefaultVeleroPlugins: []oadpv1alpha1.DefaultPlugin{
						oadpv1alpha1.DefaultPluginGCP,
					},
				},
			},
			pluginName: common.VeleroPluginForGCP,
			wantImage:  common.GCPPluginImage,
			setEnvVars: make(map[string]string),
		},
		{
			name: "given default Velero CR with env var set, image should be built via env vars",
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					DefaultVeleroPlugins: []oadpv1alpha1.DefaultPlugin{
						oadpv1alpha1.DefaultPluginGCP,
					},
				},
			},
			pluginName: common.VeleroPluginForGCP,
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
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					DefaultVeleroPlugins: []oadpv1alpha1.DefaultPlugin{
						oadpv1alpha1.DefaultPluginMicrosoftAzure,
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
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					DefaultVeleroPlugins: []oadpv1alpha1.DefaultPlugin{
						oadpv1alpha1.DefaultPluginMicrosoftAzure,
					},
				},
			},
			pluginName: common.VeleroPluginForAzure,
			wantImage:  common.AzurePluginImage,
			setEnvVars: make(map[string]string),
		},
		{
			name: "given default Velero CR with env var set, image should be built via env vars",
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					DefaultVeleroPlugins: []oadpv1alpha1.DefaultPlugin{
						oadpv1alpha1.DefaultPluginMicrosoftAzure,
					},
				},
			},
			pluginName: common.VeleroPluginForAzure,
			wantImage:  "quay.io/konveyor/velero-plugin-for-microsoft-azure:latest",
			setEnvVars: map[string]string{
				"REGISTRY":                 "quay.io",
				"PROJECT":                  "konveyor",
				"VELERO_AZURE_PLUGIN_REPO": "velero-plugin-for-microsoft-azure",
				"VELERO_AZURE_PLUGIN_TAG":  "latest",
			},
		},

		// CSI tests
		{
			name: "given csi plugin override, custom csi image should be returned",
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					DefaultVeleroPlugins: []oadpv1alpha1.DefaultPlugin{
						oadpv1alpha1.DefaultPluginCSI,
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
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					DefaultVeleroPlugins: []oadpv1alpha1.DefaultPlugin{
						oadpv1alpha1.DefaultPluginCSI,
					},
				},
			},
			pluginName: common.VeleroPluginForCSI,
			wantImage:  common.CSIPluginImage,
			setEnvVars: make(map[string]string),
		},
		{
			name: "given default Velero CR with env var set, image should be built via env vars",
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					DefaultVeleroPlugins: []oadpv1alpha1.DefaultPlugin{
						oadpv1alpha1.DefaultPluginCSI,
					},
				},
			},
			pluginName: common.VeleroPluginForCSI,
			wantImage:  "quay.io/konveyor/velero-plugin-for-csi:latest",
			setEnvVars: map[string]string{
				"REGISTRY":               "quay.io",
				"PROJECT":                "konveyor",
				"VELERO_CSI_PLUGIN_REPO": "velero-plugin-for-csi",
				"VELERO_CSI_PLUGIN_TAG":  "latest",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for key, value := range tt.setEnvVars {
				os.Setenv(key, value)
				defer os.Unsetenv(key)
			}
			gotImage := getPluginImage(tt.pluginName, tt.VeleroCR)
			if gotImage != tt.wantImage {
				t.Errorf("Expected plugin image %v did not match %v", tt.wantImage, gotImage)
			}
		})
	}
}
