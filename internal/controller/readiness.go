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
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	k8serror "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/common"
)

// updateReadinessConditions updates all component readiness conditions.
// Returns true if all components are ready, false otherwise.
func (r *DataProtectionApplicationReconciler) updateReadinessConditions() (bool, error) {
	allReady := true

	// Always check Velero (required component)
	veleroReady, err := r.updateVeleroReadinessCondition()
	if err != nil {
		return false, err
	}
	allReady = allReady && veleroReady

	// Check NodeAgent (optional - only if enabled)
	nodeAgentReady, err := r.updateNodeAgentReadinessCondition()
	if err != nil {
		return false, err
	}
	allReady = allReady && nodeAgentReady

	// Check NonAdmin (optional - only if enabled)
	nonAdminReady, err := r.updateNonAdminReadinessCondition()
	if err != nil {
		return false, err
	}
	allReady = allReady && nonAdminReady

	// Check VMFileRestore (optional - only if enabled)
	vmfrReady, err := r.updateVMFileRestoreReadinessCondition()
	if err != nil {
		return false, err
	}
	allReady = allReady && vmfrReady

	return allReady, nil
}

// updateVeleroReadinessCondition checks the Velero deployment readiness and updates the condition.
func (r *DataProtectionApplicationReconciler) updateVeleroReadinessCondition() (bool, error) {
	deployment := &appsv1.Deployment{}
	err := r.Get(r.Context, types.NamespacedName{
		Name:      common.Velero,
		Namespace: r.dpa.Namespace,
	}, deployment)

	if err != nil {
		if k8serror.IsNotFound(err) {
			apimeta.SetStatusCondition(&r.dpa.Status.Conditions, metav1.Condition{
				Type:    oadpv1alpha1.ConditionVeleroReady,
				Status:  metav1.ConditionFalse,
				Reason:  oadpv1alpha1.ReasonComponentNotFound,
				Message: "Velero deployment not found",
			})
			return false, nil
		}
		return false, err
	}

	// Check if deployment is ready: ReadyReplicas >= 1 and == Replicas
	replicas := int32(1)
	if deployment.Spec.Replicas != nil {
		replicas = *deployment.Spec.Replicas
	}
	isReady := deployment.Status.ReadyReplicas >= 1 &&
		deployment.Status.ReadyReplicas == replicas

	if isReady {
		apimeta.SetStatusCondition(&r.dpa.Status.Conditions, metav1.Condition{
			Type:    oadpv1alpha1.ConditionVeleroReady,
			Status:  metav1.ConditionTrue,
			Reason:  oadpv1alpha1.ReasonDeploymentReady,
			Message: fmt.Sprintf("Velero deployment ready: %d/%d replicas", deployment.Status.ReadyReplicas, replicas),
		})
	} else {
		apimeta.SetStatusCondition(&r.dpa.Status.Conditions, metav1.Condition{
			Type:    oadpv1alpha1.ConditionVeleroReady,
			Status:  metav1.ConditionFalse,
			Reason:  oadpv1alpha1.ReasonDeploymentNotReady,
			Message: fmt.Sprintf("Velero deployment not ready: %d/%d replicas ready", deployment.Status.ReadyReplicas, replicas),
		})
	}

	return isReady, nil
}

// updateNodeAgentReadinessCondition checks the NodeAgent DaemonSet readiness and updates the condition.
func (r *DataProtectionApplicationReconciler) updateNodeAgentReadinessCondition() (bool, error) {
	// If NodeAgent not enabled, mark as disabled
	if !isNodeAgentEnabled(r.dpa) {
		apimeta.SetStatusCondition(&r.dpa.Status.Conditions, metav1.Condition{
			Type:    oadpv1alpha1.ConditionNodeAgentReady,
			Status:  metav1.ConditionTrue,
			Reason:  oadpv1alpha1.ReasonComponentDisabled,
			Message: "NodeAgent is disabled",
		})
		return true, nil
	}

	ds := &appsv1.DaemonSet{}
	err := r.Get(r.Context, types.NamespacedName{
		Name:      common.NodeAgent,
		Namespace: r.dpa.Namespace,
	}, ds)

	if err != nil {
		if k8serror.IsNotFound(err) {
			apimeta.SetStatusCondition(&r.dpa.Status.Conditions, metav1.Condition{
				Type:    oadpv1alpha1.ConditionNodeAgentReady,
				Status:  metav1.ConditionFalse,
				Reason:  oadpv1alpha1.ReasonComponentNotFound,
				Message: "NodeAgent DaemonSet not found",
			})
			return false, nil
		}
		return false, err
	}

	// DaemonSet readiness: NumberReady == DesiredNumberScheduled
	// Handle edge case: DesiredNumberScheduled == 0 (no nodes match selector)
	isReady := ds.Status.DesiredNumberScheduled == 0 ||
		ds.Status.NumberReady == ds.Status.DesiredNumberScheduled

	if isReady {
		apimeta.SetStatusCondition(&r.dpa.Status.Conditions, metav1.Condition{
			Type:    oadpv1alpha1.ConditionNodeAgentReady,
			Status:  metav1.ConditionTrue,
			Reason:  oadpv1alpha1.ReasonDaemonSetReady,
			Message: fmt.Sprintf("NodeAgent DaemonSet ready: %d/%d pods ready", ds.Status.NumberReady, ds.Status.DesiredNumberScheduled),
		})
	} else {
		apimeta.SetStatusCondition(&r.dpa.Status.Conditions, metav1.Condition{
			Type:    oadpv1alpha1.ConditionNodeAgentReady,
			Status:  metav1.ConditionFalse,
			Reason:  oadpv1alpha1.ReasonDaemonSetNotReady,
			Message: fmt.Sprintf("NodeAgent DaemonSet not ready: %d/%d pods ready", ds.Status.NumberReady, ds.Status.DesiredNumberScheduled),
		})
	}

	return isReady, nil
}

// updateNonAdminReadinessCondition checks the Non-Admin controller deployment readiness and updates the condition.
func (r *DataProtectionApplicationReconciler) updateNonAdminReadinessCondition() (bool, error) {
	// If NonAdmin not enabled, mark as disabled
	if !r.checkNonAdminEnabled() {
		apimeta.SetStatusCondition(&r.dpa.Status.Conditions, metav1.Condition{
			Type:    oadpv1alpha1.ConditionNonAdminReady,
			Status:  metav1.ConditionTrue,
			Reason:  oadpv1alpha1.ReasonComponentDisabled,
			Message: "Non-Admin controller is disabled",
		})
		return true, nil
	}

	deployment := &appsv1.Deployment{}
	err := r.Get(r.Context, types.NamespacedName{
		Name:      nonAdminObjectName,
		Namespace: r.dpa.Namespace,
	}, deployment)

	if err != nil {
		if k8serror.IsNotFound(err) {
			apimeta.SetStatusCondition(&r.dpa.Status.Conditions, metav1.Condition{
				Type:    oadpv1alpha1.ConditionNonAdminReady,
				Status:  metav1.ConditionFalse,
				Reason:  oadpv1alpha1.ReasonComponentNotFound,
				Message: "Non-Admin controller deployment not found",
			})
			return false, nil
		}
		return false, err
	}

	replicas := int32(1)
	if deployment.Spec.Replicas != nil {
		replicas = *deployment.Spec.Replicas
	}
	isReady := deployment.Status.ReadyReplicas >= 1 &&
		deployment.Status.ReadyReplicas == replicas

	if isReady {
		apimeta.SetStatusCondition(&r.dpa.Status.Conditions, metav1.Condition{
			Type:    oadpv1alpha1.ConditionNonAdminReady,
			Status:  metav1.ConditionTrue,
			Reason:  oadpv1alpha1.ReasonDeploymentReady,
			Message: fmt.Sprintf("Non-Admin controller ready: %d/%d replicas", deployment.Status.ReadyReplicas, replicas),
		})
	} else {
		apimeta.SetStatusCondition(&r.dpa.Status.Conditions, metav1.Condition{
			Type:    oadpv1alpha1.ConditionNonAdminReady,
			Status:  metav1.ConditionFalse,
			Reason:  oadpv1alpha1.ReasonDeploymentNotReady,
			Message: fmt.Sprintf("Non-Admin controller not ready: %d/%d replicas ready", deployment.Status.ReadyReplicas, replicas),
		})
	}

	return isReady, nil
}

// updateVMFileRestoreReadinessCondition checks the VM File Restore controller deployment readiness and updates the condition.
func (r *DataProtectionApplicationReconciler) updateVMFileRestoreReadinessCondition() (bool, error) {
	// If VMFileRestore not enabled, mark as disabled
	if !r.checkVMFileRestoreEnabled() {
		apimeta.SetStatusCondition(&r.dpa.Status.Conditions, metav1.Condition{
			Type:    oadpv1alpha1.ConditionVMFileRestoreReady,
			Status:  metav1.ConditionTrue,
			Reason:  oadpv1alpha1.ReasonComponentDisabled,
			Message: "VM File Restore controller is disabled",
		})
		return true, nil
	}

	deployment := &appsv1.Deployment{}
	err := r.Get(r.Context, types.NamespacedName{
		Name:      vmFileRestoreObjectName,
		Namespace: r.dpa.Namespace,
	}, deployment)

	if err != nil {
		if k8serror.IsNotFound(err) {
			apimeta.SetStatusCondition(&r.dpa.Status.Conditions, metav1.Condition{
				Type:    oadpv1alpha1.ConditionVMFileRestoreReady,
				Status:  metav1.ConditionFalse,
				Reason:  oadpv1alpha1.ReasonComponentNotFound,
				Message: "VM File Restore controller deployment not found",
			})
			return false, nil
		}
		return false, err
	}

	replicas := int32(1)
	if deployment.Spec.Replicas != nil {
		replicas = *deployment.Spec.Replicas
	}
	isReady := deployment.Status.ReadyReplicas >= 1 &&
		deployment.Status.ReadyReplicas == replicas

	if isReady {
		apimeta.SetStatusCondition(&r.dpa.Status.Conditions, metav1.Condition{
			Type:    oadpv1alpha1.ConditionVMFileRestoreReady,
			Status:  metav1.ConditionTrue,
			Reason:  oadpv1alpha1.ReasonDeploymentReady,
			Message: fmt.Sprintf("VM File Restore controller ready: %d/%d replicas", deployment.Status.ReadyReplicas, replicas),
		})
	} else {
		apimeta.SetStatusCondition(&r.dpa.Status.Conditions, metav1.Condition{
			Type:    oadpv1alpha1.ConditionVMFileRestoreReady,
			Status:  metav1.ConditionFalse,
			Reason:  oadpv1alpha1.ReasonDeploymentNotReady,
			Message: fmt.Sprintf("VM File Restore controller not ready: %d/%d replicas ready", deployment.Status.ReadyReplicas, replicas),
		})
	}

	return isReady, nil
}
