# NetworkPolicies for OADP (OpenShift API for Data Protection)

This directory contains NetworkPolicy configurations for securing OADP components in your Kubernetes cluster. NetworkPolicies provide a way to control network traffic flow between pods and other network endpoints at the application layer.

## What are NetworkPolicies?

NetworkPolicies are Kubernetes resources that define how groups of pods are allowed to communicate with each other and other network endpoints. They work at the IP address or port level (OSI layer 3 or 4) and provide a way to implement network segmentation and micro-segmentation within your cluster.

As described in the [Kubernetes documentation](https://kubernetes.io/docs/concepts/services-networking/network-policies/), NetworkPolicies use labels to select pods and define rules that specify what traffic is allowed to and from those pods.

## Prerequisites

- Your cluster must be using a Container Network Interface (CNI) plugin that supports NetworkPolicies (such as Calico, Cilium, Weave Net, or others)
- NetworkPolicies are namespaced resources and only affect pods in the same namespace

## OADP NetworkPolicy Configuration

The `network_policy.yaml` file in this directory defines a NetworkPolicy specifically designed for OADP components. Here's what it does:

### Policy Overview

```yaml
name: default-deny-labelled
```

This policy implements a **default-deny approach** for pods managed by the OADP operator, meaning that by default, all network traffic is blocked except for explicitly allowed connections.

### Pod Selection

```yaml
podSelector:
  matchLabels:
    app.kubernetes.io/managed-by: "oadp-operator"
```

The policy applies to all pods that have the label `app.kubernetes.io/managed-by: "oadp-operator"`. This typically includes:
- Velero pods
- OADP operator pods
- Other OADP-related components

### Traffic Rules

#### Ingress Rules (Incoming Traffic)

```yaml
ingress:
- from:
  - podSelector: {}
```

**What this allows:**
- Allows incoming traffic from **any pod within the same namespace**
- This enables inter-pod communication for OADP components that need to communicate with each other

#### Egress Rules (Outgoing Traffic)

The policy defines two egress rules:

**1. HTTPS Internet Access**
```yaml
- to:
    - ipBlock:
        cidr: 0.0.0.0/0
  ports:
    - protocol: TCP
      port: 443
```

**What this allows:**
- Outbound HTTPS traffic (port 443) to any IP address on the internet
- Essential for OADP components to communicate with cloud storage providers (AWS S3, Azure Blob, Google Cloud Storage, etc.)
- Allows downloading container images and accessing external APIs

**2. DNS Resolution**
```yaml
- to:
  - namespaceSelector:
      matchLabels:
        kubernetes.io/metadata.name: openshift-dns
  ports:
  - protocol: UDP
    port: 53
  - protocol: TCP
    port: 53
```

**What this allows:**
- DNS queries to the OpenShift DNS service (both UDP and TCP on port 53)
- Essential for resolving domain names for cloud storage endpoints and other external services
- Uses the standard OpenShift DNS namespace label

## Why These Rules Matter for OADP

### Security Benefits

1. **Principle of Least Privilege**: Only allows the minimum network access required for OADP to function
2. **Attack Surface Reduction**: Limits potential network-based attacks on OADP components
3. **Compliance**: Helps meet security compliance requirements for network segmentation

### OADP-Specific Requirements

OADP components need specific network access to function properly:

- **Cloud Storage Access**: HTTPS (443) for communicating with cloud storage APIs
- **DNS Resolution**: Required to resolve cloud storage endpoint names
- **Inter-Component Communication**: OADP pods may need to communicate with each other within the namespace

## Applying the NetworkPolicy

To apply this NetworkPolicy to your cluster:

```bash
kubectl apply -f network_policy.yaml -n <oadp-namespace>
```

Replace `<oadp-namespace>` with the namespace where OADP is installed (typically `openshift-adp`).

## Monitoring and Troubleshooting

### Verifying the Policy

Check if the NetworkPolicy is applied:
```bash
kubectl get networkpolicy -n <oadp-namespace>
kubectl describe networkpolicy default-deny-labelled -n <oadp-namespace>
```

### Common Issues

1. **DNS Resolution Failures**: If OADP components can't resolve domain names, verify the DNS egress rule
2. **Cloud Storage Connection Issues**: Ensure the HTTPS egress rule allows traffic to your storage provider
3. **Inter-Pod Communication Problems**: Check that pods have the correct labels and the ingress rule is configured properly

### Debugging Network Connectivity

If you suspect NetworkPolicy issues:

1. Check pod labels:
   ```bash
   kubectl get pods -n <oadp-namespace> --show-labels
   ```

2. Test connectivity from within a pod:
   ```bash
   kubectl exec -it <pod-name> -n <oadp-namespace> -- nslookup <domain-name>
   kubectl exec -it <pod-name> -n <oadp-namespace> -- curl -I https://<endpoint>
   ```

## Customizing the Policy

You may need to modify the NetworkPolicy based on your specific requirements:

### Adding Additional Egress Rules

If your OADP setup requires access to additional services, add more egress rules:

```yaml
egress:
- to:
  - ipBlock:
      cidr: 10.0.0.0/8  # Private network range
  ports:
  - protocol: TCP
    port: 9000  # MinIO or other S3-compatible storage
```

### Restricting Cloud Access

To limit access to specific cloud provider IP ranges instead of allowing all internet traffic:

```yaml
egress:
- to:
  - ipBlock:
      cidr: 52.219.0.0/16  # AWS S3 IP range example
  ports:
  - protocol: TCP
    port: 443
```

## Important Considerations

### Pod Lifecycle

As noted in the [Kubernetes documentation](https://kubernetes.io/docs/concepts/services-networking/network-policies/), when a NetworkPolicy is first applied, there may be a brief period where pods are started without full network isolation. OADP pods should be resilient to temporary network connectivity issues during startup.

### CNI Plugin Compatibility

Ensure your CNI plugin fully supports NetworkPolicies. Some features (like `endPort` ranges) may not be supported by all plugins.

### Default Behavior

Remember that NetworkPolicies implement a "default deny" model. If no NetworkPolicy selects a pod, all traffic is allowed. Once a pod is selected by any NetworkPolicy, only traffic explicitly allowed by those policies will be permitted.

## Additional Resources

- [Kubernetes NetworkPolicy Documentation](https://kubernetes.io/docs/concepts/services-networking/network-policies/)
- [OADP Documentation](https://docs.openshift.com/container-platform/latest/backup_and_restore/application_backup_and_restore/oadp-features-plugins.html)
- [OpenShift Network Security](https://docs.openshift.com/container-platform/latest/networking/network_policy/about-network-policy.html)

## Contributing

When modifying NetworkPolicies for OADP:

1. Test thoroughly in a development environment
2. Verify that all OADP functionality continues to work
3. Document any changes and the reasoning behind them
4. Consider the security implications of any new rules
