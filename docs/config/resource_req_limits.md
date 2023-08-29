<hr style="height:1px;border:none;color:#333;">
<h1 align="center">Resource Requests and Limits Customization</h1>
<hr style="height:1px;border:none;color:#333;">

### Setting resource limits and requests for Velero and Node Agent Pods

In order to set specific resource(cpu, memory) `limits` and `requests` for the 
Velero pod, you need use the `configuration.velero.podConfig.resourceAllocations` specification field in 
the `oadp_v1alpha1_dpa.yaml` file during the deployment.

For instance, the `configuration.velero.podConfig.resourceAllocations` can look somewhat similar to:

```
  configuration:
    velero:
      podConfig:
        resourceAllocations:
          limits:
            cpu: "2"
            memory: 512Mi
          requests:
            cpu: 500m
            memory: 256Mi
```

Similarly, you can use the `configuration.nodeAgent.podConfig.resourceAllocations` specification field for 
setting specific resource `limits` and `requests` for the Node Agent pods.

```
  configuration:
    nodeAgent:
    [...]
      podConfig:
        resourceAllocations:
          limits:
            cpu: "2"
            memory: 512Mi
          requests:
            cpu: 500m
            memory: 256Mi
```

<b>Note:</b> 
- The values for the resource requests and limits flags follow the same format 
as [Kubernetes resource requirements](https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/)
- Also, if the `configuration.velero.podConfig.resourceAllocations` / `configuration.nodeAgent.podConfig.resourceAllocations` is not 
defined by the user, then the default resources specification for Velero/Node Agent 
pod(s) is:

  ```
  resources:
    requests:
      cpu: 500m
      memory: 128Mi
  ```

This differs from upstream Velero/Node Agent pod(s) in that the default resources which [has resource limits as well as resource requests](https://velero.io/docs/v1.9/customize-installation/#customize-resource-requests-and-limits).
