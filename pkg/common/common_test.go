package common

import (
	"errors"
	"reflect"
	"testing"

	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/openshift/oadp-operator/pkg/storage/aws"
)

func TestAppendUniqueKeyTOfTMaps(t *testing.T) {
	type args struct {
		userLabels []map[string]string
	}
	tests := []struct {
		name    string
		args    args
		want    map[string]string
		wantErr bool
	}{
		{
			name: "append unique labels together",
			args: args{
				userLabels: []map[string]string{
					{"a": "a"},
					{"b": "b"},
				},
			},
			want: map[string]string{
				"a": "a",
				"b": "b",
			},
		},
		{
			name: "append unique labels together, with valid duplicates",
			args: args{
				userLabels: []map[string]string{
					{"a": "a"},
					{"b": "b"},
					{"b": "b"},
				},
			},
			want: map[string]string{
				"a": "a",
				"b": "b",
			},
		},
		{
			name: "append unique labels together - nil sandwich",
			args: args{
				userLabels: []map[string]string{
					{"a": "a"},
					nil,
					{"b": "b"},
				},
			},
			want: map[string]string{
				"a": "a",
				"b": "b",
			},
		},
		{
			name: "should error when append duplicate label keys with different value together",
			args: args{
				userLabels: []map[string]string{
					{"a": "a"},
					{"a": "b"},
				},
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := AppendUniqueKeyTOfTMaps(tt.args.userLabels...)
			if (err != nil) != tt.wantErr {
				t.Errorf("AppendUniqueKeyTOfTMaps() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("AppendUniqueKeyTOfTMaps() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Test that a copy of the map is returned and not the original map
func TestAppendTTMapAsCopy(t *testing.T) {
	base := map[string]string{
		"a": "a",
	}
	add := map[string]string{
		"b": "b",
	}
	want := map[string]string{
		"a": "a",
		"b": "b",
	}
	t.Run("original map is not returned", func(t *testing.T) {
		if got := AppendTTMapAsCopy(base, add); !reflect.DeepEqual(got, want) {
			t.Errorf("AppendTTMapAsCopy() = %v, want %v", got, want)
		}
		if !reflect.DeepEqual(base, map[string]string{"a": "a"}) {
			t.Errorf("AppendTTMapAsCopy() = %v, want %v", base, map[string]string{"a": "a"})
		}
	})
}

func TestGetImagePullPolicy(t *testing.T) {
	tests := []struct {
		name     string
		image    string
		override *corev1.PullPolicy
		result   corev1.PullPolicy
	}{
		{
			name:   "Image without digest",
			image:  "quay.io/konveyor/velero:oadp-1.4",
			result: corev1.PullAlways,
		},
		{
			name:   "Image with sha256 digest",
			image:  "test.com/foo@sha256:1234567890098765432112345667890098765432112345667890098765432112",
			result: corev1.PullIfNotPresent,
		},
		{
			name:   "Image with wrong sha256 digest",
			image:  "test.com/foo@sha256:123456789009876543211234566789009876543211234566789009876543211",
			result: corev1.PullAlways,
		},
		{
			name:   "Image with sha512 digest",
			image:  "test.com/foo@sha512:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			result: corev1.PullIfNotPresent,
		},
		{
			name:   "Image with wrong sha512 digest",
			image:  "test.com/foo@sha512:Ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			result: corev1.PullAlways,
		},
		{
			name:   "Image with non sha512 nor sha512 digest",
			image:  "test.com/foo@sha256+b64u:LCa0a2j_xo_5m0U8HTBBNBNCLXBkg7-g-YpeiGJm564",
			result: corev1.PullAlways,
		},
		{
			name:     "Image without digest, but with override to Never",
			image:    "quay.io/konveyor/velero:oadp-1.4",
			override: ptr.To(corev1.PullNever),
			result:   corev1.PullNever,
		},
		{
			name:     "Image with sha256 digest, but with override to Never",
			image:    "test.com/foo@sha256:1234567890098765432112345667890098765432112345667890098765432112",
			override: ptr.To(corev1.PullNever),
			result:   corev1.PullNever,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := GetImagePullPolicy(test.override, test.image)
			if err != nil {
				t.Errorf("Error occurred in test: %s", err)
			}
			if result != test.result {
				t.Errorf("Results differ: got '%v' but expected '%v'", result, test.result)
			}
		})
	}
}

func TestGenerateCliArgsFromConfigMap(t *testing.T) {
	tests := []struct {
		name          string
		cliSubCommand []string
		configMap     *corev1.ConfigMap
		expectedArgs  []string
	}{
		{
			name:          "Boolean argument variations",
			cliSubCommand: []string{"server"},
			configMap: &corev1.ConfigMap{
				Data: map[string]string{
					"--default-snapshot-move":   "true",
					"--another-key-mix-letters": "TrUe",
					"key-no-prefix":             "False",
					"string-not-bool":           "'False'",
				},
			},
			expectedArgs: []string{
				"server",
				"--another-key-mix-letters=true",
				"--default-snapshot-move=true",
				"--key-no-prefix=false",
				"--string-not-bool='False'",
			},
		},
		{
			name:          "All arguments with spaces, some without single quotes",
			cliSubCommand: []string{"server"},
			configMap: &corev1.ConfigMap{
				Data: map[string]string{
					"default-volume-snapshot-locations": "aws:backups-primary, azure:backups-secondary",
					"log-level":                         "'debug'",
				},
			},
			expectedArgs: []string{
				"server",
				"--default-volume-snapshot-locations=aws:backups-primary, azure:backups-secondary",
				"--log-level='debug'",
			},
		},
		{
			name:          "Preserve single and double '-' as key prefix",
			cliSubCommand: []string{"server"},
			configMap: &corev1.ConfigMap{
				Data: map[string]string{
					"--default-volume-snapshot-locations": "aws:backups-primary, azure:backups-secondary",
					"--log-level":                         "debug",
					"--string-bool":                       "False",
					"-n":                                  "mynamespace",
				},
			},
			expectedArgs: []string{
				"server",
				"--default-volume-snapshot-locations=aws:backups-primary, azure:backups-secondary",
				"--log-level=debug",
				"--string-bool=false",
				"-n=mynamespace",
			},
		},
		{
			name:          "Non-Boolean argument with space",
			cliSubCommand: []string{"server"},
			configMap: &corev1.ConfigMap{
				Data: map[string]string{
					"default-snapshot-move-data": " true",
				},
			},
			expectedArgs: []string{
				"server",
				"--default-snapshot-move-data= true",
			},
		},
		{
			name:          "Mixed arguments",
			cliSubCommand: []string{"server"},
			configMap: &corev1.ConfigMap{
				Data: map[string]string{
					"--default-volume-snapshot-locations": "aws:backups-primary,azure:backups-secondary",
					"--log-level":                         "debug",
					"--log-format":                        "json",
					"--default-snapshot-move-data":        "True",
					"-v":                                  "3",
					"a":                                   "somearg",
				},
			},
			expectedArgs: []string{
				"server",
				"--a=somearg",
				"--default-snapshot-move-data=true",
				"--default-volume-snapshot-locations=aws:backups-primary,azure:backups-secondary",
				"--log-format=json",
				"--log-level=debug",
				"-v=3",
			},
		},
		{
			name:          "Empty ConfigMap",
			cliSubCommand: []string{"server"},
			configMap: &corev1.ConfigMap{
				Data: map[string]string{},
			},
			expectedArgs: []string{
				"server",
			},
		},
		{
			name:          "Multiple SubCommands",
			cliSubCommand: []string{"node-agent", "server"},
			configMap: &corev1.ConfigMap{
				Data: map[string]string{
					"key": "value",
				},
			},
			expectedArgs: []string{
				"node-agent",
				"server",
				"--key=value",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotArgs := GenerateCliArgsFromConfigMap(tt.configMap, tt.cliSubCommand...)
			if !reflect.DeepEqual(gotArgs, tt.expectedArgs) {
				t.Errorf("GenerateCliArgsFromConfigMap() = %v, want %v", gotArgs, tt.expectedArgs)
			}
		})
	}
}

// TestMergeExtraArgs validates the merge behavior of extra CLI args including
// replacement, deduplication, key normalization, and sorted appending.
func TestMergeExtraArgs(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		extraArgs map[string]string
		want      []string
	}{
		{
			name:      "nil extraArgs is a no-op",
			args:      []string{"server", "--log-level=info"},
			extraArgs: nil,
			want:      []string{"server", "--log-level=info"},
		},
		{
			name:      "empty extraArgs is a no-op",
			args:      []string{"server", "--log-level=info"},
			extraArgs: map[string]string{},
			want:      []string{"server", "--log-level=info"},
		},
		{
			name:      "new flags are appended in sorted order",
			args:      []string{"server", "--log-level=info"},
			extraArgs: map[string]string{"resource-timeout": "20m", "custom-flag": "val"},
			want:      []string{"server", "--log-level=info", "--custom-flag=val", "--resource-timeout=20m"},
		},
		{
			name:      "existing equals-format flag is replaced in place",
			args:      []string{"server", "--log-level=info", "--fs-backup-timeout=4h"},
			extraArgs: map[string]string{"log-level": "debug"},
			want:      []string{"server", "--log-level=debug", "--fs-backup-timeout=4h"},
		},
		{
			name:      "existing space-separated flag is replaced in place",
			args:      []string{"server", "--log-level", "info", "--fs-backup-timeout=4h"},
			extraArgs: map[string]string{"log-level": "debug"},
			want:      []string{"server", "--log-level", "debug", "--fs-backup-timeout=4h"},
		},
		{
			name:      "mix of replace and append",
			args:      []string{"server", "--log-level=info", "--fs-backup-timeout=4h"},
			extraArgs: map[string]string{"log-level": "debug", "new-flag": "value"},
			want:      []string{"server", "--log-level=debug", "--fs-backup-timeout=4h", "--new-flag=value"},
		},
		{
			name:      "does not mutate input slice",
			args:      []string{"server", "--log-level=info"},
			extraArgs: map[string]string{"log-level": "debug"},
			want:      []string{"server", "--log-level=debug"},
		},
		{
			name:      "duplicate flags are deduplicated",
			args:      []string{"server", "--log-level=info", "--log-level=warning"},
			extraArgs: map[string]string{"log-level": "debug"},
			want:      []string{"server", "--log-level=debug"},
		},
		{
			name:      "duplicate space-separated flags are deduplicated",
			args:      []string{"server", "--log-level", "info", "--log-level", "warning"},
			extraArgs: map[string]string{"log-level": "debug"},
			want:      []string{"server", "--log-level", "debug"},
		},
		{
			name:      "keys with leading dashes are normalized",
			args:      []string{"server", "--log-level=info"},
			extraArgs: map[string]string{"--log-level": "debug"},
			want:      []string{"server", "--log-level=debug"},
		},
		{
			name:      "empty and whitespace-only keys are skipped",
			args:      []string{"server", "--log-level=info"},
			extraArgs: map[string]string{"": "val1", "  ": "val2", "custom": "val3"},
			want:      []string{"server", "--log-level=info", "--custom=val3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := make([]string, len(tt.args))
			copy(original, tt.args)

			got := MergeExtraArgs(tt.args, tt.extraArgs)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("MergeExtraArgs() = %v, want %v", got, tt.want)
			}
			if !reflect.DeepEqual(tt.args, original) {
				t.Errorf("MergeExtraArgs() mutated input: got %v, original was %v", tt.args, original)
			}
		})
	}
}

func TestUpdateBackupStorageLocation(t *testing.T) {
	tests := []struct {
		name        string
		bsl         *velerov1.BackupStorageLocation
		bslSpec     velerov1.BackupStorageLocationSpec
		expectedBsl *velerov1.BackupStorageLocation
	}{
		{
			name: "Azure registry deployment with empty storageAccountKeyEnvVar",
			bsl:  &velerov1.BackupStorageLocation{},
			bslSpec: velerov1.BackupStorageLocationSpec{
				Provider: "azure",
				Config: map[string]string{
					"storageAccountKeyEnvVar": "",
				},
			},
			expectedBsl: &velerov1.BackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						RegistryDeploymentLabel: "False",
					},
				},
				Spec: velerov1.BackupStorageLocationSpec{
					Provider: "azure",
					Config: map[string]string{
						"storageAccountKeyEnvVar": "",
					},
				},
			},
		},
		{
			name: "AWS URL with default port 80 stripping",
			bsl:  &velerov1.BackupStorageLocation{},
			bslSpec: velerov1.BackupStorageLocationSpec{
				Provider: "aws",
				Config: map[string]string{
					"s3Url": "http://s3.amazonaws.com:80",
				},
			},
			expectedBsl: &velerov1.BackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						RegistryDeploymentLabel: "True",
					},
				},
				Spec: velerov1.BackupStorageLocationSpec{
					Provider: "aws",
					Config: map[string]string{
						"checksumAlgorithm": "",
						"s3Url":             "http://s3.amazonaws.com",
					},
				},
			},
		},
		{
			name: "AWS URL with default port 443 stripping",
			bsl:  &velerov1.BackupStorageLocation{},
			bslSpec: velerov1.BackupStorageLocationSpec{
				Provider: "aws",
				Config: map[string]string{
					"s3Url": "https://s3.amazonaws.com:443",
				},
			},
			expectedBsl: &velerov1.BackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						RegistryDeploymentLabel: "True",
					},
				},
				Spec: velerov1.BackupStorageLocationSpec{
					Provider: "aws",
					Config: map[string]string{
						"checksumAlgorithm": "",
						"s3Url":             "https://s3.amazonaws.com",
					},
				},
			},
		},
		{
			name: "AWS checksumAlgorithm set to empty if not specified",
			bsl:  &velerov1.BackupStorageLocation{},
			bslSpec: velerov1.BackupStorageLocationSpec{
				Provider: "aws",
				Config: map[string]string{
					"s3Url": "http://s3.amazonaws.com",
				},
			},
			expectedBsl: &velerov1.BackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						RegistryDeploymentLabel: "True",
					},
				},
				Spec: velerov1.BackupStorageLocationSpec{
					Provider: "aws",
					Config: map[string]string{
						"s3Url":             "http://s3.amazonaws.com",
						"checksumAlgorithm": "",
					},
				},
			},
		},
		{
			name: "Nil labels should initialize labels map",
			bsl:  &velerov1.BackupStorageLocation{},
			bslSpec: velerov1.BackupStorageLocationSpec{
				Provider: "aws",
				Config: map[string]string{
					"s3Url": "http://s3.amazonaws.com",
				},
			},
			expectedBsl: &velerov1.BackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						RegistryDeploymentLabel: "True",
					},
				},
				Spec: velerov1.BackupStorageLocationSpec{
					Provider: "aws",
					Config: map[string]string{
						"checksumAlgorithm": "",
						"s3Url":             "http://s3.amazonaws.com",
					},
				},
			},
		},
		{
			name: "Existing labels should not be overwritten, RegistryDeploymentLabel should be True",
			bsl: &velerov1.BackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"existing-label": "value",
					},
				},
			},
			bslSpec: velerov1.BackupStorageLocationSpec{
				Provider: "azure",
				Config: map[string]string{
					"storageAccountKeyEnvVar": "something",
				},
			},
			expectedBsl: &velerov1.BackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"existing-label":        "value",
						RegistryDeploymentLabel: "True",
					},
				},
				Spec: velerov1.BackupStorageLocationSpec{
					Provider: "azure",
					Config: map[string]string{
						"storageAccountKeyEnvVar": "something",
					},
				},
			},
		},
	}

	// Mock GetBucketRegionFunc to return a region for the test bucket
	originalGetBucketRegionFunc := aws.GetBucketRegionFunc
	aws.GetBucketRegionFunc = func(bucket string) (string, error) {
		if bucket == "openshift-velero-plugin-s3-auto-region-test-1" {
			return "us-east-1", nil
		}
		return "", errors.New("bucket region not discoverable")
	}
	defer func() { aws.GetBucketRegionFunc = originalGetBucketRegionFunc }()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bslCopy := tt.bsl.DeepCopy()
			bslSpecCopy := tt.bslSpec.DeepCopy()

			UpdateBackupStorageLocation(bslCopy, *bslSpecCopy)

			if !reflect.DeepEqual(bslCopy, tt.expectedBsl) {
				t.Errorf("UpdateBackupStorageLocation() = %v, want %v", bslCopy, tt.expectedBsl)
			}
		})
	}
}
