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

The configuration for [OADP's Velero plugin for Legacy AWS repo](https://github.com/openshift/velero-plugin-for-legacy-aws) can be found in
- https://github.com/openshift/release/tree/master/core-services/prow/02_config/openshift/velero-plugin-for-legacy-aws
- https://github.com/openshift/release/tree/master/ci-operator/config/openshift/velero-plugin-for-legacy-aws
- https://github.com/openshift/release/tree/master/ci-operator/jobs/openshift/velero-plugin-for-legacy-aws

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

> **Note**: To avoid changing upstream `OWNERS` files on `openshift` organization forks, we use `DOWNSTREAM_OWNERS` files on those repos. Reference https://github.com/openshift/release/blob/dd6b8b25a85bfd92ca74fdf1435ee9f21cd22516/core-services/prow/02_config/_plugins.yaml#L664-L678

### Creating new release branches

To create new OADP release branches (they must follow the pattern `oadp-major.minor`, [example](https://github.com/openshift/oadp-operator/tree/oadp-1.4)):
- First, create CI files for each new repository branch.
- Create the release branches in each one of related repositories of OADP, and OADP operator repository itself.
- The new release branches must be updated:
  - Downstream repositories release branches may not need updates (except OADP operator, which always need updates).
  - Upstream repositories release branches need to be rebased.

> **Note**: Try to always create release branches from default branches (usually called master).

> **Note**: Documentation should live only in default branches. For example, for OADP operator, `docs/` and `blogs/` folders can be deleted in release branches.

**TODO automate this process**

#### Example: create CI files for OADP operator repository

CI files for OADP operator (and all of its related repositories) live in https://github.com/openshift/release repository.

For this example, lets say new release branch is `oadp-1.4`.

The new release branch must be added to `core-services/prow/02_config/openshift/oadp-operator/_prowconfig.yaml` changing the following.
```diff
...
   - includedBranches:
     - master
     - oadp-1.0
     - oadp-1.1
     - oadp-1.2
     - oadp-1.3
+    - oadp-1.4
     labels:
...
```

Create `ci-operator/config/openshift/oadp-operator/openshift-oadp-operator-oadp-1.4.yaml`. To make it easier, copy the contents of `ci-operator/config/openshift/oadp-operator/openshift-oadp-operator-master.yaml` (if release branch was created from master branch), changing the following.
```diff
...
 images:
 - dockerfile_path: Dockerfile
   from: src
-  to: oadp-operator
+  to: oadp-operator-1.4
 promotion:
...
 zz_generated_metadata:
-  branch: master
+  branch: oadp-1.4
   org: openshift
   repo: oadp-operator
```

> **Note**: to get diff between files, you can run `diff -ruN ci-operator/config/openshift/oadp-operator/openshift-oadp-operator-master.yaml ci-operator/config/openshift/oadp-operator/openshift-oadp-operator-oadp-1.4.yaml`.

Create `ci-operator/config/openshift/oadp-operator/openshift-oadp-operator-oadp-1.4__4.VERSION.yaml` files. To make it easier, copy the contents of `ci-operator/config/openshift/oadp-operator/openshift-oadp-operator-master__4.VERSION.yaml` files (if release branch was created from master branch), changing the following.
```diff
...
 images:
 - dockerfile_path: Dockerfile
   from: src
-  to: oadp-operator
+  to: oadp-operator-1.4
 - dockerfile_path: hack/ci.Dockerfile
   from: src
-  to: test-oadp-operator
+  to: test-oadp-operator-1.4
 operator:
   bundles:
   - dockerfile_path: bundle.Dockerfile
   substitutions:
-  - pullspec: quay.io/konveyor/oadp-operator:latest
-    with: oadp-operator
+  - pullspec: quay.io/konveyor/oadp-operator:oadp-1.4
+    with: oadp-operator-1.4
 releases:
...
     env:
-      OO_CHANNEL: stable
+      OO_CHANNEL: stable-1.4
       OO_INSTALL_NAMESPACE: openshift-adp
...
         namespace: test-credentials
-      from: test-oadp-operator
+      from: test-oadp-operator-1.4
       resources:
...
     env:
-      OO_CHANNEL: stable
+      OO_CHANNEL: stable-1.4
       OO_INSTALL_NAMESPACE: openshift-adp
...
         namespace: test-credentials
-      from: test-oadp-operator
+      from: test-oadp-operator-1.4
       resources:
...
     env:
-      OO_CHANNEL: stable
+      OO_CHANNEL: stable-1.4
       OO_INSTALL_NAMESPACE: openshift-adp
...
         namespace: test-credentials
-      from: test-oadp-operator
+      from: test-oadp-operator-1.4
       resources:
...
 zz_generated_metadata:
-  branch: master
+  branch: oadp-1.4
   org: openshift
   repo: oadp-operator
...
```

> **Note**: to get diff between files, you can run `diff -ruN diff -ruN ci-operator/config/openshift/oadp-operator/openshift-oadp-operator-master__4.VERSION.yaml ci-operator/config/openshift/oadp-operator/openshift-oadp-operator-oadp-1.4__4.VERSION.yaml`.

After creating these files, run `make jobs`.

Finally, add image to `core-services/image-mirroring/konveyor/mapping_konveyor_latest`
```diff
...
 registry.ci.openshift.org/konveyor/oadp-operator:oadp-operator-1.3 quay.io/konveyor/oadp-operator:oadp-1.3-latest quay.io/konveyor/oadp-operator:oadp-1.3-amd64 quay.io/konveyor/oadp-operator:oadp-1.3
+registry.ci.openshift.org/konveyor/oadp-operator:oadp-operator-1.4 quay.io/konveyor/oadp-operator:oadp-1.4-latest quay.io/konveyor/oadp-operator:oadp-1.4-amd64 quay.io/konveyor/oadp-operator:oadp-1.4
...
```

#### Example: updating release branch for OADP operator repository

For this example, lets say new release branch is `oadp-1.4`.

Update `Makefile`, changing the following (if release branch was created from master branch).
```diff
...
-DEFAULT_VERSION := 99.0.0
+DEFAULT_VERSION := 1.4.0
...
-CHANNELS = "stable"
+CHANNELS = "stable-1.4"
...
-DEFAULT_CHANNEL = "stable"
+DEFAULT_CHANNEL = "stable-1.4"
...
-IMG ?= quay.io/konveyor/oadp-operator:latest
+IMG ?= quay.io/konveyor/oadp-operator:oadp-1.4
...
```

> **Note**: after creating release branch, update `Makefile` in master branch, changing the following.
>```diff
> # A valid Git branch from https://github.com/openshift/oadp-operator
>-PREVIOUS_CHANNEL ?= oadp-1.3
>+PREVIOUS_CHANNEL ?= oadp-1.4
> # Go version in go.mod in that branch
>-PREVIOUS_CHANNEL_GO_VERSION ?= 1.20
>+PREVIOUS_CHANNEL_GO_VERSION ?= 1.22
>...
>```
>`PREVIOUS_CHANNEL` points to new branch and `PREVIOUS_CHANNEL_GO_VERSION` points to new branch Go version (in `go.mod`).
>
>Upgrade E2E tests must also be updated.

> **Note**: to get diff between files, you can run `git diff master oadp-1.4 Makefile`.

Update `config/manifests/bases/oadp-operator.clusterserviceversion.yaml`, changing the following (if release branch was created from master branch).
```diff
...
-    containerImage: quay.io/konveyor/oadp-operator:latest
+    containerImage: quay.io/konveyor/oadp-operator:oadp-1.4
...
-    olm.skipRange: '>=0.0.0 <99.0.0'
+    olm.skipRange: '>=0.0.0 <1.4.0'
...
-  name: oadp-operator.v99.0.0
+  name: oadp-operator.v1.4.0
...
-  version: 99.0.0
+  version: 1.4.0
```

> **Note**: to get diff between files, you can run `git diff master oadp-1.4 config/manifests/bases/oadp-operator.clusterserviceversion.yaml`.

Update `config/manager/manager.yaml`, changing the following (if release branch was created from master branch).
```diff
...
           - name: RELATED_IMAGE_VELERO
-            value: quay.io/konveyor/velero:latest
+            value: quay.io/konveyor/velero:oadp-1.4
           - name: RELATED_IMAGE_VELERO_RESTORE_HELPER
-            value: quay.io/konveyor/velero-restore-helper:latest
+            value: quay.io/konveyor/velero-restore-helper:oadp-1.4
           - name: RELATED_IMAGE_OPENSHIFT_VELERO_PLUGIN
-            value: quay.io/konveyor/openshift-velero-plugin:latest
+            value: quay.io/konveyor/openshift-velero-plugin:oadp-1.4
           - name: RELATED_IMAGE_VELERO_PLUGIN_FOR_AWS
-            value: quay.io/konveyor/velero-plugin-for-aws:latest
+            value: quay.io/konveyor/velero-plugin-for-aws:oadp-1.4
           - name: RELATED_IMAGE_VELERO_PLUGIN_FOR_MICROSOFT_AZURE
-            value: quay.io/konveyor/velero-plugin-for-microsoft-azure:latest
+            value: quay.io/konveyor/velero-plugin-for-microsoft-azure:oadp-1.4
           - name: RELATED_IMAGE_VELERO_PLUGIN_FOR_GCP
-            value: quay.io/konveyor/velero-plugin-for-gcp:latest
+            value: quay.io/konveyor/velero-plugin-for-gcp:oadp-1.4
...
```

> **Note**: to get diff between files, you can run `git diff master oadp-1.4 config/manager/manager.yaml`.

After updating these files, run `make bundle`.

Update `pkg/common/common.go`, changing the following (if release branch was created from master branch).
```diff
...
 // Images
 const (
-	VeleroImage          = "quay.io/konveyor/velero:latest"
+	VeleroImage          = "quay.io/konveyor/velero:oadp-1.4"
-	OpenshiftPluginImage = "quay.io/konveyor/openshift-velero-plugin:latest"
+	OpenshiftPluginImage = "quay.io/konveyor/openshift-velero-plugin:oadp-1.4"
-	AWSPluginImage       = "quay.io/konveyor/velero-plugin-for-aws:latest"
+	AWSPluginImage       = "quay.io/konveyor/velero-plugin-for-aws:oadp-1.4"
-	AzurePluginImage     = "quay.io/konveyor/velero-plugin-for-microsoft-azure:latest"
+	AzurePluginImage     = "quay.io/konveyor/velero-plugin-for-microsoft-azure:oadp-1.4"
-	GCPPluginImage       = "quay.io/konveyor/velero-plugin-for-gcp:latest"
+	GCPPluginImage       = "quay.io/konveyor/velero-plugin-for-gcp:oadp-1.4"
	RegistryImage        = "quay.io/konveyor/registry:latest"
	KubeVirtPluginImage  = "quay.io/konveyor/kubevirt-velero-plugin:v0.7.0"
 )
...
```

Update `controllers/nonadmin_controller.go`, changing the following (if release branch was created from master branch).
```diff
...
 	// TODO https://github.com/openshift/oadp-operator/issues/1379
-	return "quay.io/konveyor/oadp-non-admin:latest"
+	return "quay.io/konveyor/oadp-non-admin:oadp-1.4"
 }
...
```

Update `tests/e2e/lib/apps.go`, changing the following (if release branch was created from master branch).
```diff
...
 	mustGatherCmd := "must-gather"
-	mustGatherImg := "--image=quay.io/konveyor/oadp-must-gather:latest"
+	mustGatherImg := "--image=quay.io/konveyor/oadp-must-gather:oadp-1.4"
	destDir := "--dest-dir=" + artifact_dir
...
```

Update `README.md`, changing the following (if release branch was created from master branch).
```diff
...
-  [![Go Report Card](https://goreportcard.com/badge/github.com/openshift/oadp-operator)](https://goreportcard.com/report/github.com/openshift/oadp-operator) [![codecov](https://codecov.io/gh/openshift/oadp-operator/branch/master/graph/badge.svg?token=qLM0hAzjpD)](https://codecov.io/gh/openshift/oadp-operator) [![License](https://img.shields.io/:license-apache-blue.svg)](https://www.apache.org/licenses/LICENSE-2.0.html) [![Go Reference](https://pkg.go.dev/badge/github.com/openshift/oadp-operator.svg)](https://pkg.go.dev/github.com/openshift/oadp-operator)
+  [![Go Report Card](https://goreportcard.com/badge/github.com/openshift/oadp-operator)](https://goreportcard.com/report/github.com/openshift/oadp-operator) [![codecov](https://codecov.io/gh/openshift/oadp-operator/branch/oadp-1.4/graph/badge.svg?token=qLM0hAzjpD)](https://codecov.io/gh/openshift/oadp-operator) [![License](https://img.shields.io/:license-apache-blue.svg)](https://www.apache.org/licenses/LICENSE-2.0.html) [![Go Reference](https://pkg.go.dev/badge/github.com/openshift/oadp-operator.svg)](https://pkg.go.dev/github.com/openshift/oadp-operator)
...
-Periodic Unit Tests [![Unit tests](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-unit-test-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-unit-test-periodic)
+Periodic Unit Tests [![Unit tests](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-oadp-1.4-unit-test-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-oadp-1.4-unit-test-periodic)
...
 AWS :
-[![AWS builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.12-e2e-test-aws-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.12-e2e-test-aws-periodic)
+[![AWS builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-oadp-1.4-4.12-e2e-test-aws-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-oadp-1.4-4.12-e2e-test-aws-periodic)
-[![AWS builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.13-e2e-test-aws-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.13-e2e-test-aws-periodic)
+[![AWS builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-oadp-1.4-4.13-e2e-test-aws-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-oadp-1.4-4.13-e2e-test-aws-periodic)
-[![AWS builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.14-e2e-test-aws-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.14-e2e-test-aws-periodic)
+[![AWS builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-oadp-1.4-4.14-e2e-test-aws-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-oadp-1.4-4.14-e2e-test-aws-periodic)
...
-Documentation in this repository are considered unofficial and for development purposes only.
+Development documentation of this repository can be found in [master branch](https://github.com/openshift/oadp-operator).
# delete everything after this line
```

> **Note**: to get diff between files, you can run `git diff master oadp-1.4 README.md`.

### Update Go version

To update the Go version used in CI jobs of a branch:
- Update the image used in `build_root` instruction of ALL jobs of that branch.

> **Note:** images referenced in `build_root` instruction ([which creates `root` image, that is tagged into `src`](https://docs.ci.openshift.org/docs/internals/)) of the config files, are defined in [here](https://github.com/openshift-eng/ocp-build-data/tree/main), under the specific branch. For example, `openshift-4.14` images are defined [here](https://github.com/openshift-eng/ocp-build-data/blob/openshift-4.14/streams.yml). The OCP version of this images do NOT have to be the same as the OCP version that the job is being tested against. We only care about the `Go` version in these images. But we DO have to use the same image between the jobs of the same branch, for consistency.

**TODO automate this process**

### Update OCP version

To update an OCP version in a branch:
- Rename the oldest OCP version config file to the new version and change the occurrences in the file, and update jobs file.
- Update links in README file.
- Update envtest Kubernetes version in Makefile.

Example: update `4.13` to `4.16` in master branch.

`openshift-oadp-operator-master__4.13.yaml` needs to be renamed `openshift-oadp-operator-master__4.16.yaml`, in `ci-operator/config/openshift/oadp-operator/` folder of https://github.com/openshift/release repo.

`4.13` occurrences in the file need to be updated to `4.16`.
```diff
...
 releases:
   latest:
     release:
       channel: fast
-      version: "4.13"
+      version: "4.16"
 resources:
   '*':
     limits:
...
   branch: master
   org: openshift
   repo: oadp-operator
-  variant: "4.13"
+  variant: "4.16"
```

> **Note**: if OCP version is not in https://openshift-release.apps.ci.l2s4.p1.openshiftapps.com/#4-stable, you may also need to change
> ```diff
> ...
>  releases:
>    latest:
> -    release:
> -      channel: fast
> -      version: "4.13"
> +    candidate:
> +      product: ocp
> +      stream: nightly
> +      version: "4.16"
> ...
> ```

After that, run `make jobs` to update job files in `ci-operator/jobs/openshift/oadp-operator` folder of https://github.com/openshift/release repo.

`4.13` occurrences in README.md need to be updated to `4.16` in master branch of https://github.com/openshift/oadp-operator repo.

```diff
-4.13, 4.14, 4.15 Periodic E2E Tests
+4.14, 4.15, 4.16 Periodic E2E Tests

 AWS :
-[![AWS builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.13-e2e-test-aws-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.13-e2e-test-aws-periodic)
 [![AWS builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.14-e2e-test-aws-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.14-e2e-test-aws-periodic)
 [![AWS builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.15-e2e-test-aws-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.15-e2e-test-aws-periodic)
+[![AWS builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.16-e2e-test-aws-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.16-e2e-test-aws-periodic)
```

Finally, update envtest version in Makefile needs to be updated to point to Kubernetes version of OCP `4.16` in master branch of https://github.com/openshift/oadp-operator repo.

```diff
-# Kubernetes version from OpenShift 4.15.x https://openshift-release.apps.ci.l2s4.p1.openshiftapps.com/#4-stable
+# Kubernetes version from OpenShift 4.16.x https://openshift-release.apps.ci.l2s4.p1.openshiftapps.com/#4-stable
 # ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
-ENVTEST_K8S_VERSION = 1.28
+ENVTEST_K8S_VERSION = 1.29
```

**TODO automate this process**
