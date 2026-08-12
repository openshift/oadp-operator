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
func setPodResourcesDefaults(pr *kube.PodResources) {
	if pr == nil {
		return
	}

	if pr.CPURequest == "" {
		pr.CPURequest = unbounded
	}
	if pr.CPULimit == "" {
		pr.CPULimit = unbounded
	}
	if pr.MemoryRequest == "" {
		pr.MemoryRequest = unbounded
	}
	if pr.MemoryLimit == "" {
		pr.MemoryLimit = unbounded
	}
	if pr.EphemeralStorageRequest == "" {
		pr.EphemeralStorageRequest = unbounded
	}
	if pr.EphemeralStorageLimit == "" {
		pr.EphemeralStorageLimit = unbounded
	}
}
