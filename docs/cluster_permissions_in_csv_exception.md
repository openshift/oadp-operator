# **Knowledge Base Article: Why the OADP Operator Pod Requires `clusterPermissions` in the ClusterServiceVersion (CSV)**

---

## **Rationale for Allowing `clusterPermissions` in the ClusterServiceVersion (CSV)**

The **OADP Operator** specifies `clusterPermissions` in its **ClusterServiceVersion (CSV)** to obtain the required access for managing both namespaced and cluster-scoped resources, and for interacting with the Kubernetes API server properly.

---

## **Why the OADP Operator Requires Cluster-Level Access**

The OADP Operator, which relies on Velero, manages backup and restore operations that involve not only namespace-scoped resources but also cluster-wide components. These include:

- `ClusterRoles`
- `ClusterRoleBindings`
- `PersistentVolumes`
- `StorageClasses`
- `VolumeSnapshotClasses`
- `CustomResourceDefinitions (CRDs)`
- And more

To perform these operations, both the OADP Operator and Velero must interact with cluster-scoped Kubernetes resources. These interactions—such as managing `ClusterRoles`, `PersistentVolumes`, and CRDs—require access to cluster-level API groups.

Additionally, when configured to back up volume data without CSI snapshotting, elevated privileges are needed to access the underlying storage infrastructure and copy the volume data to the backup location.

---

## **Why `clusterPermissions` Are Essential**

### **1. Ensuring Proper Access Control**
- `clusterPermissions` explicitly define the Kubernetes API groups, verbs, and resources the operator can access.
- This helps enforce the principle of least privilege by limiting access to only those permissions necessary for functionality.

### **2. Facilitating Seamless Operator Functionality**
- When declared in the CSV, `clusterPermissions` are granted automatically by OLM (Operator Lifecycle Manager) during installation.
- This avoids the need for manual RBAC configuration and ensures backup and restore operations can run immediately and correctly.

### **3. Compliance with Kubernetes Security Best Practices**
- Declaring `clusterPermissions` within the CSV is a best practice for any operator that needs cluster-scoped access.
- It increases transparency, simplifies audits, and ensures security requirements are met consistently.

---

## **Required RBAC Permissions**

The following service accounts and permissions are necessary for full OADP functionality:

### `non-admin-controller` Service Account

Used for the **NonAdminBackup** feature.

- **Core APIs**:
  - `namespaces`: get, list, watch
  - `secrets`: create, delete, get, list, patch, update, watch

- **Custom Resources (`oadp.openshift.io`)**:
  - `dataprotectionapplications`: list
  - `nonadminbackups`, `nonadminbackupstoragelocationrequests`, `nonadminbackupstoragelocations`, `nonadmindownloadrequests`, `nonadminrestores`: full access
  - Finalizers: update
  - Status subresources: get, patch, update

- **Velero (`velero.io`)**:
  - `backups`, `backupstoragelocations`, `deletebackuprequests`, `downloadrequests`, `restores`: full access
  - `backupstoragelocations/status`, `downloadrequests/status`: get, patch, update
  - `datadownloads`, `datauploads`, `podvolumebackups`, `podvolumerestores`: get, list, watch

---

### `openshift-adp-controller-manager` Service Account

Used by the OADP Operator controller.

- **Core & Kubernetes APIs**:
  - `configmaps`, `endpoints`, `events`, `persistentvolumeclaims`, `pods`, `secrets`, `services`, `serviceaccounts`, `namespaces`: full access
  - `apps/daemonsets`, `apps/deployments`: full access
  - `coordination.k8s.io/secrets`: full access
  - `authentication.k8s.io/tokenreviews`, `authorization.k8s.io/subjectaccessreviews`: create

- **OpenShift-specific**:
  - `cloudcredential.openshift.io/credentialsrequests`: create, get, update
  - `config.openshift.io/infrastructures`: get, list, watch
  - `security.openshift.io/securitycontextconstraints`: full access
  - Use of `privileged` SCC: use
  - `route.openshift.io/routes`: full access

- **OADP CRDs**:
  - `cloudstorages`, `dataprotectionapplications`, `dataprotectiontests`: full access
  - Finalizers and statuses: update

- **Monitoring**:
  - `monitoring.coreos.com/servicemonitors`: full access

- **Velero Resources**:
  - `velero.io/*`: full access

- **CSI Volume Snapshots**:
  - `snapshot.storage.k8s.io/volumesnapshots`, `volumesnapshotclasses`, `volumesnapshotcontents`: full access (except for create on some)

---

### `velero` Service Account

Used by Velero pods.

- **Full Access to**:
  - All Velero resources (`velero.io/*`)
  - All OpenShift migration and build APIs
  - RBAC APIs
  - Core APIs including `serviceaccounts`
  - `packages.operators.coreos.com/packagemanifests`

- **Non-resource URLs**:
  - All (`*`): all verbs

- **SCC Usage**:
  - Use of the `privileged` SCC

---

## **Conclusion**

While `clusterPermissions` should always be evaluated carefully, they are crucial for the OADP Operator to deliver its core functionality. From managing cluster-scoped backup objects to integrating with storage that are not using CSI snapshotting, cloud credentials, and OpenShift security constructs, these permissions are what allow OADP and Velero to provide seamless and secure backup/restore operations.
