package controller

import (
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/common"
)

func veleroPredicate(scheme *runtime.Scheme) predicate.Predicate {
	return predicate.Funcs{
		// Update returns true if the Update event should be processed
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration() {
				return isObjectOurs(scheme, e.ObjectOld)
			}
			// Generation unchanged — still reconcile if a functionally
			// consumed annotation was added, removed, or modified.
			if hasRelevantAnnotationChange(e.ObjectOld, e.ObjectNew) {
				return isObjectOurs(scheme, e.ObjectOld)
			}
			return false
		},
		// Create returns true if the Create event should be processed
		CreateFunc: func(e event.CreateEvent) bool {
			return isObjectOurs(scheme, e.Object)
		},
		// Delete returns true if the Delete event should be processed
		DeleteFunc: func(e event.DeleteEvent) bool {
			return !e.DeleteStateUnknown && isObjectOurs(scheme, e.Object)
		},
	}
}

// hasRelevantAnnotationChange returns true if any functionally consumed
// DPA annotation has changed between the old and new object.
func hasRelevantAnnotationChange(oldObj, newObj client.Object) bool {
	relevantAnnotations := []string{
		common.UnsupportedVeleroServerArgsAnnotation,
		common.UnsupportedNodeAgentServerArgsAnnotation,
	}
	oldAnnotations := oldObj.GetAnnotations()
	newAnnotations := newObj.GetAnnotations()
	for _, key := range relevantAnnotations {
		if oldAnnotations[key] != newAnnotations[key] {
			return true
		}
	}
	return false
}

// isObjectOurs returns true if the object is ours.
// it first checks if the object has our group, version, and kind
// else it will check for non empty OadpOperatorlabel labels
func isObjectOurs(scheme *runtime.Scheme, object client.Object) bool {
	objGVKs, _, err := scheme.ObjectKinds(object)
	if err != nil {
		return false
	}
	if len(objGVKs) != 1 {
		return false
	}
	gvk := objGVKs[0]
	if gvk.Group == oadpv1alpha1.GroupVersion.Group && gvk.Version == oadpv1alpha1.GroupVersion.Version && gvk.Kind == "DataProtectionApplication" {
		return true
	}
	return object.GetLabels()[oadpv1alpha1.OadpOperatorLabel] != ""
}
