# Multi Cloud Test Suite Updates for Backup / Restore cases 

### Overview
Running backup/restore test cases with cloud-provider specific snapshots enabled.

The Test Suite now takes different env variables related to the cloud as flags and create the VSLs based on that cloud provider. Validating the backup / restore test cases on the same.

### How multiple profiles are enabled to support Backup / Restore.
In ideal cases, both the credentials / profile for BSL and VSL would be the same and usually we dont mention the separate credentials for them, but this is different in OpenShift CI environments. In a OpenShift / Prow CI environment, the cluster is provisioned in either AWS / GCP / Azure Cloud. Although we have access to the OpenShfit CI Cluster, we do not have access to the cloud, hence supporting volume backup using our credentials which is mounted in OpenShift CI Cluster is not a valid option. Hence we are using different methods to support these.

#### AWS Multi Profile Support

The CI Cloud credential is present at this location in OpenShift CI Cluster: 
`/var/run/secrets/ci.openshift.io/cluster-profile/.awscred` 

Our Cloud credential used for BSL is present at this location: 
`/var/run/oadp-credentials/new-aws-credentials`

Here since they are two profiles, we are using the concept of credentialsFile in BSL config [ref] (https://github.com/vmware-tanzu/velero/issues/3428)

We are also mounting credentials [here](https://github.com/openshift/oadp-operator/blob/master/pkg/credentials/credentials.go#L37)

#### GCP

The CI Cloud credential is present at this location in OpenShift CI Cluster: 
`/var/run/secrets/ci.openshift.io/cluster-profile/gce.json` 

Our Cloud credential used for BSL is present at this location: 
`/var/run/oadp-credentials/gcp-credentials`

Here since they are two different credentials and not profiles, we are using the concept of credentialsFile in BSL config [ref](https://github.com/vmware-tanzu/velero/issues/3430)

We are also mounting credentials [here](https://github.com/openshift/oadp-operator/blob/master/pkg/credentials/credentials.go#L47)

#### [Azure](https://github.com/vmware-tanzu/velero/issues/3429)

The CI Cloud credential is present at this location in OpenShift CI Cluster: 
`/var/run/secrets/ci.openshift.io/cluster-profile/osServicePrincipal.json` 

Our Cloud credential used for BSL is present at this location: 
`/var/run/oadp-credentials/azure-credentials`

The required variables for e2e tests are 

For object storage with backup of registy support, We need the below credentials
```
{
  "subscriptionId": "xxxxx",
  "clientId": "xxxxx",
  "clientSecret": “xxxxxx”,
  "tenantId": "xxxx",
  "resourceGroup": "Deepak_Velero_Backups",
  "storageAccountAccessKey": "xxxxxx",
  "storageAccount": "velerodpk68c64591c324"
}
```

The below is given to the volume backup credentials in CI Environment

```
{
  "subscriptionId": "xxxx",
  "clientId": "xxxx",
  "clientSecret": “xxxxxx”,
  "tenantId": "xxxx"
}
```

The resource group is different in OpenShift CI environment. After some research, it was found that the resource group is same as the "<cluster_name>-rg" where the cluster group can be derived from

```
sh-4.4$ cat metadata.json 
{"clusterName":"ci-op-w718n0np-32d40","clusterID":"6de2d426-68af-43d3-9d1a-d72666edc550","infraID":"ci-op-w718n0np-32d40-4fdtv","azure":{"cloudName":"AzurePublicCloud","region":"eastus","resourceGroupName":""}}
```

In the end, for VSL all we needed was the subscriptionId and resourceGroup from the OpenShift CI environment and by default the VSL uses 'cloud-credential-\<platform>' secret for VSL. 

### Pre-requisites for setting up envs in various cloud from local env.

```
drajds@drajds-mac oadp-operator % cat ~/.oadp-aws
export CLUSTER_TYPE=aws
export OADP_TEST_NAMESPACE=openshift-adp
export BSL_REGION=us-east-1
export VSL_REGION=us-west-2
export OADP_CRED_FILE=/Users/drajds/.aws/credentials
export OADP_BUCKET_FILE=/Users/drajds/.aws/bucket
export VELERO_INSTANCE_NAME=example-velero
export BSL_AWS_PROFILE=migration-engineering
export CLUSTER_PROFILE_DIR=/Users/drajds/.aws
export OADP_CRED_DIR=/Users/drajds/.aws
export CI_CRED_FILE=/Users/drajds/.aws/ci-credentials
```

* VSL_REGION - the region the cluster is spawned on
* OADP_CRED_FILE - credentials file for BSL
* OADP_BUCKET_FILE - bucket file for BSL - has only the bucket name - no json
* CLUSTER_PROFILE_DIR - directory containing credentials for VSL
* OADP_CRED_DIR - directory containing credentials and bucket file for BSL

For GCE & Azure, put your credentials file with name `${OADP_CRED_DIR}/<provider>-credentials` and bucket with name `${OADP_CRED_DIR}/azure-velero-bucket-name`

#### GCP 

```
drajds@drajds-mac oadp-operator % cat ~/.oadp-gcp
export CLUSTER_TYPE=gcp
export OADP_TEST_NAMESPACE=openshift-adp
export VSL_REGION=us-central1
export OADP_CRED_FILE=aos-serviceaccount.json
export OADP_BUCKET_FILE=/Users/drajds/.gcp/bucket
export VELERO_INSTANCE_NAME=gcp-example-velero
export CLUSTER_PROFILE_DIR=/Users/drajds/.gcp
export OADP_CRED_DIR=/Users/drajds/.gcp
export CI_CRED_FILE=gcp_sa.json
```

#### Azure

```
drajds@drajds-mac oadp-operator % cat ~/.oadp-azure
export CLUSTER_TYPE=azure4
export OADP_TEST_NAMESPACE=openshift-adp
export VELERO_INSTANCE_NAME=azure-example-velero
export CLUSTER_PROFILE_DIR=/Users/drajds/.azure
export AZURE_RESOURCE_FILE=/Users/drajds/.azure/resource.yaml
export OADP_BUCKET_FILE=/Users/drajds/.azure/bucket
export OADP_CRED_DIR=/Users/drajds/.azure
```

* `OADP_CRED_DIR` - is a directory that contains
  * bsl-\<cloud>-credentials
  * new-velero-bucket-name
* `CLUSTER_PROFILE_DIR` - is a directory that contains
  * vsl/ci-\<cloud>-credentials
* `OADP_BUCKET_FILE` - this file contains name of the bucket in plain text.