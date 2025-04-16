# Advanced Node Agent and Workload Configuration User Guide for the OADP components configurable in the Data Protection Application (DPA) CRD

This guide is intended for users configuring node selection, affinity, anti-affinity, and concurrency features in the OADP components configurable in the Data Protection Application (DPA) CRD. It includes examples and step-by-step instructions.

The following components can be scheduled on a specific nodes:
 - [NodeAgent and DataMover pods](#1-configuring-node-agent-with-current-and-new-features)
 - [Repository Maintenance jobs](#2-configuring-repository-maintenance-new-in-oadp-15)
 - [Velero pod](#3-configuring-velero-pod-with-podconfig-and-affinity)

---

## 1. Configuring Node-Agent with Current and New Features

NodeAgent can be scheduled on specific nodes using `podConfig.nodeSelector` (existing feature), and further refined using `loadAffinity` and `loadConcurrency` (new in OADP 1.5 feature).

Additional configuration for the DataMover workload is available under the `nodeAgent.podResources` field.

### a. Set NodeAgent NodeSelector

The following example ensures NodeAgent pods are scheduled only on nodes with the label *`label.io/role: cpu-1`* AND *`other-label.io/other-role: cpu-2`*. The logical operator in the `podConfig.nodeSelector` is always `AND`.
```yaml
spec:
  configuration:
    nodeAgent:
      enable: true
      podConfig:
        nodeSelector:
        label.io/role: cpu-1
        other-label.io/other-role: cpu-2
```

### b. Configure NodeAgent Load Affinity (New in OADP 1.5)

To restrict NodeAgent pods (and indirectly the DataMover pods) further using matchExpressions. The following example ensures NodeAgent pods are scheduled only on nodes with the label *`label.io/role: cpu-1`* AND the *`label.io/hostname`* is *`node1`* OR *`node2`*.

For more details on the `loadAffinity` feature see the [Understanding node affinity ](https://docs.redhat.com/en/documentation/openshift_container_platform/latest/html/nodes/controlling-pod-placement-onto-nodes-scheduling#nodes-scheduler-node-affinity-about_nodes-scheduler-node-affinity).

> **Note:** In the DPA CRD specification the above OpenShift `nodeAffinity` is defined as `loadAffinity`.

```yaml
spec:
  configuration:
    nodeAgent:
      enable: true
      loadAffinity:
        - nodeSelector:
            matchLabels:
              label.io/role: cpu-1
            matchExpressions:
              - key: label.io/hostname
                operator: In
                values:
                  - node1
                  - node2
```

#### Rules for NodeAgent Configuration

- For simple node matching it is perfectly fine to use `podConfig.nodeSelector`.

- For more complex scenarios it is **recommended** to use `loadAffinity.nodeSelector` without `podConfig.nodeSelector` to avoid confusion.

- You **can** use both `podConfig.nodeSelector` and `loadAffinity.nodeSelector`, but `loadAffinity` must be equal to or more restrictive. The `podConfig.nodeSelector` labels must be a subset of the labels used in `loadAffinity.nodeSelector`. 
  
- You **cannot** use `matchExpressions` and `matchLabels` in case both `podConfig.nodeSelector` and `loadAffinity.nodeSelector` are defined.

- Example Configuration with both `podConfig.nodeSelector` and `loadAffinity.nodeSelector`:
    ```yaml
    spec:
      configuration:
        nodeAgent:
          enable: true
          loadAffinity:
            - nodeSelector:
                matchLabels:
                  label.io/location: 'US'
                  label.io/gpu: 'no'
          podConfig:
            nodeSelector:
              label.io/gpu: 'no'
    ```


### c. Configure NodeAgent Load Concurrency (New in OADP 1.5)

This controls how many concurrent number of node-agent loads per node are allowed. Use `globalConfig` for all nodes, or `perNodeConfig` with selectors.

Please refer to the Velero documentation for more details on the `loadConcurrency` feature: [Node Agent Concurrency](https://velero.io/docs/main/node-agent-concurrency/).

Global configuration is applied to all nodes. Per-node configuration is applied to the nodes that match the `nodeSelector`.

The `nodeSelector` uses the same syntax for selecting nodes as `loadAffinity.nodeSelector` and it may be used in combination with `matchExpressions` for advanced node selection.

```yaml
spec:
  configuration:
    nodeAgent:
      enable: true
      loadConcurrency:
        globalConfig: 2
        perNodeConfig:
          - nodeSelector:
              matchLabels:
                label.io/gpu: 'no'
              matchExpressions:
                - key: label.io/location
                  operator: In
                  values:
                    - US
                    - EU
            number: 1
          - nodeSelector:
              matchLabels:
                label.io/gpu: 'no'
              matchExpressions:
                - key: label.io/location
                  operator: NotIn
                  values:
                    - US
                    - EU
            number: 3
```

> Note: This creates a ConfigMap used by the NodeAgent to control concurrency settings.

### d. Configure DataMover Pod Resource (New in OADP 1.5)

The `nodeAgent.podResources` field controls the resource requests and limits for the DataMover pod.

> **Note:** This will **not** affect the NodeAgent pod resources.

Please refer to the Velero documentation for more details on the `nodeAgent.podResources` feature: [Data Movement Pod Resource Configuration](https://velero.io/docs/main/data-movement-pod-resource-configuration/).

```yaml
spec:
  configuration:
    nodeAgent:
      enable: true
      podResources:
        cpuLimit: "2"
        cpuRequest: "1"
        memoryLimit: "2Gi"
        memoryRequest: "1Gi"
```

---

## 2. Configuring Repository Maintenance (New in OADP 1.5)

Repository Maintenance is a background job that can be configured independently of NodeAgent.

This maintenance job affinity settings applies only to the Repository Maintenance when kopia is used as the repository.

The Repository Maintenance pod can be scheduled on the node where the NodeAgent is or isn't running.

For more details on the repository maintenance affinity settings see the Velero documentation: [Repository Maintenance](https://velero.io/docs/main/repository-maintenance/).

### a. Configure Affinity for All Repositories (Global)

```yaml
spec:
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
              matchLabels:
                label.io/gpu: 'no'
              matchExpressions:
                - key: label.io/location
                  operator: In
                  values:
                    - US
                    - EU
```

### b. Configure Per-Repository Affinity

Together with global configuration you can specify different affinity settings for each repository (by name, namespace, or type):

```yaml
spec:
  configuration:
    repositoryMaintenance:
      myrepositoryname:
        loadAffinity:
          - nodeSelector:
              matchLabels:
                label.io/cpu: 'yes'
```

> Note: The NodeAgent does **not** need to run on the same node as repository maintenance jobs.

---

## 3. Configuring Velero Pod with podConfig and Affinity

You can define where the Velero pod is scheduled using `podConfig.nodeSelector` and `velero.loadAffinity`.

There is always one Velero pod per OADP deployment and it's main purpose is to schedule Velero workloads, which is not very resource intensive operation in comparison to the other workloads.

You can still use the `podConfig.nodeSelector` to assign Velero to specific nodes, however you can also configure `velero.loadAffinity` for pod-level Affinity and Anti-Affinity. The scheduling is done by the OpenShift scheduler and the rules are applied to the Velero pod deployment.

Please refer to the OpenShift documentation for more details on the `loadAffinity` feature: [Understanding node affinity](https://docs.redhat.com/en/documentation/openshift_container_platform/latest/html/nodes/controlling-pod-placement-onto-nodes-scheduling#nodes-scheduler-node-affinity-about_nodes-scheduler-node-affinity).

> **Note**: The `velero.loadAffinity` name is different then OpenShift pod placement `affinity.nodeAffinity`, but it behaves the same way.

### a. Set Velero NodeSelector

```yaml
spec:
  configuration:
    velero:
      podConfig:
        nodeSelector:
          some-label.io/custom-node-role: backup-core
```

### b. Configure Velero Load Affinity (New in OADP 1.5)

```yaml
spec:
  configuration:
    velero:
      loadAffinity:
        - nodeSelector:
            matchLabels:
              label.io/gpu: 'no'
            matchExpressions:
              - key: label.io/location
                operator: In
                values:
                  - US
                  - EU
```

