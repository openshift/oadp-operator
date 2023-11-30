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

The configuration for [OADP's Velero plugin for CSI repo](https://github.com/openshift/velero-plugin-for-csi) can be found in (OADP < 1.4)
- https://github.com/openshift/release/tree/master/core-services/prow/02_config/openshift/velero-plugin-for-csi
- https://github.com/openshift/release/tree/master/ci-operator/config/openshift/velero-plugin-for-csi
- https://github.com/openshift/release/tree/master/ci-operator/jobs/openshift/velero-plugin-for-csi

The configuration for [OADP's restic repo](https://github.com/openshift/restic) can be found in (OADP < 1.3)
- https://github.com/openshift/release/tree/master/core-services/prow/02_config/openshift/restic
- https://github.com/openshift/release/tree/master/ci-operator/config/openshift/restic
- https://github.com/openshift/release/tree/master/ci-operator/jobs/openshift/restic

The configuration for [Non Admin repo](https://github.com/migtools/oadp-non-admin) can be found in (OADP >= 1.5)
- https://github.com/openshift/release/tree/master/core-services/prow/02_config/migtools/oadp-non-admin
- https://github.com/openshift/release/tree/master/ci-operator/config/migtools/oadp-non-admin
- https://github.com/openshift/release/tree/master/ci-operator/jobs/migtools/oadp-non-admin

The images mirroring to [quay.io](https://quay.io/organization/konveyor) configuration for all OADP related images can be found in
- https://github.com/openshift/release/tree/master/core-services/image-mirroring/konveyor

The jobs run can be seen in PRs and in the links in the README.md file of OADP repo.

OADP operator master branch is tested against the last 3 minor OCP releases. To Update an OCP version the project is tested against, see [Update OCP version](#update-ocp-version).

### Flakes

TODO link to wiki

### Creating new release branch

To create new OADP release branch:
- A new branch (following the pattern `oadp-major.minor`, [example](https://github.com/openshift/oadp-operator/tree/oadp-1.3)) must be created in each one of related repos of OADP, and OADP repo itself.
- The new OADP branch must be updated to point to new release.
- CI files for each new repo branch, must be created.

Example: create `oadp-1.3` branch.

OADP repo `Makefile` in `oadp-1.3` branch must be updated:
```diff
...
-DEFAULT_VERSION := 99.0.0
+DEFAULT_VERSION := 1.3.0
...
-CHANNELS = "stable"
+CHANNELS = "stable-1.3"
...
-DEFAULT_CHANNEL = "stable"
+DEFAULT_CHANNEL = "stable-1.3"
...
-IMG ?= quay.io/konveyor/oadp-operator:latest
+IMG ?= quay.io/konveyor/oadp-operator:oadp-1.3
...
 # A valid Git branch from https://github.com/openshift/oadp-operator
-PREVIOUS_CHANNEL ?= oadp-1.2
+PREVIOUS_CHANNEL ?= oadp-1.3
...
```

OADP repo `config/manifests/bases/oadp-operator.clusterserviceversion.yaml` in `oadp-1.3` branch must be updated:
```diff
...
-    containerImage: quay.io/konveyor/oadp-operator:latest
+    containerImage: quay.io/konveyor/oadp-operator:oadp-1.3
...
-    olm.skipRange: '>=0.0.0 <99.0.0'
+    olm.skipRange: '>=0.0.0 <1.3.0'
...
-  name: oadp-operator.v99.0.0
+  name: oadp-operator.v1.3.0
...
-  version: 99.0.0
+  version: 1.3.0
```

OADP repo `config/manager/manager.yaml` in `oadp-1.3` branch must be updated:
```diff
...
           - name: RELATED_IMAGE_VELERO
-            value: quay.io/konveyor/velero:latest
+            value: quay.io/konveyor/velero:oadp-1.3
           - name: RELATED_IMAGE_VELERO_RESTORE_HELPER
-            value: quay.io/konveyor/velero-restore-helper:latest
+            value: quay.io/konveyor/velero-restore-helper:oadp-1.3
           - name: RELATED_IMAGE_OPENSHIFT_VELERO_PLUGIN
-            value: quay.io/konveyor/openshift-velero-plugin:latest
+            value: quay.io/konveyor/openshift-velero-plugin:oadp-1.3
           - name: RELATED_IMAGE_VELERO_PLUGIN_FOR_AWS
-            value: quay.io/konveyor/velero-plugin-for-aws:latest
+            value: quay.io/konveyor/velero-plugin-for-aws:oadp-1.3
           - name: RELATED_IMAGE_VELERO_PLUGIN_FOR_MICROSOFT_AZURE
-            value: quay.io/konveyor/velero-plugin-for-microsoft-azure:latest
+            value: quay.io/konveyor/velero-plugin-for-microsoft-azure:oadp-1.3
           - name: RELATED_IMAGE_VELERO_PLUGIN_FOR_GCP
-            value: quay.io/konveyor/velero-plugin-for-gcp:latest
+            value: quay.io/konveyor/velero-plugin-for-gcp:oadp-1.3
           - name: RELATED_IMAGE_VELERO_PLUGIN_FOR_CSI
-            value: quay.io/konveyor/velero-plugin-for-csi:latest
+            value: quay.io/konveyor/velero-plugin-for-csi:oadp-1.3
...
```

After updating these files, run `make bundle`.

OADP repo `README.md` in `oadp-1.3` branch must be updated:
```diff
-Periodic Unit Tests [![Unit tests](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-unit-test-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-unit-test-periodic)
+Periodic Unit Tests [![Unit tests](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-oadp-1.3-unit-test-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-oadp-1.3-unit-test-periodic)
...
 AWS :
-[![AWS builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.12-e2e-test-aws-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.12-e2e-test-aws-periodic)
+[![AWS builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-oadp-1.3-4.12-e2e-test-aws-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-oadp-1.3-4.12-e2e-test-aws-periodic)
-[![AWS builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.13-e2e-test-aws-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.13-e2e-test-aws-periodic)
+[![AWS builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-oadp-1.3-4.13-e2e-test-aws-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-oadp-1.3-4.13-e2e-test-aws-periodic)
-[![AWS builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.14-e2e-test-aws-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.14-e2e-test-aws-periodic)
+[![AWS builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-oadp-1.3-4.14-e2e-test-aws-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-oadp-1.3-4.14-e2e-test-aws-periodic)

 GCP:
-[![GCP builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.12-e2e-test-gcp-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.12-e2e-test-gcp-periodic)
+[![GCP builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-oadp-1.3-4.12-e2e-test-gcp-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-oadp-1.3-4.12-e2e-test-gcp-periodic)
-[![GCP builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.13-e2e-test-gcp-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.13-e2e-test-gcp-periodic)
+[![GCP builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-oadp-1.3-4.13-e2e-test-gcp-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-oadp-1.3-4.13-e2e-test-gcp-periodic)
-[![GCP builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.14-e2e-test-gcp-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.14-e2e-test-gcp-periodic)
+[![GCP builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-oadp-1.3-4.14-e2e-test-gcp-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-oadp-1.3-4.14-e2e-test-gcp-periodic)

 Azure:
-[![Azure builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.12-e2e-test-azure-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.12-e2e-test-azure-periodic)
+[![Azure builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-oadp-1.3-4.12-e2e-test-azure-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-oadp-1.3-4.12-e2e-test-azure-periodic)
-[![Azure builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.13-e2e-test-azure-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.13-e2e-test-azure-periodic)
+[![Azure builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-oadp-1.3-4.13-e2e-test-azure-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-oadp-1.3-4.13-e2e-test-azure-periodic)
-[![Azure builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.14-e2e-test-azure-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.14-e2e-test-azure-periodic)
+[![Azure builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-oadp-1.3-4.14-e2e-test-azure-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-oadp-1.3-4.14-e2e-test-azure-periodic)
```

Also, new configs must be added to each new branch of related OADP repos in https://github.com/openshift/release repo. As an example, here are the changes needed for OADP repo CI.

The new branch must be added to `core-services/prow/02_config/openshift/oadp-operator/_prowconfig.yaml`
```diff
...
   - includedBranches:
     - master
     - oadp-1.0
     - oadp-1.1
     - oadp-1.2
+    - oadp-1.3
     labels:
...
```

`ci-operator/config/openshift/oadp-operator/openshift-oadp-operator-oadp-1.3.yaml` file must be created. To make it easier, copy the contents of `ci-operator/config/openshift/oadp-operator/openshift-oadp-operator-master.yaml`, changing the following.
```diff
...
 images:
 - dockerfile_path: Dockerfile
   from: src
-  to: oadp-operator
+  to: oadp-operator-1.3
 promotion:
...
 zz_generated_metadata:
-  branch: master
+  branch: oadp-1.3
   org: openshift
   repo: oadp-operator
```

`ci-operator/config/openshift/oadp-operator/openshift-oadp-operator-oadp-1.3__4.VERSION.yaml` files must be created. To make it easier, copy the contents of `ci-operator/config/openshift/oadp-operator/openshift-oadp-operator-master__4.14.VERSION`, changing the following.
```diff
...
 images:
 - dockerfile_path: Dockerfile
   from: src
-  to: oadp-operator
+  to: oadp-operator-1.3
 - dockerfile_path: build/ci-Dockerfile
   from: src
-  to: test-oadp-operator
+  to: test-oadp-operator-1.3
 operator:
   bundles:
   - dockerfile_path: build/Dockerfile.bundle
   substitutions:
-  - pullspec: quay.io/konveyor/oadp-operator:latest
-    with: oadp-operator
+  - pullspec: quay.io/konveyor/oadp-operator:oadp-1.3
+    with: oadp-operator-1.3
 releases:
...
     env:
-      OO_CHANNEL: stable
+      OO_CHANNEL: stable-1.3
       OO_INSTALL_NAMESPACE: openshift-adp
...
         namespace: test-credentials
-      from: test-oadp-operator
+      from: test-oadp-operator-1.3
       resources:
...
     env:
-      OO_CHANNEL: stable
+      OO_CHANNEL: stable-1.3
       OO_INSTALL_NAMESPACE: openshift-adp
...
         namespace: test-credentials
-      from: test-oadp-operator
+      from: test-oadp-operator-1.3
       resources:
...
     env:
-      OO_CHANNEL: stable
+      OO_CHANNEL: stable-1.3
       OO_INSTALL_NAMESPACE: openshift-adp
...
         namespace: test-credentials
-      from: test-oadp-operator
+      from: test-oadp-operator-1.3
       resources:
...
     env:
-      OO_CHANNEL: stable
+      OO_CHANNEL: stable-1.3
       OO_INSTALL_NAMESPACE: openshift-adp
...
         namespace: test-credentials
-      from: test-oadp-operator
+      from: test-oadp-operator-1.3
       resources:
...
     env:
-      OO_CHANNEL: stable
+      OO_CHANNEL: stable-1.3
       OO_INSTALL_NAMESPACE: openshift-adp
...
         namespace: test-credentials
-      from: test-oadp-operator
+      from: test-oadp-operator-1.3
       resources:
...
     env:
-      OO_CHANNEL: stable
+      OO_CHANNEL: stable-1.3
       OO_INSTALL_NAMESPACE: openshift-adp
...
         namespace: test-credentials
-      from: test-oadp-operator
+      from: test-oadp-operator-1.3
       resources:
         requests:
           cpu: 1000m
           memory: 512Mi
     workflow: optional-operators-ci-azure
 zz_generated_metadata:
-  branch: master
+  branch: oadp-1.3
   org: openshift
   repo: oadp-operator
   variant: "4.14"
```

After creating these files, run `make jobs`.

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
