<div align="center">
  <h1> OADP Operator </h1>
  <p>  OpenShift API for Data Protection </p>

  [![Go Report Card](https://goreportcard.com/badge/github.com/openshift/oadp-operator)](https://goreportcard.com/report/github.com/openshift/oadp-operator) [![codecov](https://codecov.io/gh/openshift/oadp-operator/branch/master/graph/badge.svg?token=qLM0hAzjpD)](https://codecov.io/gh/openshift/oadp-operator) [![License](https://img.shields.io/:license-apache-blue.svg)](https://www.apache.org/licenses/LICENSE-2.0.html) [![Go Reference](https://pkg.go.dev/badge/github.com/openshift/oadp-operator.svg)](https://pkg.go.dev/github.com/openshift/oadp-operator)

Periodic Unit Tests [![Unit tests](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-unit-test-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-unit-test-periodic)

Periodic AWS E2E Tests in OpenShift versions
| 4.14 | 4.15 | 4.16 | 4.17 | 4.18-dev-preview |
| ---------- | ---------- | ---------- | ---------- | ---------- |
| [![AWS builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.14-e2e-test-aws-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.14-e2e-test-aws-periodic) | [![AWS builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.15-e2e-test-aws-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.15-e2e-test-aws-periodic) | [![AWS builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.16-e2e-test-aws-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.16-e2e-test-aws-periodic) | [![AWS builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.17-e2e-test-aws-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.17-e2e-test-aws-periodic) | [![AWS builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.18-dev-preview-e2e-test-aws-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.18-dev-preview-e2e-test-aws-periodic) |
<!-- GCP:
[![GCP builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.12-e2e-test-gcp-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.12-e2e-test-gcp-periodic)
[![GCP builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.13-e2e-test-gcp-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.13-e2e-test-gcp-periodic)
[![GCP builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.14-e2e-test-gcp-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.14-e2e-test-gcp-periodic) -->


<!-- Azure:
[![Azure builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.12-e2e-test-azure-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.12-e2e-test-azure-periodic)
[![Azure builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.13-e2e-test-azure-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.13-e2e-test-azure-periodic)
[![Azure builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.14-e2e-test-azure-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.14-e2e-test-azure-periodic) -->

Periodic AWS E2E Virtualization Tests in OpenShift 4.17
[![VM tests](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.17-e2e-test-kubevirt-aws-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.17-e2e-test-kubevirt-aws-periodic)

OADP repositories images job
| OADP | OpenShift Velero plugin | Velero | Velero plugin for AWS | Velero plugin for Legacy AWS | Velero plugin for GCP | Velero plugin for Microsoft Azure | Non Admin |
| ---------- | ---------- | ---------- | ---------- | ---------- | ---------- | ---------- | ---------- |
| [![OADP repository](https://prow.ci.openshift.org/badge.svg?jobs=branch-ci-openshift-oadp-operator-master-images)](https://prow.ci.openshift.org/job-history/gs/test-platform-results/logs/branch-ci-openshift-oadp-operator-master-images) | [![OpenShift Velero plugin repository](https://prow.ci.openshift.org/badge.svg?jobs=branch-ci-openshift-openshift-velero-plugin-master-images)](https://prow.ci.openshift.org/job-history/gs/test-platform-results/logs/branch-ci-openshift-openshift-velero-plugin-master-images) | [![OADP's Velero repository](https://prow.ci.openshift.org/badge.svg?jobs=branch-ci-openshift-velero-konveyor-dev-images)](https://prow.ci.openshift.org/job-history/gs/test-platform-results/logs/branch-ci-openshift-velero-konveyor-dev-images) | [![OADP's Velero plugin for AWS repository](https://prow.ci.openshift.org/badge.svg?jobs=branch-ci-openshift-velero-plugin-for-aws-konveyor-dev-images)](https://prow.ci.openshift.org/job-history/gs/test-platform-results/logs/branch-ci-openshift-velero-plugin-for-aws-konveyor-dev-images) | [![OADP's Velero plugin for Legacy AWS repository](https://prow.ci.openshift.org/badge.svg?jobs=branch-ci-openshift-velero-plugin-for-legacy-aws-konveyor-dev-images)](https://prow.ci.openshift.org/job-history/gs/test-platform-results/logs/branch-ci-openshift-velero-plugin-for-legacy-aws-konveyor-dev-images) | [![OADP's Velero plugin for GCP repository](https://prow.ci.openshift.org/badge.svg?jobs=branch-ci-openshift-velero-plugin-for-gcp-konveyor-dev-images)](https://prow.ci.openshift.org/job-history/gs/test-platform-results/logs/branch-ci-openshift-velero-plugin-for-gcp-konveyor-dev-images) | [![OADP's Velero plugin for Microsoft Azure repository](https://prow.ci.openshift.org/badge.svg?jobs=branch-ci-openshift-velero-plugin-for-microsoft-azure-konveyor-dev-images)](https://prow.ci.openshift.org/job-history/gs/test-platform-results/logs/branch-ci-openshift-velero-plugin-for-microsoft-azure-konveyor-dev-images) | [![Non Admin repository](https://prow.ci.openshift.org/badge.svg?jobs=branch-ci-migtools-oadp-non-admin-master-images)](https://prow.ci.openshift.org/job-history/gs/test-platform-results/logs/branch-ci-migtools-oadp-non-admin-master-images) |

Mirroring images to quay.io [![Mirror images](https://prow.ci.openshift.org/badge.svg?jobs=periodic-image-mirroring-konveyor)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-image-mirroring-konveyor)
</div>

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
5. [Upgrade from 0.2](docs/upgrade.md)
6. Examples
    1. [Stateless App Backup/Restore](docs/examples/stateless.md)
    2. [Stateful App Backup/Restore](docs/examples/stateful.md)
    3. [CSI Backup/Restore](docs/examples/CSI)
    4. [Data Mover (OADP 1.2 or below)](/docs/examples/data_mover.md)
7. [Troubleshooting](/docs/TROUBLESHOOTING.md)
8. Contribute
    1. [Install & Build from Source](docs/developer/install_from_source.md)
    2. [OLM Integration](docs/developer/olm_hacking.md)
    3. [Test Operator Changes](docs/developer/local_dev.md)
    4. [E2E Test Suite](docs/developer/TESTING.md)
9.  [Velero Version Relationship](#version)


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

| OADP Version    | Velero Version |
|:----------------|---------------:|
| v0.2.6          |         v1.6.0 |
| v0.5.5          |         v1.7.1 |
| v1.0.0 - v1.0.z |         v1.7.1 |
| v1.1.0          |         v1.9.1 |
| v1.1.1          |         v1.9.4 |
| v1.1.2 - v1.1.5 |         v1.9.5 |
| v1.2.0          |        v1.11.0 |
| v1.3.0          |        v1.12.1 |
| v1.3.1          |        v1.12.4 |
| v1.4.0          |        v1.14.0 |

