package controllers

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/event"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/common"
)

func TestHasRelevantAnnotationChange(t *testing.T) {
	tests := []struct {
		name           string
		oldAnnotations map[string]string
		newAnnotations map[string]string
		want           bool
	}{
		{
			name:           "no annotations on either object",
			oldAnnotations: nil,
			newAnnotations: nil,
			want:           false,
		},
		{
			name:           "velero server args annotation added",
			oldAnnotations: nil,
			newAnnotations: map[string]string{
				common.UnsupportedVeleroServerArgsAnnotation: "my-configmap",
			},
			want: true,
		},
		{
			name: "velero server args annotation removed",
			oldAnnotations: map[string]string{
				common.UnsupportedVeleroServerArgsAnnotation: "my-configmap",
			},
			newAnnotations: nil,
			want:           true,
		},
		{
			name: "velero server args annotation value changed",
			oldAnnotations: map[string]string{
				common.UnsupportedVeleroServerArgsAnnotation: "old-configmap",
			},
			newAnnotations: map[string]string{
				common.UnsupportedVeleroServerArgsAnnotation: "new-configmap",
			},
			want: true,
		},
		{
			name:           "node agent server args annotation added",
			oldAnnotations: nil,
			newAnnotations: map[string]string{
				common.UnsupportedNodeAgentServerArgsAnnotation: "my-configmap",
			},
			want: true,
		},
		{
			name: "node agent server args annotation removed",
			oldAnnotations: map[string]string{
				common.UnsupportedNodeAgentServerArgsAnnotation: "my-configmap",
			},
			newAnnotations: nil,
			want:           true,
		},
		{
			name: "irrelevant annotation changed",
			oldAnnotations: map[string]string{
				"some-other-annotation": "old-value",
			},
			newAnnotations: map[string]string{
				"some-other-annotation": "new-value",
			},
			want: false,
		},
		{
			name: "relevant annotation unchanged, irrelevant changed",
			oldAnnotations: map[string]string{
				common.UnsupportedVeleroServerArgsAnnotation: "same-configmap",
				"some-other-annotation":                      "old-value",
			},
			newAnnotations: map[string]string{
				common.UnsupportedVeleroServerArgsAnnotation: "same-configmap",
				"some-other-annotation":                      "new-value",
			},
			want: false,
		},
		{
			name: "both relevant annotations changed",
			oldAnnotations: map[string]string{
				common.UnsupportedVeleroServerArgsAnnotation:    "old-velero-cm",
				common.UnsupportedNodeAgentServerArgsAnnotation: "old-nodeagent-cm",
			},
			newAnnotations: map[string]string{
				common.UnsupportedVeleroServerArgsAnnotation:    "new-velero-cm",
				common.UnsupportedNodeAgentServerArgsAnnotation: "new-nodeagent-cm",
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldObj := &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: tt.oldAnnotations,
				},
			}
			newObj := &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: tt.newAnnotations,
				},
			}
			got := hasRelevantAnnotationChange(oldObj, newObj)
			if got != tt.want {
				t.Errorf("hasRelevantAnnotationChange() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVeleroPredicateUpdateFunc_AnnotationChange(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = oadpv1alpha1.AddToScheme(scheme)

	pred := veleroPredicate(scheme)

	tests := []struct {
		name string
		old  *oadpv1alpha1.DataProtectionApplication
		new  *oadpv1alpha1.DataProtectionApplication
		want bool
	}{
		{
			name: "generation changed - should reconcile",
			old: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
			},
			new: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{Generation: 2},
			},
			want: true,
		},
		{
			name: "generation same, no annotation change - should NOT reconcile",
			old: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
			},
			new: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
			},
			want: false,
		},
		{
			name: "generation same, velero annotation added - should reconcile",
			old: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
			},
			new: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
					Annotations: map[string]string{
						common.UnsupportedVeleroServerArgsAnnotation: "my-cm",
					},
				},
			},
			want: true,
		},
		{
			name: "generation same, node agent annotation removed - should reconcile",
			old: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
					Annotations: map[string]string{
						common.UnsupportedNodeAgentServerArgsAnnotation: "my-cm",
					},
				},
			},
			new: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
			},
			want: true,
		},
		{
			name: "generation same, irrelevant annotation changed - should NOT reconcile",
			old: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Generation:  1,
					Annotations: map[string]string{"kubectl.kubernetes.io/last-applied": "old"},
				},
			},
			new: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Generation:  1,
					Annotations: map[string]string{"kubectl.kubernetes.io/last-applied": "new"},
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := event.UpdateEvent{
				ObjectOld: tt.old,
				ObjectNew: tt.new,
			}
			got := pred.Update(e)
			if got != tt.want {
				t.Errorf("veleroPredicate.Update() = %v, want %v", got, tt.want)
			}
		})
	}
}
