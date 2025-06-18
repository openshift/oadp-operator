package hcp

import (
	"context"
	"testing"

	"github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
)

func TestAddHCPPluginToDPA(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	tests := []struct {
		name      string
		dpa       *oadpv1alpha1.DataProtectionApplication
		overrides bool
	}{
		{
			name: "Add plugin without overrides",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{},
						},
					},
				},
			},
			overrides: false,
		},
		{
			name: "Add plugin with overrides",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{},
						},
					},
				},
			},
			overrides: true,
		},
		{
			name: "Plugin already exists",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
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
			overrides: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new scheme with DataProtectionApplication registered
			scheme := runtime.NewScheme()
			err := oadpv1alpha1.AddToScheme(scheme)
			g.Expect(err).NotTo(gomega.HaveOccurred())

			// Create a new client with the scheme
			client := fake.NewClientBuilder().WithScheme(scheme).Build()

			// Create the handler
			h := &HCHandler{
				Ctx:    context.Background(),
				Client: client,
			}

			// Create DPA
			err = client.Create(context.Background(), tt.dpa)
			g.Expect(err).NotTo(gomega.HaveOccurred())

			// Call AddHCPPluginToDPA
			err = h.AddHCPPluginToDPA(tt.dpa.Namespace, tt.dpa.Name, tt.overrides)
			g.Expect(err).NotTo(gomega.HaveOccurred())

			// Verify DPA was updated
			updatedDPA := &oadpv1alpha1.DataProtectionApplication{}
			err = client.Get(context.Background(), types.NamespacedName{Name: tt.dpa.Name, Namespace: tt.dpa.Namespace}, updatedDPA)
			g.Expect(err).NotTo(gomega.HaveOccurred())

			// Check if plugin was added
			pluginFound := false
			for _, plugin := range updatedDPA.Spec.Configuration.Velero.DefaultPlugins {
				if plugin == oadpv1alpha1.DefaultPluginHypershift {
					pluginFound = true
					break
				}
			}
			g.Expect(pluginFound).To(gomega.BeTrue())

			// Check if overrides were added
			if tt.overrides {
				g.Expect(updatedDPA.Spec.UnsupportedOverrides).To(gomega.HaveKey(oadpv1alpha1.HypershiftPluginImageKey))
			}
		})
	}
}

func TestRemoveHCPPluginFromDPA(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	tests := []struct {
		name string
		dpa  *oadpv1alpha1.DataProtectionApplication
	}{
		{
			name: "Remove plugin",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
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
					UnsupportedOverrides: map[oadpv1alpha1.UnsupportedImageKey]string{
						oadpv1alpha1.HypershiftPluginImageKey: "quay.io/redhat-user-workloads/ocp-art-tenant/oadp-hypershift-oadp-plugin-main:main",
					},
				},
			},
		},
		{
			name: "Plugin not present",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new scheme with DataProtectionApplication registered
			scheme := runtime.NewScheme()
			err := oadpv1alpha1.AddToScheme(scheme)
			g.Expect(err).NotTo(gomega.HaveOccurred())

			// Create a new client with the scheme
			client := fake.NewClientBuilder().WithScheme(scheme).Build()

			// Create the handler
			h := &HCHandler{
				Ctx:    context.Background(),
				Client: client,
			}

			// Create DPA
			err = client.Create(context.Background(), tt.dpa)
			g.Expect(err).NotTo(gomega.HaveOccurred())

			// Call RemoveHCPPluginFromDPA
			err = h.RemoveHCPPluginFromDPA(tt.dpa.Namespace, tt.dpa.Name)
			g.Expect(err).NotTo(gomega.HaveOccurred())

			// Verify DPA was updated
			updatedDPA := &oadpv1alpha1.DataProtectionApplication{}
			err = client.Get(context.Background(), types.NamespacedName{Name: tt.dpa.Name, Namespace: tt.dpa.Namespace}, updatedDPA)
			g.Expect(err).NotTo(gomega.HaveOccurred())

			// Check if plugin was removed
			pluginFound := false
			for _, plugin := range updatedDPA.Spec.Configuration.Velero.DefaultPlugins {
				if plugin == oadpv1alpha1.DefaultPluginHypershift {
					pluginFound = true
					break
				}
			}
			g.Expect(pluginFound).To(gomega.BeFalse())

			// Check if overrides were removed
			g.Expect(updatedDPA.Spec.UnsupportedOverrides).NotTo(gomega.HaveKey(oadpv1alpha1.HypershiftPluginImageKey))
		})
	}
}

func TestIsHCPPluginAdded(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	tests := []struct {
		name           string
		dpa            *oadpv1alpha1.DataProtectionApplication
		expectedResult bool
	}{
		{
			name: "HCP plugin exists",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
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
			expectedResult: true,
		},
		{
			name: "HCP plugin does not exist",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
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
			expectedResult: false,
		},
		{
			name:           "DPA is nil",
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new scheme with DataProtectionApplication registered
			scheme := runtime.NewScheme()
			err := oadpv1alpha1.AddToScheme(scheme)
			g.Expect(err).NotTo(gomega.HaveOccurred())

			// Create a new client with the scheme
			client := fake.NewClientBuilder().WithScheme(scheme).Build()

			// Create DPA if it exists in the test case
			if tt.dpa != nil {
				err := client.Create(context.Background(), tt.dpa)
				g.Expect(err).NotTo(gomega.HaveOccurred())
			}

			// Call IsHCPPluginAdded
			var result bool
			if tt.dpa != nil {
				result = IsHCPPluginAdded(client, tt.dpa.Namespace, tt.dpa.Name)
			} else {
				result = IsHCPPluginAdded(client, "non-existent", "non-existent")
			}
			g.Expect(result).To(gomega.Equal(tt.expectedResult))
		})
	}
}
