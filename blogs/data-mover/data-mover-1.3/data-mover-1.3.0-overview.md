<h1>OADP 1.3 Data Mover</h1>

<h2>Introduction</h2>

<p dir="auto">OADP 1.3 includes a built-in Data Mover that you can use to move Container Storage Interface (CSI) volume snapshots to a remote object store.  Data Mover provides portability and durability of CSI volume snapshots by relocating snapshots into an object storage location during backup of a stateful application. These snapshots are then available for restore during instances of disaster scenarios. This blog will discuss the new changes in OADP 1.3.0 and the various Data Mover components and how they work together to complete this process.</p>

<h2>How Data Mover has evolved</h2>

The OADP engineering team first introduced the Data Mover feature in OADP 1.1.0. The first step in this journey leveraged Volsync to move volumes off cluster one at a time.   Like many first steps in the industry the feature worked reliably but was not performant for production environments. We knew we had to be able to move multiple volumes at one time and the work would have to be done in the upstream Velero project.  After much deliberation and collaboration in the upstream the OADP team completed it's design for handling <a href=https://github.com/vmware-tanzu/velero/blob/main/design/Implemented/general-progress-monitoring.md>asynchronous operations (BIA/RIA V2)</a> for backups and restores.  This design laid the foundations for Data Mover in both OADP 1.2.0 and OADP 1.3.0.

The Data Mover released in OADP 1.2.0 was performant for production workloads and was on average five times faster in uploading and downloading volumes than OADP 1.1.0.  Around the same time OADP 1.2.0 was released <a href=https://github.com/vmware-tanzu/velero/blob/main/design/Implemented/unified-repo-and-kopia-integration/unified-repo-and-kopia-integration.md>Kopia</a> was introduced and supported in Velero.  Kopia and asynchronous operations opened the door to a built in Data Mover in Velero itself.  A built in Data Mover allows for a more simplified workflow by not having the complexity of integrating an additional component like Volsync.  A design for <a href=https://github.com/vmware-tanzu/velero/blob/main/design/volume-snapshot-data-movement/volume-snapshot-data-movement.md>a built Data Mover</a> was proposed and accepted in the Velero project.  Thus far Red Hat engineering has found this new design for Data Mover to be reliable, performant and easier to maintain for future releases of OADP.  OADP 1.3.0 will bring this new design for a Data Mover to our customers as tech preview and we expect full support of the feature in OADP 1.3.2 to be released early in 2024.

<h2>Why We Need Data Mover</h2>

<p dir="auto">During a backup using Velero with CSI, CSI snapshotting is performed. This snapshot is created on the storage provider where the snapshot was taken. This means that for some providers, such as ODF (OpenShift Data Foundation), the snapshot resides on the cluster. Due to this poor durability, in the case of a cluster level disaster scenario, the snapshot is also subjected to disaster.</p>

<h2>Improvements to Data Mover for Block Mode Volumes and OpenShift Virtualization</h2>
Previous implementations of OADP did not support the data movement of volumes defined with <a href=https://kubernetes.io/docs/concepts/storage/persistent-volumes/#volume-mode>volumeMode: Block</a>.  We are pleased to report that the OADP 1.3 Data Mover now can successfully backup and restore volumes in either Filesystem or Block Mode.  By default <a href= https://docs.openshift.com/container-platform/latest/virt/about_virt/about-virt.html >OpenShift Virtualization</a> utilizes block mode volumes as persistent storage for virtual machines.  The lack of support for block mode PVs limited the utility of OADP to successfully provide disaster recovery services for OpenShift Virtualization workloads. Now in OADP-1.3 OpenShift Virtualization customers will be able to backup VMs, move the VM backup off cluster and restore their VMs as needed.<br><br>

For backup, Kopia's default uploader was extended to use the <a href=https://pkg.go.dev/github.com/kopia/kopia@v0.13.0/fs#StreamingFile>StreamingFile api</a> and mapping the block mode volume as device allows Kopia to correctly access the data and copy it to the Unified Repository. To restore, the block device data is copied back to the cluster via Kopia to a provisioned block device in /var/lib/kubelet/plugins by following a symbolic link to the device in /var/lib/kubelet/pods. <br>

We would like to thank our partners <a href=https://cloudcasa.io/>CloudCasa</a> and  <a href=https://www.vmware.com/>VMware</a> for their collaboration and contributions in the upstream Velero project to enable this feature.
Further improvements to block mode volumes are in progress to improve the utility and performance of the feature.  Please follow our work in the <a href=https://github.com/vmware-tanzu/velero>Velero Project</a> as we improve this critical feature.


<h2>What Is CSI?</h2>

<p dir="auto">One of the more important components of Data Mover to understand is CSI, or Container Storage Interface. CSI provides a layer of abstraction between container orchestration tools and storage systems such that storage vendors can develop a plugin once and have it work across a number of container orchestration systems. CSI defines an API for storage plugins to enable the creation of a snapshot that provides point-in-time snapshotting of volumes.</p>

<p dir="auto">CSI compliant storage plugins are now the industry standard and are the preferred storage plugin type for most container orchestrators including Kubernetes. Most of Kubernetes "in-tree" drivers developed prior to CSI all have a target removal date as most storage vendors move towards deprecating non CSI plugins. However, issues concerning CSI volumes still remain. Some volumes have vendor-specific requirements, and can prevent proper portability and durability. Data Mover works to solve this case, which will be discussed more in the next section.</p>

<p dir="auto">You can read more about CSI<span>&nbsp;</span><a href="https://kubernetes-csi.github.io/docs/">here</a>.</p>

<h2>Components</h2>

<h3><a href="https://github.com/openshift/oadp-operator">OADP OPERATOR</a>:</h3>

<p dir="auto">OADP is the OpenShift API for Data Protection operator. This open source operator sets up and installs Velero on the OpenShift platform, allowing users to backup and restore applications. We will be installing Velero alongside the velero-plugin-for-csi plugin.</p>

<h3><a href="https://github.com/vmware-tanzu/velero-plugin-for-csi">CSI PLUGIN</a>:</h3>

<p dir="auto">The collection of Velero plugins for snapshotting CSI backed PVCs using the <a href="https://kubernetes.io/docs/concepts/storage/volume-snapshots/">Kubernetes snapshot API</a>.</p>

<h3><a href="https://github.com/migtools/kopia">Kopia</a>:</h3>

<p>Kopia is a fast and secure open-source backup/restore tool that allows you to create encrypted snapshots of your data and save the snapshots to remote or cloud object storage of your choice, to network-attached object storage or server, or local object storage on your machine.</p>

<h3><a href="https://github.com/vmware-tanzu/velero/blob/main/design/Implemented/unified-repo-and-kopia-integration/unified-repo-and-kopia-integration.md">Unified Repository</a>:</h3>

<p>A target storage interface that works with both Restic and Kopia.</p>

<h3>The DataUpload and DataDownload CR</h3>
<p>
The DataUpload (DUCR) and DataDownload (DDCR) are Kubernetes CRs that act as the protocol between data mover plugins and data movers</p>

<h3>The Data Mover (DM)</h3> 
<p>
DM is a collective of modules to finish the data movement, specifically, data upload and data download. The modules may include the data mover controllers to reconcile DUCR/DDCR and the data path to transfer data.</p>

<h3>The Velero Built-in Data Mover (VBDM)</h3>
<p>
VBDM is the built-in data mover shipped along with Velero, it includes Velero data mover controllers and Velero generic data path.</p>

<h3>The Node-Agent</h3>
<p>Node-Agent is an existing Velero module that will be used to host VBDM</p>

<h3>The Exposer</h3>
<p>Exposer is used to expose the snapshot/target volume as a path/device name/endpoint that are recognizable by the Velero generic data path. For different snapshot types/snapshot accesses, the Exposer may be different. This isolation guarantees that when we want to support other snapshot types/snapshot access methods, only the Exposer component would need to be replaced.</p>

<h3>Velero Generic Data Path</h3>
<p>VGDP is the collective of modules that is introduced in Unified Repository design. Velero uses these modules to finish data transmission for various purposes. In includes uploaders and the backup repository</p>


<h3>DataUpload (DUCR) spec</h3>
<p>
A Kubernetes CR that acts as the protocol between data mover plugins and data movers
</p>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>backupStorageLocation</td>
<td>BackupStorageLocation is the name of the backup storage location where the
     backup repository is stored.</td>
</tr>
<tr>
<td>cancel</td>
<td>Cancel indicates request to cancel the ongoing DataUpload. It can be set
     when the DataUpload is in InProgress phase</td>
</tr>
<tr>
<td>csiSnapshot</td>
<td>If SnapshotType is CSI, CSISnapshot provides the information of the CSI
     snapshot.</td>
</tr>
<tr>
<td>dataMoverConfig</td>
<td>DataMoverConfig is for data-mover-specific configuration fields.</td>
</tr>
<tr>
<td>datamover</td>
<td>DataMover specifies the data mover to be used by the backup. If DataMover
     is "" or "velero", the built-in data mover will be used.</td>
</tr>
<tr>
<td>operationTimeout</td>
<td>OperationTimeout specifies the time used to wait internal operations,
     before returning error as timeout.</td>
</tr>
<tr>
<td>snapshotType</td>
<td>SnapshotType is the type of the snapshot to be backed up. Currently the only valid value is CSI</td>
</tr>
<tr>
<td>sourceNamespace</td>
<td>SourceNamespace is the original namespace where the volume is backed up
     from. It is the same namespace for SourcePVC and CSI namespaced objects.</td>
</tr>
<tr>
<td>sourcePVC</td>
<td>SourcePVC is the name of the PVC which the snapshot is taken for.</td>
</tr>
</tbody>
</table>

<h3>Note: For additional specification information please see the <a href=https://pkg.go.dev/github.com/vmware-tanzu/velero@v1.12.2/pkg/apis/velero/v2alpha1#DataUploadSpec>DataUpload API reference documentation</a>

<h3>DataDownload (DDCR) spec</h3>
<p>
A Kubernetes CR that acts as the protocol between data mover plugins and data movers
</p>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>backupStorageLocation</td>
<td>BackupStorageLocation is the name of the backup storage location where the
     backup repository is stored.</td>
</tr>
<tr>
<td>cancel</td>
<td>Cancel indicates request to cancel the ongoing DataUpload. It can be set
     when the DataUpload is in InProgress phase</td>
</tr>
<tr>
<td>dataMoverConfig</td>
<td>DataMoverConfig is for data-mover-specific configuration fields.</td>
</tr>
<tr>
<td>datamover</td>
<td>DataMover specifies the data mover to be used by the backup. If DataMover
     is "" or "velero", the built-in data mover will be used.</td>
</tr>
<tr>
<td>operationTimeout</td>
<td>OperationTimeout specifies the time used to wait internal operations,
     before returning error as timeout.</td>
</tr>
<tr>
<td>snapshotID</td>
<td>SnapshotID is the ID of the Velero backup snapshot to be restored from.</td>
</tr>
<tr>
<td>sourceNamespace</td>
<td>SourceNamespace is the original namespace where the volume is backed up
     from. It is the same namespace for SourcePVC and CSI namespaced objects.</td>
</tr>
<tr>
<td>targetVolume</td>
<td>TargetVolume is the information of the target PVC and PV.</td>
</tr>
</tbody>
</table>

<h3>Note: For additional specification information please see the <a href=https://pkg.go.dev/github.com/vmware-tanzu/velero@v1.12.2/pkg/apis/velero/v2alpha1#DataDownloadSpec>DataDownload API reference documentation</a>


<h3>DataUpload (DUCR) and DataDownload (DDCR) status descriptions</h3>

<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>New</td>
<td>The DUCR has been created but not processed by a controller</td>
</tr>
<tr>
<td>Accepted</td>
<td>The object lock has been acquired for this DUCR and the elected controller is trying to expose the snapshot</td>
</tr>
<tr>
<td>Prepared</td>
<td>The snapshot has been exposed, the related controller is starting to process the upload</td>
</tr>
<tr>
<td>InProgress</td>
<td>The data upload is in progress</td>
</tr>
<tr>
<td>Canceling</td>
<td>The data upload is being canceled</td>
</tr>
<tr>
<td>Canceled</td>
<td>The data upload has been canceled</td>
</tr>
<tr>
<td>Completed</td>
<td>The data upload has completed</td>
</tr>
<tr>
<td>Failed</td>
<td>The data upload has failed</td>
</tr>
</tbody>
</table>

<h3>Note: For additional specification information please see the <a href=https://pkg.go.dev/github.com/vmware-tanzu/velero@v1.12.2/pkg/apis/velero/v2alpha1#DataUploadStatus>DataUploadStatus</a> and <a href=https://pkg.go.dev/github.com/vmware-tanzu/velero@v1.12.2/pkg/apis/velero/v2alpha1#DataDownloadStatus>DataDownloadStatus</a> API reference documentation

<h2>Backup Process</h2>
<div>
A user creates a backup CR with the snapshotMoveData option set to true. The velero-plugin-for-csi (based on Asynchronous BackupItemAction/BIA V2 plugin API) creates a CSI VolumeSnapshot of the PVC included in the backup. The backup CR status will move from `New` to `InProgress`.<br><br>

After the VolumeSnapshots are created, you will see one or more DataUpload CRs created.  You may also see some temporary objects (i.e., pods, PVCs, PVs) created in protected (OADP operator's namespace) namespace. The temporary objects are created to assist in the data movement. The status of the DataUpload object will progress from `New` to `Accepted` to `InProgress`. <br>

Now working from the CSI plugin the CSI snapshot is mounted from the Node-Agent. The DataUpload Controller then works with Kopia ( the uploader ) to move the object off cluster to the Unified Repo Backup repository off cluster.  The status is once again reconciled and the backup CR is moved to complete.<br>

Users can see the DataUpload objects move to a terminal status of either `Completed`, `Failed` or `Canceled`
Once the object has been uploaded, any intermediate objects like the VolumeSnapshot and VolumeSnapshotContents will be removed. Finally the backup object status will be updated with it's terminal status.

</div>
<hr>

<p dir="auto"><img alt="backup-13-workflow" src="backup-13-workflow.png" width="850" /></p>

<div>
<hr>
A more in depth visualization of the backup workflow with Data Mover is found below.
<hr>
</div>

<p dir="auto"><img alt="data-mover-13-backup-sequence" src="data-mover-13-backup-sequence.png" width="850" /></p>

<h2>Restore Process</h2>
<div>
A user creates a restore CR, no additional data mover options or parameters are required. Velero's CSI plugin (based on RIA V2 plugin API) creates a PV and PVC in the protected namespace (OADP operator's namespace). The restore CR status will move from `New` to `InProgress`.<br><br>

The data from backup is queried from the remotely stored DataUpload CR and written to the in-cluster ConfigMaps.  A ConfigMap is created for each PV to be restored. These Configmaps are temporary objects that are deleted upon the restore's workflow completion. The ConfigMap stores vital information like the Repo Snapshot ID or VolumeSnapshotContent name.  The data stored in the ConfigMap is then used to build the DataDownload CR spec. 

The CSI plugin creates the DataDownload CR and DataDownload Controller reconciles on the CR.  The Node-Agent begins the download of the backed up PV data from S3. 

As the data from the backup is downloaded via DataDownload Controller via Kopia the target volume is marked as not ready. In order  to prevent the volume from binding the spec.VolumeName set to empty (""). The status of the download can be viewed from the DataDownload CR object as `Accepted`, `Prepared`, or `InProgress`.  Similarly with the Data Mover backup process a user may find temporary objects (i.e., pods, PVCs, PVs) created in the protected namespace (OADP operator's namespace) during this step.

Once the DataDownload is in a terminal status `Completed`, the target PVC should have been created in the target user namespace and waiting for binding.  The PV's claim reference is written to the target PVC in the target user namespace and the PVC will be immediately bound to the target PV.  

</div>
<hr>

<p dir="auto"><img alt="restore-13-workflow" src="restore-13-workflow.png" width="850" /></p>

<div>
<hr>
A more in depth visualization of the restore workflow with Data Mover is found below.
<hr>
</div>

<p dir="auto"><img alt="data-mover-13-restore-sequence" src="data-mover-13-restore-sequence.png" width="850" /></p>

<h2>Thank you!</h2>
The source of this blog post can be found in the <a href="https://github.com/openshift/oadp-operator/tree/master/blogs/data-mover">oadp-operator repository</a>

The original upstream Velero design for the VBDM can be found <a href=https://github.com/vmware-tanzu/velero/blob/main/design/volume-snapshot-data-movement/volume-snapshot-data-movement.md>here.</a> Information and diagrams have been sourced directly from the design.  