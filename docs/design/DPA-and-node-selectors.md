# Node Selector Configuration for Data Protection Components

## Abstract
This proposal introduces a standardized approach for configuring node selectors in the DataProtectionApplication (DPA) custom resource for various use cases that are common across different Velero settings.

## Background
Currently, node selection for Velero and NodeAgent components is defined in an inconsistent manner within the DPA specification.
For example, `nodeAgent.podConfig.nodeSelector` specifies nodes using matchLabels, while `loadConcurrency.perNodeConfig` allows for both matchLabels and matchExpressions.
Additionally, `velero.podConfig.nodeSelector` is not explicitly defined, leading to ambiguity.

A unified structure for node selector configuration will provide clarity and consistency, allowing users to specify node selection criteria effectively.
This will improve usability, maintainability, and alignment with Kubernetes best practices.

## Goals
- Define a standard way to configure node selectors for Velero and NodeAgent components in the DPA specification.
- Provide a clear and structured approach for users to define node selection criteria.
- Ensure backwards compatibility with existing configurations.
- Allow for both matchLabels and matchExpressions for flexibility in node selection.
- Ensure pod affinity, anti-affinity are respected.
- Ensure the DataMover pods are scheduled on the same nodes as node-agent pods.
- Deprecation warnings for the existing node selector configuration.

## Non Goals
- Removal of the existing node selector configuration.


## High-Level Design
The proposed change introduces a unified nodeSelector structure for all relevant components within the DPA custom resource.
This will allow users to specify node selection using either matchLabels or matchExpressions, ensuring flexibility while maintaining consistency across configurations.

The modified structure will be applied to the new DPA sections:

- `configuration.nodeAgent.loadConcurrency`
- `configuration.nodeAgent.loadAffinity`
- `configuration.velero.loadAffinity`
- `configuration.repositoryMaintenance`

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

          ## In the new design we will add more fields such as matchExpressions and matchLabels and deprecate the existing nodeAgent fields.
          ## 
```
> Note:
>
> There is also `restic.podConfig`, however it's deprecated in the OADP 1.5, so it's not included in this design.


## Changes to existing configuration settings

In the examples below each `operator` is a logical operator for OCP to use when interpreting the rules.
Possible operators are:
- In
- NotIn
- Exists
- DoesNotExist
- Gt
- Lt

### Changes to current `nodeAgent.podConfig.nodeSelector` fields

No changes will be made to the current `nodeAgent.podConfig.nodeSelector` CRD field.
The following logic will be implemented on the reconcile:
- Use of the `nodeAgent.podConfig.nodeSelector` will print deprecation warning to the user with the recommendation to use `nodeAgent.loadAffinity` settings.
- Use of `nodeAgent.podConfig.nodeSelector` and `nodeAgent.loadAffinity` at the same time will cause reconcile error with `BackingOff` state and explanation that those two cannot be used at the same time.

#### New settings specific to the `loadConcurrency` will be added to the nodeAgent section.
The `loadConcurrency` section is used to configure the number of concurrent backups that can be performed on a single node.

The `loadConcurrency` section is explained in the upstream Velero documentation: https://velero.io/docs/main/node-agent-concurrency/

This section is not validated against the global `nodeSelector` field in which the user can specify the nodeSelector for the node-agent pod.
If it matches the loadConcurrency settings will be applied to that node-agent pod.

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

#### New settings specific to the `loadAffinity` will be added to the nodeAgent section.

The `loadAffinity` section is used to configure the node-agent pod to run on nodes that match the specified label selector.

The `loadAffinity` section is explained in the upstream Velero documentation: https://velero.io/docs/main/data-movement-backup-node-selection/

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

### Changes to current `velero.podConfig.nodeSelector` fields

No changes will be made to the current `velero.podConfig.nodeSelector` CRD field.
The following logic will be implemented on the reconcile:
- Use of the `velero.podConfig.nodeSelector` will print deprecation warning to the suer with the recommendation to use `velero.loadAffinity` settings.
- Use of `velero.podConfig.nodeSelector` and `velero.loadAffinity` at the same time will cause reconcile error with `BackingOff` state and explanation that those two cannot be used at the same time.

### Velero `podConfig.loadAffinity` field
The nodeSelector field will be updated to support both matchLabels and matchExpressions.
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
The proposal maintains backward compatibility by supporting the existing structures with deprecation warnings.
Existing configurations that use only matchLabels will continue to function without modification. In the future API versions we may want to deprecate those.