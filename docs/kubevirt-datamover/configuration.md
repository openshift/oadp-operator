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

- OpenShift Virtualization (HCO) version 1.18 or later
- KubeVirt version 1.8.2 or later (this version includes a fix for a QEMU backup abort race that KubeVirt DataMover depends on)
- Changed Block Tracking enabled at the HCO level (see below)
- An object storage backend configured as a Velero BackupStorageLocation, with `snapshotMoveData: true` set on the DPA (KubeVirt DataMover always moves data to object storage, it does not do in-cluster snapshots)

### Enabling Changed Block Tracking

CBT is an HCO-level feature gate. Enable it on the `HyperConverged` custom resource:

```bash
oc patch hyperconverged kubevirt-hyperconverged -n openshift-cnv \
  --type=json -p '[{"op": "add", "path": "/spec/featureGates/enableCommonBootImageImport", "value": true}]'
```

The actual flag name and path can differ across HCO releases, so check the `HyperConverged` CR in your cluster and the OpenShift Virtualization release notes for the exact feature gate name that enables CBT for your version. Once enabled, KubeVirt itself will support incremental backups based on changed disk blocks for VMs whose storage class and volume mode support it (CBT currently works with block-mode PVCs on CSI drivers that expose block volumes).

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
      staleDataUploadThreshold: 3h
    velero:
      defaultPlugins:
        - openshift
        - kubevirt
        - kubevirt-datamover
```

- **maxIncrementalBackups**: the number of incremental (changed-blocks-only) backups the controller will chain together before it forces a full backup for a given VM. Set to `0` (the default) for unlimited incrementals, meaning the controller will keep chaining incremental backups indefinitely unless something else forces a full backup, such as a broken checkpoint chain. You can also override this per VM with the `kubevirt-datamover.io/max-incremental-backups` annotation on the VirtualMachine, which takes priority over the DPA-wide setting.
- **staleDataUploadThreshold**: how long a `DataUpload` can sit in an active phase (Accepted, Prepared, InProgress) before the controller treats it as stale and stops letting it block newer DataUploads for the same VM. Defaults to 2 hours. Raise this if your VMs have very large disks and backups routinely take longer than 2 hours to complete.

Both fields are optional. If you leave them unset, the controller uses its built-in defaults.

## Volume policy configuration

Velero decides which backup path to use for a given PVC through volume policies. To route a VM's disks through KubeVirt DataMover rather than a CSI snapshot, add a custom action to your Velero volume policy that targets `kubevirt` as the datamover:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: change-storage-class-config
  namespace: openshift-adp
data:
  volume-policy.yaml: |
    version: v1
    volumePolicies:
      - conditions:
          csi:
            driver: "*"
        action:
          type: custom
          parameters:
            datamover: kubevirt
```

Reference this ConfigMap from your Velero `Backup` (or from the DPA if you configure volume policies globally, depending on how your Velero setup handles policy ConfigMaps). Velero treats `custom` actions with unrecognized parameters as a signal to hand off data movement to whichever plugin claims the volume, which in this case is the kubevirt-datamover-plugin.

If you back up a namespace that has both VMs and regular workloads, this policy only changes behavior for volumes attached to VirtualMachines. PVCs that are not owned by a VM continue to use Velero's normal CSI snapshot or File System Backup path.

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
