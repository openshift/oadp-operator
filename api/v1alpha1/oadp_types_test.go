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

package v1alpha1

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

// test that args for time.Duration can accept a string

func Test_dpaArgsCanAcceptStringAsTime(t *testing.T) {
	tests := []struct {
		name     string
		dpa      string
		wantTime metav1.Duration
	}{
		{
			name: "test1",
			dpa: `
apiVersion: oadp.openshift.io/v1alpha1
kind: DataProtectionApplication
metadata:
  name: velero-sample
spec:
  configuration:
    velero:
      args:
        backup-sync-period: "1s"
`,
			wantTime: metav1.Duration{Duration: time.Second},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// parse dpa string into dpa object using k8s yaml unmarshal
			// then test that the time duration is equal to the expected time duration
			var dpa DataProtectionApplication
			err := yaml.Unmarshal([]byte(tt.dpa), &dpa)
			if err != nil {
				t.Errorf("error unmarshalling dpa: %v", err)
			}
			if dpa.Spec.Configuration.Velero.Args.BackupSyncPeriod == nil {
				t.Errorf("expected non-nil BackupSyncPeriod")
			}
			if *dpa.Spec.Configuration.Velero.Args.BackupSyncPeriod != tt.wantTime {
				t.Errorf("got %v, want %v", *dpa.Spec.Configuration.Velero.Args.BackupSyncPeriod, tt.wantTime)
			}
		})
	}
}
