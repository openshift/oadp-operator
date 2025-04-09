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

func isBackupRepositoryCmRequired(config *oadpv1alpha1.NodeAgentConfig) bool {
	return config != nil && (config.KopiaRepoOptions.CacheLimitMB != nil || len(config.KopiaRepoOptions.FullMaintenanceInterval) > 0)
}

// updateBackupRepositoryCM handles the creation or update of the BackupRepository ConfigMap with all required data.
func (r *DataProtectionApplicationReconciler) updateBackupRepositoryCM(cm *corev1.ConfigMap) error {
	// Set the owner reference to ensure the ConfigMap is managed by the DPA
	if err := controllerutil.SetControllerReference(r.dpa, cm, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}

	// Convert KopiaRepoOptions to a generic map
	configBackupRepositoryJSON, err := json.Marshal(r.dpa.Spec.Configuration.NodeAgent.KopiaRepoOptions)
	if err != nil {
		return fmt.Errorf("failed to serialize backup repository config: %w", err)
	}

	cm.Name = common.BackupRepoConfigMapPrefix + r.dpa.Name
	cm.Namespace = r.NamespacedName.Namespace
	cm.Labels = map[string]string{
		"app.kubernetes.io/instance":   r.dpa.Name,
		"app.kubernetes.io/managed-by": common.OADPOperator,
		"app.kubernetes.io/component":  "backup-repository-config",
		oadpv1alpha1.OadpOperatorLabel: "True",
	}

	if cm.Data == nil {
		cm.Data = make(map[string]string)
	}
	cm.Data["kopia"] = string(configBackupRepositoryJSON)

	return nil
}

// GetBackupRepositoryConfigMapName returns the NamespacedName of the BackupRepository ConfigMap
func (r *DataProtectionApplicationReconciler) GetBackupRepositoryConfigMapName() types.NamespacedName {
	return types.NamespacedName{
		Name:      common.BackupRepoConfigMapPrefix + r.dpa.Name,
		Namespace: r.NamespacedName.Namespace,
	}
}

// ReconcileBackupRepositoryConfigMap handles creation, update, and deletion of the BackupRepository ConfigMap.
func (r *DataProtectionApplicationReconciler) ReconcileBackupRepositoryConfigMap(log logr.Logger) (bool, error) {
	dpa := r.dpa
	cmName := r.GetBackupRepositoryConfigMapName()
	configMap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName.Name,
			Namespace: cmName.Namespace,
		},
	}

	// Delete CM if it is not required
	if !isBackupRepositoryCmRequired(dpa.Spec.Configuration.NodeAgent) {
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
		r.EventRecorder.Event(&configMap, corev1.EventTypeNormal, "DeletedBackupRepositoryConfigMap", "BackupRepository config map deleted")
		return true, nil
	}

	op, err := controllerutil.CreateOrPatch(r.Context, r.Client, &configMap, func() error {
		return r.updateBackupRepositoryCM(&configMap)
	})
	if err != nil {
		return false, fmt.Errorf("failed to create or patch config map: %w", err)
	}

	if op == controllerutil.OperationResultCreated {
		r.EventRecorder.Event(&configMap, corev1.EventTypeNormal, "CreatedBackupRepositoryConfigMap", "BackupRepository config map created")
	} else if op == controllerutil.OperationResultUpdated {
		r.EventRecorder.Event(&configMap, corev1.EventTypeNormal, "UpdatedBackupRepositoryConfigMap", "BackupRepository config map updated")
	}

	return true, nil
}
