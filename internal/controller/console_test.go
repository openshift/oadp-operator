package controller

import (
	"testing"

	consolev1 "github.com/openshift/api/console/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
)

func TestIsConsoleCRDAvailable(t *testing.T) {
	log := ctrl.Log.WithName("test")

	tests := []struct {
		name              string
		includeConsoleCRD bool
		wantAvailable     bool
	}{
		{
			name:              "returns false when ConsoleCLIDownload CRD is not registered",
			includeConsoleCRD: false,
			wantAvailable:     false,
		},
		{
			name:              "returns true when ConsoleCLIDownload CRD is registered",
			includeConsoleCRD: true,
			wantAvailable:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var groupVersions []schema.GroupVersion
			if tt.includeConsoleCRD {
				groupVersions = append(groupVersions, consolev1.GroupVersion)
			}
			mapper := meta.NewDefaultRESTMapper(groupVersions)
			if tt.includeConsoleCRD {
				mapper.Add(consolev1.GroupVersion.WithKind("ConsoleCLIDownload"), meta.RESTScopeRoot)
			}

			available, err := IsConsoleCRDAvailable(mapper, log)
			require.NoError(t, err)
			assert.Equal(t, tt.wantAvailable, available)
		})
	}
}
