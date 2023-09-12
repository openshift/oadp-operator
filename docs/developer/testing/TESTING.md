<h1 align="center">E2E Testing</h1>

## Prerequisites

### Install Ginkgo

<!-- Can we add this as pre run for make test-e2e? -->
```bash
go install -v github.com/onsi/ginkgo/v2/ginkgo
```

### Setup backup storage configuration

To get started, the test suite expects 2 files to use as configuration for
Velero's backup storage. One file that contains your credentials, and another
that contains additional configuration options (for now, the name of the
bucket).

The default test suite expects these files in `/var/run/oadp-credentials`, but
can be overridden with the environment variables `OADP_CRED_FILE` and
`OADP_BUCKET_FILE`.

To get started, create these 2 files:
`OADP_CRED_FILE`:
```
[<INSERT_PROFILE_NAME>]
aws_access_key_id=<access_key>
aws_secret_access_key=<secret_key>
```
If you use one profile name different from `default`, also change `BSL_AWS_PROFILE`.

<!-- change to env var? -->
`OADP_BUCKET_FILE`:
```
<bucket_name>
```

## Run all e2e tests

<!-- How this is run in CI? Can we add this as pre run for make test-e2e? -->
TODO set needed variables
- OADP_TEST_NAMESPACE ?= test-oadp-operator
- OADP_CRED_DIR ?= MATEUS_PATH/oadp-credentials
- CLUSTER_PROFILE_DIR ?= MATEUS_PATH/oadp-credentials
- CI_CRED_FILE ?= ${CLUSTER_PROFILE_DIR}/new-aws-credentials
- BSL_REGION ?= br-sao
```bash
make deploy-olm
make test-e2e
```
## Run selected test

You can run a particular e2e test(s) by placing an `F` at the beginning of Ginkgo objects. Example

```go
FDescribe("test description", func() { ... })
FContext("test scenario", func() { ... })
FIt("the assertion", func() { ... })
```

These need to be removed to run all specs.

## Clean up

To clean environment after running E2E tests, run
```bash
# Delete leftover objects from tests (like backups and restores)
make undeploy-olm
oc delete namespace $OADP_TEST_NAMESPACE
# And clean bucket in your provider
```

## CI jobs

TODO

## Debugging e2e tests with Visual Studio

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
    * The critical paramaters to change are under `func init()`:
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
	timeoutMultiplierInput := flag.Int64("timeout_multiplier", 1, "Customize timeout multiplier from default (1)")
	timeoutMultiplier = 1
	if timeoutMultiplierInput != nil && *timeoutMultiplierInput >= 1 {
		timeoutMultiplier = time.Duration(*timeoutMultiplierInput)
	}
}

```
Example settings file could be found under oadp-operator/tests/e2e/templates/default_settings.json, and can be overriden used with different providers with similar structure.


* Note that your shell overrides documented [here](https://github.com/openshift/oadp-operator/blob/master/docs/developer/TESTING.md) are not accessible to Visual Studio Code.

### Execute

* Ensure the file you intend to set break points on has focus in Visual Studio Code
* Set break points as needed in Visual Studio Code
* Launch and debug according to Visual Studio Code's [debug instructions](https://code.visualstudio.com/docs/editor/debugging)
