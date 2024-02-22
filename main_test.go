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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

// Tests that addPodSecurityPrivilegedLabels do not override the existing labels in OADP namespace
func TestAddPodSecurityPrivilegedLabels(t *testing.T) {
	var testNamespaceName = "openshift-adp"
	tests := []struct {
		name           string
		existingLabels map[string]string
		expectedLabels map[string]string
	}{
		{
			name: "PSA labels do not exist in the namespace",
			existingLabels: map[string]string{
				"existing-label": "existing-value",
			},
			expectedLabels: map[string]string{
				"existing-label": "existing-value",
				enforceLabel:     privileged,
				auditLabel:       privileged,
				warnLabel:        privileged,
			},
		},
		{
			name: "PSA labels exist in the namespace, but are not set to privileged",
			existingLabels: map[string]string{
				"user-label": "user-value",
				enforceLabel: "baseline",
				auditLabel:   "baseline",
				warnLabel:    "baseline",
			},
			expectedLabels: map[string]string{
				"user-label": "user-value",
				enforceLabel: privileged,
				auditLabel:   privileged,
				warnLabel:    privileged,
			},
		},
		{
			name: "PSA labels exist in the namespace, and are set to privileged",
			existingLabels: map[string]string{
				"another-label": "another-value",
				enforceLabel:    privileged,
				auditLabel:      privileged,
				warnLabel:       privileged,
			},
			expectedLabels: map[string]string{
				"another-label": "another-value",
				enforceLabel:    privileged,
				auditLabel:      privileged,
				warnLabel:       privileged,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new namespace with the existing labels
			namespace := corev1.Namespace{
				ObjectMeta: v1.ObjectMeta{
					Name:   testNamespaceName,
					Labels: tt.existingLabels,
				},
			}
			testClient := fake.NewSimpleClientset(&namespace)
			err := addPodSecurityPrivilegedLabels(testNamespaceName, testClient)
			if err != nil {
				t.Errorf("addPodSecurityPrivilegedLabels() error = %v", err)
			}
			testNamespace, err := testClient.CoreV1().Namespaces().Get(context.TODO(), testNamespaceName, v1.GetOptions{})
			if err != nil {
				t.Errorf("Get test namespace error = %v", err)
			}
			// assert that existing labels are not overridden
			for key, value := range tt.existingLabels {
				if testNamespace.Labels[key] != value {
					// only error if changing non PSA labels
					if key != enforceLabel && key != auditLabel && key != warnLabel {
						t.Errorf("namespace label %v has value %v, instead of %v", key, testNamespace.Labels[key], value)
					}
				}
			}
			for key, value := range tt.expectedLabels {
				if testNamespace.Labels[key] != value {
					t.Errorf("namespace label %v has value %v, instead of %v", key, testNamespace.Labels[key], value)
				}
			}
		})
	}
}
