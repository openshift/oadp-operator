<div align="center">
  <h1> OADP Operator </h1>
  <p>  OpenShift API for Data Protection </p>

  [![Go Report Card](https://goreportcard.com/badge/github.com/openshift/oadp-operator)](https://goreportcard.com/report/github.com/openshift/oadp-operator) [![codecov](https://codecov.io/gh/openshift/oadp-operator/branch/master/graph/badge.svg?token=qLM0hAzjpD)](https://codecov.io/gh/openshift/oadp-operator) [![License](https://img.shields.io/:license-apache-blue.svg)](https://www.apache.org/licenses/LICENSE-2.0.html) [![Go Reference](https://pkg.go.dev/badge/github.com/openshift/oadp-operator.svg)](https://pkg.go.dev/github.com/openshift/oadp-operator)

  AWS: [![AWS builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.8-operator-e2e-aws-periodic-slack)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.8-operator-e2e-aws-periodic-slack)
  GCP: [![GCP builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.8-operator-e2e-gcp-periodic-slack)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.8-operator-e2e-gcp-periodic-slack)
  Azure: [![Azure builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-4.8-operator-e2e-azure-periodic-slack)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-4.8-operator-e2e-azure-periodic-slack)
</div>

# Table of Contents

1. [About](#about)
2. [Basic Install using OperatorHub](docs/install_olm.md)
3. [API References](docs/API_ref.md)
4. API Usage
    1. [Configure Plugins](docs/config/plugins.md)
    2. [Backup Storage Locations and Volume Snapshot Locations](docs/config/bsl_and_vsl.md)
    3. [Resource Requests and Limits](docs/config/resource_req_limits.md)
    4. [Self-Signed Certificate](docs/config/self_signed_certs.md)
    5. [Use NooBaa as a Backup Storage Location](docs/config/noobaa/install_oadp_noobaa.md) 
    6. [Use Velero --features flag](docs/config/features_flag.md)
    6. [Use Custom Plugin Images for Velero ](docs/config/custom_plugin_images.md)
6. [Upgrade from 0.2](docs/upgrade.md)
7. Examples
    1. [Stateless App Backup/Restore](docs/examples/stateless.md)
    2. [Stateful App Backup/Restore](docs/examples/stateful.md)
    2. [CSI Backup/Restore](docs/examples/csi_example.md)
8. [Troubleshooting](docs/TROUBLESHOOTING.md)
9. Contribute
    1. [Install & Build from Source (Non-OLM)](docs/developer/install_non-olm.md)
    2. [OLM Integration](docs/developer/olm_hacking.md)
    3. [Test Operator Changes](docs/developer/local_dev.md)
    4. [E2E Test Suite](docs/developer/TESTING.md)
10. [Velero Version Relationship](#version)


<hr style="height:1px;border:none;color:#333;">

<h1 align="center">About<a id="about"></a></h1>

OADP is the OpenShift API for Data Protection operator. This open source operator 
sets up and installs <a href="https://velero.io/">Velero</a> on the OpenShift 
platform, allowing users to backup and restore applications. 

<hr style="height:1px;border:none;color:#333;">
<h1 align="center">Velero Version Relationship<a id="version"></a></h1>

By default, OADP will install the forked versions of Velero that exist under the 
`openshift` organization.  These images have minor tweaks to support the OpenShift 
specific use cases of using Velero with OCP. The `openshift` images tend to lag 
behind Velero upstream releases as we are more cautious about supporting older 
versions. Here is the default mapping of versions:

| OADP Version | Velero Version |
|:-------------|   -----------: |
| v0.2.6       | v1.6.0         |
| v0.5.5       | v1.7.1         |
| v1.0.0       | v1.7.1         |

