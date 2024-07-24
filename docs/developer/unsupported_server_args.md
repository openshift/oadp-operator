<h1 align="center">Customizing Velero and Node-Agent with Unsupported Override Arguments</h1>

Usually the container arguments for Velero deployment and Node-Agent Daemon Set are configured via the DataProtectionApplication (DPA) CR. However, in some cases. there may not be an option to configure a particular Velero/Node-Agent 
container argument via DPA CR. In such scenarios this feature comes in handy. Using this feature you can pass/configure the container arguments that the DPA CR does not support but Velero/Node-Agent supports them. We will be using configmap objects
to pass the arguments to Velero/Node-Agent. 

Feature Design Reference: https://github.com/openshift/oadp-operator/pull/1400


**Note:**
- These configmaps need to exist in the OADP Operator install namespace
- If the Unsupported args annotation exists on the DPA and the configmap corresponding to the value of the annotation:
  - is empty: then no error on DPA
  - does not exist: then DPA status is in error state
- The server args and values specified in the configmaps will override all the existing server args.
- As the args and values specified in the configmaps will override the existing ones, please make sure if you need the current container args then add those in the configmap as well.
- If you want to see what container args are available to be configured then you can use the Velero CLI's `/velero help` command in velero/node-agent container shell.
- The DPA is NOT updated via the custom configmaps. The DPA and Velero/node-agent config will become out of sync and the definitive view will be from the deployment/Node-Agent containers.
- In OADP must-gather 1.3+, the configmaps are located in: `$dir/namespaces/openshift-adp/core/configmaps.yaml`

### Prerequisites
- OpenShift CLI
- OADP Operator installed on OpenShift cluster
- OADP versions 1.3.3+ and 1.4.1+


### Unsupported server args override for Velero deployment container

- Create a ConfigMap with key-value pairs in the same namespace as OADP Operator, the keys correspond to the velero server arguments and values correspond to the argument value. Sample configmap is as follows:
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
- Now create/update the DPA with the annotation `oadp.openshift.io/unsupported-velero-server-args:<ConfigMap-Name>`. Considering our example the annotation would be:
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
**Tip:** You can use a command similar to `oc get pod -n openshift-adp -l component=velero -l deploy=velero -o json | jq '.items[].spec.containers[].args'` to view the args configured

### Unsupported server args override for Node-agent daemon set container

- Create a ConfigMap with key-value pairs in the same namespace as OADP Operator, the keys correspond to the node-agent server arguments and values correspond to the argument value. Sample configmap is as follows:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: my-config-node-agent-server
  namespace: openshift-adp
data:
  # Configuration values
  data-mover-prepare-timeout: "45m"
  resource-timeout: "15m"
```
- Now create/update the DPA with the annotation `oadp.openshift.io/unsupported-node-agent-server-args:<ConfigMap-Name>`. Considering our example the annotation would be:
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
**Tip:** You can use a command similar to `oc get pod -n openshift-adp -l component=velero -l name=node-agent -o json | jq '.items[].spec.containers[].args'` to view the args configured