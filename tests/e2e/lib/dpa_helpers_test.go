package lib

import (
	"testing"
)

func TestDpaCustomResource_ProviderStorageClassName(t *testing.T) {
	type fields struct {
		Provider          string
	}
	tests := []struct {
		name    string
		fields  fields
		want    string
		wantErr bool
	}{
		{
			name: "expect aws to return gp2-csi",
			fields: fields{
				Provider: "aws",
			},
			want:    "gp2-csi",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &DpaCustomResource{
				Provider:          tt.fields.Provider,
			}
			got, err := v.ProviderStorageClassName()
			if (err != nil) != tt.wantErr {
				t.Errorf("DpaCustomResource.ProviderStorageClassName() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("DpaCustomResource.ProviderStorageClassName() = %v, want %v", got, tt.want)
			}
		})
	}
}
