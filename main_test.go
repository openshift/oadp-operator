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

package main

import (
	"context"
	"testing"

	coreV1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

// Tests that addPodSecurityPrivilegedLabels do not override the existing labels in oadp namespace
func Test_addPodSecurityPrivilegedLabels(t *testing.T) {
	var watchNamespaceName = "openshift-adp"
	tests := []struct {
		name    string
		existingLabels map[string]string
		expectedLabels map[string]string
		wantErr bool
	}{
		{
			name: "existing labels",
			existingLabels: map[string]string{
				"existing-label": "existing-value",
			},
			expectedLabels: map[string]string{
				"existing-label": "existing-value",
				enforceLabel: privileged,
				auditLabel: privileged,
				warnLabel: privileged,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// make a copy of the existing labels to avoid modifying the test case map
			var labels map[string]string
			labels = make(map[string]string)
			for key, value := range tt.existingLabels {
				labels[key] = value
			}
			// Create a new namespace with the existing labels
			namespace := coreV1.Namespace{
				ObjectMeta: v1.ObjectMeta{
					Name: watchNamespaceName,
					Labels: labels,
				},
			}
			// fake clientset
			clientset := fake.NewSimpleClientset(&namespace)
			if err := addPodSecurityPrivilegedLabelsWithClientSet(watchNamespaceName, clientset); (err != nil) != tt.wantErr {
				t.Errorf("addPodSecurityPrivilegedLabels() error = %v, wantErr %v", err, tt.wantErr)
			}
			nsFromCluster, err := clientset.CoreV1().Namespaces().Get(context.TODO(), watchNamespaceName, v1.GetOptions{})
			if err != nil {
				t.Errorf("addPodSecurityPrivilegedLabels() error = %v", err)
			}
			// assert that existing labels are not overridden
			for key, value := range tt.existingLabels {
				if nsFromCluster.Labels[key] != value {
					t.Errorf("namespace from cluster label for key %v is %v, want %v", key, nsFromCluster.Labels[key], value)
				}
			}
			for key, value := range tt.expectedLabels {
				if nsFromCluster.Labels[key] != value {
					t.Errorf("namespace from cluster label for key %v is %v, want %v", key, nsFromCluster.Labels[key], value)
				}
			}
		})
	}
}
