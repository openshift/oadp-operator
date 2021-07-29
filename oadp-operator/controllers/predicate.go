package controllers

import (
	oadpApi "github.com/openshift/oadp-operator/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

func veleroPredicate() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			// Ignore updates to CR status in which case metadata.Generation does not change
			if e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration() {
				return false
			}
			return isObjectOurs(e.ObjectOld)
		},
		CreateFunc: func(e event.CreateEvent) bool {
			return isObjectOurs(e.Object)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return !e.DeleteStateUnknown && isObjectOurs(e.Object)
		},
	}
}

func isObjectOurs(object client.Object) bool {
	gvk := object.GetObjectKind().GroupVersionKind()
	if gvk.Group == oadpApi.GroupVersion.Group && gvk.Version == oadpApi.GroupVersion.Version && gvk.Kind == oadpApi.Kind {
		return false
	}

	return object.GetLabels()[oadpApi.OadpOperatorLabel] == ""
}
