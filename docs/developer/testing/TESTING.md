<h1 align="center">E2E Testing</h1>

## Prerequisites

### OADP operator installed

You need to have OADP operator already installed in your cluster to run E2E tests (except for upgrade tests). For testing PRs, use `make deploy-olm` to install it with the code changes introduced by them. [More info](../install_from_source.md).

### AWS setup

> **Note:** If you are using IBM Cloud, or OpenStack follow this section.

To get started, you need to provide the following **required** environment variables.

| Variable | Description | Default Value | required |
|----------|----------|---------------|---------------|
| `OADP_CRED_FILE` | The path to credentials file for backupLocations | `/var/run/oadp-credentials/new-aws-credentials` | true |
| `OADP_BUCKET` | The bucket name to store backups | `OADP_BUCKET_FILE` file content | true |
| `CI_CRED_FILE` | The path to credentials file for snapshotLocations | `/Users/drajds/.aws/.awscred` | true |
| `VSL_REGION` | The region of snapshotLocations | - | true |
| `BSL_REGION` | The region of backupLocations | `us-east-1` | false |
| `OADP_TEST_NAMESPACE` | The namespace where OADP operator is installed | `openshift-adp` | false |
| `OPENSHIFT_CI` | Disable colored output from tests suite run | `true` | false |
| `TEST_VIRT` | Exclusively run Virtual Machine backup/restore testing | `false` | false |
| `TEST_UPGRADE` | Exclusively run upgrade tests. Need to first run `make catalog-test-upgrade`, if testing non production operator | `false` | false |

The expected format for `OADP_CRED_FILE` and `CI_CRED_FILE` files is:
```
[<INSERT_PROFILE_NAME>]
aws_access_key_id=<access_key>
aws_secret_access_key=<secret_key>
```
> **Note:** If you use one profile name different from `default` for backupLocations, also change `BSL_AWS_PROFILE` environment variable. snapshotLocations hardcodes to `default` profile name in e2e code.

To set the environment variables, run
```sh
export OADP_CRED_FILE=<path_to_backupLocations_credentials_file>
export OADP_BUCKET=<bucket_name>
export CI_CRED_FILE=<path_to_snapshotLocations_credentials_file>
export VSL_REGION=<snapshotLocations_region>
# non required
export BSL_REGION=<backupLocations_region>
export OADP_TEST_NAMESPACE=<test_namespace>
export OPENSHIFT_CI=false
```

It is also possible to set the environment variables during make command call. Example
```sh
OADP_CRED_FILE=<path_to_backupLocations_credentials_file> \
OADP_BUCKET=<bucket_name> \
CI_CRED_FILE=<path_to_snapshotLocations_credentials_file> \
VSL_REGION=<snapshotLocations_region> \
BSL_REGION=<backupLocations_region> \
OADP_TEST_NAMESPACE=<test_namespace> \
OPENSHIFT_CI=false \
make test-e2e
```

### OpenStack setup

* no changes needed, just setup s3 storage in aws. See the above AWS setup.

### Azure setup

TODO

### GCP setup

TODO

## Run tests

To run all E2E tests for your provider, run
```bash
make test-e2e
```
Check [Debugging section](#debugging) for detailed explanation of the command.

### Run selected test

You can run a particular e2e test(s) by placing an `F` at the beginning of Ginkgo objects. Example

```go
FDescribe("test description", func() { ... })
FContext("test scenario", func() { ... })
FIt("the assertion", func() { ... })
...
```
These need to be removed to run all specs. Checks [Ginkgo docs](https://onsi.github.io/ginkgo/) for more info.

You can also execute make test-e2e with a $GINKGO_ARGS variable set. Example:

```bash
make test-e2e GINKGO_ARGS="--ginkgo.focus='MySQL application DATAMOVER'"
```

### Run tests with custom images

You can run tests with custom images by setting the following environment variables:
```bash
export VELERO_IMAGE=<velero_image>
export AWS_PLUGIN_IMAGE=<aws_plugin_image>
export OPENSHIFT_PLUGIN_IMAGE=<openshift_plugin_image>
export AZURE_PLUGIN_IMAGE=<azure_plugin_image>
export GCP_PLUGIN_IMAGE=<gcp_plugin_image>
export CSI_PLUGIN_IMAGE=<csi_plugin_image>
export RESTORE_IMAGE=<restore_image>
export KUBEVIRT_PLUGIN_IMAGE=<kubevirt_plugin_image>
export NON_ADMIN_IMAGE=<non_admin_image>
```
For further details, see [tests/e2e/scripts/](../../../tests/e2e/scripts/)

## Clean up

To clean environment after running E2E tests, run
```bash
make test-e2e-cleanup
```
And clean the bucket in your provider.

## Debugging

When you run `make test-e2e`, the following steps are executed
- `make test-e2e-setup` runs creating base DPA used for tests run
- `make install-ginkgo` installs Ginkgo
- Ginkgo executes the tests

To check DPA spec that is being used as base for tests run, run
```bash
make test-e2e-setup
cat /tmp/test-settings/oadpcreds
```
Check if format looks as expected.
> **Note:** DPA spec used for tests may not be the same as the result, because different tests case (CSI, DataMover, etc) use different plugins, feature flags, etc.

To get tests help, run
```sh
make install-ginkgo
ginkgo run -mod=mod tests/e2e/ -- --help
```
Some of the flags are defined in `tests/e2e/e2e_suite_test.go` file.

E2E tests are defined to run in the CI, so to run the locally, you may need to change some parameters. For example, to run CSI tests with different drives and storage classes, you need to edit
- the related VolumeSnapshotClass to your provider in `tests/e2e/sample-applications/snapclass-csi/` folder with your driver (to list cluster drivers, run `oc get csidrivers`)
- the related PersistentVolumeClaims to your provider in `tests/e2e/sample-applications/mysql-persistent/pvc-twoVol/`, `tests/e2e/sample-applications/mysql-persistent/pvc/` and `tests/e2e/sample-applications/mongo-persistent/pvc/` folders with your storage classes (to list cluster drivers, run `oc get storageclasses`)
- Optionally, the user can use the default storage class by choosing the pvc/default_sc.yaml files.

If running E2E tests against operator created from `make deploy-olm`, remember its image expires, which may cause tests to fail.

When running Virtual Machine backup/restore tests on IBM Cloud, it is better to manually install OpenShift Virtualization operator, instead of automatically installing it through `make test-e2e`. Because this may cause the error of Virtual Machines never starting to run. Example test log:
```
...
2024/07/24 15:12:57 VM cirros-test/cirros-test status is: Stopped
2024/07/24 15:13:07 VM cirros-test/cirros-test status is: Stopped
2024/07/24 15:13:17 VM cirros-test/cirros-test status is: Stopped
2024/07/24 15:13:27 VM cirros-test/cirros-test status is: Stopped
...
```
From events, printed after test failure, you can get the necessary solution. Example test log:
```
  Event: DataVolume.storage spec is missing accessMode and volumeMode, cannot get access mode from StorageProfile ibmc-vpc-block-10iops-tier, Type: Warning, Count: 17, Src: {DataVolume cirros-test cirros-test-disk 4d522545-1170-4b82-ae0c-39441de839f4 cdi.kubevirt.io/v1beta1 453012805 }, Reason: ErrClaimNotValid
```
In this case, solution would be to run `oc patch storageprofile ibmc-vpc-block-10iops-tier --type=merge -p '{"spec": {"claimPropertySets": [{"accessModes": ["ReadWriteOnce"], "volumeMode": "Block"}]}}'`.

### With Visual Studio

TODO update

Optionally developers can debug the Ginkgo tests in tests/e2e with [Visual Studio Code](https://code.visualstudio.com/docs/editor/debugging).

* Ensure you have a properly configured launch.json in your .vscode directory. Ensure that your kubeconfig provides access to the k8s or OpenShift environment.

Example Configuration: **launch.json**
```json
{
    // Use IntelliSense to learn about possible attributes.
    // Hover to view descriptions of existing attributes.
    // For more information, visit: https://go.microsoft.com/fwlink/?linkid=830387
    "version": "0.2.0",
    "configurations": [
        {
            "name": "Launch Package Test",
            "type": "go",
            "request": "launch",
            "mode": "test",
            "program": "${fileDirname}",
            "env": {
                "KUBECONFIG": "/home/user/my_kubeconfig",
                "KUBERNETES_MASTER": "http://localhost:8080"
            }
        }

    ]
}

```

* The [e2e_suite_test.go](https://github.com/openshift/oadp-operator/blob/master/tests/e2e/e2e_suite_test.go) file must be overridden with parameters specific to your environment and aws buckets.
    * The critical parameters to change are under `func init()`:
        * cloud
        * settings
        * namespace
        * cluster_profile

Example Configuration: **e2e_suite_test.go**
```go
func init() {
	flag.StringVar(&cloud, "cloud", "/home/user/oadp_e2e/aws_credentials", "Cloud Credentials file path location")
	flag.StringVar(&namespace, "velero_namespace", "oadp-operator", "DPA Namespace")
	flag.StringVar(&settings, "settings", "./templates/default_settings.json", "Settings of the velero instance")
	flag.StringVar(&instanceName, "velero_instance_name", "example-velero", "Velero Instance Name")
	flag.StringVar(&clusterProfile, "cluster_profile", "aws", "Cluster profile")
}

```
Example settings file could be found under oadp-operator/tests/e2e/templates/default_settings.json, and can be overridden used with different providers with similar structure.


* Note that your shell overrides documented [here](https://github.com/openshift/oadp-operator/blob/master/docs/developer/TESTING.md) are not accessible to Visual Studio Code.

### Execute

* Ensure the file you intend to set break points on has focus in Visual Studio Code
* Set break points as needed in Visual Studio Code
* Launch and debug according to Visual Studio Code's [debug instructions](https://code.visualstudio.com/docs/editor/debugging)
