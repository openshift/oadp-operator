***
## NooBaa Debugging
***

There are some scenarios in which the installation of OADP Operator with NooBaa might fail and Backup/Resore Operations might not work properly.

### Absence of NooBaa CRDs

In this scenario the installation of NooBaa with the OADP Operator might fail with the following error message in the OADP Operator logs

```
NooBaa CRDs are not present, please install the OCS Operator (Noobaa Operator)
```

This error messae implies that the NooBaa CRDs are not present in the OpenShift cluster. It means either the OCS (OpenShift Container Storage Operator) was not installed from the OperatorHub before performing the NooBaa steps or it might be the case the OCS Operator installation might have failed.

<b>Note: </b> The OCS Operator should be installed explicitly in the `oadp-operator` namespace.

### NooBaa Operator (OCS Operator) transient issue

In this scenario the NooBaa deployment fails, this is a transient issue and might hinder the installation at random times. Following are some of the indicators of this issue:
- The noobaa-core pod keeps on restarting.
- The NooBaa deployment returns back to `Connecting` phase from `Ready` phase.
- Absence of lib-bucket-provisionor operator (should come out of the box with OCS Operator but sometimes the lib-provisioner operator does not get installed)
- And the most sure shot way indicator is the `modeCode` of `oadp-storage-pv-pool-backing-store` backingstore, the `modeCode` should be `OPTIMAL`.
- Also, if the `modeCode` of this `oadp-storage-pv-pool-backing-store` backingstore remains in `INITIALIZING` mode even after 10 min.

In order get out of this scenario, you need to perform the [cleanup](docs/../cleanup_oadp_noobaa.md) steps and do a fresh [install](docs/../install_oadp_noobaa.md).

<b>Note: </b> Use the command `oc get backingstores oadp-storage-pv-pool-backing-store -o yaml` to check up on the `oadp-storage-pv-pool-backing-store` backingstore.

### Configuration of AWS BackupStorageLocations or VolumeSnapshotLocations via Velero CR

Just a reminder that we do not require AWS BackupStorageLocations or VolumeSnapshotLocations for NooBaa to work with OADP Operator and perform Backup/Restore Operations on OpenShift workloads. If AWS BackupStorageLocations or VolumeSnapshotLocations are configured then you might get errors logs in the `velero` pod pertaing to the status codes `401` and `500`. Please make sure there are no AWS BackupStorageLocations or VolumeSnapshotLocations configured, only the `noobaa` ones are valid and required for our use case.

