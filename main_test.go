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
	"errors"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	fakediscovery "k8s.io/client-go/discovery/fake"
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
				ObjectMeta: metav1.ObjectMeta{
					Name:   testNamespaceName,
					Labels: tt.existingLabels,
				},
			}
			testClient := fake.NewSimpleClientset(&namespace)
			err := addPodSecurityPrivilegedLabels(testNamespaceName, testClient)
			if err != nil {
				t.Errorf("addPodSecurityPrivilegedLabels() error = %v", err)
			}
			testNamespace, err := testClient.CoreV1().Namespaces().Get(context.TODO(), testNamespaceName, metav1.GetOptions{})
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

// TestGetRequiredCRDs verifies that getRequiredCRDs returns the expected CRDs
func TestGetRequiredCRDs(t *testing.T) {
	crds := getRequiredCRDs()

	if len(crds) != 2 {
		t.Errorf("expected 2 required CRDs, got %d", len(crds))
	}

	// Verify Route CRD is included
	foundRoute := false
	foundSCC := false
	for _, crd := range crds {
		if crd.groupVersion == "route.openshift.io/v1" && crd.resourceName == "routes" {
			foundRoute = true
		}
		if crd.groupVersion == "security.openshift.io/v1" && crd.resourceName == "securitycontextconstraints" {
			foundSCC = true
		}
	}

	if !foundRoute {
		t.Error("expected Route CRD (route.openshift.io/v1/routes) to be in required CRDs")
	}
	if !foundSCC {
		t.Error("expected SCC CRD (security.openshift.io/v1/securitycontextconstraints) to be in required CRDs")
	}
}

// mockDiscoveryClient implements discovery.DiscoveryInterface for testing
type mockDiscoveryClient struct {
	fakediscovery.FakeDiscovery
	resources      map[string]*metav1.APIResourceList
	resourcesError error
}

func (m *mockDiscoveryClient) ServerResourcesForGroupVersion(groupVersion string) (*metav1.APIResourceList, error) {
	if m.resourcesError != nil {
		return nil, m.resourcesError
	}
	if resources, ok := m.resources[groupVersion]; ok {
		return resources, nil
	}
	// Return error if group version not found (simulates CRD not registered)
	return nil, errors.New("the server could not find the requested resource")
}

// Ensure mockDiscoveryClient implements the interface we need
var _ discovery.DiscoveryInterface = &mockDiscoveryClient{}

// TestIsCRDAvailable tests the isCRDAvailable function
func TestIsCRDAvailable(t *testing.T) {
	tests := []struct {
		name         string
		groupVersion string
		resourceName string
		resources    map[string]*metav1.APIResourceList
		expectError  error
		expected     bool
	}{
		{
			name:         "CRD is available",
			groupVersion: "route.openshift.io/v1",
			resourceName: "routes",
			resources: map[string]*metav1.APIResourceList{
				"route.openshift.io/v1": {
					GroupVersion: "route.openshift.io/v1",
					APIResources: []metav1.APIResource{
						{Name: "routes", Kind: "Route"},
					},
				},
			},
			expected: true,
		},
		{
			name:         "CRD group version exists but resource not found",
			groupVersion: "route.openshift.io/v1",
			resourceName: "routes",
			resources: map[string]*metav1.APIResourceList{
				"route.openshift.io/v1": {
					GroupVersion: "route.openshift.io/v1",
					APIResources: []metav1.APIResource{
						{Name: "otherresource", Kind: "Other"},
					},
				},
			},
			expected: false,
		},
		{
			name:         "CRD group version does not exist",
			groupVersion: "route.openshift.io/v1",
			resourceName: "routes",
			resources:    map[string]*metav1.APIResourceList{},
			expected:     false,
		},
		{
			name:         "SCC CRD is available",
			groupVersion: "security.openshift.io/v1",
			resourceName: "securitycontextconstraints",
			resources: map[string]*metav1.APIResourceList{
				"security.openshift.io/v1": {
					GroupVersion: "security.openshift.io/v1",
					APIResources: []metav1.APIResource{
						{Name: "securitycontextconstraints", Kind: "SecurityContextConstraints"},
					},
				},
			},
			expected: true,
		},
		{
			name:         "Multiple resources in group version",
			groupVersion: "security.openshift.io/v1",
			resourceName: "securitycontextconstraints",
			resources: map[string]*metav1.APIResourceList{
				"security.openshift.io/v1": {
					GroupVersion: "security.openshift.io/v1",
					APIResources: []metav1.APIResource{
						{Name: "podsecuritypolicyselfsubjectreviews", Kind: "PodSecurityPolicySelfSubjectReview"},
						{Name: "securitycontextconstraints", Kind: "SecurityContextConstraints"},
						{Name: "rangeallocations", Kind: "RangeAllocation"},
					},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &mockDiscoveryClient{
				resources:      tt.resources,
				resourcesError: tt.expectError,
			}

			available := isCRDAvailable(mockClient, tt.groupVersion, tt.resourceName)
			if available != tt.expected {
				t.Errorf("isCRDAvailable() = %v, expected %v", available, tt.expected)
			}
		})
	}
}

// TestIsCRDAvailableWithDiscoveryError tests isCRDAvailable when discovery API returns an error
func TestIsCRDAvailableWithDiscoveryError(t *testing.T) {
	mockClient := &mockDiscoveryClient{
		resources:      nil,
		resourcesError: errors.New("connection refused"),
	}

	// When discovery returns an error, isCRDAvailable should return false
	// because this indicates the CRD is not available (yet)
	available := isCRDAvailable(mockClient, "route.openshift.io/v1", "routes")
	if available {
		t.Error("isCRDAvailable() should return false when discovery fails")
	}
}

// dynamicMockDiscoveryClient is a mock that can change its responses over time
// to simulate CRDs becoming available after some delay
type dynamicMockDiscoveryClient struct {
	fakediscovery.FakeDiscovery
	pollCount      int // Number of complete poll iterations
	availableAfter int // CRDs become available after this many poll iterations
	resources      map[string]*metav1.APIResourceList
	routeCalls     int // Track individual calls for debugging
	sccCalls       int
}

func (m *dynamicMockDiscoveryClient) ServerResourcesForGroupVersion(groupVersion string) (*metav1.APIResourceList, error) {
	// Track which CRD is being checked
	if groupVersion == "route.openshift.io/v1" {
		m.routeCalls++
		// Count poll iterations based on Route checks (called first)
		m.pollCount = m.routeCalls
	} else if groupVersion == "security.openshift.io/v1" {
		m.sccCalls++
	}

	if m.pollCount >= m.availableAfter {
		if resources, ok := m.resources[groupVersion]; ok {
			return resources, nil
		}
	}
	// CRDs not yet available
	return nil, errors.New("the server could not find the requested resource")
}

var _ discovery.DiscoveryInterface = &dynamicMockDiscoveryClient{}

// TestWaitForRequiredCRDs_CRDsAvailableImmediately tests that waitForRequiredCRDs
// returns immediately when all CRDs are already available
func TestWaitForRequiredCRDs_CRDsAvailableImmediately(t *testing.T) {
	// Create a mock that has CRDs available from the start
	mockClient := &dynamicMockDiscoveryClient{
		availableAfter: 1, // Available on first call
		resources: map[string]*metav1.APIResourceList{
			"route.openshift.io/v1": {
				GroupVersion: "route.openshift.io/v1",
				APIResources: []metav1.APIResource{
					{Name: "routes", Kind: "Route"},
				},
			},
			"security.openshift.io/v1": {
				GroupVersion: "security.openshift.io/v1",
				APIResources: []metav1.APIResource{
					{Name: "securitycontextconstraints", Kind: "SecurityContextConstraints"},
				},
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := waitForRequiredCRDsWithClient(ctx, mockClient)
	if err != nil {
		t.Errorf("waitForRequiredCRDs() unexpected error = %v", err)
	}
}

// TestWaitForRequiredCRDs_CRDsBecomeAvailable tests that waitForRequiredCRDs
// waits and succeeds when CRDs become available after some delay
func TestWaitForRequiredCRDs_CRDsBecomeAvailable(t *testing.T) {
	// Create a mock that has CRDs available after 3 poll iterations
	mockClient := &dynamicMockDiscoveryClient{
		availableAfter: 3,
		resources: map[string]*metav1.APIResourceList{
			"route.openshift.io/v1": {
				GroupVersion: "route.openshift.io/v1",
				APIResources: []metav1.APIResource{
					{Name: "routes", Kind: "Route"},
				},
			},
			"security.openshift.io/v1": {
				GroupVersion: "security.openshift.io/v1",
				APIResources: []metav1.APIResource{
					{Name: "securitycontextconstraints", Kind: "SecurityContextConstraints"},
				},
			},
		},
	}

	// Use longer timeout to allow for poll iterations
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := waitForRequiredCRDsWithClient(ctx, mockClient)
	if err != nil {
		t.Errorf("waitForRequiredCRDs() unexpected error = %v", err)
	}

	// Verify we actually waited (polled multiple times)
	if mockClient.pollCount < 3 {
		t.Errorf("expected at least 3 poll iterations, got %d", mockClient.pollCount)
	}
}

// TestWaitForRequiredCRDs_Timeout tests that waitForRequiredCRDs returns an error
// when context times out before CRDs become available
func TestWaitForRequiredCRDs_Timeout(t *testing.T) {
	// Create a mock that never has CRDs available
	mockClient := &dynamicMockDiscoveryClient{
		availableAfter: 1000, // Never becomes available within test timeout
		resources:      map[string]*metav1.APIResourceList{},
	}

	// Use a very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := waitForRequiredCRDsWithClient(ctx, mockClient)
	if err == nil {
		t.Error("waitForRequiredCRDs() expected timeout error, got nil")
	}
}

// TestWaitForRequiredCRDs_PartialAvailability tests that waitForRequiredCRDs
// waits until ALL required CRDs are available, not just some
func TestWaitForRequiredCRDs_PartialAvailability(t *testing.T) {
	// Create a mock where only Route is available, SCC is not
	mockClient := &dynamicMockDiscoveryClient{
		availableAfter: 1,
		resources: map[string]*metav1.APIResourceList{
			"route.openshift.io/v1": {
				GroupVersion: "route.openshift.io/v1",
				APIResources: []metav1.APIResource{
					{Name: "routes", Kind: "Route"},
				},
			},
			// SCC is missing
		},
	}

	// Use a short timeout - should fail because SCC is never available
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := waitForRequiredCRDsWithClient(ctx, mockClient)
	if err == nil {
		t.Error("waitForRequiredCRDs() expected error when not all CRDs available, got nil")
	}
}
