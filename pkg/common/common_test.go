package common

import (
	"reflect"
	"testing"
)

func TestAppendLabels(t *testing.T) {
	type args struct {
		userLabels []map[string]string
	}
	tests := []struct {
		name string
		args args
		want map[string]string
	}{
		{
			name: "append labels together",
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AppendLabels(tt.args.userLabels...); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("AppendLabels() = %v, want %v", got, tt.want)
			}
		})
	}
}
