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
	"reflect"

	"github.com/vmware-tanzu/velero/pkg/util/kube"
)

const unbounded = "0"

// PodResources with emptystring will trigger parsing errors in Velero.
// Replace empty string with unbounded so partial resource setting is accepted.
// If additional fields from Velero are added that are strings but not quantities this workflow will break.
//
// Returns a new PodResources object with any empty string fields set to "0".
// If nil, returns the existing nil pointer.
func newPodResourcesWithUnboundedDefaults(pr *kube.PodResources) *kube.PodResources {
	if pr == nil {
		return nil
	}

	prWithUnboundedDefaults := *pr
	// Velero 1.18.1 adds ephemeralStorageRequest and ephemeralStorageLimits not in prior versions.
	// Reflection handles new versions provided the new underlying fields are strings.
	// Velero 1.18.1 and are handled due to all fields are strings.
	reflectNewPr := reflect.ValueOf(&prWithUnboundedDefaults).Elem()
	// for prior to oadp-dev/oadp-1.6: NumField was used instead of Fields as only supported in go1.26
	for i := range reflectNewPr.NumField() {
		f := reflectNewPr.Field(i)
		if f.CanSet() && f.Kind() == reflect.String && f.IsZero() {
			f.SetString(unbounded)
		}
	}
	return &prWithUnboundedDefaults
}
