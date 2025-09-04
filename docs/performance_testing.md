# OADP Performance Testing

This document provides guidance on performance testing OADP (OpenShift API for Data Protection) with Velero using comprehensive testing tools and methodologies.

## Overview

OADP performance testing is critical for understanding backup and restore behavior at scale. The OADP team has developed a comprehensive performance testing toolkit that uses industry-standard tools to simulate realistic workloads and measure Velero's performance characteristics.

## Performance Testing Toolkit

The OADP team maintains a dedicated performance testing repository that provides:

- **Automated test scripts** for creating large-scale Kubernetes objects (30k-300k)
- **Velero backup/restore performance testing** with detailed analysis
- **Industry-standard tooling** using [kube-burner](https://github.com/kube-burner/kube-burner) for efficient object creation
- **Comprehensive documentation** and usage guides
- **Performance analysis scripts** for identifying bottlenecks

### Repository Access

**GitHub Repository**: [https://github.com/shubham-pampattiwar/velero-performance-testing](https://github.com/shubham-pampattiwar/velero-performance-testing)

The repository contains everything needed for comprehensive OADP performance testing, including:
- Pre-configured test scenarios (30k and 300k objects)
- Velero installation and setup scripts
- Performance analysis tools
- Detailed documentation

## Quick Start Guide

### Prerequisites

- OpenShift/Kubernetes cluster with sufficient resources
- [kube-burner](https://github.com/kube-burner/kube-burner) installed
- Cluster-admin privileges
- OADP operator installed and configured

### Basic Performance Test Workflow

1. **Clone the performance testing repository**:
   ```bash
   git clone https://github.com/shubham-pampattiwar/velero-performance-testing.git
   cd velero-performance-testing
   ```

2. **Run a simple test** (30k objects):
   ```bash
   ./scripts/run-simple-test.sh
   ```

3. **Test OADP backup performance**:
   ```bash
   ./velero/backup-performance-test.sh
   ```

4. **Analyze results**:
   ```bash
   ./velero/analyze-performance.sh <backup-name>
   ```

5. **Clean up**:
   ```bash
   ./scripts/cleanup-simple.sh
   ```

### Large-Scale Testing

For enterprise-scale testing with 300k objects:

```bash
# Create large-scale test objects
./scripts/run-large-scale-test.sh

# Test backup performance
./velero/backup-performance-test.sh

# Analyze and cleanup
./velero/analyze-performance.sh <backup-name>
./scripts/cleanup-large-scale.sh
```

### Performance Analysis

Use the toolkit's analysis scripts to identify bottlenecks:
```bash
# Detailed performance analysis
./velero/analyze-performance.sh <backup-name>

# Check resource utilization
kubectl top nodes
kubectl top pods -n openshift-adp-operator
```

## Best Practices

### Testing Guidelines

1. **Start small**: Begin with 30k object tests before attempting large-scale tests
2. **Monitor resources**: Keep an eye on cluster resource utilization
3. **Test incrementally**: Gradually increase object counts to find limits
4. **Document results**: Track performance metrics across different configurations

### Production Considerations

1. **Test in staging**: Never run large-scale performance tests in production
2. **Resource planning**: Ensure sufficient cluster resources before testing
3. **Backup windows**: Plan backup windows based on performance test results
4. **Monitoring**: Implement monitoring based on performance testing insights

## Support and Contributing

For questions about performance testing:
1. Review the [performance testing repository documentation](https://github.com/shubham-pampattiwar/velero-performance-testing)
2. Check existing [OADP issues](https://github.com/openshift/oadp-operator/issues)
3. Contribute improvements to the performance testing toolkit

## Performance Testing Repository Structure

The external repository includes:
- **Automated scripts** for object creation and cleanup
- **Velero integration** scripts for backup/restore testing
- **Performance analysis** tools and reports
- **Multiple test scenarios** (30k, 300k objects)
- **Comprehensive documentation** with troubleshooting guides

For complete usage instructions, refer to the [Velero Performance Testing Repository](https://github.com/shubham-pampattiwar/velero-performance-testing).