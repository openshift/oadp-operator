package client

// +kubebuilder:object:generate=true
type VeleroConfig struct {
	// We use same namespace as DataProtectionApplication
	// Namespace string `json:"namespace,omitempty"`

	// Features is an existing field in DataProtectionApplication
	// --kubebuilder:validation:Enum=EnableCSI;EnableAPIGroupVersions;EnableUploadProgress
	// Features  []VeleroFeatureFlag `json:"features,omitempty"`

	// Show colored output in TTY
	// +optional
	Colorized *bool `json:"colorized,omitempty"`
	// CACert is not a flag in velero server
}
