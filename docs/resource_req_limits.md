***
## Resource requests and limits customization
***

### Setting resource limits and requests for Velero and Restic Pods

In order to set specific resource(cpu, memory) `limits` and `requests` for the Velero pod, you need use the `velero_resource_allocation` specification field in the `konveyor.openshift.io_v1alpha1_velero_cr.yaml` file during the deployment.

For instance, the `velero_resource_allocation` can look somewhat similar to:
```
velero_resource_allocation:
  limits:
    cpu: "2"
    memory: 512Mi
  requests:
    cpu: 500m
    memory: 256Mi
```

Similarly, you can use the `restic_resource_allocation` specification field for setting specific resource `limits` and `requests` for the Restic pods.

```
restic_resource_allocation:
  limits:
    cpu: "2"
    memory: 512Mi
  requests:
    cpu: 500m
    memory: 256Mi
```

<b>Note:</b> 
- The values for the resource requests and limits flags follow the same format as [Kubernetes resource requirements](https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/)
- Also, if the `velero_resource_allocation`/`restic_resource_allocation` is not defined by the user then the default resources specification for Velero/Restic pod(s) is 
  ```
  resources:
    limits:
      cpu: "1"
      memory: 256Mi
    requests:
      cpu: 500m
      memory: 128Mi
  ```
