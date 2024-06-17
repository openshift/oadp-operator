# NodeAgentConfig configuration with restic/kopia
Date: 2023-07-25

## Abstract

New configuration for the file system backup & restore that will be used by the OADP and allow to choose Restic or Kopia uploader type.

## Background

For the file system backup and restore the Velero may use Restic or Kopia as the uploader mechanism. A number of tests were [performed](https://velero.io/docs/main/performance-guidance/) by the Velero community to compare those mechanisms. In many cases Kopia uploader is a much better performing mechanism and as such should be added to the OADP operator.

Please refer to the upstream [kopia uploader integration design](https://github.com/vmware-tanzu/velero/blob/main/design/unified-repo-and-kopia-integration/unified-repo-and-kopia-integration.md) for underlying Velero design and backup & restore workflow.

Upstream [kopia](https://github.com/kopia/kopia) project which is used by Velero and configured by the OADP as this design proposes.

## Goals

- A new option `nodeAgent` under `configuration` section to allow configuration of restic or kopia uploader
- Backwards compatibility with the current OADP configuration schema options, namely `restic`
- Preparation for the future deprecation of the current `restic` configuration option
- Allow new schema(s) to be used by the datamover node agent
- Enablment of datamover node agent
- Deprecation of the `restic` configuration option
- New environment option `FS_PV_HOSTPATH` that is used as a replacement for `RESTIC_PV_HOSTPATH`. See [Compatibility](#compatibility) section for more details.
- Removal of the `restic-restore-action-config` ConfigMap with direct replacement by `fs-restore-action-config`. See [Compatibility](#compatibility) section for more details.

## Non Goals
- Removal of the `restic` configuration option
- Removal of the `RESTIC_PV_HOSTPATH` environment option
- Support for the downgreade of OADP operator with new configuration options
- E2E tests for the `kopia` or `restic` uploader, however they should be added in the near future to cover dpa testing of the new fields and we need backup/restore e2e tests which test both kopia and restic (and datamover eventually) using this new struct.

## High-Level Design

Since new `nodeAgent` configuration option is a sibling of the `restic` one, the new common structure `NodeAgentCommonFields` will be created which will be exactly the same data structure as current `ResticConfig` and will be used by both `Restic` and the new `NodeAgentConfig` structure. The only difference between `NodeAgentConfig` and `Restic` is an addition of one `UploaderType` option to the `NodeAgentConfig` that will be either `kopia` or `restic`.

When `nodeAgent` is used, the `UploaderType` option is a required one, so the user have to select either `kopia` or `restic`.

## Detailed Design


### New data structures

A new structure will be added, that includes the ResticConfig inline and extends this with one new parameter `UploaderType`:

```go
type NodeAgentConfig struct {
	// Embedding NodeAgentCommonFields
	// +optional
	NodeAgentCommonFields `json:",inline"`

	// The type of uploader to transfer the data of pod volumes, the supported values are 'restic' or 'kopia'
	// +kubebuilder:validation:Enum=restic;kopia
	// +kubebuilder:validation:Required
	UploaderType string `json:"uploaderType"`
}
```

The `NodeAgentCommonFields` structure is 1-1 as the current `Restic` structure.

```go
type NodeAgentCommonFields struct {
	// enable defines a boolean pointer whether we want the daemonset to
	// exist or not
	// +optional
	Enable *bool `json:"enable,omitempty"`
	// supplementalGroups defines the linux groups to be applied to the NodeAgent Pod
	// +optional
	SupplementalGroups []int64 `json:"supplementalGroups,omitempty"`
	// timeout defines the NodeAgent timeout, default value is 1h
	// +optional
	Timeout string `json:"timeout,omitempty"`
	// Pod specific configuration
	PodConfig *PodConfig `json:"podConfig,omitempty"`
}
```

Current `Restic` structure will embedd the `NodeAgentCommonFields` without any additional options or changes.

The above `NodeAgentConfig` is a member of `ApplicationConfig`, which already includes `ResticConfig`, however we do not replace the `ResticConfig`, instead for backwards compatibility we add new `NodeAgentConfig` parameter:

```go
// ApplicationConfig defines the configuration for the Data Protection Application
type ApplicationConfig struct {
	Velero *VeleroConfig `json:"velero,omitempty"`
	// (deprecation warning) ResticConfig is the configuration for restic DaemonSet.
	// Restic is for backwards compatibility and is replaced by the nodeAgent
	// Restic will be removed in the future
	// +kubebuilder:deprecatedversion:warning=1.3
	// +optional
	Restic *ResticConfig `json:"restic,omitempty"`

	// NodeAgentConfig is needed to allow selection between kopia or restic
	// +kubebuilder:validation:Optional
	NodeAgent *NodeAgentConfig `json:"nodeAgent,omitempty"`
}
```

### Configuration YAML options

The `nodeAgent` configuration options and `restic` configuration options will be exactly the same and will contain same options as the current `restic` schema with additional `uploaderType` field under `nodeAgent`. The part of YAML that presents the additions:

```yaml
configuration:
	description: configuration is used to configure the data protection application's server config
	properties:

	nodeAgent:
		description: NodeAgent is needed to allow selection between kopia or restic
		properties:

		[...] // <Same as all current restic configuration options>

		uploaderType:
			description: The type of uploader to transfer the data of pod volumes, the supported values are 'restic' or 'kopia'
			enum:
			- restic
			- kopia
			type: string
		type: object

	restic:
		description: (Deprecation Warning) Use nodeAgent instead of restic, which is deprecated and will be removed in the future
		properties:

		[...] // <Same as all current restic configuration options>

```

### Validations

It is important to disallow user from using both options `restic` and `nodeAgent` in OADP 1.3 together (error state).

### Deprecation of the `restic` coiguration option in OADP 1.3 and it's future removal

The `restic` configuration option will be deprecated in OADP 1.3. It will be removed in a future OADP release. However, restic backup functionality is still fully-supported, but restic users are encouraged to use the new `nodeAgent` configuration option instead, so that they won't be impacted on upgrade when the legacy struct is removed in a future release.

There were few alternatices considered (see [Alternatives Considered](#alternatives-considered)). We will have three places where the deprecation information will be presented to the user:
1. Description of the `resic` property will have the deprecation warning
2. If the `restic` is used, the DPA event will contain a `warning` message to inform user that the `restic` is deprecated, that will appear in the DPA `Events`.
3. If the `restic` is used, the application log will have `warning` message to inform user that the `restic` is deprecated
4. Release notes for OADP 1.3 will contain information about new configuration option `nodeAgent` that should be used instead of `restic`


## Alternatives Considered

- leave `restic` as the only option and do not allow to use `kopia`
- remove the `restic` and use `kopia` as the default and only available option

### Schema structure

There were alternative schema structure for the `Restic` and new `NodeAgentConfig` considered, however because structures may be directly used by other `go` applications, outside of the API schema, the decision was to use structure as described in the [New data structures](#new-data-structures) section:
- Keeping `Restic` as is and including inline into `NodeAgentConfig`
- Duplicating all the fields from `Restic` within `NodeAgentConfig`
- Moving all the fields from the `Restic` to the `NodeAgentConfig` and ignoring new field which will appear in the `Restic`

### Deprecation warnings
For informing user about deprecation of the `restic` we considered few additional options:
- Embedding `warning` within the OpenShift console. This would be the nicest way as the push notifications will appear in the main OCP console, however it would require bigger implementation effort.

- Creating custom information on the DPA object itself, so whenever user would describe created object that had the `restic` the deprecation warning would appear in the `events` section. That option also requires publishing custom `events` fromt he reconcile function and still requires user to pull that information rather then push method.

- Using kubebuilder annotation
	```
	// +kubebuilder:deprecatedversion:warning=<string>
	```
	This option is not for one particular onfiguration field, but entire CRD, which is outside of our desired needs as we do not want to deprecate entire CRD and create a new version of it, just one field rename.

- Adding additional Reconcile condition with warning message
	Currently there are two `Reasons` that the main DPA reconcail status may have: 
	```
	const ReconciledReasonComplete = "Complete"
	const ReconciledReasonError = "Error"
	```

	There is also message that is attached for each of this `Reason`. For an `Error` reason there is propagated error message and for the `Complete` reason there is one string, which is:
	```
	const ReconcileCompleteMessage = "Reconcile complete"
	```

	Additional Reconcile reason, which would have it's own message could be added, that will be similar to `Complete`, so the operator is functioning properly, however there are some messages that require user attention.
	```
	const ReconciledReasonWarning = "Warning"
	```

	This however would require refactoring of the `Reconcile` functions, and that would require separate design doc and separate implementation. 

## Security Considerations

The enablement of an extra upload mechanism could potentially introduce security implications due to the integration of a new backend, which might contain vulnerabilities.

## Compatibility

### Current `restic` configuration option

This design does not change current configuration options, however it proposes addition of the deprecation warning to the current `Restic` schema. The `Restic` configuration option will be removed in the future.

### RESTIC_PV_HOSTPATH environment option

This design does not remove `RESTIC_PV_HOSTPATH` used to redefine the default `/var/lib/kubelet/pods`, however it adds the environment variable `FS_PV_HOSTPATH` that may be used the same way as `RESTIC_PV_HOSTPATH`.

The `RESTIC_PV_HOSTPATH` takes precedence over `FS_PV_HOSTPATH`

### Replacing `restic-restore-action-config` with `fs-restore-action-config`

The `restic-restore-action-config` was [removed](https://github.com/vmware-tanzu/helm-charts/commit/7484d8e8365ab91da698e5b5d6346153cde30af4) in the Velero v1.10.0 it should also be removed and replaced with the [fs-restore-action-config](https://github.com/vmware-tanzu/helm-charts/blob/c15d13a916c255c93ef48e8bf9c59a3ba198d5ca/charts/velero/templates/NOTES.txt#L62).

## Implementation

Implementation for the new data structure will follow the [New data structures](#new-data-structures) design.

There are two uses of the additional `--uploader-type` that will specify restic or kopia uploader type.

- One within call to the method from the `github.com/vmware-tanzu/velero` package:
  ```go
  func Deployment(namespace string, opts ...podTemplateOption) *appsv1.Deployment
  ```

- Second is within the OADP `pkg/velero/server/args.go` that is used for testing and development purposes.

We will also rename the `restic.go` module to be named `nodeagent.go` and relevant functions within the `restic.go`, so they are more generic.

## Open Issues

- n/a
