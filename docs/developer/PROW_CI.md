## PROW CI

The project uses [PROW CI](https://docs.ci.openshift.org/docs/) as a Continuous Integration/Delivery tool.

The configuration for OADP repo can be found in
- https://github.com/openshift/release/tree/master/core-services/prow/02_config/openshift/oadp-operator
- https://github.com/openshift/release/tree/master/ci-operator/config/openshift/oadp-operator
- https://github.com/openshift/release/tree/master/ci-operator/jobs/openshift/oadp-operator
- https://github.com/openshift/release/tree/master/core-services/image-mirroring/konveyor

The jobs run can be seen in PRs and in the links in the README.md file.

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
- ename the oldest OCP version config file to the new version and change the occurrences in the file, and update jobs file.
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
-[![AWS builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.11-operator-e2e-aws-periodic-slack)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.11-operator-e2e-aws-periodic-slack)
 [![AWS builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.12-operator-e2e-aws-periodic-slack)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.12-operator-e2e-aws-periodic-slack)
 [![AWS builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.13-operator-e2e-aws-periodic-slack)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.13-operator-e2e-aws-periodic-slack)
+[![AWS builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.14-operator-e2e-aws-periodic-slack)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.14-operator-e2e-aws-periodic-slack)

 GCP:
-[![GCP builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.11-operator-e2e-gcp-periodic-slack)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.11-operator-e2e-gcp-periodic-slack)
 [![GCP builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.12-operator-e2e-gcp-periodic-slack)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.12-operator-e2e-gcp-periodic-slack)
 [![GCP builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.13-operator-e2e-gcp-periodic-slack)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.13-operator-e2e-gcp-periodic-slack)
+[![GCP builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.14-operator-e2e-gcp-periodic-slack)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.14-operator-e2e-gcp-periodic-slack)

 Azure:
-[![Azure builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.11-operator-e2e-azure-periodic-slack)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.11-operator-e2e-azure-periodic-slack)
 [![Azure builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.12-operator-e2e-azure-periodic-slack)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.12-operator-e2e-azure-periodic-slack)
 [![Azure builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.13-operator-e2e-azure-periodic-slack)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.13-operator-e2e-azure-periodic-slack)
+[![Azure builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.14-operator-e2e-azure-periodic-slack)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.14-operator-e2e-azure-periodic-slack)
```

**TODO automate this process**
