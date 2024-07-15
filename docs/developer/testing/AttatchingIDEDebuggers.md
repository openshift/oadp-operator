
# Attatching IDE Debuggers to project.

## VSCode
Create `.vscode/launch.json` file with following content

```json
{
    // Use IntelliSense to learn about possible attributes.
    // Hover to view descriptions of existing attributes.
    // For more information, visit: https://go.microsoft.com/fwlink/?linkid=830387
    "version": "0.2.0",
    "configurations": [
        {
            "name": "Debug E2E Test",
            "type": "go",
            "request": "launch",
            "mode": "test",
            "program": "${workspaceRoot}/tests/e2e/e2e_suite_test.go",
            "env": {
                // "KUBECONFIG": "/path/to/.kube/config",
                "KUBERNETES_MASTER": "http://localhost:8080",
                // modifying values from test-e2e flags
                "E2E_USE_ENV_FLAGS": "true",
                "VELERO_NAMESPACE": "openshift-adp",
                "SETTINGS": "${workspaceRoot}/.vscode/default_settings.json",
                "CLOUD_CREDENTIALS": "",
                "VELERO_INSTANCE_NAME": "",
                "PROVIDER": "",
                "CI_CRED_FILE": "",
                "ARTIFACT_DIR": "",
                "OC_CLI": "",
            }
        },
        {
            "name": "Launch main.go",
            "type": "go",
            "request": "launch",
            "mode": "debug",
            "program": "${workspaceRoot}/main.go",
            "env": {
                "WATCH_NAMESPACE": "openshift-adp",
                // "KUBECONFIG": "",
                "KUBERNETES_MASTER": "http://localhost:8080"
            }
        },
    ]
}

```

copy `default_settings.json` file from `test/e2e/templates/` directory to `.vscode/` directory and modify it as per your needs.

You can now use Run and Debug menu to launch
- Debug E2E Test
  - This runs the test suite in debug mode. You can add breakpoints to step through the end to end test.
  - Prerequisites:
    - You have installed OADP Operator. To install current commit follow steps from  [Installing the Operator](../install_from_source.md#installing-the-operator).
- Launch main.go
  - This runs the operator on your machine in debug mode. You can add breakpoints and step through the code.
  - You will not see OADP in Installed Operators but it will be watching for DPA resources in the namespace as if it was installed.
  - Prerequisites:
    - Run `make install apply-velerosa-role` to install CRDs and create service accounts, a task normally handled by OLM but not in this case.