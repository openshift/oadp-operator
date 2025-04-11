package controller

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/common"
)

func isRepositoryMaintenanceCmRequired(config *oadpv1alpha1.ApplicationConfig) bool {
	return config != nil && config.RepositoryMaintenance != nil && len(config.RepositoryMaintenance) > 0
}

// updateRepositoryMaintenanceCM handles the creation or update of the RepositoryMaintenance ConfigMap with all required data.
func (r *DataProtectionApplicationReconciler) updateRepositoryMaintenanceCM(cm *corev1.ConfigMap) error {
	// Set the owner reference to ensure the ConfigMap is managed by the DPA
	if err := controllerutil.SetControllerReference(r.dpa, cm, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}

	// Convert NodeAgentConfigMapSettings to a generic map
	configRepositoryMaintenanceJSON, err := json.Marshal(r.dpa.Spec.Configuration.RepositoryMaintenance)
	if err != nil {
		return fmt.Errorf("failed to serialize repository maintenance config: %w", err)
	}

	cm.Name = common.RepoMaintConfigMapPrefix + r.dpa.Name
	cm.Namespace = r.NamespacedName.Namespace
	cm.Labels = map[string]string{
		"app.kubernetes.io/instance":   r.dpa.Name,
		"app.kubernetes.io/managed-by": common.OADPOperator,
		"app.kubernetes.io/component":  "repository-maintenance-config",
		oadpv1alpha1.OadpOperatorLabel: "True",
	}

	if cm.Data == nil {
		cm.Data = make(map[string]string)
	}
	cm.Data["repository-maintenance-config"] = string(configRepositoryMaintenanceJSON)

	return nil
}

// GetRepositoryMaintenanceConfigMapName returns the NamespacedName of the RepositoryMaintenance ConfigMap
func (r *DataProtectionApplicationReconciler) GetRepositoryMaintenanceConfigMapName() types.NamespacedName {
	return types.NamespacedName{
		Name:      common.RepoMaintConfigMapPrefix + r.dpa.Name,
		Namespace: r.NamespacedName.Namespace,
	}
}

// ReconcileRepositoryMaintenanceConfigMap handles creation, update, and deletion of the RepositoryMaintenance ConfigMap.
func (r *DataProtectionApplicationReconciler) ReconcileRepositoryMaintenanceConfigMap(log logr.Logger) (bool, error) {
	dpa := r.dpa
	cmName := r.GetRepositoryMaintenanceConfigMapName()
	configMap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName.Name,
			Namespace: cmName.Namespace,
		},
	}

	// Delete CM if it is not required
	if !isRepositoryMaintenanceCmRequired(dpa.Spec.Configuration) {
		err := r.Get(r.Context, cmName, &configMap)
		if err != nil && !errors.IsNotFound(err) {
			return false, err
		}
		if errors.IsNotFound(err) {
			return true, nil
		}
		deleteContext := context.Background()
		if err := r.Delete(deleteContext, &configMap); err != nil {
			if errors.IsNotFound(err) {
				return true, nil
			}
			return false, err
		}
		r.EventRecorder.Event(&configMap, corev1.EventTypeNormal, "DeletedRepositoryMaintenanceConfigMap", "RepositoryMaintenance config map deleted")
		return true, nil
	}

	op, err := controllerutil.CreateOrPatch(r.Context, r.Client, &configMap, func() error {
		return r.updateRepositoryMaintenanceCM(&configMap)
	})
	if err != nil {
		return false, fmt.Errorf("failed to create or patch config map: %w", err)
	}

	if op == controllerutil.OperationResultCreated {
		r.EventRecorder.Event(&configMap, corev1.EventTypeNormal, "CreatedRepositoryMaintenanceConfigMap", "RepositoryMaintenance config map created")
	} else if op == controllerutil.OperationResultUpdated {
		r.EventRecorder.Event(&configMap, corev1.EventTypeNormal, "UpdatedRepositoryMaintenanceConfigMap", "RepositoryMaintenance config map updated")
	}

	return true, nil
}
