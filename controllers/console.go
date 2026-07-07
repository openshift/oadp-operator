package controllers

import (
	"fmt"

	"github.com/go-logr/logr"
	consolev1 "github.com/openshift/api/console/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// IsConsoleCRDAvailable checks whether the ConsoleCLIDownload CRD is registered
// with the API server. Returns false when the Console capability is not enabled
// (e.g. SNO clusters).
func IsConsoleCRDAvailable(mapper meta.RESTMapper, log logr.Logger) (bool, error) {
	_, err := mapper.RESTMapping(
		schema.GroupKind{Group: consolev1.GroupVersion.Group, Kind: "ConsoleCLIDownload"},
	)
	if err != nil {
		if meta.IsNoMatchError(err) {
			log.Info("ConsoleCLIDownload CRD not available (Console capability not enabled), skipping setup")
			return false, nil
		}
		return false, fmt.Errorf("failed to check ConsoleCLIDownload CRD availability: %w", err)
	}
	return true, nil
}
