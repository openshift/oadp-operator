package credentials

import (
	"context"
	"os"
	"testing"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/client"
	"github.com/openshift/oadp-operator/pkg/common"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
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

func TestSecretContainsShortLivedCredential(t *testing.T) {
	type args struct {
		decodedSecret string
		provider      string
		config        map[string]string
	}
	tests := []struct {
		name    string
		args    args
		want    bool
		wantErr bool
	}{
		{
			name: "given gcp decoded with service_account, should return false",
			args: args{
				decodedSecret: `{
  "type": "service_account",
  "project_id": "PROJECT_ID",
  "private_key_id": "KEY_ID",
  "private_key": "-----BEGIN PRIVATE KEY-----\nPRIVATE_KEY\n-----END PRIVATE KEY-----\n",
  "client_email": "SERVICE_ACCOUNT_EMAIL",
  "client_id": "CLIENT_ID",
  "auth_uri": "https://accounts.google.com/o/oauth2/auth",
  "token_uri": "https://accounts.google.com/o/oauth2/token",
  "auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
  "client_x509_cert_url": "https://www.googleapis.com/robot/v1/metadata/x509/SERVICE_ACCOUNT_EMAIL"
}
`,
				provider: "gcp",
				config:   nil,
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "given gcp decoded with external_account, should return true",
			args: args{
				decodedSecret: `{
  "type": "external_account",
  "audience": "//iam.googleapis.com/locations/global/workforcePools/WORKFORCE_POOL_ID/providers/PROVIDER_ID",
  "subject_token_type": "urn:ietf:params:oauth:token-type:id_token",
  "token_url": "https://sts.googleapis.com/v1/token",
  "workforce_pool_user_project": "WORKFORCE_POOL_USER_PROJECT",
  "credential_source": {
    "file": "PATH_TO_OIDC_CREDENTIALS_FILE"
  }
}
`,
				provider: "gcp",
				config:   nil,
			},
			want:    true,
			wantErr: false,
		},
	}
	testEnv := &envtest.Environment{}
	//start testEnv
	cfg, err := testEnv.Start()
	if err != nil {
		t.Fatalf("failed to start testEnv: %v", err)
	}
	defer testEnv.Stop()
	_, err = client.NewClientFromConfig(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	namespace := "test-ns"
	secretName := "test-secret"
	secretKey := "cloud"
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns := corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}
			err = client.CreateOrUpdate(context.Background(), &ns)
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: namespace,
				},
				Data: map[string][]byte{
					secretKey: []byte(tt.args.decodedSecret),
				},
			}
			err = client.CreateOrUpdate(context.Background(), secret)
			if err != nil {
				t.Fatalf("failed to create secret: %v", err)
			}
			if got, err := SecretContainsShortLivedCredential(secretName, secretKey, tt.args.provider, namespace, tt.args.config); got != tt.want {
				t.Errorf("SecretContainsShortLivedCredential() = %v, want %v", got, tt.want)
			} else if err != nil && !tt.wantErr {
				t.Errorf("SecretContainsShortLivedCredential() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
