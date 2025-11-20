<hr style="height:1px;border:none;color:#333;">
<h1 align="center">VM File Restore Configuration</h1>
<hr style="height:1px;border:none;color:#333;">

### Enable VM File Restore

The VM File Restore feature allows you to perform granular file-level restore from VM backups without restoring the entire virtual machine. This is particularly useful when you only need to recover specific files or directories from a backed-up VM.

### Prerequisites

Before enabling VM File Restore, ensure the following prerequisites are met:

1. **OpenShift Virtualization**: KubeVirt must be installed and configured in your OpenShift cluster
2. **Required Plugins**: The following Velero plugins must be configured in your DPA:
   - `kubevirt` - Provides VM backup and restore capabilities
   - `openshift` - Required for OpenShift-specific functionality

### Basic Configuration

To enable VM File Restore, add the `vmFileRestore` section to your DataProtectionApplication CR:

```yaml
apiVersion: oadp.openshift.io/v1alpha1
kind: DataProtectionApplication
metadata:
  name: dpa-sample
  namespace: openshift-adp
spec:
  configuration:
    velero:
      defaultPlugins:
        - openshift
        - aws
        - kubevirt  # Required for VM file restore

  # Enable VM File Restore
  vmFileRestore:
    enable: true

  backupLocations:
    - name: default
      velero:
        provider: aws
        default: true
        objectStorage:
          bucket: my-backup-bucket
          prefix: velero
        config:
          region: us-east-1
```

### Custom Resource Configuration

You can specify custom resource limits and requests for the VM File Restore controller:

```yaml
apiVersion: oadp.openshift.io/v1alpha1
kind: DataProtectionApplication
metadata:
  name: dpa-sample
  namespace: openshift-adp
spec:
  configuration:
    velero:
      defaultPlugins:
        - openshift
        - aws
        - kubevirt

  vmFileRestore:
    enable: true
    resources:
      limits:
        cpu: "1"
        memory: 256Mi
      requests:
        cpu: 100m
        memory: 128Mi

  backupLocations:
    - name: default
      velero:
        provider: aws
        default: true
        objectStorage:
          bucket: my-backup-bucket
          prefix: velero
        config:
          region: us-east-1
```

**Default Resources** (if not specified):
- CPU Limit: 500m
- Memory Limit: 128Mi
- CPU Request: 10m
- Memory Request: 64Mi

### Image Overrides

For disconnected environments or testing, you can override the VM File Restore controller images using `unsupportedOverrides`:

```yaml
apiVersion: oadp.openshift.io/v1alpha1
kind: DataProtectionApplication
metadata:
  name: dpa-sample
  namespace: openshift-adp
spec:
  configuration:
    velero:
      defaultPlugins:
        - openshift
        - aws
        - kubevirt

  vmFileRestore:
    enable: true

  unsupportedOverrides:
    vmFileRestoreControllerImageFqin: "my-registry.io/oadp-vm-file-restore:v1.0.0"
    vmFileRestoreAccessImageFqin: "my-registry.io/oadp-vmfr-access:v1.0.0"
    vmFileRestoreSSHImageFqin: "my-registry.io/oadp-vmfr-access-sshd:v1.0.0"
    vmFileRestoreBrowserImageFqin: "my-registry.io/oadp-vmfr-access-filebrowser:v1.0.0"

  backupLocations:
    - name: default
      velero:
        provider: aws
        default: true
        objectStorage:
          bucket: my-backup-bucket
          prefix: velero
        config:
          region: us-east-1
```

### Using VM File Restore

Once enabled, the VM File Restore controller will be deployed in the OADP namespace. You can then use the following Custom Resources:

#### 1. VirtualMachineBackupsDiscovery (VMBD)

Discover available VM backups:

```yaml
apiVersion: oadp.openshift.io/v1alpha1
kind: VirtualMachineBackupsDiscovery
metadata:
  name: my-vm-discovery
  namespace: openshift-adp
spec:
  backupName: my-vm-backup
```

#### 2. VirtualMachineFileRestore (VMFR)

Restore specific files from a VM backup:

```yaml
apiVersion: oadp.openshift.io/v1alpha1
kind: VirtualMachineFileRestore
metadata:
  name: my-vm-file-restore
  namespace: openshift-adp
spec:
  backupName: my-vm-backup
  volumeName: my-vm-disk
  fileAccessMethod: ssh  # or filebrowser
```

### File Access Methods

The VM File Restore feature supports two methods for accessing restored files:

1. **SSH Access**: Connect to restored files via SSH (requires SSH client)
2. **FileBrowser**: Web-based file browser interface for easy file access

### Validation Requirements

When VM File Restore is enabled, OADP validates:

- ✓ Velero configuration is present
- ✓ kubevirt-velero-plugin is configured in defaultPlugins
- ✓ openshift-velero-plugin is configured in defaultPlugins
- ✓ Only one DPA instance across the cluster has VM File Restore enabled

### Disabling VM File Restore

To disable VM File Restore, set `enable` to `false`:

```yaml
apiVersion: oadp.openshift.io/v1alpha1
kind: DataProtectionApplication
metadata:
  name: dpa-sample
  namespace: openshift-adp
spec:
  configuration:
    velero:
      defaultPlugins:
        - openshift
        - aws
        - kubevirt

  vmFileRestore:
    enable: false  # Disables VM file restore
```

The VM File Restore controller deployment will be automatically removed when disabled.

### Troubleshooting

**VM File Restore controller not deploying:**
- Check that both `kubevirt` and `openshift` plugins are in defaultPlugins
- Verify that KubeVirt is installed in your cluster
- Check OADP operator logs for validation errors

**Cannot create VirtualMachineFileRestore resources:**
- Ensure VM File Restore is enabled in the DPA
- Verify the controller deployment is running: `oc get deployment -n openshift-adp oadp-vm-file-restore-controller-manager`
- Check controller logs for errors: `oc logs -n openshift-adp deployment/oadp-vm-file-restore-controller-manager`

**Multiple DPA error:**
- Only one DPA instance across the entire cluster can have VM File Restore enabled
- Disable VM File Restore in other DPA instances if the error occurs

### Additional Resources

- [OpenShift Virtualization Documentation](https://docs.openshift.com/container-platform/latest/virt/about_virt/about-virt.html)
- [KubeVirt Project](https://kubevirt.io/)
- [OADP VM File Restore Repository](https://github.com/migtools/oadp-vm-file-restore)
