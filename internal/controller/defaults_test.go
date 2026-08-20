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
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vmware-tanzu/velero/pkg/util/kube"
)

func Test_newPodResourcesWithUnboundedDefaults(t *testing.T) {
	tests := []struct {
		name  string
		input *kube.PodResources
		want  *kube.PodResources
	}{
		{
			name:  "nil returns nil",
			input: nil,
			want:  nil,
		},
		{
			name: "partially set fields get unset values replaced with unbounded",
			input: &kube.PodResources{
				CPURequest:    "100m",
				MemoryRequest: "128Mi",
			},
			want: &kube.PodResources{
				CPURequest:              "100m",
				CPULimit:                "0",
				MemoryRequest:           "128Mi",
				MemoryLimit:             "0",
				EphemeralStorageRequest: "0",
				EphemeralStorageLimit:   "0",
			},
		},
		{
			name: "all fields set returns unchanged",
			input: &kube.PodResources{
				CPURequest:              "100m",
				CPULimit:                "200m",
				MemoryRequest:           "128Mi",
				MemoryLimit:             "256Mi",
				EphemeralStorageRequest: "1Gi",
				EphemeralStorageLimit:   "2Gi",
			},
			want: &kube.PodResources{
				CPURequest:              "100m",
				CPULimit:                "200m",
				MemoryRequest:           "128Mi",
				MemoryLimit:             "256Mi",
				EphemeralStorageRequest: "1Gi",
				EphemeralStorageLimit:   "2Gi",
			},
		},
		{
			name:  "zero-value struct gets all fields set to unbounded",
			input: &kube.PodResources{},
			want: &kube.PodResources{
				CPURequest:              "0",
				CPULimit:                "0",
				MemoryRequest:           "0",
				MemoryLimit:             "0",
				EphemeralStorageRequest: "0",
				EphemeralStorageLimit:   "0",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := newPodResourcesWithUnboundedDefaults(tt.input)
			require.Equal(t, tt.want, got)

			// require the output object is not a mutation of the existing object
			if tt.input != nil {
				require.NotSame(t, tt.input, got)
			}
		})
	}
}
