Based on the [migtools/oadp-non-admin](https://github.com/migtools/oadp-non-admin) repository, OADP's self-service feature is implemented through the **OADP Non Admin Controller (NAC)**.

## Brief Description

The OADP self-service feature enables **non-cluster-administrative users** to perform backup and restore operations within their designated namespaces, while maintaining cluster security and administrative control.

### Key Capabilities:

- **Non-Admin Backup/Restore**: Regular users can create `NonAdminBackup` and `NonAdminRestore` resources to protect and recover their applications without requiring cluster admin privileges.

- **Admin-Controlled Access**: Cluster administrators configure which namespaces non-admin users can backup/restore through the OADP operator's DPA (DataProtectionApplication) configuration.

- **Policy Enforcement**: Admins can enforce company policies by using templated configurations that require specific field values and restrict access to cluster-scoped resources.

- **Automatic Security Restrictions**: The system automatically excludes sensitive cluster-scoped objects (SCCs, ClusterRoles, CRDs, etc.) from non-admin backup/restore operations.

### Workflow:
1. **Admin Setup**: Configure DPA with non-admin feature enabled and set enforcement policies
2. **User Self-Service**: Non-admin users create their own backup/restore operations using `NonAdminBackup` and `NonAdminRestore` CRDs
3. **Controlled Access**: Users can only backup/restore within their permitted namespaces with admin-defined constraints

This feature requires **OADP operator version 1.5+** and provides a secure way to democratize backup/restore operations while maintaining enterprise governance and security controls.

## Troubleshooting

### Common Issues and Solutions

**For more detailed information about non-admin user constraints, see the [OADP Non-Admin README](https://github.com/migtools/oadp-non-admin?tab=readme-ov-file#notes-on-non-admin-permissions-and-enforcements).**

#### Issue: Unable to retrieve backup logs as a non-admin user

**Problem**: As a non-admin user, I cannot access the logs of my backup.

**Solution**: Non-admin users should use a `NonAdminBackupStorageLocation` (NABSL) when creating a `NonAdminBackup` (NAB). Non-admin users do not have permission to access logs directly from the underlying Backup Storage Location for security reasons.