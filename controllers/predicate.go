package controllers

import (
	oadpApi "github.com/openshift/oadp-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

func veleroPredicate(scheme *runtime.Scheme) predicate.Predicate {
	return predicate.Funcs{
		// Update returns true if the Update event should be processed
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectOld.GetGeneration() == e.ObjectNew.GetGeneration() {
				return false
			}
			return isObjectOurs(scheme, e.ObjectOld)
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
	if gvk.Group == oadpApi.GroupVersion.Group && gvk.Version == oadpApi.GroupVersion.Version && gvk.Kind == oadpApi.Kind {
		return true
	}
	return object.GetLabels()[oadpApi.OadpOperatorLabel] != ""
}
