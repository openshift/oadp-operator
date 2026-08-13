package controller

import (
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
// Returns a new PodResources objects with any default values set to "0".
// If nil, returns the existing nil pointer.
func newPodResourcesWithUnboundedDefaults(pr *kube.PodResources) *kube.PodResources {
	if pr == nil {
		return pr
	}

	prWithUnboundedDefaults := &kube.PodResources{}
	if pr.CPURequest == "" {
		prWithUnboundedDefaults.CPURequest = unbounded
	} else {
		prWithUnboundedDefaults.CPURequest = pr.CPURequest
	}
	if pr.CPULimit == "" {
		prWithUnboundedDefaults.CPULimit = unbounded
	} else {
		prWithUnboundedDefaults.CPULimit = pr.CPULimit
	}
	if pr.MemoryRequest == "" {
		prWithUnboundedDefaults.MemoryRequest = unbounded
	} else {
		prWithUnboundedDefaults.MemoryRequest = pr.MemoryRequest
	}
	if pr.MemoryLimit == "" {
		prWithUnboundedDefaults.MemoryLimit = unbounded
	} else {
		prWithUnboundedDefaults.MemoryLimit = pr.MemoryLimit
	}
	if pr.EphemeralStorageRequest == "" {
		prWithUnboundedDefaults.EphemeralStorageRequest = unbounded
	} else {
		prWithUnboundedDefaults.EphemeralStorageRequest = pr.EphemeralStorageRequest
	}
	if pr.EphemeralStorageLimit == "" {
		prWithUnboundedDefaults.EphemeralStorageLimit = unbounded
	} else {
		prWithUnboundedDefaults.EphemeralStorageLimit = pr.EphemeralStorageLimit
	}

	return prWithUnboundedDefaults
}
