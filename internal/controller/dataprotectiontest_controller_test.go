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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
)

func TestDetermineVendor(t *testing.T) {
	tests := []struct {
		name           string
		serverHeader   string
		extraHeaders   map[string]string
		expectedVendor string
	}{
		{
			name:           "Detect AWS via Server header",
			serverHeader:   "AmazonS3",
			expectedVendor: "AWS",
		},
		{
			name:         "Detect AWS via x-amz-request-id",
			serverHeader: "",
			extraHeaders: map[string]string{
				"x-amz-request-id": "some-aws-request-id",
			},
			expectedVendor: "AWS",
		},
		{
			name:           "Detect MinIO via Server header",
			serverHeader:   "MinIO",
			expectedVendor: "MinIO",
		},
		{
			name:         "Detect MinIO via x-minio-region",
			serverHeader: "",
			extraHeaders: map[string]string{
				"x-minio-region": "us-east-1",
			},
			expectedVendor: "MinIO",
		},
		{
			name:           "Detect Ceph via Server header",
			serverHeader:   "Ceph",
			expectedVendor: "Ceph",
		},
		{
			name:         "Detect Ceph via x-rgw-request-id",
			serverHeader: "",
			extraHeaders: map[string]string{
				"x-rgw-request-id": "abc123",
			},
			expectedVendor: "Ceph",
		},
		{
			name:           "Unknown vendor fallback",
			serverHeader:   "SomethingElse",
			expectedVendor: "somethingelse",
		},
		{
			name:           "No headers at all",
			serverHeader:   "",
			expectedVendor: "Unknown",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Fake HTTP server with HEAD response
			testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.serverHeader != "" {
					w.Header().Set("Server", tc.serverHeader)
				}
				for k, v := range tc.extraHeaders {
					w.Header().Set(k, v)
				}
			}))
			defer testServer.Close()

			dpt := &oadpv1alpha1.DataProtectionTest{
				Spec: oadpv1alpha1.DataProtectionTestSpec{
					BackupLocationSpec: &velerov1.BackupStorageLocationSpec{
						Provider: "aws",
						Config: map[string]string{
							"s3Url": testServer.URL,
						},
					},
				},
			}

			reconciler := &DataProtectionTestReconciler{}

			err := reconciler.determineVendor(context.Background(), dpt, dpt.Spec.BackupLocationSpec)
			require.NoError(t, err)
			require.Equal(t, tc.expectedVendor, dpt.Status.S3Vendor)
		})
	}
}
