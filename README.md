<div align="center">
  <h1> OADP Operator </h1>
  <p>  OpenShift API for Data Protection </p>

  [![Go Report Card](https://goreportcard.com/badge/github.com/openshift/oadp-operator)](https://goreportcard.com/report/github.com/openshift/oadp-operator) [![codecov](https://codecov.io/gh/openshift/oadp-operator/branch/oadp-dev/graph/badge.svg?token=qLM0hAzjpD)](https://codecov.io/gh/openshift/oadp-operator) [![License](https://img.shields.io/:license-apache-blue.svg)](https://www.apache.org/licenses/LICENSE-2.0.html) [![Go Reference](https://pkg.go.dev/badge/github.com/openshift/oadp-operator.svg)](https://pkg.go.dev/github.com/openshift/oadp-operator)
</div>

## Periodic Unit Tests
[![Unit tests](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-oadp-dev-unit-test-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-oadp-dev-unit-test-periodic)

## Periodic E2E Tests in OpenShift

> **Status and known CI issues:**
> See the [CI Status & Issues Wiki](https://github.com/openshift/oadp-operator/wiki#current-ci-status) for up-to-date CI status, known problems, and troubleshooting guidance.


### Periodic AWS E2E Tests in OpenShift
See the wiki: [Current CI Status](https://github.com/openshift/oadp-operator/wiki#current-ci-status)


## OADP repositories images job
| OADP | OpenShift Velero plugin | Velero | Velero plugin for AWS | Velero plugin for Legacy AWS | Velero plugin for GCP | Velero plugin for Microsoft Azure | Non Admin | KubeVirt Velero Plugin | Must Gather | CLI | KubeVirt Datamover Controller | VM File Restore | Filebrowser | VMDP | KubeVirt Datamover Plugin |
| ---------- | ---------- | ---------- | ---------- | ---------- | ---------- | ---------- | ---------- | ---------- | ---------- | ---------- | ---------- | ---------- | ---------- | ---------- | ---------- |
| [![OADP repository](https://prow.ci.openshift.org/badge.svg?jobs=branch-ci-openshift-oadp-operator-oadp-dev-images)](https://prow.ci.openshift.org/job-history/gs/test-platform-results/logs/branch-ci-openshift-oadp-operator-oadp-dev-images) | [![OpenShift Velero plugin repository](https://prow.ci.openshift.org/badge.svg?jobs=branch-ci-openshift-openshift-velero-plugin-oadp-dev-images)](https://prow.ci.openshift.org/job-history/gs/test-platform-results/logs/branch-ci-openshift-openshift-velero-plugin-oadp-dev-images) | [![OADP's Velero repository](https://prow.ci.openshift.org/badge.svg?jobs=branch-ci-openshift-velero-oadp-dev-images)](https://prow.ci.openshift.org/job-history/gs/test-platform-results/logs/branch-ci-openshift-velero-oadp-dev-images) | [![OADP's Velero plugin for AWS repository](https://prow.ci.openshift.org/badge.svg?jobs=branch-ci-openshift-velero-plugin-for-aws-oadp-dev-images)](https://prow.ci.openshift.org/job-history/gs/test-platform-results/logs/branch-ci-openshift-velero-plugin-for-aws-oadp-dev-images) | [![OADP's Velero plugin for Legacy AWS repository](https://prow.ci.openshift.org/badge.svg?jobs=branch-ci-openshift-velero-plugin-for-legacy-aws-oadp-dev-images)](https://prow.ci.openshift.org/job-history/gs/test-platform-results/logs/branch-ci-openshift-velero-plugin-for-legacy-aws-oadp-dev-images) | [![OADP's Velero plugin for GCP repository](https://prow.ci.openshift.org/badge.svg?jobs=branch-ci-openshift-velero-plugin-for-gcp-oadp-dev-images)](https://prow.ci.openshift.org/job-history/gs/test-platform-results/logs/branch-ci-openshift-velero-plugin-for-gcp-oadp-dev-images) | [![OADP's Velero plugin for Microsoft Azure repository](https://prow.ci.openshift.org/badge.svg?jobs=branch-ci-openshift-velero-plugin-for-microsoft-azure-oadp-dev-images)](https://prow.ci.openshift.org/job-history/gs/test-platform-results/logs/branch-ci-openshift-velero-plugin-for-microsoft-azure-oadp-dev-images) | [![Non Admin repository](https://prow.ci.openshift.org/badge.svg?jobs=branch-ci-migtools-oadp-non-admin-oadp-dev-images)](https://prow.ci.openshift.org/job-history/gs/test-platform-results/logs/branch-ci-migtools-oadp-non-admin-oadp-dev-images) | [![KubeVirt Velero Plugin repository](https://prow.ci.openshift.org/badge.svg?jobs=branch-ci-migtools-kubevirt-velero-plugin-main-images)](https://prow.ci.openshift.org/job-history/gs/test-platform-results/logs/branch-ci-migtools-kubevirt-velero-plugin-main-images) | [![Must Gather repository](https://prow.ci.openshift.org/badge.svg?jobs=branch-ci-openshift-oadp-must-gather-oadp-dev-images)](https://prow.ci.openshift.org/job-history/gs/test-platform-results/logs/branch-ci-openshift-oadp-must-gather-oadp-dev-images) | [![CLI repository](https://prow.ci.openshift.org/badge.svg?jobs=branch-ci-migtools-oadp-cli-oadp-dev-images)](https://prow.ci.openshift.org/job-history/gs/test-platform-results/logs/branch-ci-migtools-oadp-cli-oadp-dev-images) | [![KubeVirt Datamover Controller repository](https://prow.ci.openshift.org/badge.svg?jobs=branch-ci-migtools-kubevirt-datamover-controller-oadp-dev-images)](https://prow.ci.openshift.org/job-history/gs/test-platform-results/logs/branch-ci-migtools-kubevirt-datamover-controller-oadp-dev-images) | [![VM File Restore repository](https://prow.ci.openshift.org/badge.svg?jobs=branch-ci-migtools-oadp-vm-file-restore-oadp-dev-images)](https://prow.ci.openshift.org/job-history/gs/test-platform-results/logs/branch-ci-migtools-oadp-vm-file-restore-oadp-dev-images) | [![Filebrowser repository](https://prow.ci.openshift.org/badge.svg?jobs=branch-ci-migtools-filebrowser-oadp-dev-images)](https://prow.ci.openshift.org/job-history/gs/test-platform-results/logs/branch-ci-migtools-filebrowser-oadp-dev-images) | [![VMDP repository](https://prow.ci.openshift.org/badge.svg?jobs=branch-ci-migtools-oadp-vmdp-oadp-dev-images)](https://prow.ci.openshift.org/job-history/gs/test-platform-results/logs/branch-ci-migtools-oadp-vmdp-oadp-dev-images) | [![KubeVirt Datamover Plugin repository](https://prow.ci.openshift.org/badge.svg?jobs=branch-ci-migtools-kubevirt-datamover-plugin-oadp-dev-images)](https://prow.ci.openshift.org/job-history/gs/test-platform-results/logs/branch-ci-migtools-kubevirt-datamover-plugin-oadp-dev-images) |

### Mirroring images to quay.io [![Mirror images](https://prow.ci.openshift.org/badge.svg?jobs=periodic-image-mirroring-konveyor)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-image-mirroring-konveyor)
</div>

## Rebase status from upstream Velero

* [OADP Rebase Repository](https://github.com/oadp-rebasebot/oadp-rebase)

### 🌊 Wave I - Independent Dependencies
| Component | oadp-dev | oadp-1.5 | main |
|-----------|----------|----------|------|
| [kopia](https://github.com/migtools/kopia/pulls?q=is%3Apr+%28is%3Aopen+OR+is%3Aclosed%29+in%3Atitle+%22Merge+https%3A%2F%2Fgithub.com%2F%22) | [![oadp-dev](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-eng-rebasebot-main-migtools-kopia-oadp-dev)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-eng-rebasebot-main-migtools-kopia-oadp-dev) | [![oadp-1.5](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-eng-rebasebot-main-migtools-kopia-oadp-1-5)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-eng-rebasebot-main-migtools-kopia-oadp-1-5) | |
| [kubevirt-velero-plugin](https://github.com/migtools/kubevirt-velero-plugin/pulls?q=is%3Apr+%28is%3Aopen+OR+is%3Aclosed%29+in%3Atitle+%22Merge+https%3A%2F%2Fgithub.com%2F%22) | | | [![main](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-eng-rebasebot-main-migtools-kubevirt-velero-plugin-main)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-eng-rebasebot-main-migtools-kubevirt-velero-plugin-main) |
| [restic](https://github.com/openshift/restic/pulls?q=is%3Apr+%28is%3Aopen+OR+is%3Aclosed%29+in%3Atitle+%22Merge+https%3A%2F%2Fgithub.com%2F%22) | ![Skipped](https://img.shields.io/badge/status-skipped-blue) | ![Skipped](https://img.shields.io/badge/status-skipped-blue) | |
| [udistribution](https://github.com/migtools/udistribution/pulls?q=is%3Apr+%28is%3Aopen+OR+is%3Aclosed%29+in%3Atitle+%22Merge+https%3A%2F%2Fgithub.com%2F%22) | | | [![main](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-eng-rebasebot-main-migtools-udistribution-main)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-eng-rebasebot-main-migtools-udistribution-main) |

### 🌊 Wave II - Velero Integration
| Component | oadp-dev | oadp-1.5 |
|-----------|----------|----------|
| [velero](https://github.com/openshift/velero/pulls?q=is%3Apr+%28is%3Aopen+OR+is%3Aclosed%29+in%3Atitle+%22Merge+https%3A%2F%2Fgithub.com%2F%22) | [![oadp-dev](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-eng-rebasebot-main-openshift-velero-oadp-dev)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-eng-rebasebot-main-openshift-velero-oadp-dev) | [![oadp-1.5](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-eng-rebasebot-main-openshift-velero-oadp-1-5)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-eng-rebasebot-main-openshift-velero-oadp-1-5) |

### 🌊 Wave III - Plugins and Operator
| Component | oadp-dev | oadp-1.5 |
|-----------|----------|----------|
| [oadp-operator](https://github.com/openshift/oadp-operator/pulls?q=is%3Apr+%28is%3Aopen+OR+is%3Aclosed%29+in%3Atitle+%22Merge+https%3A%2F%2Fgithub.com%2F%22) | [![oadp-dev](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-eng-rebasebot-main-openshift-oadp-operator-oadp-dev)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-eng-rebasebot-main-openshift-oadp-operator-oadp-dev) | [![oadp-1.5](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-eng-rebasebot-main-openshift-oadp-operator-oadp-1-5)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-eng-rebasebot-main-openshift-oadp-operator-oadp-1-5) |
| [velero-plugin-for-aws](https://github.com/openshift/velero-plugin-for-aws/pulls?q=is%3Apr+%28is%3Aopen+OR+is%3Aclosed%29+in%3Atitle+%22Merge+https%3A%2F%2Fgithub.com%2F%22) | [![oadp-dev](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-eng-rebasebot-main-openshift-velero-plugin-for-aws-oadp-dev)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-eng-rebasebot-main-openshift-velero-plugin-for-aws-oadp-dev) | [![oadp-1.5](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-eng-rebasebot-main-openshift-velero-plugin-for-aws-oadp-1-5)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-eng-rebasebot-main-openshift-velero-plugin-for-aws-oadp-1-5) |
| [velero-plugin-for-csi](https://github.com/openshift/velero-plugin-for-csi/pulls?q=is%3Apr+%28is%3Aopen+OR+is%3Aclosed%29+in%3Atitle+%22Merge+https%3A%2F%2Fgithub.com%2F%22) | ![Skipped](https://img.shields.io/badge/status-skipped-blue) | |
| [velero-plugin-for-gcp](https://github.com/openshift/velero-plugin-for-gcp/pulls?q=is%3Apr+%28is%3Aopen+OR+is%3Aclosed%29+in%3Atitle+%22Merge+https%3A%2F%2Fgithub.com%2F%22) | [![oadp-dev](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-eng-rebasebot-main-openshift-velero-plugin-for-gcp-oadp-dev)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-eng-rebasebot-main-openshift-velero-plugin-for-gcp-oadp-dev) | [![oadp-1.5](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-eng-rebasebot-main-openshift-velero-plugin-for-gcp-oadp-1-5)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-eng-rebasebot-main-openshift-velero-plugin-for-gcp-oadp-1-5) |
| [velero-plugin-for-legacy-aws](https://github.com/openshift/velero-plugin-for-legacy-aws/pulls?q=is%3Apr+%28is%3Aopen+OR+is%3Aclosed%29+in%3Atitle+%22Merge+https%3A%2F%2Fgithub.com%2F%22) | ![Skipped](https://img.shields.io/badge/status-skipped-blue) | ![Skipped](https://img.shields.io/badge/status-skipped-blue) |
| [velero-plugin-for-microsoft-azure](https://github.com/openshift/velero-plugin-for-microsoft-azure/pulls?q=is%3Apr+%28is%3Aopen+OR+is%3Aclosed%29+in%3Atitle+%22Merge+https%3A%2F%2Fgithub.com%2F%22) | [![oadp-dev](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-eng-rebasebot-main-openshift-velero-plugin-for-microsoft-azure-oadp-dev)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-eng-rebasebot-main-openshift-velero-plugin-for-microsoft-azure-oadp-dev) | [![oadp-1.5](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-eng-rebasebot-main-openshift-velero-plugin-for-microsoft-azure-oadp-1-5)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-eng-rebasebot-main-openshift-velero-plugin-for-microsoft-azure-oadp-1-5) |

### 🌊 Wave IV - Non-Admin Controller and OpenShift Plugin
| Component | oadp-dev | oadp-1.5 |
|-----------|----------|----------|
| [oadp-non-admin](https://github.com/migtools/oadp-non-admin/pulls?q=is%3Apr+%28is%3Aopen+OR+is%3Aclosed%29+in%3Atitle+%22Merge+https%3A%2F%2Fgithub.com%2F%22) | [![oadp-dev](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-eng-rebasebot-main-migtools-oadp-non-admin-oadp-dev)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-eng-rebasebot-main-migtools-oadp-non-admin-oadp-dev) | [![oadp-1.5](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-eng-rebasebot-main-migtools-oadp-non-admin-oadp-1-5)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-eng-rebasebot-main-migtools-oadp-non-admin-oadp-1-5) |
| [openshift-velero-plugin](https://github.com/openshift/openshift-velero-plugin/pulls?q=is%3Apr+%28is%3Aopen+OR+is%3Aclosed%29+in%3Atitle+%22Merge+https%3A%2F%2Fgithub.com%2F%22) | [![oadp-dev](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-eng-rebasebot-main-openshift-openshift-velero-plugin-oadp-dev)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-eng-rebasebot-main-openshift-openshift-velero-plugin-oadp-dev) | [![oadp-1.5](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-eng-rebasebot-main-openshift-openshift-velero-plugin-oadp-1-5)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-eng-rebasebot-main-openshift-openshift-velero-plugin-oadp-1-5) |

### 🌊 Wave V - OADP Must-Gather
| Component | oadp-dev | oadp-1.5 |
|-----------|----------|----------|
| [oadp-must-gather](https://github.com/openshift/oadp-must-gather/pulls?q=is%3Apr+%28is%3Aopen+OR+is%3Aclosed%29+in%3Atitle+%22Merge+https%3A%2F%2Fgithub.com%2F%22) | [![oadp-dev](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-eng-rebasebot-main-openshift-oadp-must-gather-oadp-dev)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-eng-rebasebot-main-openshift-oadp-must-gather-oadp-dev) | [![oadp-1.5](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-eng-rebasebot-main-openshift-oadp-must-gather-oadp-1-5)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-eng-rebasebot-main-openshift-oadp-must-gather-oadp-1-5) |

Note: Official Overview and documentation can be found in the [OpenShift Documentation](https://docs.openshift.com/container-platform/latest/backup_and_restore/application_backup_and_restore/oadp-intro.html)

Documentation in this repository are considered unofficial and for development purposes only.
# Table of Contents

1. [About](#about)
2. [Installing OADP](https://docs.openshift.com/container-platform/latest/backup_and_restore/application_backup_and_restore/installing/about-installing-oadp.html)
3. [API References](docs/API_ref.md)
4. API Usage
    1. [Configure Plugins](docs/config/plugins.md)
    2. [Backup Storage Locations and Volume Snapshot Locations](docs/config/bsl_and_vsl.md)
    3. [Resource Requests and Limits](docs/config/resource_req_limits.md)
    4. [Self-Signed Certificate](docs/config/self_signed_certs.md)
    5. [Use NooBaa as a Backup Storage Location](docs/config/noobaa/install_oadp_noobaa.md)
    6. [Use Velero --features flag](docs/config/features_flag.md)
    7. [Use Custom Plugin Images for Velero ](docs/config/custom_plugin_images.md)
    8. [Enable VM File Restore](docs/config/vm_file_restore.md)
5. Examples
    1. [Sample Apps used in OADP CI](https://github.com/openshift/oadp-operator/tree/oadp-dev/tests/e2e/sample-applications)
    2. [Stateless App Backup/Restore](docs/examples/stateless.md)
    3. [Stateful App Backup/Restore](docs/examples/stateful.md)
    4. [CSI Backup/Restore](docs/examples/CSI)
    
6. [Troubleshooting](/docs/TROUBLESHOOTING.md)
7. Contribute
    1. [Install & Build from Source](docs/developer/install_from_source.md)
    2. [OLM Integration](docs/developer/olm_hacking.md)
    3. [E2E Test Suite](docs/developer/testing/TESTING.md)
8. [Velero Version Relationship](#version)


<hr style="height:1px;border:none;color:#333;">

<h1 align="center">About<a id="about"></a></h1>

OADP is the OpenShift API for Data Protection operator. This open source operator
sets up and installs <a href="https://velero.io/">Velero</a> on the OpenShift
platform, allowing users to backup and restore applications. [See video demo!](https://www.youtube.com/watch?v=iyoxuP2xb2E)

- We maintain an up to date FAQ page [here](https://access.redhat.com/articles/5456281)

<hr style="height:1px;border:none;color:#333;">
<h1 align="center">Velero Version Relationship<a id="version"></a></h1>

By default, OADP will install the forked versions of Velero that exist under the
`openshift` organization.  These images have minor tweaks to support the OpenShift
specific use cases of using Velero with OCP. The `openshift` images tend to lag
behind Velero upstream releases as we are more cautious about supporting older
versions. Here is the default mapping of versions:

For the current and planned supported versions, please refer to the [version compatibility table in PARTNERS.md](PARTNERS.md#current-and-planned-supported-versions).
