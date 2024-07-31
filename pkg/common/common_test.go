package common

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
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

func TestStripDefaultPorts(t *testing.T) {
	tests := []struct {
		name string
		base string
		want string
	}{
		{
			name: "port-free URL is returned unchanged",
			base: "https://s3.region.cloud-object-storage.appdomain.cloud/bucket-name",
			want: "https://s3.region.cloud-object-storage.appdomain.cloud/bucket-name",
		},
		{
			name: "HTTPS port is removed from URL",
			base: "https://s3.region.cloud-object-storage.appdomain.cloud:443/bucket-name",
			want: "https://s3.region.cloud-object-storage.appdomain.cloud/bucket-name",
		},
		{
			name: "HTTP port is removed from URL",
			base: "http://s3.region.cloud-object-storage.appdomain.cloud:80/bucket-name",
			want: "http://s3.region.cloud-object-storage.appdomain.cloud/bucket-name",
		},
		{
			name: "alternate HTTP port is preserved",
			base: "http://10.0.188.30:9000",
			want: "http://10.0.188.30:9000",
		},
		{
			name: "alternate HTTPS port is preserved",
			base: "https://10.0.188.30:9000",
			want: "https://10.0.188.30:9000",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := StripDefaultPorts(tt.base)
			if err != nil {
				t.Errorf("An error occurred: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("StripDefaultPorts() = %v, want %v", got, tt.want)
			}
		})
	}
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
