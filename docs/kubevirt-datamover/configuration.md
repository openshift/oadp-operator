# KubeVirt DataMover Configuration

This guide explains how to enable and configure the KubeVirt DataMover feature in OADP. KubeVirt DataMover backs up and restores KubeVirt VirtualMachines by using QEMU Changed Block Tracking (CBT) instead of CSI snapshots, which lets you back up VMs on storage backends that do not support CSI snapshots or Container Storage Interface volume clones.

## How it fits together

KubeVirt DataMover is made up of three pieces:

- **kubevirt-datamover-plugin**: a Velero plugin (Backup Item Action, Restore Item Action, Delete Item Action) that intercepts VirtualMachine backups and creates `DataUpload`/`DataDownload` custom resources instead of letting Velero snapshot the underlying PVC directly.
- **kubevirt-datamover-controller**: a separate controller, deployed by the OADP operator, that watches those `DataUpload`/`DataDownload` resources and drives the actual CBT-based backup and restore using KubeVirt's `VirtualMachineBackup`/`VirtualMachineBackupTracker` APIs.
- **oadp-operator**: deploys the controller, wires up RBAC, and exposes configuration through the DataProtectionApplication (DPA) custom resource.

You do not interact with the plugin or the controller directly. Everything is driven through the DPA and normal Velero `Backup`/`Restore` objects.

## Prerequisites

Before enabling KubeVirt DataMover, make sure your cluster meets these requirements:

- OpenShift Virtualization (HCO) version 1.18 or later. HCO 1.18+ and the `backup.kubevirt.io` CRDs are required for CBT support.
- KubeVirt version 1.8.2 or later (this version includes a fix for a QEMU backup abort race that KubeVirt DataMover depends on).
- Changed Block Tracking enabled at the HCO level (see below).
- The VM you want to back up must be running. Offline (stopped) VM backup is not supported.
- An object storage backend configured as a Velero BackupStorageLocation. Backups that use KubeVirt DataMover must set `snapshotMoveData: true` on the Velero `Backup` object (or `spec.configuration.velero.defaultSnapshotMoveData: true` on the DPA to make it the default for all backups), since KubeVirt DataMover always moves data to object storage rather than leaving it as an in-cluster snapshot.

### Enabling Changed Block Tracking

CBT enablement is a two-part configuration on the `HyperConverged` (HCO) custom resource: enabling the feature gate, and telling KubeVirt which VM label to treat as opting a VM into CBT.

First, enable the `incrementalBackup` feature gate. This is a first-class field on the HCO CR and automatically turns on the underlying `IncrementalBackup` and `UtilityVolumes` feature gates on the KubeVirt CR:

```bash
oc patch hyperconverged kubevirt-hyperconverged -n openshift-cnv --type merge -p '
spec:
  featureGates:
    incrementalBackup: true
'
```

Second, configure the label selector KubeVirt uses to decide which VMs have CBT enabled. This field, `changedBlockTrackingLabelSelectors`, lives on the underlying KubeVirt CR that HCO manages, so it has to be injected through a `kubevirt.kubevirt.io/jsonpatch` annotation on the HCO CR rather than set directly:

```bash
oc annotate hyperconverged kubevirt-hyperconverged -n openshift-cnv --overwrite \
  kubevirt.kubevirt.io/jsonpatch='[{"op":"add","path":"/spec/configuration/changedBlockTrackingLabelSelectors","value":{"virtualMachineLabelSelector":{"matchLabels":{"changedBlockTracking":"true"}}}}]'
```

This example selector matches any VM labeled `changedBlockTracking: "true"`, which is the label used throughout this documentation and the sample manifests. You can choose a different label or match expression if you prefer, as long as it stays consistent between this HCO configuration and the labels you put on your VMs.

Verify the configuration took effect:

```bash
oc get kubevirt kubevirt-kubevirt-hyperconverged -n openshift-cnv \
  -o jsonpath='{.spec.configuration.changedBlockTrackingLabelSelectors}'
```

Expected output:

```json
{"virtualMachineLabelSelector":{"matchLabels":{"changedBlockTracking":"true"}}}
```

Once both of these are in place, KubeVirt supports incremental backups based on changed disk blocks for VMs that carry the matching label, provided the VM's disks use `volumeMode: Block` (CBT tracking does not work with filesystem-mode volumes). See [backup-restore.md](./backup-restore.md) for how to label a VM and confirm CBT is active on it.

## Enabling the plugin in the DPA

Add `kubevirt-datamover` to `spec.configuration.velero.defaultPlugins` in your DataProtectionApplication:

```yaml
apiVersion: oadp.openshift.io/v1alpha1
kind: DataProtectionApplication
metadata:
  name: velero-sample
  namespace: openshift-adp
spec:
  configuration:
    velero:
      defaultPlugins:
        - openshift
        - kubevirt
        - kubevirt-datamover
      podConfig:
        nodeSelector: {}
    nodeAgent:
      enable: true
      uploaderType: kopia
  snapshotLocations: []
  backupLocations:
    - velero:
        provider: aws
        default: true
        config:
          region: us-east-1
          profile: "default"
        credential:
          key: cloud
          name: cloud-credentials
        objectStorage:
          bucket: my-backup-bucket
          prefix: velero
  features: {}
```

A few important notes about this configuration:

- Always add both `kubevirt` and `kubevirt-datamover` together. The `kubevirt` plugin (kubevirt-velero-plugin) handles VM metadata and file-level restore concerns, while `kubevirt-datamover` handles the actual disk data movement. The operator will still let you enable `kubevirt-datamover` on its own, but it logs a warning if `kubevirt` is missing, and VM restores will not work correctly without it.
- KubeVirt DataMover can only be enabled on one DPA per cluster. If you try to enable it on a second DPA while another DPA already has it enabled and its controller deployment exists, the OADP operator rejects the DPA with a validation error. This is because the datamover controller is a cluster-scoped singleton, not a per-namespace component.
- You do not need to add anything to `snapshotLocations` for KubeVirt DataMover. It writes data straight to the object storage configured in your `backupLocations`.

When the plugin is enabled, the OADP operator deploys a `Deployment` named `oadp-kubevirt-datamover-controller-manager` in the same namespace as the DPA. You can confirm it came up correctly:

```bash
oc get deployment oadp-kubevirt-datamover-controller-manager -n openshift-adp
oc get pods -n openshift-adp -l control-plane=oadp-kubevirt-datamover-controller
```

## Tuning controller behavior

The DPA exposes a small set of tuning knobs under `spec.configuration.kubevirtDatamover`:

```yaml
apiVersion: oadp.openshift.io/v1alpha1
kind: DataProtectionApplication
metadata:
  name: velero-sample
  namespace: openshift-adp
spec:
  configuration:
    kubevirtDatamover:
      maxIncrementalBackups: 10
      maxConcurrentDataMovers: 5
      staleDataUploadThreshold: 3h
    velero:
      defaultPlugins:
        - openshift
        - kubevirt
        - kubevirt-datamover
```

- **maxIncrementalBackups**: the number of incremental (changed-blocks-only) backups the controller will chain together before it forces a full backup for a given VM. Set to `0` (the default) for unlimited incrementals, meaning the controller will keep chaining incremental backups indefinitely unless something else forces a full backup, such as a broken checkpoint chain. You can also override this per VM with the `kubevirt-datamover.io/max-incremental-backups` annotation on the VirtualMachine, which takes priority over the DPA-wide setting.
- **maxConcurrentDataMovers**: the maximum number of active DataUploads the controller will process at the same time, and separately, the maximum number of active DataDownloads it will process at the same time. It's the same configured number applied to both, but DataUploads and DataDownloads are counted independently against it, so a value of `5` allows up to 5 backups and up to 5 restores running concurrently, not 5 total. Set to `0` (the default) for unlimited. If you have a large number of VMs, set this explicitly rather than leaving it unlimited, a Backup that targets many VMs at once will otherwise try to run all of their DataUploads concurrently, which can overload your storage backend or object storage endpoint. (There is an open proposal to change the shipped default from `0` to `3`, see [migtools/kubevirt-datamover-controller#193](https://github.com/migtools/kubevirt-datamover-controller/issues/193). This doc reflects the current default as of this writing.)
- **staleDataUploadThreshold**: how long a `DataUpload` can sit in an active phase (Accepted, Prepared, InProgress) before the controller treats it as stale and stops letting it block newer DataUploads for the same VM. Defaults to 2 hours. Raise this if your VMs have very large disks and backups routinely take longer than 2 hours to complete.

All three fields are optional. If you leave them unset, the controller uses its built-in defaults.

You can also override the temporary backup PVC size on a per-VM basis with the `kubevirt-datamover.io/backup-pvc-size` annotation on the VirtualMachine (a Kubernetes quantity, for example `50Gi`). The controller normally calculates this size from the VM's disk, so you only need this if you have a VM where the automatic sizing isn't giving you enough headroom.

## Volume policy configuration

Velero decides which backup path to use for a given PVC through volume policies. To route a VM's disks through KubeVirt DataMover rather than a CSI snapshot, add a custom action to your Velero volume policy that targets `kubevirt` as the datamover. The ConfigMap needs to hold exactly one data entry, Velero rejects a ConfigMap with zero or more than one key, but the key itself can be named anything you like. Velero just reads whatever single value is in `data` and treats it as the policy YAML, it does not look for a key called `policy.yaml` specifically. Most examples (including this one) use `policy.yaml` by convention, but if you create the ConfigMap from a file with a different name, that works fine too.

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: kubevirt-volume-policy
  namespace: openshift-adp
data:
  policy.yaml: |
    version: v1
    volumePolicies:
      - conditions: {}
        action:
          type: custom
          parameters:
            datamover: kubevirt
```

You can also create the same ConfigMap directly from a policy file on disk instead of writing it inline:

```bash
oc create cm kubevirt-volume-policy -n openshift-adp --from-file policy.yaml
```

An empty `conditions: {}` matches every volume, so combine this with `includedNamespaces` on your `Backup` (or a separate policy entry) if you need to be more selective about which PVCs get routed through KubeVirt DataMover. Velero evaluates volume policy entries in order and stops at the first match, so a catch-all entry like the one above always needs to come last in your `volumePolicies` list. If you put it first, it will match every volume and none of the more specific entries after it will ever be considered. If you want to condition on the CSI driver instead, Velero's `csi.driver` condition requires an exact driver name and does not support wildcards, so a plain `csi: {}` (matches any CSI-backed volume) or a fully spelled out driver name works, but `driver: "*"` does not match anything.

Reference this ConfigMap from your Velero `Backup` object's `spec.resourcePolicy`:

```yaml
apiVersion: velero.io/v1
kind: Backup
metadata:
  name: my-vm-backup
  namespace: openshift-adp
spec:
  includedNamespaces:
  - my-vm-namespace
  snapshotMoveData: true
  resourcePolicy:
    kind: ConfigMap
    name: kubevirt-volume-policy
```

Or reference it directly when creating the backup with the `oc oadp` CLI plugin instead of writing the YAML by hand:

```bash
oc oadp backup create my-vm-backup --include-namespaces my-vm-namespace --resource-policies-configmap kubevirt-volume-policy --snapshot-move-data
```

Velero treats `custom` actions with unrecognized parameters as a signal to hand off data movement to whichever plugin claims the volume, which in this case is the kubevirt-datamover-plugin.

If you back up a namespace that has both VMs and regular workloads, this policy only changes behavior for volumes attached to VirtualMachines. PVCs that are not owned by a VM do not meet the kubevirt-datamover-plugin's prerequisites, so the plugin will not pick them up even though the volume policy matched them. Velero itself has no way to tell in advance whether a given PVC belongs to a VM, so for non-VM volumes this custom policy effectively behaves the same as a `skip` action: Velero hands the volume off looking for a datamover plugin to claim it, none does, and the volume ends up not being moved by this policy at all. If you need non-VM PVCs in the same namespace to still get backed up through CSI snapshots or File System Backup, keep this custom policy scoped with `includedNamespaces` or a label selector so it only ever matches VM-owned volumes, rather than relying on it to fall back safely for everything else.

## Storage provider and credential setup

KubeVirt DataMover writes checkpoint data directly to the object storage bucket configured in your BackupStorageLocation. It supports the same providers OADP already supports for Velero:

- **AWS S3 and S3-compatible storage**: standard secret-based credentials, or a projected service account token when using AWS STS (the controller automatically requests a token with the `openshift` audience and refreshes it, the same pattern used by the rest of OADP's STS support).
- **Azure Blob Storage**: storage account key, or Azure Workload Identity. If you have Workload Identity configured for OADP already (the `AZURE_TENANT_ID`, `AZURE_CLIENT_ID`, and federated token secret), the operator automatically propagates the same identity to the datamover controller pod, so there is no separate Azure setup step.
- **Google Cloud Storage**: service account key with signing permissions.

If your BackupStorageLocation credentials already work for regular Velero backups, they will work for KubeVirt DataMover without any extra configuration, because the controller reads the same BackupStorageLocation and credential secret that Velero uses.

## Verifying the setup

Once the DPA is applied and reconciled, check that the DPA condition reports the controller as ready:

```bash
oc get dpa velero-sample -n openshift-adp -o jsonpath='{.status.conditions}' | jq
```

Look for a condition of type `KubevirtDatamoverReady`. If it is not `True`, check the controller pod logs:

```bash
oc logs -n openshift-adp deployment/oadp-kubevirt-datamover-controller-manager
```

At this point you are ready to back up and restore VMs. See [backup-restore.md](./backup-restore.md) for the end-to-end workflow, and [troubleshooting.md](./troubleshooting.md) if something does not work as expected.
