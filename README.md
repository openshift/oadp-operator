<div align="center">
  <h1> OADP Operator </h1>
  <p>  OpenShift API for Data Protection </p>

  [![Go Report Card](https://goreportcard.com/badge/github.com/openshift/oadp-operator)](https://goreportcard.com/report/github.com/openshift/oadp-operator) [![codecov](https://codecov.io/gh/openshift/oadp-operator/branch/oadp-dev/graph/badge.svg?token=qLM0hAzjpD)](https://codecov.io/gh/openshift/oadp-operator) [![License](https://img.shields.io/:license-apache-blue.svg)](https://www.apache.org/licenses/LICENSE-2.0.html) [![Go Reference](https://pkg.go.dev/badge/github.com/openshift/oadp-operator.svg)](https://pkg.go.dev/github.com/openshift/oadp-operator)
</div>

### Periodic Unit Tests 
[![Unit tests](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-oadp-dev-unit-test-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-oadp-dev-unit-test-periodic)

### Periodic AWS E2E Tests in OpenShift

| OpenShift Version | Test Status |
|-------------------|-------------|
| OCP 4.19 | [![AWS tests OCP 4.19](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-oadp-dev-4.19-e2e-test-aws-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-oadp-dev-4.19-e2e-test-aws-periodic) |
| OCP 4.20 | [![AWS tests OCP 4.20](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-oadp-dev-4.20-e2e-test-aws-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-oadp-dev-4.20-e2e-test-aws-periodic) 

### Periodic AWS E2E Virtualization Tests in OpenShift

| OpenShift Version | Test Status |
|-------------------|-------------|
| OCP 4.19 | [![VM tests](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-oadp-dev-4.19-e2e-test-kubevirt-aws-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-oadp-dev-4.19-e2e-test-kubevirt-aws-periodic) |
| OCP 4.20 | [![VM tests](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-oadp-dev-4.20-e2e-test-kubevirt-aws-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-oadp-dev-4.20-e2e-test-kubevirt-aws-periodic) |

### Periodic AWS E2E Hypershift Tests in OpenShift

| OpenShift Version | Test Status |
|-------------------|-------------|
| OCP 4.19 | [![HCP tests](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-oadp-dev-4.19-e2e-test-hcp-aws-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-oadp-dev-4.19-e2e-test-hcp-aws-periodic) |
| OCP 4.20 | [![HCP tests](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-oadp-dev-4.20-e2e-test-hcp-aws-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-oadp-dev-4.20-e2e-test-hcp-aws-periodic) |

### Periodic AWS E2E OADP CLI Tests in OpenShift
| OpenShift Version | Test Status |
|-------------------|-------------|
| OCP 4.19          | [![CLI 4.19 AWS](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-oadp-dev-4.19-e2e-test-cli-aws-periodic)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-oadp-dev-4.19-e2e-test-cli-aws-periodic)|
| OCP 4.20          | TBD         |

### OADP repositories images job
| OADP | OpenShift Velero plugin | Velero | Velero plugin for AWS | Velero plugin for Legacy AWS | Velero plugin for GCP | Velero plugin for Microsoft Azure | Non Admin |
| ---------- | ---------- | ---------- | ---------- | ---------- | ---------- | ---------- | ---------- |
| [![OADP repository](https://prow.ci.openshift.org/badge.svg?jobs=branch-ci-openshift-oadp-operator-oadp-dev-images)](https://prow.ci.openshift.org/job-history/gs/test-platform-results/logs/branch-ci-openshift-oadp-operator-oadp-dev-images) | [![OpenShift Velero plugin repository](https://prow.ci.openshift.org/badge.svg?jobs=branch-ci-openshift-openshift-velero-plugin-oadp-dev-images)](https://prow.ci.openshift.org/job-history/gs/test-platform-results/logs/branch-ci-openshift-openshift-velero-plugin-oadp-dev-images) | [![OADP's Velero repository](https://prow.ci.openshift.org/badge.svg?jobs=branch-ci-openshift-velero-oadp-dev-images)](https://prow.ci.openshift.org/job-history/gs/test-platform-results/logs/branch-ci-openshift-velero-oadp-dev-images) | [![OADP's Velero plugin for AWS repository](https://prow.ci.openshift.org/badge.svg?jobs=branch-ci-openshift-velero-plugin-for-aws-oadp-dev-images)](https://prow.ci.openshift.org/job-history/gs/test-platform-results/logs/branch-ci-openshift-velero-plugin-for-aws-oadp-dev-images) | [![OADP's Velero plugin for Legacy AWS repository](https://prow.ci.openshift.org/badge.svg?jobs=branch-ci-openshift-velero-plugin-for-legacy-aws-oadp-dev-images)](https://prow.ci.openshift.org/job-history/gs/test-platform-results/logs/branch-ci-openshift-velero-plugin-for-legacy-aws-oadp-dev-images) | [![OADP's Velero plugin for GCP repository](https://prow.ci.openshift.org/badge.svg?jobs=branch-ci-openshift-velero-plugin-for-gcp-oadp-dev-images)](https://prow.ci.openshift.org/job-history/gs/test-platform-results/logs/branch-ci-openshift-velero-plugin-for-gcp-oadp-dev-images) | [![OADP's Velero plugin for Microsoft Azure repository](https://prow.ci.openshift.org/badge.svg?jobs=branch-ci-openshift-velero-plugin-for-microsoft-azure-oadp-dev-images)](https://prow.ci.openshift.org/job-history/gs/test-platform-results/logs/branch-ci-openshift-velero-plugin-for-microsoft-azure-oadp-dev-images) | [![Non Admin repository](https://prow.ci.openshift.org/badge.svg?jobs=branch-ci-migtools-oadp-non-admin-oadp-dev-images)](https://prow.ci.openshift.org/job-history/gs/test-platform-results/logs/branch-ci-migtools-oadp-non-admin-oadp-dev-images) |

### Mirroring images to quay.io [![Mirror images](https://prow.ci.openshift.org/badge.svg?jobs=periodic-image-mirroring-konveyor)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-image-mirroring-konveyor)
</div>

### Rebase status from upstream Velero

* [OADP Rebase](https://github.com/oadp-rebasebot/oadp-rebase)
** UNDER-CONSTRUCTION **

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
5. Examples
    1. [Sample Apps used in OADP CI](https://github.com/openshift/oadp-operator/tree/oadp-dev/tests/e2e/sample-applications)
    2. [Stateless App Backup/Restore](docs/examples/stateless.md)
    3. [Stateful App Backup/Restore](docs/examples/stateful.md)
    4. [CSI Backup/Restore](docs/examples/CSI)
    
7. [Troubleshooting](/docs/TROUBLESHOOTING.md)
8. Contribute
    1. [Install & Build from Source](docs/developer/install_from_source.md)
    2. [OLM Integration](docs/developer/olm_hacking.md)
    3. [Test Operator Changes](docs/developer/local_dev.md)
    4. [E2E Test Suite](docs/developer/TESTING.md)
10. [Velero Version Relationship](#version)


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
