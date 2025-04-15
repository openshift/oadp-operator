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
	"context"
	"github.com/stretchr/testify/require"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
)

var _ = Describe("DataProtectionTest Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		dataprotectiontest := &oadpv1alpha1.DataProtectionTest{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind DataProtectionTest")
			err := k8sClient.Get(ctx, typeNamespacedName, dataprotectiontest)
			if err != nil && errors.IsNotFound(err) {
				resource := &oadpv1alpha1.DataProtectionTest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					// TODO(user): Specify other spec details if needed.
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &oadpv1alpha1.DataProtectionTest{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance DataProtectionTest")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &DataProtectionTestReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
	})
})

func TestDetermineVendor(t *testing.T) {
	tests := []struct {
		name           string
		serverHeader   string
		expectedVendor string
	}{
		{
			name:           "Detect AWS",
			serverHeader:   "AmazonS3",
			expectedVendor: "AWS",
		},
		{
			name:           "Detect MinIO",
			serverHeader:   "MinIO",
			expectedVendor: "MinIO",
		},
		{
			name:           "Detect Ceph",
			serverHeader:   "Ceph",
			expectedVendor: "Ceph",
		},
		{
			name:           "Unknown vendor",
			serverHeader:   "SomethingElse",
			expectedVendor: "somethingelse",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Server", tc.serverHeader)
			}))
			defer testServer.Close()

			dpt := &oadpv1alpha1.DataProtectionTest{}
			dpt.Spec.BackupLocationSpec = &velerov1.BackupStorageLocationSpec{
				Config: map[string]string{
					"s3Url": testServer.URL,
				},
			}

			reconciler := &DataProtectionTestReconciler{}

			err := reconciler.determineVendor(context.Background(), dpt)
			require.NoError(t, err)
			require.Equal(t, tc.expectedVendor, dpt.Status.S3Vendor)
		})
	}
}
