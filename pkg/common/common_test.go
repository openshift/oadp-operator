package common

import (
	"reflect"
	"testing"
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
