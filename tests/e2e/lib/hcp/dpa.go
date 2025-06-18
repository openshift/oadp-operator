package hcp

import (
	"context"
	"fmt"
	"log"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
)

// AddHCPPluginToDPA adds the HCP plugin to a DPA
func (h *HCHandler) AddHCPPluginToDPA(namespace, name string, overrides bool) error {
	addHCPlugin := true

	log.Printf("Adding HCP default plugin to DPA")
	dpa := &oadpv1alpha1.DataProtectionApplication{}
	err := h.Client.Get(h.Ctx, types.NamespacedName{Namespace: namespace, Name: name}, dpa)
	if err != nil {
		return err
	}

	// Check if the hypershift plugin is already in the default plugins
	for _, plugin := range dpa.Spec.Configuration.Velero.DefaultPlugins {
		if plugin == oadpv1alpha1.DefaultPluginHypershift {
			log.Printf("HCP plugin already in DPA")
			if overrides {
				log.Printf("Override set to true, removing HCP plugin from DPA")
				addHCPlugin = false
				break
			}
			return nil
		}
	}

	if addHCPlugin {
		dpa.Spec.Configuration.Velero.DefaultPlugins = append(dpa.Spec.Configuration.Velero.DefaultPlugins, oadpv1alpha1.DefaultPluginHypershift)
	}

	if overrides {
		dpa.Spec.UnsupportedOverrides = map[oadpv1alpha1.UnsupportedImageKey]string{
			oadpv1alpha1.HypershiftPluginImageKey: "quay.io/redhat-user-workloads/ocp-art-tenant/oadp-hypershift-oadp-plugin-oadp-1-5:oadp-1.5",
		}
	}

	err = h.Client.Update(h.Ctx, dpa)
	if err != nil {
		return fmt.Errorf("failed to update DPA: %v", err)
	}
	log.Printf("HCP plugin added to DPA")
	return nil
}

// RemoveHCPPluginFromDPA removes the HCP plugin from a DPA
func (h *HCHandler) RemoveHCPPluginFromDPA(namespace, name string) error {
	log.Printf("Removing HCP plugin from DPA")
	dpa := &oadpv1alpha1.DataProtectionApplication{}
	err := h.Client.Get(h.Ctx, types.NamespacedName{Namespace: namespace, Name: name}, dpa)
	if err != nil {
		return err
	}
	delete(dpa.Spec.UnsupportedOverrides, oadpv1alpha1.HypershiftPluginImageKey)
	// remove hypershift plugin from default plugins
	for i, plugin := range dpa.Spec.Configuration.Velero.DefaultPlugins {
		if plugin == oadpv1alpha1.DefaultPluginHypershift {
			dpa.Spec.Configuration.Velero.DefaultPlugins = append(dpa.Spec.Configuration.Velero.DefaultPlugins[:i], dpa.Spec.Configuration.Velero.DefaultPlugins[i+1:]...)
			break
		}
	}
	err = h.Client.Update(h.Ctx, dpa)
	if err != nil {
		return fmt.Errorf("failed to update DPA: %v", err)
	}
	log.Printf("HCP plugin removed from DPA")
	return nil
}

// IsHCPPluginAdded checks if the HCP plugin is added to a DPA
func IsHCPPluginAdded(c client.Client, namespace, name string) bool {
	dpa := &oadpv1alpha1.DataProtectionApplication{}
	err := c.Get(context.Background(), types.NamespacedName{Namespace: namespace, Name: name}, dpa)
	if err != nil {
		return false
	}

	if dpa.Spec.Configuration == nil || dpa.Spec.Configuration.Velero == nil {
		return false
	}

	for _, plugin := range dpa.Spec.Configuration.Velero.DefaultPlugins {
		if plugin == oadpv1alpha1.DefaultPluginHypershift {
			return true
		}
	}

	return false
}
