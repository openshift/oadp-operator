<h1 align="center">Unsupported server args override for Velero deployment and Node-agent Daemon set containers</h1>

### Prerequisites
- OpenShift CLI
- OADP Operator installed on OpenShift cluster
- OADP versions 1.3 and above


### Unsupported server args override for Velero deployment container

- Create a ConfigMap with key-value pairs, the keys correspond to the velero server arguments and values correspond to the argument value. Sample configmap is as follows:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: my-config-velero-server
  namespace: openshift-adp
data:
  # Configuration values
  features: "EnableCSI"
  uploader-type: "kopia"
  fs-backup-timeout: "10h"
```
- Now create/update the DPA with the annotation `oadp.openshift.io/unsupported-velero-server-args=<ConfigMap-Name>`. Considering our example the annotation would be:
```yaml
  annotations:
    oadp.openshift.io/unsupported-velero-server-args: my-config-velero-server
```
- Finally, once the DPA is successfully reconciled then verify the velero deployment container args and check whether they match with the ones specified in the configmap or not. For our example, it would look like:
```yaml
  containers:
  - args:
    - server
    - --features=EnableCSI
    - --fs-backup-timeout=10h
    - --uploader-type=kopia
    command:
    - /velero
```

### Unsupported server args override for Node-agent daemon set container

- Create a ConfigMap with key-value pairs, the keys correspond to the node-agent server arguments and values correspond to the argument value. Sample configmap is as follows:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: my-config-node-agent-server
  namespace: openshift-adp
data:
  # Configuration values
  data-mover-prepare-timeout: 45m
  resource-timeout: 15m
```
- Now create/update the DPA with the annotation `oadp.openshift.io/unsupported-node-agent-server-args=<ConfigMap-Name>`. Considering our example the annotation would be:
```yaml
  annotations:
    oadp.openshift.io/unsupported-node-agent-server-args: my-config-node-agent-server
```
- Finally, once the DPA is successfully reconciled then verify the node-agent daemon set container args and check whether they match with the ones specified in the configmap or not. For our example, it would look like:
```yaml
  containers:
    - args:
        - node-agent
        - server
        - --data-mover-prepare-timeout=45m
        - --resource-timeout=15m
      command:
        - /velero
```

**Note:**
- These configmaps need to exist in the OADP Operator install namespace
- If the Unsupported args annotation exists on the DPA and the configmap corresponding to the value of the annotation:
  - is empty: then no error on DPA
  - does not exist: then DPA status is in error state