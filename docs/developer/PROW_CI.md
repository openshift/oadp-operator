## PROW CI

The project uses [PROW CI](https://docs.ci.openshift.org/docs/) as a Continuous Integration/Delivery tool.

The configuration for OADP repo can be found in
- https://github.com/openshift/release/tree/master/core-services/prow/02_config/openshift/oadp-operator
- https://github.com/openshift/release/tree/master/ci-operator/config/openshift/oadp-operator
- https://github.com/openshift/release/tree/master/ci-operator/jobs/openshift/oadp-operator

The configuration for [OpenShift Velero plugin repo](https://github.com/openshift/openshift-velero-plugin) can be found in
- https://github.com/openshift/release/tree/master/core-services/prow/02_config/openshift/openshift-velero-plugin
- https://github.com/openshift/release/tree/master/ci-operator/config/openshift/openshift-velero-plugin
- https://github.com/openshift/release/tree/master/ci-operator/jobs/openshift/openshift-velero-plugin

The configuration for [Volume Snapshot Mover repo](https://github.com/migtools/volume-snapshot-mover) can be found in (OADP < 1.3)
- https://github.com/openshift/release/tree/master/core-services/prow/02_config/migtools/volume-snapshot-mover
- https://github.com/openshift/release/tree/master/ci-operator/config/migtools/volume-snapshot-mover
- https://github.com/openshift/release/tree/master/ci-operator/jobs/migtools/volume-snapshot-mover

The configuration for [Velero plugin for VSM repo](https://github.com/migtools/velero-plugin-for-vsm) can be found in (OADP < 1.3)
- https://github.com/openshift/release/tree/master/core-services/prow/02_config/migtools/velero-plugin-for-vsm
- https://github.com/openshift/release/tree/master/ci-operator/config/migtools/velero-plugin-for-vsm
- https://github.com/openshift/release/tree/master/ci-operator/jobs/migtools/velero-plugin-for-vsm

The configuration for [OADP's Velero repo](https://github.com/openshift/velero) can be found in
- https://github.com/openshift/release/tree/master/core-services/prow/02_config/openshift/velero
- https://github.com/openshift/release/tree/master/ci-operator/config/openshift/velero
- https://github.com/openshift/release/tree/master/ci-operator/jobs/openshift/velero

The configuration for [OADP's Velero plugin for AWS repo](https://github.com/openshift/velero-plugin-for-aws) can be found in
- https://github.com/openshift/release/tree/master/core-services/prow/02_config/openshift/velero-plugin-for-aws
- https://github.com/openshift/release/tree/master/ci-operator/config/openshift/velero-plugin-for-aws
- https://github.com/openshift/release/tree/master/ci-operator/jobs/openshift/velero-plugin-for-aws

The configuration for [OADP's Velero plugin for GCP repo](https://github.com/openshift/velero-plugin-for-gcp) can be found in
- https://github.com/openshift/release/tree/master/core-services/prow/02_config/openshift/velero-plugin-for-gcp
- https://github.com/openshift/release/tree/master/ci-operator/config/openshift/velero-plugin-for-gcp
- https://github.com/openshift/release/tree/master/ci-operator/jobs/openshift/velero-plugin-for-gcp

The configuration for [OADP's Velero plugin for Microsoft Azure repo](https://github.com/openshift/velero-plugin-for-microsoft-azure) can be found in
- https://github.com/openshift/release/tree/master/core-services/prow/02_config/openshift/velero-plugin-for-microsoft-azure
- https://github.com/openshift/release/tree/master/ci-operator/config/openshift/velero-plugin-for-microsoft-azure
- https://github.com/openshift/release/tree/master/ci-operator/jobs/openshift/velero-plugin-for-microsoft-azure

The configuration for [OADP's Velero plugin for CSI repo](https://github.com/openshift/velero-plugin-for-csi) can be found in
- https://github.com/openshift/release/tree/master/core-services/prow/02_config/openshift/velero-plugin-for-csi
- https://github.com/openshift/release/tree/master/ci-operator/config/openshift/velero-plugin-for-csi
- https://github.com/openshift/release/tree/master/ci-operator/jobs/openshift/velero-plugin-for-csi

The configuration for [OADP's restic repo](https://github.com/openshift/restic) can be found in (OADP < 1.3)
- https://github.com/openshift/release/tree/master/core-services/prow/02_config/openshift/restic
- https://github.com/openshift/release/tree/master/ci-operator/config/openshift/restic
- https://github.com/openshift/release/tree/master/ci-operator/jobs/openshift/restic

The configuration for [Non Admin repo](https://github.com/migtools/oadp-non-admin) can be found in (OADP >= 1.4)
- https://github.com/openshift/release/tree/master/core-services/prow/02_config/migtools/oadp-non-admin
- https://github.com/openshift/release/tree/master/ci-operator/config/migtools/oadp-non-admin
- https://github.com/openshift/release/tree/master/ci-operator/jobs/migtools/oadp-non-admin

The images mirroring to [quay.io](https://quay.io/organization/konveyor) configuration for all OADP related images can be found in
- https://github.com/openshift/release/tree/master/core-services/image-mirroring/konveyor

The jobs run can be seen in PRs and in the links in the README.md file of OADP repo.

OADP operator master branch is tested against the last 3 minor OCP releases. To Update an OCP version the project is tested against, see [Update OCP version](#update-ocp-version).

### TODO Creating new release branch

https://github.com/openshift/oadp-operator/issues/1227

**TODO automate this process**

### Update Go version

To update the Go version used in CI jobs of a branch:
- Update the image used in `build_root` instruction of ALL jobs of that branch.

> **Note:** images referenced in `build_root` instruction ([which creates `root` image, that is tagged into `src`](https://docs.ci.openshift.org/docs/internals/)) of the config files, are defined in [here](https://github.com/openshift-eng/ocp-build-data/tree/main), under the specific branch. For example, `openshift-4.14` images are defined [here](https://github.com/openshift-eng/ocp-build-data/blob/openshift-4.14/streams.yml). The OCP version of this images do NOT have to be the same as the OCP version that the job is being tested against. We only care about the `Go` version in these images. But we DO have to use the same image between the jobs of the same branch, for consistency.

**TODO automate this process**

### Update OCP version

To update an OCP version in a branch:
- Rename the oldest OCP version config file to the new version and change the occurrences in the file, and update jobs file.
- Update links in README file.

Example: update `4.11` to `4.14` in master branch.

`openshift-oadp-operator-master__4.11.yaml` needs to be renamed `openshift-oadp-operator-master__4.14.yaml`, in `ci-operator/config/openshift/oadp-operator/` folder of https://github.com/openshift/release repo.

`4.11` occurrences in the file need to be updated to `4.14`.
```diff
...
 releases:
   latest:
     release:
       channel: fast
-      version: "4.11"
+      version: "4.14"
 resources:
   '*':
     limits:
...
   branch: master
   org: openshift
   repo: oadp-operator
-  variant: "4.11"
+  variant: "4.14"
```

After that, run `make jobs` to update job files in `ci-operator/jobs/openshift/oadp-operator` folder of https://github.com/openshift/release repo.

`4.11` occurrences in README.md need to be updated to `4.14` in master branch of https://github.com/openshift/oadp-operator repo.

```diff
-4.11, 4.12, 4.13 Periodic E2E Tests
+4.12, 4.13, 4.14 Periodic E2E Tests

 AWS :
-[![AWS builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.11-e2e-test-aws-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.11-e2e-test-aws-periodic)
 [![AWS builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.12-e2e-test-aws-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.12-e2e-test-aws-periodic)
 [![AWS builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.13-e2e-test-aws-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.13-e2e-test-aws-periodic)
+[![AWS builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.14-e2e-test-aws-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.14-e2e-test-aws-periodic)

 GCP:
-[![GCP builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.11-e2e-test-gcp-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.11-e2e-test-gcp-periodic)
 [![GCP builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.12-e2e-test-gcp-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.12-e2e-test-gcp-periodic)
 [![GCP builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.13-e2e-test-gcp-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.13-e2e-test-gcp-periodic)
+[![GCP builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.14-e2e-test-gcp-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.14-e2e-test-gcp-periodic)

 Azure:
-[![Azure builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.11-e2e-test-azure-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.11-e2e-test-azure-periodic)
 [![Azure builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.12-e2e-test-azure-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.12-e2e-test-azure-periodic)
 [![Azure builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.13-e2e-test-azure-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.13-e2e-test-azure-periodic)
+[![Azure builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.14-e2e-test-azure-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.14-e2e-test-azure-periodic)
```

**TODO automate this process**
