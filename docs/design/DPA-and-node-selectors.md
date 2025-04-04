# Node Selector Configuration for Data Protection Components

## Abstract
This proposal introduces a standardized approach for configuring node selectors in the DataProtectionApplication (DPA) custom resource for various use cases that are common across different Velero settings.

## Background
Currently, node selection for Velero and NodeAgent components is inconsistently defined within the DPA specification.

- `spec.configuration.nodeAgent.podConfig.nodeSelector` specifies nodes using matchLabels. This setting is used to define in which nodes the node-agent pod will be scheduled.
- `spec.configuration.nodeAgent.loadConcurrency.perNodeConfig` supports both matchLabels and matchExpressions. Node-agent concurrency configurations allows to configure the concurrent number of node-agent loads per node, but only for the nodes where the node-agent pod is scheduled.
- `spec.configuration.velero.podConfig.nodeSelector` specifies node selector for the velero pod.

The NodeAgent pod is either scheduled on every node in the cluster or limited to specific nodes where the NodeAgent pod is scheduled via `spec.configuration.nodeAgent.podConfig.nodeSelector` setting.

This design enhances flexibility by allowing Affinity and Anti-Affinity settings for scheduling NodeAgent pods with their DataMover workloads or repository maintenance pods.

## Goals
- Provide a clear and structured approach for users to define node selection criteria for the NodeAgent, DataMover and repository maintenance pods using the DPA specification.
- Support both `matchLabels` and `matchExpressions` for flexible node selection for the NodeAgent, DataMover and repository maintenance pods.
- Ensure pod Affinity and Anti-Affinity rules are respected for the NodeAgent, DataMover and repository maintenance pods.
- Guarantee that DataMover pods are scheduled on the same nodes as NodeAgent pods.
- Ensure backward compatibility with existing configurations.
- Ensure user experience is not degraded by the changes by providing Reconcile warnings and errors when the existing node selector configurations are used and may cause issues with the new Affinity and Anti-Affinity settings.

## Non Goals
- Removing or deprecating existing node selector configurations.
- Use advanced Affinity settings that are not specified in teh Veleros' loadAffinity CRD such as `requiredDuringSchedulingIgnoredDuringExecution` and `preferredDuringSchedulingIgnoredDuringExecution`.
- Extend current Velero specific `spec.configuration.velero.podConfig` with the `spec.configuration.velero.loadAffinity` DPA CRD. The current nodeSelector from the `spec.configuration.velero.podConfig` will be used to schedule the Velero pod.

## High-Level Design
The proposed change introduces a unified nodeSelector structure for all relevant components within the DPA custom resource.
This will allow users to specify node selection using either matchLabels or matchExpressions, ensuring flexibility while maintaining consistency across configurations.

The modified structure will be applied to the new DPA sections:

- `spec.configuration.nodeAgent.loadConcurrency`
- `spec.configuration.nodeAgent.loadAffinity`
- `spec.configuration.repositoryMaintenance`

In the future this design may be extended to include Velero specific settings in the:
- `spec.configuration.velero.loadAffinity`

## Detailed Design

### Current podConfig fields
Prior to OADP 1.5 the following nodeSelector fields are available in the DPA custom resource:

```yaml
apiVersion: oadp.openshift.io/v1alpha1
kind: DataProtectionApplication
metadata:
  name: <dpa_sample>
spec:
...
  configuration:
    velero:
      podConfig:
        nodeSelector:
          some-label.io/custom-node-role: cpu-2
    nodeAgent:
      podConfig:
        nodeSelector:
          some-label.io/custom-node-role: cpu-1
```
> Note:
>
> There is also `spec.configuration.restic.podConfig`, however it's deprecated in the OADP 1.5, so it's not included in this design.


## Changes to existing configuration settings

Note: In the examples below each `operator` is a logical operator for OCP to use when interpreting the rules.
Possible operators are:
- In
- NotIn
- Exists
- DoesNotExist
- Gt
- Lt

### Changes to current `spec.configuration.nodeAgent.podConfig.nodeSelector` fields

No changes will be made to the current `spec.configuration.nodeAgent.podConfig.nodeSelector` CRD field.

The following logic will be implemented on the reconcile:
- The `spec.configuration.nodeAgent.podConfig.nodeSelector` and the `spec.configuration.nodeAgent.loadAffinity.nodeSelector` will be used for scheduling the **NodeAgent** pods.
- The `spec.configuration.nodeAgent.podConfig.nodeSelector` will be used by the **NodeAgent** DaemonSet.
- The **NodeAgent** DaemonSet will use Affinity and Anti-Affinity settings from the `spec.configuration.nodeAgent.loadAffinity` section to schedule the **NodeAgent** pods. This will translate to the `requiredDuringSchedulingIgnoredDuringExecution` fields from the `v1.Pod` CRD.
- The `spec.configuration.nodeAgent.loadAffinity.nodeSelector` will be used to generate a **ConfigMap** containing the **DataMover** pod affinity settings.
- To ensure **DataMover** pods are scheduled on the same nodes as the **NodeAgent** pods, the labels from the `spec.configuration.nodeAgent.podConfig.nodeSelector` will be applied to `spec.configuration.nodeAgent.loadAffinity.nodeSelector` field as a simple `matchLabels` selector. This will generate a **ConfigMap** containing the **NodeAgent** pod affinity settings, which will then be used in the **NodeAgent** pod command to schedule the **DataMover** pods.
- The user will be able to use different `matchLabels` between the `spec.configuration.nodeAgent.loadAffinity.nodeSelector` and the `spec.configuration.nodeAgent.podConfig.nodeSelector` fields, with the following caveats:
  - The user will not be allowed to use `matchExpressions` in the `spec.configuration.nodeAgent.loadAffinity.nodeSelector` and the `spec.configuration.nodeAgent.podConfig.nodeSelector` at the same time. This is to ensure that the **DataMover** pods which are using only **ConfigMap** as a scheduler source will be scheduled on the same nodes as the **NodeAgent** pods, that could be scheduled on different nodes from the `spec.configuration.nodeAgent.podConfig.nodeSelector` field. A situation like this is likely to occur when the user wants to specify Anti-Affinity settings for the **DataMover** pods.
  - All labels from the `spec.configuration.nodeAgent.podConfig.nodeSelector` will be added to the `spec.configuration.nodeAgent.loadAffinity.nodeSelector` field, making the `spec.configuration.nodeAgent.loadAffinity.nodeSelector` field more restrictive (a subset of the `spec.configuration.nodeAgent.podConfig.nodeSelector` field). This allows to specify more `matchLabels` in the `spec.configuration.nodeAgent.loadAffinity.nodeSelector` field. In such case the **NodeAgent** pods will be scheduled on the nodes that match all the labels from the `spec.configuration.nodeAgent.podConfig.nodeSelector` and the **DataMover** pods will additionally be limited to the nodes from the `spec.configuration.nodeAgent.loadAffinity.nodeSelector` fields.
- For more complex cases, the user can use `matchExpressions` in the `spec.configuration.nodeAgent.loadAffinity.nodeSelector` field. When this is used the `spec.configuration.nodeAgent.podConfig.nodeSelector` will not be allowed to be used. This will allow to specify more fine-grained node selection criteria for the **DataMover** and the **NodeAgent** pods.
- The reconcile will error out if the user configures the `spec.configuration.nodeAgent.podConfig.nodeSelector` and `spec.configuration.nodeAgent.loadAffinity.nodeSelector` outside of the above restrictions and caveats.


#### New settings specific to the `loadConcurrency` will be added to the nodeAgent section.
The `loadConcurrency` section is used to configure the number of concurrent backups that can be performed on a single node.

The `loadConcurrency` section is explained in the upstream Velero documentation: https://velero.io/docs/main/node-agent-concurrency/

Specifying the `loadConcurrency` settings will generate a **ConfigMap**. This **ConfigMap** is then used to configure concullrent backups and passed to the **NodeAgent** pod command in the same way as it is passed for the **DataMover** workloads.

The nodes to which the settings are applied are **not validated** against the `nodeSelector` fields from the `spec.configuration.nodeAgent.podConfig.nodeSelector` nor the `spec.configuration.nodeAgent.loadAffinity.nodeSelector` fields. If there is a match the loadConcurrency settings will be applied to that node-agent pod.

```yaml
apiVersion: oadp.openshift.io/v1alpha1
kind: DataProtectionApplication
metadata:
  name: <dpa_sample>
spec:
...
  configuration:
    nodeAgent:
      loadConcurrency:
        globalConfig: 2
        perNodeConfig:
          - nodeSelector:
            matchLabels:
              some-label.io/custom-node-role: cpu-1
            matchExpressions:
              - key: kubernetes.io/hostname
                operator: In
                values:
                  - node1
                  - node2
              - key: some-label.io/critical-workload
                  operator: DoesNotExist
            number: 1
```

#### New settings specific to the `spec.configuration.nodeAgent.loadAffinity` will be added to the nodeAgent section.

The `loadAffinity` section workflow is already explained in the above paragrafs of this design document. 
The `loadAffinity` section is also explained in the upstream Velero documentation: https://velero.io/docs/main/data-movement-backup-node-selection/

```yaml
apiVersion: oadp.openshift.io/v1alpha1
kind: DataProtectionApplication
metadata:
  name: <dpa_sample>
spec:
...
  configuration:
    nodeAgent:
      loadAffinity:
        - nodeSelector:
          matchLabels:
            some-label.io/custom-node-role: cpu-1
          matchExpressions:
            - key: kubernetes.io/hostname
              operator: In
              values:
                - node1
                - node2
            - key: some-label.io/critical-workload
                operator: DoesNotExist
          number: 1
```

#### Repository maintenance job Node Affinity

The repository maintenance job is a background job that is used to clean up the repository.

The **NodeAgent** pod is not required to run on the same node as the repository maintenance job and as such the `repositoryMaintenance` section is not bound to the `nodeAgent` section.

The `podResources` and `loadAffinity` are explained in the upstream Velero documentation: https://velero.io/docs/main/repository-maintenance/#affinity-example

New settings specific to the `podResources` and `loadAffinity` will be added to the `repositoryMaintenance` section.

repositoryMaintenance `key` maps a BackupRepository identifier to its configuration.
Keys can be:
 - `global` : Reserved to apply to all repositories without specific config.
 - `<namespace>` : The namespace in which BackupRepository backs up volume data.
 - `<repository name>` : The BackupRepository referenced BackupStorageLocation’s name.
 - `<repository type>` : BackupRepository type. Either `kopia` or `restic`.

```yaml
apiVersion: oadp.openshift.io/v1alpha1
kind: DataProtectionApplication
metadata:
  name: <dpa_sample>
spec:
...
  configuration:
    repositoryMaintenance:
        global:
            podResources:
                cpuRequest: "100m"
                cpuLimit: "200m"
                memoryRequest: "100Mi"
                memoryLimit: "200Mi"
            loadAffinity:
                - nodeSelector:
                    matchExpressions:
                        - key: cloud.google.com/machine-family
                          operator: In
                          values:
                              - e2
                - nodeSelector:
                    matchExpressions:
                        - key: kubernetes.io/hostname
                          operator: In
                          values:
                            - node1
                            - node2
        myrepositoryname:
            podResources:
                cpuRequest: "200m"
                cpuLimit: "400m"
                memoryRequest: "200Mi"
                memoryLimit: "400Mi"
            loadAffinity:
                - nodeSelector:
                    matchExpressions:
                        - key: kubernetes.io/hostname
                          operator: DoesNotExist
```

### Changes to current `spec.configuration.velero.podConfig.nodeSelector` fields

No changes will be made to the current `spec.configuration.velero.podConfig.nodeSelector` CRD field.

Currently the `spec.configuration.velero.podConfig.nodeSelector` field is used to schedule the Velero pod, in the future the following mechanism could be added to extend the Velero pod configuration:

The `spec.configuration.velero.loadAffinity` section will be used to schedule the Velero pod. This will be translated to the `affinity.nodeAffinity` from the `v1.Pod` CRD. The decision to use `spec.configuration.velero.loadAffinity` instead of `spec.configuration.velero.podConfig.affinity.nodeAffinity` was made to allow the user to specify the node affinity settings for the Velero pod in a similar way as it is done for the **NodeAgent**, **DataMover** pods and the repository maintenance job. This will translate to the `requiredDuringSchedulingIgnoredDuringExecution` fields from the `v1.Pod` CRD.

The use of `spec.configuration.velero.loadAffinity` will **NOT** be validated against the `spec.configuration.velero.podConfig.nodeSelector` field. This is not required restriction, because the `spec.configuration.velero.loadAffinity` is not used only to schedule the Velero pod and no dependent workloads are scheduled using the **ConfigMap** mechanism.

### Velero `spec.configuration.velero.loadAffinity` field
The `spec.configuration.velero.loadAffinity` will be added to the DPA CRD.
The updated schema will be as follows:

```yaml
apiVersion: oadp.openshift.io/v1alpha1
kind: DataProtectionApplication
metadata:
  name: <dpa_sample>
spec:
...
  configuration:
    velero:
      loadAffinity:
        nodeSelector:
          matchLabels:
            kubernetes.io/hostname: node3
          matchExpressions:
            - key: node-role.kubernetes.io/backup
              operator: In
              values:
                - true
``` 


## Alternatives Considered
1. Keeping the Current Structure
   - Pros: No changes required.
   - Cons: Lack of consistency, limited flexibility in node selection or pod affinity/anti-affinity.

## Security Considerations
This change does not introduce new security risks.

> **Note:**  
> Improper scheduling configurations could lead to unintended pod placements, potentially exposing sensitive workloads to unauthorized nodes.  
> It is recommended to carefully validate node selector settings to avoid security or performance issues.

## Compatibility
The proposal maintains backward compatibility by supporting the existing structures.
Existing configurations that use only matchLabels will continue to function without modification. In the future API versions we may want to deprecate those.