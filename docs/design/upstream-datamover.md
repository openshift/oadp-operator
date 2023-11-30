# Design for Integrating Velero Built-in Data Mover (VBDM) support into OADP


## Abstract
One of the primary new features of Velero 1.12 is the new Velero Built-in Data Mover (VBDM).
The OADP Operator needs to add configuration (and e2e tests) to support this new datamover and remove any configuration and testing of the legacy OADP datamover.

## Background
Prior to the release of Velero 1.12, OADP 1.1 and 1.2 integrated an OADP-specific datamover based on Volsync into OADP and the OADP-installed Velero version.
This took the form of some modifications to Velero core and the Velero CSI plugins, a new VSM plugin, and a separate VSM controller that used Volsync to store Volume data in the BackupStorageLocation.
In addition to this, DPA and installation support for the OADP datamover was included in the OADP operator.

In order to integrate VBDM into OADP, all of the above legacy OADP datamover parts will be removed from the OADP operator, the OADP Velero build, and the OADP build of the Velero Plugin for CSI.

## Goals
- Enable Velero Built-in Data Mover (VBDM) in OADP.
- Remove support for the legacy Volsync-based OADP Data Mover.

## Non Goals
- Enabling restore of OADP 1.1 or 1.2 Data Mover backups in OADP with Velero Built-in Datamover enabled


## High-Level Design
Built-in Data Mover functionality will be available in Velero 1.12.
This proposal will allow for enabling this new datamover for backup and restore in OADP.
As part of implementation, any OADP support for the previous Volsync-based datamover will be removed.
All OADP end-to-end tests for data movement will be updated to use the new Built-in Data Mover instead of the Volsync-based data mover.


## Detailed Design

### Volsync Datamover removal

All Volsync Datamover-related code and configuration will be removed from oadp-operator.

#### DPA CRD

For the `DataProtectionApplication` CRD, remove the `DataMover` (and related) structs, including `RetainPolicy` and `VolumeOptions`.
Specifically, `DataMover` will be removed from the `Features` struct which is included in DPA spec.
`Features` will be empty for now, but available for future tech preview features that need explicit configuration here.
```
// Features defines the configuration for the DPA to enable the tech preview features
type Features struct {
}
```
Other places where DataMover-related fields will be removed from the DPA:
1. Remove `dataMoverImageFqin` from the list of valid keys for `UnsupportedOverrides`.
1. Remove `vsmPluginImageFqin` from the list of valid keys for `UnsupportedOverrides`.
1. Remove vsm from the list of default plugins.
1. Remove related consts and labels.

#### OADP controllers

In addition, related controller actions, validation, and unit tests for Volsync data mover will be removed:
1. controllers/validator.go: datamover-related validation
1. controllers/datamover: the entire file
1. controllers/datamover_test.go: the entire file
1. controllers/velero.go: datamover-related velero configuration
1. controllers/dpa_controller.go: datamover-related entries in `ReconcileBatch`
1. pkg/common/common.go: datamover-related const definitions
1. pkg/velero/server/config.go: datamover CRDs removed from restore priorities list

#### Other OADP CRDs
On the CRD side, the `VolumeSnapshotBackup` and `VolumeSnapshotRestore` CRDs will be removed from `config/crd/bases` and from `make bundle` processing.
The new velero `DataUpload` and `DataDownload` will be added.

#### OADP container images

References to `velero-plugin-for-vsm` and `volume-snapshot-mover` images need to be removed from `config/manager/manager.yaml`, golang code, and `hack/disconnected-prep.sh`.

#### Velero code

In addition to pulling in Velero 1.12 upstream changes into the OADP Velero fork, we will need to remove all of the Velero customizations in our velero and velero-plugin-for-csi repos that were added to support the Volsync data mover.

### Velero Built-in Data Mover configuration

No new DPA fields will be needed for VBDM -- to enable it, the user just needs to include the CSI plugin and enable the node agent. For 1.3, users should use the new `NodeAgentConfig` spec field for this, although the existing (deprecated in 1.3) `Restic` spec field will also work here..

Note that unlike in the OADP Volsync data mover, with VBDM, to use the datamover for a specific backup, that backup must be created with the `--snapshot-move-data` flag to `velero backup create` or with `Spec.SnapshotMoveData` set to `true` on the Backup CR.

### End-to-end test modification

Existing end-to-end tests for datamover use cases will be modified to configure OADP (and Backup CRs) for VBDM.
This will involve removing any Volsync datamover-specific configuration and test assertions, replacing it with VBDM configuration and test assertions.
Existing DPA end-to-end tests will also be modified.
For the DPA tests, this will mostly involve removal of code, since there is no new server/installation configuration needed to enable data movement.


## Alternatives Considered
1. Sticking with Volsync data mover in OADP. This was rejected because the upstream solution will allow us to leverage the existing community and maintainership of Velero to drive features, testing, and bug fixing. In addition, the future of Volsync is in question.
1. Keeping Volsync data mover active in parallel with VBDM to ease user/customer transition. This was rejected due to the maintanence complexity of making the two solutions compatible with each other, and with Volsync data mover in tech preview, future support is not required.

## Security Considerations
From a security point of view, volume data backed up via VBDM is equivalent to data backed up via the existing filesystem backup mechanism.
Any future security enhancements made for the fs backup feature will apply to VBDM as well.

## Compatibility
VBDM is not compatible with the previous OADP 1.2 Volsync-based data mover.
Volsync backups from OADP 1.2 will not be restoreable with any OADP version incorporatig VBDM.

## Implementation
Initial work for this will be done on the velero-datamover topic branch of oadp-operator.
Once the design changes are fully approved and basic functionality for OADP is fully working again (possibly with the exception of the e2e test changes), it will be merged to master and work will continue there.

The velero and velero-plugin-for-csi forks have already been updated with the current VBDM changes but will continue to be updated periodically throughout the remainder of the Velero 1.12 development cycle.

The next steps for implementation will be the OADP DPA and other CRD changes and the controller work.
The final implementation task will be updating the OADP e2e tests.

## Open Issues
- Given that supporting restore of OADP 1.1/1.2 datamover backups in 1.3 is explicitly called out as *not* supported here, we need a plan around what to tell users so that they can navigate the 1.2->1.3 upgrade without having no valid backups. There are a few things to consider (some more feasible than others):
  - Inform users that if they want to have easily-restorable backups immediately upon upgrade, they should do a final backup of all of their workloads with restic prior to upgrading. Restic backups from 1.2 will continue to be fully supported in 1.3.
  - If data is needed from a prior backup after upgrade (for some reason the restic backup taken above is unusable, or was not taken in the first place), things get more difficult.
    - One possibility would be to downgrade to 1.2 and restore -- any new-in-1.3 DPA fields would need to be removed prior to downgrade
    - Another possiblity would be to install OADP 1.2 in a different cluster, restore here, and then back up again via restic. This backup could then be used in the original cluster.
    - It may be possible to manually pull volume data from the BSL with Volsync directly, but the user would have to do a lot of manual steps, including creating volumes to receive this data. This may not be a feasible solution at all.
- Is this functionality to be considered tech preview or GA for OADP 1.3? With the current design, I don't believe that  this decision will have any material impact on the implementation, since the only OADP DPA configuration needed to enable VBDM (enabling CSI plugin and restic/node-agent) is already available as GA in OADP.