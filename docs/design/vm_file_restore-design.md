# File-Level Restore for Virtual Machines in OADP

## Abstract

This design presents a Kubernetes-native solution for recovering individual files from KubeVirt Virtual Machine backups created with OADP in OpenShift.

Instead of restoring an entire VM to retrieve a single file, users can browse multiple backups and restore only the files they need - directly within Kubernetes, using familiar tools such as a web browser or command-line utilities like rsync.

This approach improves file recovery efficiency, simplifies workflows, and provides transparency for VM users, with backups managed by OpenShift cluster administrators.

## Background

### Current Problem

Recovering individual files from KubeVirt Virtual Machine backups in OpenShift is currently inefficient and complex due to several limitations:

1. **Full VM Restore Required**  
   Existing workflows require restoring the entire virtual machine to access a single file, consuming significant time and cluster resources.  

2. **Limited Backup Scope**  
   Current tools only allow access to one backup at a time, making it difficult to compare files across multiple backups or track changes over time.  

3. **Complex Workflows**  
   Users often need to install tools inside the restored VM or rely on external utilities to mount backup data. This introduces additional steps, specialized knowledge, and movement outside the Kubernetes environment.

### User Scenarios

**Incident Investigation**  
A production VM experiences a configuration issue. The user needs to compare configuration files from before and after the incident. With current tools, this requires restoring multiple VMs and performing manual file comparisons.  

**Selective Recovery**  
A user accidentally deletes critical documents from a VM. The files exist in yesterday’s backup, but restoring the entire VM would overwrite today’s changes. The user needs targeted file recovery.  

**Multi-VM Backup Discovery**  
A namespace runs ten VMs with daily backups over four weeks. A user needs to recover a file from a specific VM but doesn’t know which backup contains it. Existing workflows require inspecting each backup individually or attempting multiple restores, making the process cumbersome and error-prone.

## Goals

Provide a Kubernetes-native solution for file-level VM backup recovery that:
- Enables browsing and restoring individual files without full VM restore
- Supports accessing files from multiple backups simultaneously
- Integrates seamlessly with existing OADP and Velero backup workflows

## Non Goals

This design does not aim to:
- Support non-OADP backup systems
- Replace full VM restore capabilities (both workflows should coexist)
- Provide hot-mount capabilities into running VMs
- Support file restore for non-VM workloads (focused on KubeVirt VMs only)

## High-Level Design

### Two-Phase Approach

The system uses a two-phase workflow that separates concerns and enables reusability:

**Phase 1: Backup Discovery - VMBD**

Users create a `VirtualMachineBackupsDiscovery` resource specifying which VM from which namespace to search for and optional time range filters.
The discovery controller validates each backup to confirm it actually contains the requested VM and returns a list of valid backups.

**Phase 2: File Serving - VMFR**

Users create a `VirtualMachineFileRestore` (VMFR) resource, optionally scoping the recovery to a subset of discovered backups and selecting the preferred access method, either web-based or SSH-based.

The VMFR controller then orchestrates the following workflow:

1. **Temporary Restore Namespace**
  The controller creates a temporary namespace to host the restored storage and file-serving resources when the user does not specify an existing namespace. This ensures isolation and proper resource management.

2. **Velero Restore Execution**  
  Using the temporary namespace, the controller triggers Velero Restore resources to recover only the storage associated with the VM for each selected backup. This avoids restoring the entire VM while ensuring all relevant data is available for file access.

3. **Unified Storage Mount**  
  After the restores complete, the controller mounts all restored storage volumes, including the VM images residing on one of the recovered volumes, creating a unified file system view that aggregates files across the selected backups.

4. **File Access Service**  
  On top of the mounted storage, the controller exposes a service that allows users to browse and retrieve files. Users can interact with the recovered data via a web interface or SSH, depending on the chosen access method.

This design ensures efficient, flexible, and secure file-level recovery, while keeping the process fully Kubernetes-native and transparent to end users.

### User Workflow

```
┌───────────────────────────────────────────────┐
│ User has VM backups created by Velero         │
│ Wants to access files without full VM restore │
└────────────────┬──────────────────────────────┘
                 │
                 ▼
┌────────────────────────────────────────────────────┐
│ Phase 1: Create VirtualMachineBackupsDiscovery     │
│                                                    │
│ Specify:                                           │
│  - VM name and namespace                           │
│  - Optional Time range                             │
│        (e.g., "2025-09-05" to "2025-09-30")        │
│  - Optional backup names (even outside time range) │
└────────────────┬───────────────────────────────────┘
                 │
                 ▼
┌────────────────────────────────────────────────────┐
│ Discovery Controller Validates Backups             │
│                                                    │
│ Returns:                                           │
│  - ValidBackups: Confirmed to contain the VM       │
│  - InvalidBackups: Missing or don't contain the VM │
│  - Progress tracking and statistics                │
└────────────────┬───────────────────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────────────────────┐
│ Phase 2: Create VirtualMachineFileRestore           │
│                                                     │
│ Specify:                                            │
│  - Reference to completed discovery                 │
│  - Optional: Select specific backups from results   │
│  - Optional: Choose namespace to which restore      │
│  - Access method (web browse and/or rsync over ssh) │
└────────────────┬────────────────────────────────────┘
                 │
                 ▼
┌──────────────────────────────────────────────────────┐
│ File Restore Controller Creates Infrastructure       │
│                                                      │
│ Actions:                                             │
│  - (Opt) Creates temporary restore namespace         │
│  - Restore PVCs from selected backups                │
│  - Create file serving pod that mounts storage       │
│  - Provide access via HTTPS and/or rsync/ssh         │
└────────────────┬─────────────────────────────────────┘
                 │
                 ▼
┌────────────────────────────────────────────────┐
│ User Accesses Files                            │
│                                                │
│ Options:                                       │
│  - Browse files via web browser                │
│  - Download specific files through HTTPS       │
│  - Sync directories using rsync over ssh       │
│  - Compare files across different backup dates │
└────────────────────────────────────────────────┘
```

### Key Benefits

- **Efficiency**
  Access individual files without restoring entire VMs, saving time and compute resources.

- **Flexibility**
  Browse files from multiple backups simultaneously for comparison and investigation.

- **Simplicity**
  Use standard tools (web browsers, rsync, scp) without leaving the cluster or installing software in VMs.

- **Kubernetes native**
  Fully declarative using CRDs, controllers, RBAC, and Secrets - no external tools or additional steps are required to access the restored files.

- **Reusability**
  Perform discovery once, then create multiple file restore sessions with different backup selections if needed.

## Detailed Design

### Custom Resource Definitions

The two-phase approach requires two new CRDs that separate backup discovery from file access.

#### VirtualMachineBackupsDiscovery (VMBD)

Discovers which Velero backups contain a specific virtual machine.

**Key Fields:**
```yaml
spec.virtualMachineName: Name of the virtual machine to search for in available backups.

spec.virtualMachineNamespace: Namespace where the virtual machine originally existed.

spec.startTime: Optional starting point for the discovery time range. Accepts simple, human-readable dates such as "2025-09-30".

spec.endTime: Optional end point for the discovery time range. Accepts the same date format as startTime. If not provided, the current time is used.

spec.requestedBackups: Optional explicit list of backup names to include in the search. These backups may exist outside the specified time range.
```
**Status Information:**
```yaml
status.validBackups: List of backups confirmed to contain the specified virtual machine, including their creation timestamps.

status.invalidBackups: List of requested backups that do not contain the virtual machine or could not be found, along with reasons for exclusion.

status.discoveryStats: Summary of discovery results, providing counts of total candidate backups, successfully validated backups, and failed validations.

status.phase: Current state of the discovery process. Possible values include New, InProgress, Completed, PartiallyFailed, and Failed.

status.conditions: A list of condition objects representing the current state of the resource.  
This is the primary source of truth for the resource's status. Each condition has a unique type and reflects the state of a specific aspect of the resource.

status.observedGeneration: Represents the `.metadata.generation` value that the status was last set against.
```

**Example Usage:**

In this example, the discovery process searches for all Velero backups that include the VM `production-web-server` from the `production` namespace.

It will consider all backups created on or after `2025-08-01`, as well as two explicitly listed backups, even though they fall outside the specified date range.

```yaml
apiVersion: oadp.openshift.io/v1alpha1
kind: VirtualMachineBackupsDiscovery
metadata:
  name: find-my-vm-backups
  namespace: openshift-adp
spec:
  virtualMachineName: "find-my-vm-backups"
  virtualMachineNamespace: "production"
  startTime: "2025-08-01"
  requestedBackups:
   - "initial-backup-from-2024-01-01"
   - "last-working-from-2025-07-28"
```

#### VirtualMachineFileRestore (VMFR)

Makes files from selected backups accessible for browsing and recovery without full VM restoration.

The controller automates the complete restore workflow—including PVC restoration, file-serving pod creation, and access configuration - allowing users to browse files via web or SSH
without managing underlying Kubernetes resources.

**Key Fields:**
```yaml
spec.backupsDiscoveryRef: Name of the completed VirtualMachineBackupsDiscovery resource that this restore should reference in the same namespace as the VMFR.

spec.selectedBackups: Optional list of specific backup names from the discovery results to restore.
This field allows users to limit the restore scope once they have identified which backups are relevant. If not provided, all valid backups from the referenced discovery are used.

spec.restoreNamespace: Optional. Existing namespace for file-serving resources. If omitted, a temporary namespace is created automatically and must be accessible to the controller.

spec.namespacePrefix: Optional. Prefix for automatically generated temporary namespaces (used only if restoreNamespace is not set). Format is <prefix>-<vm-namespace>-<vm-name>-<suffix>

spec.accessMethods: List of methods to access restored files. Users can specify one or more methods.
    type: Type of access method. Supported values. "web", "ssh".
        web:
            credential: Optional reference to a Kubernetes Secret containing sensitive credentials for web access.
                name: Secret name.
                key: Key in the Secret containing the credential (e.g., password).
            <optional additional web service configuration options such as port>
        ssh:
            username: SSH user account to associate with the public key.
            publicKey: Optional SSH public key for the user account. Public key is not sensitive and can be stored in clear text in the CRD.
            <optional additional SSH service configuration options such as port>

```

**Status Information:**
```yaml
status.phase: Current state of the restore (New, InProgress, Completed, PartiallyFailed, Failed, Deleting).

status.conditions: A list of condition objects representing the current state of the resource.  
This is the primary source of truth for the resource's status. Each condition has a unique type and reflects the state of a specific aspect of the resource.

status.fileServingInfo: Details of file serving resources, including access endpoints, credentials, and instructions for browsing restored files.

status.pvcRestores: Information about PersistentVolumeClaims restored from each backup, organized by PVC.  
Includes restore details for each backup such as backup name, namespace, timestamps, restore phase, and eventual failure reason that prevented restore.

status.createdNamespace: Name of the namespace used to host file-serving resources. Set to either the specified `spec.restoreNamespace` or an auto-generated temporary namespace.

status.pvcRestores: A list of all PVCs restored from the selected backups. Each entry includes metadata about the PVC and details of the restore(s) performed for that PVC.
    status.pvcRestores:
        VeleroBackupName: Name of the Velero backup from which this PVC was restored.
        VeleroBackupNamespace: Namespace of the Velero backup.
        Timestamp: When the backup was created.
        State: State of the backup for this PVC, e.g., available, backup-deleted, backup-missing, unsupported-plugin, extraction-failed, processing, failed.
        VeleroRestoreName: Name of the Velero Restore object created for this PVC.
        VeleroRestoreNamespace: Namespace of the Velero Restore object.
        Phase: Phase of the Velero Restore object (matches Velero RestorePhase).
        CreatedAt: When the Velero Restore object was created.
        CompletedAt: When the Velero Restore completed.
        FailureReason: Optional reason for failure if the restore did not succeed.

status.observedGeneration: Represents the `.metadata.generation` value that the status was last set against.
```

**Example Usage:**

This example demonstrates how to configure a `VirtualMachineFileRestore` (VMFR) resource that accesses files from a subset of discovered VM backups using both web and SSH access methods.

```shell
$ oc create secret generic vmfr-web-credentials -n openshift-adp --from-file vmrestorecreds=<credentials_for_web_service_file_name>
```

```yaml
apiVersion: oadp.openshift.io/v1alpha1
kind: VirtualMachineFileRestore
metadata:
  name: restore-config-files
  namespace: openshift-adp
spec:
  backupsDiscoveryRef: "find-my-vm-backups"
  selectedBackups:
    - "backup-2025-09-20"
    - "last-working-from-2025-07-28"
  accessMethods:
    - type: web
      credential:
        name: vmfr-web-credentials
        key: vmrestorecreds
    - type: ssh
      username: vmuser
      publicKey: ssh-rsa AAAAB3... # public key is not sensitive, can be clear text
```

### Controller Workflows

#### VM Backup Discovery Controller

The VirtualMachineBackupsDiscovery controller performs the following workflow:

  The VirtualMachineBackupDiscovery controller:

  1. **Build candidate list**

     - If `spec.backupNames` is set: Use the specified backup list
     - Otherwise: Fetch all Velero backups in the cluster

  2. **Apply time filter** (if specified)

     Filter candidates to only backups within `spec.timeRange`.

  3. **Validate backup status**

     Skip backups not in Completed phase.

  4. **Verify VM presence**

     For each candidate, query Velero backup metadata to confirm the target VM is included.

     **Optimization:** Only backup manifests are fetched (not resource data or volumes metadata), and verification runs in batches.

     **Error handling:** Discovery waits for all candidates to complete. Individual failures are logged but don't block verification of other backups.

     **Update status**

     - The status is incrementally updated as batches are processed, providing real-time feedback on discovery progress and the state of each candidate.

     - Once all candidates are processed, conditions and status reflect the final state.

     - All candidates must be in a recoverable state before a VirtualMachineFileRestore (VMFR) object can proceed.

     - At this stage, users may refine discovery parameters - such as time ranges or specific backup names - to ensure all backups selected for restore are healthy and suitable for the VMFR operation.


#### VM File Restore Controller

The VirtualMachineFileRestore controller performs the following workflow:

  1. **Discovery Validation**

     Verify referenced VirtualMachineBackupsDiscovery exist and is complete

  2. **Backups Selection Validation**

     Verify all optional user specified backups exist in discovery ValidBackups results

  3. **Discover PVCs from backups**
  
     This is a crucial, multi-part step focused on identifying and organizing the Persistent Volume Claims (PVCs) associated with the selected VMs at the time of backup used for restore operation.

      - **Why this is needed**

        For each backup to be restored, the controller downloads and parses Velero backup metadata. This step requires more detailed information than the initial discovery phase to accurately associate PVCs with their VMs at the time of backup.
        
        At this stage the controller also has all the data necessary to ensure the backups were created using the appropriate kubevirt-velero-plugin that allows selective PVC restores by their UIDs (see [GitHub PR #396](https://github.com/kubevirt/kubevirt-velero-plugin/pull/396#issue-3449794128) for information on how namespace mappings are handled during restore).

        This step is critical because:
          - PVC UIDs may have changed over time and we need to have their UIDs at the time of backup.
          - Backups may contain additional PVCs not related to the target VM or multiple VMs from one namespace.
          - Accurate mapping ensures that only the PVCs belonging to the VM are restored and reflected correctly in the VMFR status.
          - Backups created with older kubevirt-velero-plugin versions do not support selective PVC restores.

      - **Status Tree Construction**

        The controller builds a detailed information tree. This tree links each discovered PVC to the specific backup it came from, including the backup time and other relevant status details and errors if they occur. It will be used to create Velero Restore objects.

  4. **(Optional) Temporary Namespace Creation**
    
     The controller creates a temporary namespace to host the file-serving resources if the user did not specify an existing namespace.

     Key points:
      - The namespace is marked as managed by the VMFR controller to allow automatic cleanup when the VMFR object is deleted.
      - If the user provides an existing namespace, the controller does not manage its lifecycle, and no automatic cleanup is performed.

  5. **Storage Restore**

     **Requirement**
     
      Restoring the same PVC from multiple VM backups creates PVC name conflicts (same PVC names/UIDs across backup versions to be restored into one restore namespace).

      It is crucial to ensure the restore operation uses enhanced velero plugin that allows to restore PVC to a unique name within one namespace. Such enhancement was implemented within openshift-velero-plugin ([PR #355](https://github.com/openshift/openshift-velero-plugin/pull/355)) and relies on the `oadp.openshift.io/vmfr-restore: "true"` annotation.

     **Workflow**

     For each selected backup, VMFR creates a Velero Restore object that:
     - Restores only storage resources (PVCs, VolumeSnapshots) to an isolated namespace
     - Uses label selectors to target only the VM's PVCs, using an enhanced kubevirt-velero-plugin ([GitHub PR #396](https://github.com/kubevirt/kubevirt-velero-plugin/pull/396#issue-3449794128))
     - Includes `oadp.openshift.io/vmfr-restore: "true"` annotation to enable unique naming

     Velero executes the restore while VMFR monitors progress and updates status.

     Sample Velero Restore object spec created by the VMFR object.

     ```yaml
      apiVersion: velero.io/v1
      kind: Restore
      metadata:
        name: vmfr-restore-config-files-20250728  # Generated by VMFR controller
        namespace: # Backup object namespace, usually openshift-adp
        annotations:
          oadp.openshift.io/vmfr-restore: "true"  # Must have when restoring from more than 1 backup to avoid PVC name collisions.
      spec:
        backupName: last-working-from-2025-07-28
        includedResources: # Restore only storage resources
          - persistentvolumeclaims
          - volumesnapshots
        namespaceMapping:
          production: <restore-ns> # <restore-ns> created by VMFR controller or provided by the user
        includedNamespaces:
          - production
        orLabelSelectors: # We always use orLabelSelectors, even for one PVC to make Restore object unified
          - matchLabels:
              velero.kubevirt.io/pvc-uid: 8c52a10e-dd6b-430b-b6a3-55d1082f0d20 # Select PVCs by UID (from discovered backup metadata)
          - matchLabels:
              velero.kubevirt.io/pvc-uid: c7514c0f-86a3-49d8-a8f8-18babd9c80f5
     ```

6. **Storage Access and File Serving Pod**

   The controller creates a single, unified pod that serves two tightly integrated functions:

   - **Storage Access**

      An Init container mounts all restored PVCs as read-only and exposes their filesystems in a unified, deterministic directory hierarchy.

      It includes utilities to handle a wide range of disk image formats (qcow2, raw) and filesystems (ext4, xfs, ntfs, btrfs, etc.).

      The mounting must occur in the Init container so that all subsequent sidecar containers can access the mounted files under /backups/.

     Example directory structure:
     
     ```plaintext
     /backups/
       backup-2025-09-20/
         vm-disk-root/        # Primary filesystem from main VM disk
           etc/
           var/
         vm-disk-data/        # Additional PVCs attached to VM
           application-data/
         vm-user-data/        # PVC not attached at backup time, recovered separately
           user-data/
       last-working-from-2025-07-28/
         vm-disk-root/
           etc/
           var/
         vm-disk-data/
           application-data/
     ```

   - **File Serving Sidecars**

      Sidecar containers are attached to the same pod to provide user access according to `spec.accessMethods`.
      
      Each sidecar mounts only the `/backups/` directory, ensuring a limited and secure scope of access to the restored data.

      - **Web access:**
        A FileBrowser sidecar exposes the `/backups/` directory tree via HTTPS, allowing users to browse and download files through a secure web interface. User credentials are set for secure access.

      - **SSH access:**
        An OpenSSH sidecar exposes the `/backups/` directory for secure `rsync`, `scp`, and `sftp` access. Unlike the Web access method, SSH access is configured using an SSH username and public key (not a username and password). The SSH user is restricted to `/backups/` using OpenSSH's `ChrootDirectory` option to ensure further security.

      - **Combined access:**  
        Both sidecars can run simultaneously if both access methods are enabled.


      This pod-centric design consolidates all restore and serving logic, ensures sidecars always see filesystem views, and simplifies network/service exposure. The controller creates any required Kubernetes Services for internal access and updates `status.fileServingInfo` with endpoint URLs and instructions.

   For implementation details on container images, mounting logic, and configuration, see below:
    - [Filesystem Access Mechanisms](#filesystem-access-mechanisms)
    - [File Serving Mechanisms](#file-serving-mechanisms)


### Filesystem Access Mechanisms

The controller uses a specialized container to mount and expose VM disk images from restored PVCs, making the VM’s filesystem contents available to file-serving sidecars in a consistent, read-only structure.

#### Container Implementation

**Base Technology**
- **libguestfs** — Provides access to VM disk images without booting the virtual machine.  
- **FUSE (Filesystem in Userspace)** — Allows safe, unprivileged filesystem mounting inside containers without access to host `/dev/kvm` or escalated pod privileges.

**Container Image**
- **Base OS:** Fedora 40 (upstream), RHEL 9 (downstream for OpenShift builds)  
- **Architectures:** Multi-architecture support (`amd64`, `arm64`)  
- **Design:** All-in-one image with the full toolset preinstalled for image and filesystem operations

**Supported Formats**
- **Disk images:** `qcow2`, `raw` (same formats supported by OpenShift Virtualization)  
- **Filesystems:** `ext4`, `xfs`, `ntfs`, `btrfs`, `fat`

#### Init Container Workflow

The filesystem access logic runs as an **init container** within the storage access pod, ensuring that all filesystems are mounted before the file-serving sidecars start.

1. **PVC Detection:** Scans all attached PVCs for VM disk images.  
2. **Format Detection:** Automatically identifies image formats and internal filesystems.  
3. **Mounting:** Uses `libguestfs` via FUSE to mount filesystems under `/backups/` in **read-only** mode.  
4. **Directory Organization:** Builds a deterministic directory tree grouping files by backup name and PVC.  
5. **Handoff:** Once completed, control is passed to the file-serving sidecars, which expose the mounted files.

#### Security and Performance

**Security Requirements**
- **Privileged Mode:** Not required, thanks to FUSE-based mounting. Without FUSE, direct access to the host /dev/kvm device would be needed for hardware acceleration.
- **User Context:** Runs as `root` to ensure unrestricted read access to all files across mounted filesystems, regardless of original ownership or permissions. This guarantees full visibility for recovery operations while keeping mounts strictly read-only and contained within the pod, maintaining security and isolation.
- **Read-Only Access:** Filesystems are always mounted read-only to prevent modification of backup data.  
- **PVC Mount Mode:** PVCs are mounted read-write internally to support libguestfs overlay files without altering original data.

**Performance Optimization Option**

- **Enable Hardware Acceleration:**  
  In environments where running the init container in privileged mode is acceptable, mounting speed can be significantly improved by granting access to `/dev/kvm`. This allows `libguestfs` to leverage KVM hardware virtualization, which greatly reduces mount and inspection time, especially for large or complex disk images. As an alternative, the default FUSE-based approach remains fully functional without requiring additional privileges but may be slower in comparison.

#### Compatibility

The container is designed to handle a wide range of VM backup scenarios in OpenShift environments, including:
- Standard Linux VMs (RHEL, Fedora, Ubuntu, etc.)
- Windows VMs with NTFS filesystems
- VMs using LVM-based storage layouts
- Multi-disk VM configurations with mixed storage types


### File Serving Mechanisms

This section describes how users access recovered files through web and SSH interfaces.

#### Web Browser Access (HTTPS)

Users can browse and download files through a web-based interface using FileBrowser.

**Implementation:**

- **Container**: FileBrowser (https://filebrowser.org/) runs as a sidecar container in the storage access pod
- **Mount**: Read-only access to the `/backups/` directory structure
- **Authentication**:
  - If `spec.accessMethods[].web.credential` is provided, controller retrieves credentials from the referenced Secret
  - If not provided, controller generates a secure random password and stores it in a new Secret
- **Service Exposure**:
  - Kubernetes Service exposing FileBrowser HTTPS port (default 443)
  - By default, the service is only accessible within the cluster (ClusterIP); external exposure (such as via Route or Ingress) is not automatically created.
- **Status Information**: Controller updates `status.fileServingInfo.webAccess` with URL, credential Secret reference, and usage instructions

**Features:**
- Simple file browsing UI showing directory structure across multiple backups
- File preview for common text and image formats
- Download individual files or directories as archives

**Example Status:**
```yaml
status.fileServingInfo.webAccess:
  url: https://vmfr-restore-config-files.openshift-adp.svc.cluster.local
  credentialSecret:
    name: vmfr-restore-config-files-web-creds
    key: password
  instructions: "Access the web interface at the URL above. Login with username 'admin' and the password from the secret. Browse /backups/ to view files from different backup dates."
```

#### Command-Line Access (rsync/ssh)

For power users and automation, SSH-based access provides rsync, scp, and sftp capabilities.

**Implementation:**

- **Container**: OpenSSH server runs as a sidecar container in the storage access pod
- **Mount**: Read-only access to the `/backups/` directory structure
- **User Account**: Dedicated user created with username from `spec.accessMethods[].ssh.username`
- **Authentication**:
  - If `spec.accessMethods[].ssh.publicKey` is provided, SSH server accepts that public key only
- **Security**: Key-based authentication only (no password authentication), no root login
- **Service Exposure**:
  - Kubernetes Service exposing SSH port (default 22)
  - By default, the service is only accessible within the cluster (ClusterIP); external exposure (such as via Route or Ingress) is not automatically created.
- **Status Information**: Controller updates `status.fileServingInfo.sshAccess` with connection string, credential Secret reference, and example commands

**Supported Tools:**
- **rsync**: Efficient directory synchronization with incremental transfer
- **scp**: Simple file copy operations
- **sftp**: Interactive file transfer sessions
- **ssh**: Direct shell access for manual file operations (read-only)

**Example Status:**
```yaml
status.fileServingInfo.sshAccess:
  connectionString: ssh vmuser@vmfr-restore-config-files-ssh.openshift-adp.svc.cluster.local
  port: 22
  username: vmuser
  credentialSecret:
    name: vmfr-restore-config-files-ssh-key
    key: private-key
  instructions: |
    Connect via SSH using the private key from the secret. Example commands:
      rsync -avz -e 'ssh -i /path/to/key' vmuser@<host>:/backups/backup-2025-09-20/vm-disk-root/etc/ ./etc-backup/
      scp -i /path/to/key vmuser@<host>:/backups/last-working-from-2025-07-28/vm-disk-root/var/log/app.log ./
      sftp -i /path/to/key vmuser@<host>:/backups/
```

### Integration Points

**Velero Integration**
- Uses Velero's backup download APIs for content validation
- Leverages Velero's restore mechanism for PVC recovery
- Respects Velero's backup storage location configurations
- Compatible with OpenShift Velero fork used in OADP

**KubeVirt Integration**
- Understands KubeVirt VM resource structure
- Handles various VM disk formats and configurations
- Works with KubeVirt Velero plugin backup format

**OADP Integration**
- Resources must be created in OADP namespace (typically `openshift-adp`)
- Will integrate with DPA (DataProtectionApplication) for enablment of the feature
- Modified Velero KubeVirt Plugin is handling PVC labeling, that persists only in the backup metadata
- Modified OpenShift Velero Plugin allows to restore multiple same PVCs into one namespace under modified names

## Use Cases

### Incident Investigation

**Scenario**: A production VM started misbehaving after a configuration change. The user needs to identify what changed.

**Workflow**:
1. Create discovery for backups from before and after the incident
2. Create file restore for the relevant backups
3. Use web browser to navigate to `/etc/` in both backups
4. Compare configuration files side-by-side
5. Identify the problematic change and download the correct version

**Benefit**: Investigation completes in minutes without full VM restores.

### Selective File Recovery

**Scenario**: User accidentally deleted important documents. VM has changed since the last backup.

**Workflow**:
1. Create discovery for recent backups
2. Create file restore for the most recent backup containing the files
3. Browse to document directory via web UI
4. Download only the deleted files
5. Manually copy files back to running VM or create new VM from current state with recovered files

**Benefit**: Surgical recovery without losing current VM state.

### Compliance Auditing

**Scenario**: Security team needs to examine audit logs from specific dates across multiple VMs.

**Workflow**:
1. Create separate discoveries for each VM covering the required date range
2. Create file restore referencing specific backup dates needed
3. Use rsync to synchronize log directories to audit workstation
4. Automated scripts process logs from multiple backups

**Benefit**: Parallel access to multiple backups without sequential VM restores.

### Development and Testing

**Scenario**: Developer needs to test application behavior with different versions of configuration files.

**Workflow**:
1. Create discovery for backups from different development phases
2. Create file restore for selected development milestones
3. Browse and download configuration files from each phase
4. Test application behavior with historical configurations

**Benefit**: Quick access to configuration history for regression testing.

## Alternatives Considered

### Alternative 1: Extend Velero Restore with File-Level Options

**Approach**: Add file-level restore capabilities directly to Velero's restore API.

**Pros**: Single API for all restore operations, no new CRDs needed.

**Cons**:
- Velero is designed for full resource restore, not file browsing
- Would require significant upstream Velero changes
- Doesn't address backup discovery or multi-backup access
- Less flexible for VM-specific workflows

**Decision**: Rejected. OADP-specific CRDs provide better user experience for VM file recovery.

### Alternative 2: Restore Full VM in Restricted Mode

**Approach**: Restore complete VM but prevent it from starting, mount disks externally.

**Pros**: Uses existing restore mechanisms, no new infrastructure.

**Cons**:
- Still restores entire VM resources unnecessarily
- Doesn't support multi-backup access
- Complex to prevent VM startup reliably
- Wastes resources by restoring unused components

**Decision**: Rejected. Resource waste defeats the purpose of file-level restore.

### Alternative 3: External Tool for Backup Mounting

**Approach**: Provide CLI tool that runs outside cluster to download and mount backups.

**Pros**: No cluster resources needed, works with any backup.

**Cons**:
- Requires users to leave Kubernetes environment
- No multi-backup comparison capability
- Inconsistent with Kubernetes-native workflows
- Security concerns with backup credentials outside cluster

**Decision**: Rejected. Solution should be Kubernetes-native.

### Alternative 4: Job-Based File Serving

**Approach**: Use Kubernetes Jobs instead of long-running pods for file serving.

**Pros**: Jobs have well-defined completion semantics.

**Cons**:
- Jobs are designed for one-off tasks, not interactive browsing
- Difficult to provide persistent web UI access
- Doesn't match user mental model of "serving files"

**Decision**: Rejected. Pod-based approach better matches file serving use case.

## Security Considerations

### Access Control

**Namespace Isolation**
- File serving resources created in temporary namespace separate from OADP namespace
- Namespace created and owned by VirtualMachineFileRestore resource
- Cleaned up automatically when restore is deleted

**RBAC Requirements**
- Discovery controller needs read-only access to Velero Backup resources
- File restore controller needs create/delete access for namespaces, pods, services, PVCs
- Cluster administrators need create/read/update/delete access to VMBD and VMFR resources in OADP namespace
- Cluster administrators will create restore resrouces and provide users with the access details

**Backup Storage Access**
- Uses same security model as Velero for backup storage access
- Controllers authenticate to object storage using Velero's backup storage location credentials
- Read-only access to backup data, never modifies backups

### Data Protection

**Encryption**
- HTTPS endpoints for web access with TLS certificates
- SSH access secured with generated key pairs
- Credentials stored in Kubernetes secrets

### Multi-Tenancy

**Namespace Handling**
- Multiple VirtualMachineFileRestore resources can exist simultaneously
- Each creates isolated temporary namespace
- Resources from different restores cannot interfere
- Supports restoring same VM from different backups in parallel

**Name Collision Avoidance**
- Temporary namespace names derived from VMFR resource name
- PVC names in temporary namespace use backup name as suffix
- Prevents conflicts when restoring same VM from multiple backups

## Compatibility

### Velero Compatibility

Compatible with Velero 1.13.0 and later:
- Uses Velero backup download APIs
- Leverages Velero restore mechanism for PVC recovery
- Respects Velero backup storage location configurations
- Uses `orLabelSelectors`, available since [PR #6475](https://github.com/vmware-tanzu/velero/pull/6475)

### KubeVirt Compatibility

Supports KubeVirt VMs backed up with KubeVirt Velero plugin.

### OADP Compatibility

Feature is planned for the OADP 1.6.0.

## Conclusion

This design provides a Kubernetes-native solution for VM file-level recovery that addresses real user pain points while integrating seamlessly with existing OADP and Velero workflows.
The two-phase approach separates concerns, enables reusability, and provides flexibility for different use cases.
The implementation is structured in phases to deliver value incrementally while validating design decisions with real user feedback.

## References

- [VM File Restore Implementation Issues](https://github.com/migtools/oadp-vm-file-restore/issues)
- [KubeVirt Velero Plugin](https://github.com/migtools/kubevirt-velero-plugin)
- [OpenShift Velero Plugin](https://github.com/konveyor/openshift-velero-plugin)
- [File Browser](https://github.com/filebrowser/filebrowser)
