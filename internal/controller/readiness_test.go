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

	"github.com/go-logr/logr"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/common"
)

var _ = ginkgo.Describe("Test Readiness Conditions", func() {
	var ctx = context.Background()

	// Helper to create test resources
	createTestResources := func(nsName, dpaName string) (*corev1.Namespace, *oadpv1alpha1.DataProtectionApplication, *DataProtectionApplicationReconciler) {
		namespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: nsName,
			},
		}
		gomega.Expect(k8sClient.Create(ctx, namespace)).To(gomega.Succeed())

		dpa := &oadpv1alpha1.DataProtectionApplication{
			ObjectMeta: metav1.ObjectMeta{
				Name:      dpaName,
				Namespace: nsName,
			},
			Spec: oadpv1alpha1.DataProtectionApplicationSpec{
				Configuration: &oadpv1alpha1.ApplicationConfig{
					Velero: &oadpv1alpha1.VeleroConfig{},
				},
			},
		}
		gomega.Expect(k8sClient.Create(ctx, dpa)).To(gomega.Succeed())

		r := &DataProtectionApplicationReconciler{
			Client:         k8sClient,
			Scheme:         k8sClient.Scheme(),
			Log:            logr.Discard(),
			Context:        ctx,
			NamespacedName: types.NamespacedName{Name: dpaName, Namespace: nsName},
			EventRecorder:  &record.FakeRecorder{},
			dpa:            dpa,
		}

		return namespace, dpa, r
	}

	// Helper to cleanup test resources
	cleanupTestResources := func(nsName string, namespace *corev1.Namespace, dpa *oadpv1alpha1.DataProtectionApplication) {
		// Cleanup deployments and daemonsets
		veleroDeployment := &appsv1.Deployment{}
		if k8sClient.Get(ctx, types.NamespacedName{Name: common.Velero, Namespace: nsName}, veleroDeployment) == nil {
			_ = k8sClient.Delete(ctx, veleroDeployment)
		}

		nodeAgentDS := &appsv1.DaemonSet{}
		if k8sClient.Get(ctx, types.NamespacedName{Name: common.NodeAgent, Namespace: nsName}, nodeAgentDS) == nil {
			_ = k8sClient.Delete(ctx, nodeAgentDS)
		}

		nonAdminDeployment := &appsv1.Deployment{}
		if k8sClient.Get(ctx, types.NamespacedName{Name: nonAdminObjectName, Namespace: nsName}, nonAdminDeployment) == nil {
			_ = k8sClient.Delete(ctx, nonAdminDeployment)
		}

		vmfrDeployment := &appsv1.Deployment{}
		if k8sClient.Get(ctx, types.NamespacedName{Name: vmFileRestoreObjectName, Namespace: nsName}, vmfrDeployment) == nil {
			_ = k8sClient.Delete(ctx, vmfrDeployment)
		}

		if dpa != nil {
			_ = k8sClient.Delete(ctx, dpa)
		}

		if namespace != nil {
			_ = k8sClient.Delete(ctx, namespace)
		}
	}

	ginkgo.Context("Velero Readiness", func() {
		ginkgo.It("should return false when Velero deployment not found", func() {
			namespace, dpa, r := createTestResources("readiness-velero-1", "dpa-velero-1")
			defer cleanupTestResources("readiness-velero-1", namespace, dpa)

			isReady, err := r.updateVeleroReadinessCondition()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(isReady).To(gomega.BeFalse())

			cond := findCondition(r.dpa.Status.Conditions, oadpv1alpha1.ConditionVeleroReady)
			gomega.Expect(cond).NotTo(gomega.BeNil())
			gomega.Expect(cond.Status).To(gomega.Equal(metav1.ConditionFalse))
			gomega.Expect(cond.Reason).To(gomega.Equal(oadpv1alpha1.ReasonComponentNotFound))
		})

		ginkgo.It("should return false when Velero deployment has no ready replicas", func() {
			namespace, dpa, r := createTestResources("readiness-velero-2", "dpa-velero-2")
			defer cleanupTestResources("readiness-velero-2", namespace, dpa)

			deployment := createReadinessVeleroDeployment("readiness-velero-2", 1, 0)
			gomega.Expect(k8sClient.Create(ctx, deployment)).To(gomega.Succeed())

			isReady, err := r.updateVeleroReadinessCondition()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(isReady).To(gomega.BeFalse())

			cond := findCondition(r.dpa.Status.Conditions, oadpv1alpha1.ConditionVeleroReady)
			gomega.Expect(cond).NotTo(gomega.BeNil())
			gomega.Expect(cond.Status).To(gomega.Equal(metav1.ConditionFalse))
			gomega.Expect(cond.Reason).To(gomega.Equal(oadpv1alpha1.ReasonDeploymentNotReady))
		})

		ginkgo.It("should return true when Velero deployment is ready", func() {
			namespace, dpa, r := createTestResources("readiness-velero-3", "dpa-velero-3")
			defer cleanupTestResources("readiness-velero-3", namespace, dpa)

			deployment := createReadinessVeleroDeployment("readiness-velero-3", 1, 1)
			gomega.Expect(k8sClient.Create(ctx, deployment)).To(gomega.Succeed())

			deployment.Status.ReadyReplicas = 1
			deployment.Status.Replicas = 1
			gomega.Expect(k8sClient.Status().Update(ctx, deployment)).To(gomega.Succeed())

			isReady, err := r.updateVeleroReadinessCondition()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(isReady).To(gomega.BeTrue())

			cond := findCondition(r.dpa.Status.Conditions, oadpv1alpha1.ConditionVeleroReady)
			gomega.Expect(cond).NotTo(gomega.BeNil())
			gomega.Expect(cond.Status).To(gomega.Equal(metav1.ConditionTrue))
			gomega.Expect(cond.Reason).To(gomega.Equal(oadpv1alpha1.ReasonDeploymentReady))
		})
	})

	ginkgo.Context("NodeAgent Readiness", func() {
		ginkgo.It("should return true when NodeAgent is disabled", func() {
			namespace, dpa, r := createTestResources("readiness-na-1", "dpa-na-1")
			defer cleanupTestResources("readiness-na-1", namespace, dpa)

			isReady, err := r.updateNodeAgentReadinessCondition()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(isReady).To(gomega.BeTrue())

			cond := findCondition(r.dpa.Status.Conditions, oadpv1alpha1.ConditionNodeAgentReady)
			gomega.Expect(cond).NotTo(gomega.BeNil())
			gomega.Expect(cond.Status).To(gomega.Equal(metav1.ConditionTrue))
			gomega.Expect(cond.Reason).To(gomega.Equal(oadpv1alpha1.ReasonComponentDisabled))
		})

		ginkgo.It("should return false when NodeAgent is enabled but DaemonSet not found", func() {
			namespace, dpa, r := createTestResources("readiness-na-2", "dpa-na-2")
			defer cleanupTestResources("readiness-na-2", namespace, dpa)

			r.dpa.Spec.Configuration.NodeAgent = &oadpv1alpha1.NodeAgentConfig{
				UploaderType: "kopia",
				NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
					Enable: ptr.To(true),
				},
			}

			isReady, err := r.updateNodeAgentReadinessCondition()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(isReady).To(gomega.BeFalse())

			cond := findCondition(r.dpa.Status.Conditions, oadpv1alpha1.ConditionNodeAgentReady)
			gomega.Expect(cond).NotTo(gomega.BeNil())
			gomega.Expect(cond.Status).To(gomega.Equal(metav1.ConditionFalse))
			gomega.Expect(cond.Reason).To(gomega.Equal(oadpv1alpha1.ReasonComponentNotFound))
		})

		ginkgo.It("should return true when NodeAgent DaemonSet is ready", func() {
			namespace, dpa, r := createTestResources("readiness-na-3", "dpa-na-3")
			defer cleanupTestResources("readiness-na-3", namespace, dpa)

			r.dpa.Spec.Configuration.NodeAgent = &oadpv1alpha1.NodeAgentConfig{
				UploaderType: "kopia",
				NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
					Enable: ptr.To(true),
				},
			}

			ds := createReadinessNodeAgentDaemonSet("readiness-na-3")
			gomega.Expect(k8sClient.Create(ctx, ds)).To(gomega.Succeed())

			ds.Status.DesiredNumberScheduled = 3
			ds.Status.NumberReady = 3
			gomega.Expect(k8sClient.Status().Update(ctx, ds)).To(gomega.Succeed())

			isReady, err := r.updateNodeAgentReadinessCondition()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(isReady).To(gomega.BeTrue())

			cond := findCondition(r.dpa.Status.Conditions, oadpv1alpha1.ConditionNodeAgentReady)
			gomega.Expect(cond).NotTo(gomega.BeNil())
			gomega.Expect(cond.Status).To(gomega.Equal(metav1.ConditionTrue))
			gomega.Expect(cond.Reason).To(gomega.Equal(oadpv1alpha1.ReasonDaemonSetReady))
		})

		ginkgo.It("should return true when NodeAgent has zero desired pods", func() {
			namespace, dpa, r := createTestResources("readiness-na-4", "dpa-na-4")
			defer cleanupTestResources("readiness-na-4", namespace, dpa)

			r.dpa.Spec.Configuration.NodeAgent = &oadpv1alpha1.NodeAgentConfig{
				UploaderType: "kopia",
				NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
					Enable: ptr.To(true),
				},
			}

			ds := createReadinessNodeAgentDaemonSet("readiness-na-4")
			gomega.Expect(k8sClient.Create(ctx, ds)).To(gomega.Succeed())

			ds.Status.DesiredNumberScheduled = 0
			ds.Status.NumberReady = 0
			gomega.Expect(k8sClient.Status().Update(ctx, ds)).To(gomega.Succeed())

			isReady, err := r.updateNodeAgentReadinessCondition()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(isReady).To(gomega.BeTrue())
		})
	})

	ginkgo.Context("NonAdmin Readiness", func() {
		ginkgo.It("should return true when NonAdmin is disabled", func() {
			namespace, dpa, r := createTestResources("readiness-nonadmin-1", "dpa-nonadmin-1")
			defer cleanupTestResources("readiness-nonadmin-1", namespace, dpa)

			isReady, err := r.updateNonAdminReadinessCondition()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(isReady).To(gomega.BeTrue())

			cond := findCondition(r.dpa.Status.Conditions, oadpv1alpha1.ConditionNonAdminReady)
			gomega.Expect(cond).NotTo(gomega.BeNil())
			gomega.Expect(cond.Status).To(gomega.Equal(metav1.ConditionTrue))
			gomega.Expect(cond.Reason).To(gomega.Equal(oadpv1alpha1.ReasonComponentDisabled))
		})

		ginkgo.It("should return false when NonAdmin is enabled but deployment not found", func() {
			namespace, dpa, r := createTestResources("readiness-nonadmin-2", "dpa-nonadmin-2")
			defer cleanupTestResources("readiness-nonadmin-2", namespace, dpa)

			r.dpa.Spec.NonAdmin = &oadpv1alpha1.NonAdmin{
				Enable: ptr.To(true),
			}

			isReady, err := r.updateNonAdminReadinessCondition()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(isReady).To(gomega.BeFalse())

			cond := findCondition(r.dpa.Status.Conditions, oadpv1alpha1.ConditionNonAdminReady)
			gomega.Expect(cond).NotTo(gomega.BeNil())
			gomega.Expect(cond.Status).To(gomega.Equal(metav1.ConditionFalse))
			gomega.Expect(cond.Reason).To(gomega.Equal(oadpv1alpha1.ReasonComponentNotFound))
		})

		ginkgo.It("should return true when NonAdmin deployment is ready", func() {
			namespace, dpa, r := createTestResources("readiness-nonadmin-3", "dpa-nonadmin-3")
			defer cleanupTestResources("readiness-nonadmin-3", namespace, dpa)

			r.dpa.Spec.NonAdmin = &oadpv1alpha1.NonAdmin{
				Enable: ptr.To(true),
			}

			deployment := createReadinessNonAdminDeployment("readiness-nonadmin-3")
			gomega.Expect(k8sClient.Create(ctx, deployment)).To(gomega.Succeed())

			deployment.Status.ReadyReplicas = 1
			deployment.Status.Replicas = 1
			gomega.Expect(k8sClient.Status().Update(ctx, deployment)).To(gomega.Succeed())

			isReady, err := r.updateNonAdminReadinessCondition()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(isReady).To(gomega.BeTrue())

			cond := findCondition(r.dpa.Status.Conditions, oadpv1alpha1.ConditionNonAdminReady)
			gomega.Expect(cond).NotTo(gomega.BeNil())
			gomega.Expect(cond.Status).To(gomega.Equal(metav1.ConditionTrue))
			gomega.Expect(cond.Reason).To(gomega.Equal(oadpv1alpha1.ReasonDeploymentReady))
		})
	})

	ginkgo.Context("VMFileRestore Readiness", func() {
		ginkgo.It("should return true when VMFileRestore is disabled", func() {
			namespace, dpa, r := createTestResources("readiness-vmfr-1", "dpa-vmfr-1")
			defer cleanupTestResources("readiness-vmfr-1", namespace, dpa)

			isReady, err := r.updateVMFileRestoreReadinessCondition()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(isReady).To(gomega.BeTrue())

			cond := findCondition(r.dpa.Status.Conditions, oadpv1alpha1.ConditionVMFileRestoreReady)
			gomega.Expect(cond).NotTo(gomega.BeNil())
			gomega.Expect(cond.Status).To(gomega.Equal(metav1.ConditionTrue))
			gomega.Expect(cond.Reason).To(gomega.Equal(oadpv1alpha1.ReasonComponentDisabled))
		})

		ginkgo.It("should return false when VMFileRestore is enabled but deployment not found", func() {
			namespace, dpa, r := createTestResources("readiness-vmfr-2", "dpa-vmfr-2")
			defer cleanupTestResources("readiness-vmfr-2", namespace, dpa)

			r.dpa.Spec.VMFileRestore = &oadpv1alpha1.VMFileRestore{
				Enable: ptr.To(true),
			}

			isReady, err := r.updateVMFileRestoreReadinessCondition()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(isReady).To(gomega.BeFalse())

			cond := findCondition(r.dpa.Status.Conditions, oadpv1alpha1.ConditionVMFileRestoreReady)
			gomega.Expect(cond).NotTo(gomega.BeNil())
			gomega.Expect(cond.Status).To(gomega.Equal(metav1.ConditionFalse))
			gomega.Expect(cond.Reason).To(gomega.Equal(oadpv1alpha1.ReasonComponentNotFound))
		})

		ginkgo.It("should return true when VMFileRestore deployment is ready", func() {
			namespace, dpa, r := createTestResources("readiness-vmfr-3", "dpa-vmfr-3")
			defer cleanupTestResources("readiness-vmfr-3", namespace, dpa)

			r.dpa.Spec.VMFileRestore = &oadpv1alpha1.VMFileRestore{
				Enable: ptr.To(true),
			}

			deployment := createReadinessVMFileRestoreDeployment("readiness-vmfr-3")
			gomega.Expect(k8sClient.Create(ctx, deployment)).To(gomega.Succeed())

			deployment.Status.ReadyReplicas = 1
			deployment.Status.Replicas = 1
			gomega.Expect(k8sClient.Status().Update(ctx, deployment)).To(gomega.Succeed())

			isReady, err := r.updateVMFileRestoreReadinessCondition()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(isReady).To(gomega.BeTrue())

			cond := findCondition(r.dpa.Status.Conditions, oadpv1alpha1.ConditionVMFileRestoreReady)
			gomega.Expect(cond).NotTo(gomega.BeNil())
			gomega.Expect(cond.Status).To(gomega.Equal(metav1.ConditionTrue))
			gomega.Expect(cond.Reason).To(gomega.Equal(oadpv1alpha1.ReasonDeploymentReady))
		})
	})

	ginkgo.Context("Combined Readiness", func() {
		ginkgo.It("should return false when Velero is not ready even if optional components are disabled", func() {
			namespace, dpa, r := createTestResources("readiness-combined-1", "dpa-combined-1")
			defer cleanupTestResources("readiness-combined-1", namespace, dpa)

			allReady, err := r.updateReadinessConditions()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(allReady).To(gomega.BeFalse())

			veleroCond := findCondition(r.dpa.Status.Conditions, oadpv1alpha1.ConditionVeleroReady)
			gomega.Expect(veleroCond).NotTo(gomega.BeNil())
			gomega.Expect(veleroCond.Status).To(gomega.Equal(metav1.ConditionFalse))

			nodeAgentCond := findCondition(r.dpa.Status.Conditions, oadpv1alpha1.ConditionNodeAgentReady)
			gomega.Expect(nodeAgentCond).NotTo(gomega.BeNil())
			gomega.Expect(nodeAgentCond.Status).To(gomega.Equal(metav1.ConditionTrue))
			gomega.Expect(nodeAgentCond.Reason).To(gomega.Equal(oadpv1alpha1.ReasonComponentDisabled))
		})

		ginkgo.It("should return true when all enabled components are ready", func() {
			namespace, dpa, r := createTestResources("readiness-combined-2", "dpa-combined-2")
			defer cleanupTestResources("readiness-combined-2", namespace, dpa)

			veleroDeployment := createReadinessVeleroDeployment("readiness-combined-2", 1, 1)
			gomega.Expect(k8sClient.Create(ctx, veleroDeployment)).To(gomega.Succeed())
			veleroDeployment.Status.ReadyReplicas = 1
			veleroDeployment.Status.Replicas = 1
			gomega.Expect(k8sClient.Status().Update(ctx, veleroDeployment)).To(gomega.Succeed())

			allReady, err := r.updateReadinessConditions()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(allReady).To(gomega.BeTrue())
		})
	})
})

// Helper functions for creating test resources

func createReadinessVeleroDeployment(namespace string, replicas, readyReplicas int32) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      common.Velero,
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(replicas),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "velero"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "velero"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "velero",
							Image: "velero:latest",
						},
					},
				},
			},
		},
		Status: appsv1.DeploymentStatus{
			Replicas:      replicas,
			ReadyReplicas: readyReplicas,
		},
	}
}

func createReadinessNodeAgentDaemonSet(namespace string) *appsv1.DaemonSet {
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      common.NodeAgent,
			Namespace: namespace,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "node-agent"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "node-agent"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "node-agent",
							Image: "node-agent:latest",
						},
					},
				},
			},
		},
	}
}

func createReadinessNonAdminDeployment(namespace string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nonAdminObjectName,
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(1)),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "non-admin"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "non-admin"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "non-admin",
							Image: "non-admin:latest",
						},
					},
				},
			},
		},
	}
}

func createReadinessVMFileRestoreDeployment(namespace string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vmFileRestoreObjectName,
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(1)),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "vmfr"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "vmfr"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "vmfr",
							Image: "vmfr:latest",
						},
					},
				},
			},
		},
	}
}
