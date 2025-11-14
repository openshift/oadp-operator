/*
Copyright 2024.

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
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
)

// Test labelHandler.Create method
func Test_labelHandler_Create(t *testing.T) {
	tests := []struct {
		name           string
		object         client.Object
		expectEnqueued bool
		expectedDPA    string
		expectedNS     string
	}{
		{
			name: "should enqueue when both labels are present",
			object: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-ns",
					Labels: map[string]string{
						oadpv1alpha1.OadpOperatorLabel:   "True",
						"dataprotectionapplication.name": "test-dpa",
					},
				},
			},
			expectEnqueued: true,
			expectedDPA:    "test-dpa",
			expectedNS:     "test-ns",
		},
		{
			name: "should not enqueue when OadpOperatorLabel is missing",
			object: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-ns",
					Labels: map[string]string{
						"dataprotectionapplication.name": "test-dpa",
					},
				},
			},
			expectEnqueued: false,
		},
		{
			name: "should not enqueue when dataprotectionapplication.name is missing",
			object: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-ns",
					Labels: map[string]string{
						oadpv1alpha1.OadpOperatorLabel: "True",
					},
				},
			},
			expectEnqueued: false,
		},
		{
			name: "should not enqueue when OadpOperatorLabel is empty string",
			object: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-ns",
					Labels: map[string]string{
						oadpv1alpha1.OadpOperatorLabel:   "",
						"dataprotectionapplication.name": "test-dpa",
					},
				},
			},
			expectEnqueued: false,
		},
		{
			name: "should not enqueue when no labels are present",
			object: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-ns",
				},
			},
			expectEnqueued: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &labelHandler{}
			queue := &testQueue{
				items: []reconcile.Request{},
			}

			evt := event.TypedCreateEvent[client.Object]{
				Object: tt.object,
			}

			handler.Create(context.Background(), evt, queue)

			if tt.expectEnqueued {
				assert.Len(t, queue.items, 1, "expected one item to be enqueued")
				assert.Equal(t, tt.expectedDPA, queue.items[0].Name)
				assert.Equal(t, tt.expectedNS, queue.items[0].Namespace)
			} else {
				assert.Empty(t, queue.items, "expected no items to be enqueued")
			}
		})
	}
}

// Test labelHandler.Delete method
func Test_labelHandler_Delete(t *testing.T) {
	tests := []struct {
		name           string
		object         client.Object
		expectEnqueued bool
		expectedDPA    string
		expectedNS     string
	}{
		{
			name: "should enqueue when both labels are present",
			object: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-ns",
					Labels: map[string]string{
						oadpv1alpha1.OadpOperatorLabel:   "True",
						"dataprotectionapplication.name": "test-dpa",
					},
				},
			},
			expectEnqueued: true,
			expectedDPA:    "test-dpa",
			expectedNS:     "test-ns",
		},
		{
			name: "should not enqueue when labels are missing",
			object: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-ns",
				},
			},
			expectEnqueued: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &labelHandler{}
			queue := &testQueue{
				items: []reconcile.Request{},
			}

			evt := event.TypedDeleteEvent[client.Object]{
				Object:             tt.object,
				DeleteStateUnknown: false,
			}

			handler.Delete(context.Background(), evt, queue)

			if tt.expectEnqueued {
				assert.Len(t, queue.items, 1, "expected one item to be enqueued")
				assert.Equal(t, tt.expectedDPA, queue.items[0].Name)
				assert.Equal(t, tt.expectedNS, queue.items[0].Namespace)
			} else {
				assert.Empty(t, queue.items, "expected no items to be enqueued")
			}
		})
	}
}

// Test labelHandler.Update method
func Test_labelHandler_Update(t *testing.T) {
	tests := []struct {
		name           string
		oldObject      client.Object
		newObject      client.Object
		expectEnqueued bool
		expectedDPA    string
		expectedNS     string
		description    string
	}{
		{
			name: "should enqueue when secret data changes",
			oldObject: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "cloud-credentials",
					Namespace:       "openshift-adp",
					ResourceVersion: "100",
					Labels: map[string]string{
						oadpv1alpha1.OadpOperatorLabel:   "True",
						"dataprotectionapplication.name": "test-dpa",
					},
				},
				Data: map[string][]byte{
					"credentials": []byte("old-credentials"),
				},
			},
			newObject: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "cloud-credentials",
					Namespace:       "openshift-adp",
					ResourceVersion: "101",
					Labels: map[string]string{
						oadpv1alpha1.OadpOperatorLabel:   "True",
						"dataprotectionapplication.name": "test-dpa",
					},
				},
				Data: map[string][]byte{
					"credentials": []byte("new-credentials"),
				},
			},
			expectEnqueued: true,
			expectedDPA:    "test-dpa",
			expectedNS:     "openshift-adp",
			description:    "Data content changed, should trigger reconciliation",
		},
		{
			name: "should NOT enqueue when only ResourceVersion changes (ESO scenario)",
			oldObject: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "cloud-credentials",
					Namespace:       "openshift-adp",
					ResourceVersion: "100",
					Labels: map[string]string{
						oadpv1alpha1.OadpOperatorLabel:   "True",
						"dataprotectionapplication.name": "test-dpa",
					},
					Annotations: map[string]string{
						"external-secrets.io/managed": "true",
					},
				},
				Data: map[string][]byte{
					"credentials": []byte("same-credentials"),
				},
			},
			newObject: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "cloud-credentials",
					Namespace:       "openshift-adp",
					ResourceVersion: "101", // Only ResourceVersion changed
					Labels: map[string]string{
						oadpv1alpha1.OadpOperatorLabel:   "True",
						"dataprotectionapplication.name": "test-dpa",
					},
					Annotations: map[string]string{
						"external-secrets.io/managed": "true",
						"external-secrets.io/update":  "timestamp", // ESO adds annotation
					},
				},
				Data: map[string][]byte{
					"credentials": []byte("same-credentials"),
				},
			},
			expectEnqueued: false,
			description:    "Only metadata changed (ESO update scenario), should NOT trigger reconciliation",
		},
		{
			name: "should enqueue when labels change",
			oldObject: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-secret",
					Namespace:       "test-ns",
					ResourceVersion: "100",
					Labels: map[string]string{
						oadpv1alpha1.OadpOperatorLabel:   "True",
						"dataprotectionapplication.name": "test-dpa",
					},
				},
				Data: map[string][]byte{
					"data": []byte("test-data"),
				},
			},
			newObject: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-secret",
					Namespace:       "test-ns",
					ResourceVersion: "101",
					Labels: map[string]string{
						oadpv1alpha1.OadpOperatorLabel:   "True",
						"dataprotectionapplication.name": "test-dpa",
						"new-label":                      "new-value",
					},
				},
				Data: map[string][]byte{
					"data": []byte("test-data"),
				},
			},
			expectEnqueued: true,
			expectedDPA:    "test-dpa",
			expectedNS:     "test-ns",
			description:    "Labels changed, should trigger reconciliation",
		},
		{
			name: "should enqueue when StringData changes",
			oldObject: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-ns",
					Labels: map[string]string{
						oadpv1alpha1.OadpOperatorLabel:   "True",
						"dataprotectionapplication.name": "test-dpa",
					},
				},
				StringData: map[string]string{
					"key": "old-value",
				},
			},
			newObject: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-ns",
					Labels: map[string]string{
						oadpv1alpha1.OadpOperatorLabel:   "True",
						"dataprotectionapplication.name": "test-dpa",
					},
				},
				StringData: map[string]string{
					"key": "new-value",
				},
			},
			expectEnqueued: true,
			expectedDPA:    "test-dpa",
			expectedNS:     "test-ns",
			description:    "StringData changed, should trigger reconciliation",
		},
		{
			name: "should handle non-Secret objects (ConfigMap)",
			oldObject: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-cm",
					Namespace:       "test-ns",
					ResourceVersion: "100",
					Labels: map[string]string{
						oadpv1alpha1.OadpOperatorLabel:   "True",
						"dataprotectionapplication.name": "test-dpa",
					},
				},
				Data: map[string]string{
					"key": "value1",
				},
			},
			newObject: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-cm",
					Namespace:       "test-ns",
					ResourceVersion: "101",
					Labels: map[string]string{
						oadpv1alpha1.OadpOperatorLabel:   "True",
						"dataprotectionapplication.name": "test-dpa",
					},
				},
				Data: map[string]string{
					"key": "value1",
				},
			},
			expectEnqueued: true, // Non-secrets always trigger reconciliation
			expectedDPA:    "test-dpa",
			expectedNS:     "test-ns",
			description:    "Non-Secret object should use default behavior",
		},
		{
			name: "should not enqueue when required labels are missing",
			oldObject: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-ns",
				},
			},
			newObject: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-ns",
				},
				Data: map[string][]byte{
					"data": []byte("new-data"),
				},
			},
			expectEnqueued: false,
			description:    "Missing required labels, should not trigger reconciliation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &labelHandler{}
			queue := &testQueue{
				items: []reconcile.Request{},
			}

			evt := event.TypedUpdateEvent[client.Object]{
				ObjectOld: tt.oldObject,
				ObjectNew: tt.newObject,
			}

			handler.Update(context.Background(), evt, queue)

			if tt.expectEnqueued {
				assert.Len(t, queue.items, 1, tt.description)
				assert.Equal(t, tt.expectedDPA, queue.items[0].Name)
				assert.Equal(t, tt.expectedNS, queue.items[0].Namespace)
			} else {
				assert.Empty(t, queue.items, tt.description)
			}
		})
	}
}

// Test labelHandler.Generic method
func Test_labelHandler_Generic(t *testing.T) {
	tests := []struct {
		name           string
		object         client.Object
		expectEnqueued bool
		expectedDPA    string
		expectedNS     string
	}{
		{
			name: "should enqueue when both labels are present",
			object: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-ns",
					Labels: map[string]string{
						oadpv1alpha1.OadpOperatorLabel:   "True",
						"dataprotectionapplication.name": "test-dpa",
					},
				},
			},
			expectEnqueued: true,
			expectedDPA:    "test-dpa",
			expectedNS:     "test-ns",
		},
		{
			name: "should not enqueue when labels are missing",
			object: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-ns",
				},
			},
			expectEnqueued: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &labelHandler{}
			queue := &testQueue{
				items: []reconcile.Request{},
			}

			evt := event.TypedGenericEvent[client.Object]{
				Object: tt.object,
			}

			handler.Generic(context.Background(), evt, queue)

			if tt.expectEnqueued {
				assert.Len(t, queue.items, 1, "expected one item to be enqueued")
				assert.Equal(t, tt.expectedDPA, queue.items[0].Name)
				assert.Equal(t, tt.expectedNS, queue.items[0].Namespace)
			} else {
				assert.Empty(t, queue.items, "expected no items to be enqueued")
			}
		})
	}
}

// testQueue is a mock implementation of workqueue.TypedRateLimitingInterface for testing
type testQueue struct {
	items []reconcile.Request
}

func (q *testQueue) Add(item reconcile.Request) {
	q.items = append(q.items, item)
}

func (q *testQueue) AddAfter(item reconcile.Request, duration time.Duration) {}
func (q *testQueue) AddRateLimited(item reconcile.Request)                   {}
func (q *testQueue) Forget(item reconcile.Request)                           {}
func (q *testQueue) NumRequeues(item reconcile.Request) int                  { return 0 }
func (q *testQueue) Len() int                                                { return len(q.items) }
func (q *testQueue) Get() (reconcile.Request, bool) {
	if len(q.items) == 0 {
		return reconcile.Request{}, false
	}
	item := q.items[0]
	q.items = q.items[1:]
	return item, true
}
func (q *testQueue) Done(item reconcile.Request) {}
func (q *testQueue) ShutDown()                   {}
func (q *testQueue) ShutDownWithDrain()          {}
func (q *testQueue) ShuttingDown() bool          { return false }

// Ensure testQueue implements the interface
var _ workqueue.TypedRateLimitingInterface[reconcile.Request] = (*testQueue)(nil)
