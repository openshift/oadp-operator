package controller

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
)

func TestDPAReconciler_ReconcileResourceQuota(t *testing.T) {
	ns := "test-ns"
	dpa := &oadpv1alpha1.DataProtectionApplication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dpa",
			Namespace: ns,
		},
	}

	t.Run("creates default ResourceQuota when missing", func(t *testing.T) {
		fakeClient, err := getFakeClientFromObjects(dpa)
		require.NoError(t, err)

		r := &DataProtectionApplicationReconciler{
			Client:  fakeClient,
			Scheme:  fakeClient.Scheme(),
			Context: context.Background(),
			dpa:     dpa,
		}

		cont, err := r.ReconcileResourceQuota(logr.Discard())
		require.NoError(t, err)
		assert.True(t, cont)

		got := &corev1.ResourceQuota{}
		err = fakeClient.Get(context.Background(), types.NamespacedName{
			Name:      oadpResourceQuotaName,
			Namespace: ns,
		}, got)
		require.NoError(t, err)

		assert.Equal(t, "create-if-absent", got.Annotations[oadpResourceQuotaPolicyAnnotation])
		assert.Equal(t, "oadp-operator", got.Labels["app.kubernetes.io/name"])
		assert.Equal(t, "oadp-operator", got.Labels["app.kubernetes.io/managed-by"])

		assert.True(t, resource.MustParse("200").Equal(got.Spec.Hard[corev1.ResourcePods]))
		assert.True(t, resource.MustParse("50").Equal(got.Spec.Hard[corev1.ResourceRequestsCPU]))
		assert.True(t, resource.MustParse("64Gi").Equal(got.Spec.Hard[corev1.ResourceRequestsMemory]))
		_, hasLimitsCPU := got.Spec.Hard[corev1.ResourceLimitsCPU]
		_, hasLimitsMemory := got.Spec.Hard[corev1.ResourceLimitsMemory]
		assert.False(t, hasLimitsCPU, "default quota must not set limits.cpu")
		assert.False(t, hasLimitsMemory, "default quota must not set limits.memory")
	})

	t.Run("does not overwrite existing custom ResourceQuota", func(t *testing.T) {
		custom := &corev1.ResourceQuota{
			ObjectMeta: metav1.ObjectMeta{
				Name:      oadpResourceQuotaName,
				Namespace: ns,
			},
			Spec: corev1.ResourceQuotaSpec{
				Hard: corev1.ResourceList{
					corev1.ResourcePods:           resource.MustParse("10"),
					corev1.ResourceRequestsCPU:    resource.MustParse("2"),
					corev1.ResourceRequestsMemory: resource.MustParse("4Gi"),
				},
			},
		}

		fakeClient, err := getFakeClientFromObjects(dpa, custom)
		require.NoError(t, err)

		r := &DataProtectionApplicationReconciler{
			Client:  fakeClient,
			Scheme:  fakeClient.Scheme(),
			Context: context.Background(),
			dpa:     dpa,
		}

		cont, err := r.ReconcileResourceQuota(logr.Discard())
		require.NoError(t, err)
		assert.True(t, cont)

		got := &corev1.ResourceQuota{}
		err = fakeClient.Get(context.Background(), client.ObjectKeyFromObject(custom), got)
		require.NoError(t, err)

		assert.True(t, resource.MustParse("10").Equal(got.Spec.Hard[corev1.ResourcePods]))
		assert.True(t, resource.MustParse("2").Equal(got.Spec.Hard[corev1.ResourceRequestsCPU]))
		assert.True(t, resource.MustParse("4Gi").Equal(got.Spec.Hard[corev1.ResourceRequestsMemory]))
	})

	t.Run("recreates defaults after admin deletes ResourceQuota", func(t *testing.T) {
		fakeClient, err := getFakeClientFromObjects(dpa)
		require.NoError(t, err)

		r := &DataProtectionApplicationReconciler{
			Client:  fakeClient,
			Scheme:  fakeClient.Scheme(),
			Context: context.Background(),
			dpa:     dpa,
		}

		_, err = r.ReconcileResourceQuota(logr.Discard())
		require.NoError(t, err)

		existing := &corev1.ResourceQuota{}
		err = fakeClient.Get(context.Background(), types.NamespacedName{
			Name:      oadpResourceQuotaName,
			Namespace: ns,
		}, existing)
		require.NoError(t, err)
		require.NoError(t, fakeClient.Delete(context.Background(), existing))

		cont, err := r.ReconcileResourceQuota(logr.Discard())
		require.NoError(t, err)
		assert.True(t, cont)

		got := &corev1.ResourceQuota{}
		err = fakeClient.Get(context.Background(), types.NamespacedName{
			Name:      oadpResourceQuotaName,
			Namespace: ns,
		}, got)
		require.NoError(t, err)
		assert.True(t, resource.MustParse("200").Equal(got.Spec.Hard[corev1.ResourcePods]))
	})
}
