<hr style="height:1px;border:none;color:#333;">
<h1 align="center">Resource Requests and Limits Customization</h1>
<hr style="height:1px;border:none;color:#333;">

### Setting resource limits and requests for Velero and Restic Pods

In order to set specific resource(cpu, memory) `limits` and `requests` for the 
Velero pod, you need use the `veleroResourceAllocation` specification field in 
the `oadp_v1alpha1_velero_cr.yaml` file during the deployment.

For instance, the `veleroResourceAllocation` can look somewhat similar to:

```
veleroResourceAllocation:
  limits:
    cpu: "2"
    memory: 512Mi
  requests:
    cpu: 500m
    memory: 256Mi
```

Similarly, you can use the `resticResourceAllocation` specification field for 
setting specific resource `limits` and `requests` for the Restic pods.

```
resticResourceAllocation:
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
- Also, if the `veleroResourceAllocation` / `resticResourceAllocation` is not 
defined by the user, then the default resources specification for Velero/Restic 
pod(s) is:

  ```
  resources:
    limits:
      cpu: "1"
      memory: 256Mi
    requests:
      cpu: 500m
      memory: 128Mi
  ```
