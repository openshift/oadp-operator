<hr style="height:1px;border:none;color:#333;">
<h1 align="center">Pod Configuration (podConfig)</h1>
<hr style="height:1px;border:none;color:#333;">

The `podConfig` field is available under both `configuration.velero` and `configuration.nodeAgent` in the DataProtectionApplication (DPA) CR. It allows customizing the pod scheduling and runtime configuration for Velero Deployment pods and Node Agent DaemonSet pods respectively.

See also the [Red Hat PodConfig API reference](https://docs.redhat.com/en/documentation/openshift_container_platform/4.18/html/backup_and_restore/oadp-application-backup-and-restore#podconfig-type_oadp-api) for the official supported documentation.

All available `podConfig` fields can be discovered from your cluster using:

```
oc explain dataprotectionapplication.spec.configuration.velero.podConfig
oc explain dataprotectionapplication.spec.configuration.nodeAgent.podConfig
```

The full CRD schema can also be inspected with:

```
oc get crd dataprotectionapplications.oadp.openshift.io -o yaml
```

### Available Fields

| Field | Type | Description |
|-------|------|-------------|
| `tolerations` | `[]corev1.Toleration` | Tolerations to apply to the pod, allowing scheduling on tainted nodes |
| `nodeSelector` | `map[string]string` | Node labels required for pod scheduling |
| `resourceAllocations` | `corev1.ResourceRequirements` | CPU, memory, and ephemeral-storage resource requests and limits |
| `labels` | `map[string]string` | Additional labels to add to pods |
| `env` | `[]corev1.EnvVar` | Environment variables to add to the pod |

### Tolerations

To schedule Velero or Node Agent pods on nodes with taints, configure tolerations under `podConfig`.

This is commonly needed when:
- Critical workloads run on tainted nodes and Node Agent must be present for PVC backup
- Infrastructure nodes have taints that prevent general workload scheduling

**Example: Node Agent on tainted nodes**

If your cluster has nodes with taint `critical=reserved:NoSchedule` and you need Node Agent pods to run there for PVC data backup:

```yaml
apiVersion: oadp.openshift.io/v1alpha1
kind: DataProtectionApplication
metadata:
  name: dpa-sample
  namespace: openshift-adp
spec:
  configuration:
    nodeAgent:
      enable: true
      uploaderType: kopia
      podConfig:
        tolerations:
          - key: "critical"
            operator: "Equal"
            value: "reserved"
            effect: "NoSchedule"
    velero:
      defaultPlugins:
        - openshift
        - csi
  backupLocations:
    - velero:
        provider: aws
        default: true
        objectStorage:
          bucket: my-bucket
          prefix: my-prefix
        config:
          region: us-east-1
        credential:
          name: cloud-credentials
          key: cloud
```

**Example: Velero on tainted nodes**

```yaml
  configuration:
    velero:
      podConfig:
        tolerations:
          - key: "node-role.kubernetes.io/infra"
            operator: "Exists"
            effect: "NoSchedule"
```

<b>Note:</b> Tolerations must be placed under `podConfig`, not directly under `nodeAgent` or `velero`. For example, `spec.configuration.nodeAgent.tolerations` is **not valid** and will be rejected by CRD validation. The correct path is `spec.configuration.nodeAgent.podConfig.tolerations`.

### Node Selector

To restrict Velero or Node Agent pods to specific nodes, use `nodeSelector` under `podConfig`.

```yaml
  configuration:
    velero:
      podConfig:
        nodeSelector:
          node-role.kubernetes.io/infra: ""
    nodeAgent:
      enable: true
      uploaderType: kopia
      podConfig:
        nodeSelector:
          node-role.kubernetes.io/infra: ""
```

<b>Note:</b> Like tolerations, `nodeSelector` must be placed under `podConfig`. The path `spec.configuration.nodeAgent.nodeSelector` is **not valid**.

### Resource Allocations

See [resource_req_limits.md](resource_req_limits.md) for detailed documentation on setting CPU, memory, and ephemeral-storage resource requests and limits.

### Labels

To add custom labels to Velero or Node Agent pods:

```yaml
  configuration:
    velero:
      podConfig:
        labels:
          app.kubernetes.io/part-of: "backup-system"
    nodeAgent:
      enable: true
      uploaderType: kopia
      podConfig:
        labels:
          app.kubernetes.io/part-of: "backup-system"
```

### Environment Variables

To set environment variables on Velero or Node Agent pods:

```yaml
  configuration:
    velero:
      podConfig:
        env:
          - name: HTTP_PROXY
            value: "http://proxy.example.com:8080"
          - name: HTTPS_PROXY
            value: "http://proxy.example.com:8080"
          - name: NO_PROXY
            value: ".cluster.local,.svc,10.0.0.0/8"
```

