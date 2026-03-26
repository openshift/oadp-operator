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

package v1alpha2

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/openshift/oadp-operator/api/v1alpha1"
)

func TestConvertTo_RoundTrip(t *testing.T) {
	// Create a v1alpha2 DPA with all 10 duration fields set
	src := &DataProtectionApplication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dpa",
			Namespace: "openshift-adp",
		},
		Spec: DataProtectionApplicationSpec{
			Configuration: &ApplicationConfig{
				Velero: &VeleroConfig{
					FeatureFlags:   []string{"EnableCSI"},
					DefaultPlugins: []DefaultPlugin{DefaultPluginAWS, DefaultPluginOpenShift},
					Args: &VeleroServerArgs{
						ServerFlags: ServerFlags{
							BackupSyncPeriod:            &metav1.Duration{Duration: 2 * time.Minute},
							PodVolumeOperationTimeout:   &metav1.Duration{Duration: 4 * time.Hour},
							ResourceTerminatingTimeout:  &metav1.Duration{Duration: 10 * time.Minute},
							DefaultBackupTTL:            &metav1.Duration{Duration: 720 * time.Hour},
							StoreValidationFrequency:    &metav1.Duration{Duration: 1 * time.Minute},
							ItemOperationSyncFrequency:  &metav1.Duration{Duration: 2 * time.Minute},
							RepoMaintenanceFrequency:    &metav1.Duration{Duration: 1 * time.Hour},
							GarbageCollectionFrequency:  &metav1.Duration{Duration: 1 * time.Hour},
							DefaultItemOperationTimeout: &metav1.Duration{Duration: 1 * time.Hour},
							ResourceTimeout:             &metav1.Duration{Duration: 10 * time.Minute},
							MetricsAddress:              ":8085",
							DefaultVolumesToFsBackup:    ptr.To(true),
						},
						GlobalFlags: GlobalFlags{
							Colorized: ptr.To(false),
						},
					},
				},
				NodeAgent: &NodeAgentConfig{
					NodeAgentCommonFields: NodeAgentCommonFields{
						Enable:  ptr.To(true),
						Timeout: "4h",
					},
					UploaderType: "kopia",
				},
			},
		},
	}

	// Convert to hub (v1alpha1)
	hub := &v1alpha1.DataProtectionApplication{}
	if err := src.ConvertTo(hub); err != nil {
		t.Fatalf("ConvertTo failed: %v", err)
	}

	// Verify duration fields were converted to time.Duration
	args := hub.Spec.Configuration.Velero.Args
	if args.BackupSyncPeriod == nil || *args.BackupSyncPeriod != 2*time.Minute {
		t.Errorf("BackupSyncPeriod: got %v, want 2m", args.BackupSyncPeriod)
	}
	if args.PodVolumeOperationTimeout == nil || *args.PodVolumeOperationTimeout != 4*time.Hour {
		t.Errorf("PodVolumeOperationTimeout: got %v, want 4h", args.PodVolumeOperationTimeout)
	}

	// Convert back to v1alpha2
	roundTripped := &DataProtectionApplication{}
	if err := roundTripped.ConvertFrom(hub); err != nil {
		t.Fatalf("ConvertFrom failed: %v", err)
	}

	// Verify round-trip produces identical result
	if diff := cmp.Diff(src.Spec, roundTripped.Spec); diff != "" {
		t.Errorf("Round-trip v1alpha2 -> v1alpha1 -> v1alpha2 produced diff (-want +got):\n%s", diff)
	}
}

func TestConvertFrom_RoundTrip(t *testing.T) {
	twoMin := 2 * time.Minute
	fourHour := 4 * time.Hour
	tenMin := 10 * time.Minute
	ttl := 720 * time.Hour
	oneMin := 1 * time.Minute
	oneHour := 1 * time.Hour

	// Create a v1alpha1 DPA with all 10 duration fields set
	src := &v1alpha1.DataProtectionApplication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dpa",
			Namespace: "openshift-adp",
		},
		Spec: v1alpha1.DataProtectionApplicationSpec{
			Configuration: &v1alpha1.ApplicationConfig{
				Velero: &v1alpha1.VeleroConfig{
					FeatureFlags:   []string{"EnableCSI"},
					DefaultPlugins: []v1alpha1.DefaultPlugin{v1alpha1.DefaultPluginAWS, v1alpha1.DefaultPluginOpenShift},
					Args: &v1alpha1.VeleroServerArgs{
						ServerFlags: v1alpha1.ServerFlags{
							BackupSyncPeriod:            &twoMin,
							PodVolumeOperationTimeout:   &fourHour,
							ResourceTerminatingTimeout:  &tenMin,
							DefaultBackupTTL:            &ttl,
							StoreValidationFrequency:    &oneMin,
							ItemOperationSyncFrequency:  &twoMin,
							RepoMaintenanceFrequency:    &oneHour,
							GarbageCollectionFrequency:  &oneHour,
							DefaultItemOperationTimeout: &oneHour,
							ResourceTimeout:             &tenMin,
							MetricsAddress:              ":8085",
							DefaultVolumesToFsBackup:    ptr.To(true),
						},
						GlobalFlags: v1alpha1.GlobalFlags{
							Colorized: ptr.To(false),
						},
					},
				},
				NodeAgent: &v1alpha1.NodeAgentConfig{
					NodeAgentCommonFields: v1alpha1.NodeAgentCommonFields{
						Enable:  ptr.To(true),
						Timeout: "4h",
					},
					UploaderType: "kopia",
				},
			},
		},
	}

	// Convert to v1alpha2
	spoke := &DataProtectionApplication{}
	if err := spoke.ConvertFrom(src); err != nil {
		t.Fatalf("ConvertFrom failed: %v", err)
	}

	// Verify duration fields were converted to metav1.Duration
	args := spoke.Spec.Configuration.Velero.Args
	if args.BackupSyncPeriod == nil || args.BackupSyncPeriod.Duration != 2*time.Minute {
		t.Errorf("BackupSyncPeriod: got %v, want 2m", args.BackupSyncPeriod)
	}
	if args.PodVolumeOperationTimeout == nil || args.PodVolumeOperationTimeout.Duration != 4*time.Hour {
		t.Errorf("PodVolumeOperationTimeout: got %v, want 4h", args.PodVolumeOperationTimeout)
	}

	// Convert back to v1alpha1
	roundTripped := &v1alpha1.DataProtectionApplication{}
	if err := spoke.ConvertTo(roundTripped); err != nil {
		t.Fatalf("ConvertTo failed: %v", err)
	}

	// Verify round-trip produces identical result
	if diff := cmp.Diff(src.Spec, roundTripped.Spec); diff != "" {
		t.Errorf("Round-trip v1alpha1 -> v1alpha2 -> v1alpha1 produced diff (-want +got):\n%s", diff)
	}
}

func TestConvert_NilDurationFields(t *testing.T) {
	// Create a v1alpha2 DPA with NO duration fields set
	src := &DataProtectionApplication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dpa",
			Namespace: "openshift-adp",
		},
		Spec: DataProtectionApplicationSpec{
			Configuration: &ApplicationConfig{
				Velero: &VeleroConfig{
					DefaultPlugins: []DefaultPlugin{DefaultPluginAWS},
					Args: &VeleroServerArgs{
						ServerFlags: ServerFlags{
							MetricsAddress: ":8085",
						},
					},
				},
			},
		},
	}

	// Convert to hub
	hub := &v1alpha1.DataProtectionApplication{}
	if err := src.ConvertTo(hub); err != nil {
		t.Fatalf("ConvertTo failed: %v", err)
	}

	// Verify all duration fields are nil
	args := hub.Spec.Configuration.Velero.Args
	if args.BackupSyncPeriod != nil {
		t.Error("BackupSyncPeriod should be nil")
	}
	if args.PodVolumeOperationTimeout != nil {
		t.Error("PodVolumeOperationTimeout should be nil")
	}
	if args.ResourceTerminatingTimeout != nil {
		t.Error("ResourceTerminatingTimeout should be nil")
	}
	if args.DefaultBackupTTL != nil {
		t.Error("DefaultBackupTTL should be nil")
	}
	if args.StoreValidationFrequency != nil {
		t.Error("StoreValidationFrequency should be nil")
	}
	if args.ItemOperationSyncFrequency != nil {
		t.Error("ItemOperationSyncFrequency should be nil")
	}
	if args.RepoMaintenanceFrequency != nil {
		t.Error("RepoMaintenanceFrequency should be nil")
	}
	if args.GarbageCollectionFrequency != nil {
		t.Error("GarbageCollectionFrequency should be nil")
	}
	if args.DefaultItemOperationTimeout != nil {
		t.Error("DefaultItemOperationTimeout should be nil")
	}
	if args.ResourceTimeout != nil {
		t.Error("ResourceTimeout should be nil")
	}

	// Round-trip back
	roundTripped := &DataProtectionApplication{}
	if err := roundTripped.ConvertFrom(hub); err != nil {
		t.Fatalf("ConvertFrom failed: %v", err)
	}

	if diff := cmp.Diff(src.Spec, roundTripped.Spec); diff != "" {
		t.Errorf("Round-trip with nil durations produced diff (-want +got):\n%s", diff)
	}
}

func TestConvert_NilArgs(t *testing.T) {
	// DPA with no Args at all
	src := &DataProtectionApplication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dpa",
			Namespace: "openshift-adp",
		},
		Spec: DataProtectionApplicationSpec{
			Configuration: &ApplicationConfig{
				Velero: &VeleroConfig{
					DefaultPlugins: []DefaultPlugin{DefaultPluginAWS},
				},
			},
		},
	}

	hub := &v1alpha1.DataProtectionApplication{}
	if err := src.ConvertTo(hub); err != nil {
		t.Fatalf("ConvertTo failed: %v", err)
	}

	if hub.Spec.Configuration.Velero.Args != nil {
		t.Error("Args should be nil when not set in source")
	}

	roundTripped := &DataProtectionApplication{}
	if err := roundTripped.ConvertFrom(hub); err != nil {
		t.Fatalf("ConvertFrom failed: %v", err)
	}

	if diff := cmp.Diff(src.Spec, roundTripped.Spec); diff != "" {
		t.Errorf("Round-trip with nil args produced diff (-want +got):\n%s", diff)
	}
}

func TestConvert_ZeroDuration(t *testing.T) {
	// Test zero duration (0s) converts correctly
	src := &DataProtectionApplication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dpa",
			Namespace: "openshift-adp",
		},
		Spec: DataProtectionApplicationSpec{
			Configuration: &ApplicationConfig{
				Velero: &VeleroConfig{
					DefaultPlugins: []DefaultPlugin{DefaultPluginAWS},
					Args: &VeleroServerArgs{
						ServerFlags: ServerFlags{
							StoreValidationFrequency: &metav1.Duration{Duration: 0},
						},
					},
				},
			},
		},
	}

	hub := &v1alpha1.DataProtectionApplication{}
	if err := src.ConvertTo(hub); err != nil {
		t.Fatalf("ConvertTo failed: %v", err)
	}

	args := hub.Spec.Configuration.Velero.Args
	if args.StoreValidationFrequency == nil || *args.StoreValidationFrequency != 0 {
		t.Errorf("StoreValidationFrequency: got %v, want 0", args.StoreValidationFrequency)
	}

	roundTripped := &DataProtectionApplication{}
	if err := roundTripped.ConvertFrom(hub); err != nil {
		t.Fatalf("ConvertFrom failed: %v", err)
	}

	if diff := cmp.Diff(src.Spec, roundTripped.Spec); diff != "" {
		t.Errorf("Round-trip with zero duration produced diff (-want +got):\n%s", diff)
	}
}
