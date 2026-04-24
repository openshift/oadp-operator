# Kubevirt Datamover Manual Test

This directory contains the manifests needed to manually run the kubevirt-datamover
CBT backup flow that the E2E test `"Kubevirt datamover backup with CBT"` automates.

## Prerequisites

- OpenShift cluster with OpenShift Virtualization (HCO >= 1.18) installed (KubeVirt >= v1.8.2)
  - HCO 1.18+ and the `backup.kubevirt.io` CRDs are required for VEP-25 (IncrementalBackup / CBT) support
  - KubeVirt >= v1.8.2 is required for the QEMU backup-abort fix (KubeVirt PR #16426)
- OADP operator installed
- A working BackupStorageLocation (S3 bucket with credentials)

## Step 1: Configure HyperConverged Operator for CBT

Two separate configurations are required on the HCO CR.

### 1a. Enable the incrementalBackup feature gate

Patch the HCO CR directly -- this is a first-class field:

```bash
oc patch hyperconverged kubevirt-hyperconverged -n openshift-cnv --type merge -p '
spec:
  featureGates:
    incrementalBackup: true
'
```
**note:** this may be outdated
This enables both the `IncrementalBackup` and `UtilityVolumes` feature gates on
the KubeVirt CR automatically.

### 1b. Enable the CBT label selector via jsonpatch annotation

The `changedBlockTrackingLabelSelectors` field lives on the KubeVirt CR, which is
managed by HCO. To inject it without HCO overwriting it, use the jsonpatch
annotation on the HCO CR:

```bash
oc annotate hyperconverged kubevirt-hyperconverged -n openshift-cnv --overwrite \
  kubevirt.kubevirt.io/jsonpatch='[{"op":"add","path":"/spec/configuration/changedBlockTrackingLabelSelectors","value":{"virtualMachineLabelSelector":{"matchLabels":{"changedBlockTracking":"true"}}}}]'
```

**If KVM emulation is also enabled**, combine both patches into one annotation:

```bash
oc annotate hyperconverged kubevirt-hyperconverged -n openshift-cnv --overwrite \
  kubevirt.kubevirt.io/jsonpatch='[{"op":"add","path":"/spec/configuration/developerConfiguration","value":{"useEmulation":true}},{"op":"add","path":"/spec/configuration/changedBlockTrackingLabelSelectors","value":{"virtualMachineLabelSelector":{"matchLabels":{"changedBlockTracking":"true"}}}}]'
```

### Verify CBT is configured

```bash
oc get kubevirt kubevirt-hyperconverged -n openshift-cnv -o jsonpath='{.spec.configuration.changedBlockTrackingLabelSelectors}'
```

Expected output:
```json
{"virtualMachineLabelSelector":{"matchLabels":{"changedBlockTracking":"true"}}}
```

## Step 2: Configure the DPA

The DPA must include both the `kubevirt` and `kubevirt-datamover` default plugins.
The `kubevirt-datamover` plugin causes the OADP operator to deploy:
- The kubevirt-datamover-plugin as a Velero init container
- The kubevirt-datamover-controller Deployment

Example DPA spec (adjust BSL/credentials for your environment):

```yaml
apiVersion: oadp.openshift.io/v1alpha1
kind: DataProtectionApplication
metadata:
  name: velero-test
  namespace: openshift-adp
spec:
  configuration:
    velero:
      defaultPlugins:
      - openshift
      - csi
      - aws
      - kubevirt
      - kubevirt-datamover
    nodeAgent:
      enable: true
      uploaderType: kopia
  backupLocations:
  - velero:
      provider: aws
      default: true
      objectStorage:
        bucket: <YOUR_BUCKET>
        prefix: velero
      config:
        region: <YOUR_REGION>
      credential:
        name: cloud-credentials
        key: cloud
```

### Verify the datamover controller is running

```bash
oc get deployment -n openshift-adp | grep datamover
oc get pods -n openshift-adp | grep datamover
```

## Step 3: Deploy the CirrOS VM with CBT label

```bash
oc apply -f cirros-vm-cbt.yaml
```

Wait for the VM to be Running:

```bash
oc get vm -n cirros-test cirros-test -w
```

## Step 4: Verify CBT is enabled on the VM

With KubeVirt >= v1.8.2 and the feature gate + label selector configured in Step 1,
CBT is activated when the VM first boots — no manual restart cycle is required.

If you are on an older KubeVirt version or CBT does not appear enabled after boot,
you can trigger activation with a stop/start cycle:

```bash
virtctl stop cirros-test -n cirros-test
# Wait for Stopped...
oc wait vm cirros-test -n cirros-test --for=jsonpath='{.status.printableStatus}'=Stopped --timeout=5m

virtctl start cirros-test -n cirros-test
# Wait for Running...
oc wait vm cirros-test -n cirros-test --for=jsonpath='{.status.printableStatus}'=Running --timeout=5m
```

Check that CBT is active:

```bash
oc get vm cirros-test -n cirros-test -o jsonpath='{.status.changedBlockTracking.state}'
```

Expected output: `Enabled`

## Step 5: Create the volume policy ConfigMap

This ConfigMap tells Velero to skip CSI snapshots for PVCs, allowing the
kubevirt-datamover-plugin BackupItemActionV2 to handle them instead.

```bash
oc apply -f volume-policy.yaml
```

## Step 6: Create the backup

```bash
oc apply -f backup-cirros.yaml
```

### Monitor the backup

```bash
oc get backup kubevirt-dm-backup-1 -n openshift-adp -w
```

### Check for kubevirt-datamover CRs

These CRs are created by the kubevirt-datamover-controller when it processes
the DataUpload. Their presence confirms the datamover path is active.

```bash
oc get virtualmachinebackuptrackers -A -o yaml
oc get virtualmachinebackups -A -o yaml
```

### Verify backup completed

```bash
oc get backup kubevirt-dm-backup-1 -n openshift-adp -o jsonpath='{.status.phase}'
```

Expected output: `Completed`

## Additional VM manifests

This directory also contains VM manifests for other guest OS options that use the
same CBT + datamover flow. They are not yet exercised by automated CI but can be
used for manual testing following the same steps above.

| File | VM name | Namespace | Notes |
|------|---------|-----------|-------|
| `fedora-todolist-cbt.yaml` | `fedora-todolist` | `mysql-persistent` | Fedora VM running a todolist/mariadb workload |
| `centos-stream10-cbt.yaml` | `centos-stream10-todolist` | `mysql-persistent` | CentOS Stream 10 VM running a todolist/mariadb workload |
| `backup-fedora.yaml` | — | `mysql-persistent` | Velero Backup CR for the Fedora/CentOS VMs; adjust `storageLocation` to match your DPA's BSL name |

## Cleanup

```bash
oc delete backup kubevirt-dm-backup-1 -n openshift-adp
oc delete configmap kubevirt-volume-policy -n openshift-adp
oc delete namespace cirros-test
```
