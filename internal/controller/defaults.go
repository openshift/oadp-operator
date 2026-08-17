package controller

import (
	"reflect"

	"github.com/vmware-tanzu/velero/pkg/util/kube"
)

const unbounded = "0"

// This module is for setting structure defaults where the default golang values for the type are not valid.
// int -> 0
// float -> 0.0
// string -> ""

// PodResources with emptystring will trigger parsing errors in Velero.
// Replace empty string with unbounded so partial resource setting is accepted.
//
// Returns a new PodResources object with any empty string fields set to "0".
// If nil, returns the existing nil pointer.
func newPodResourcesWithUnboundedDefaults(pr *kube.PodResources) *kube.PodResources {
	if pr == nil {
		return pr
	}

	prWithUnboundedDefaults := *pr
	// Velero 1.18.1 adds ephemeralStorageRequest and ephemeralStorageLimits not in prior versions.
	// Reflection handles new versions provided the new underlying fields are strings.
	// Velero 1.18.1 and below are handled due to all fields are strings.
	reflectedPr := reflect.ValueOf(pr).Elem()
	reflectNewPr := reflect.ValueOf(&prWithUnboundedDefaults).Elem()
	for i := range reflectedPr.NumField() {
		oldField := reflectedPr.Field(i)
		newField := reflectNewPr.Field(i)
		if oldField.Kind() == reflect.String && oldField.String() == "" {
			newField.SetString(unbounded)
		} else {
			newField.Set(oldField)
		}
	}
	return &prWithUnboundedDefaults
}
