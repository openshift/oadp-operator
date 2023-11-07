Data Mover (OADP 1.2 or below)

<h2>Introduction</h2>

<p dir="auto">Data Mover provides portability and durability of CSI volume snapshots by relocating snapshots into an object storage location during backup of a stateful application. These snapshots are then available for restore during instances of disaster scenarios. This blog will discuss the new changes in OADP 1.3.0 and the various Data Mover components and how they work together to complete this process.</p>

<h2>How Data Mover has evolved</h2>

The OADP engineering team first introduced the Data Mover feature in OADP 1.1.0. The first step in this journey leveraged Volsync to move volumes off cluster one at a time.   Like many first steps in the industry the feature worked reliably but was not performant for production environments. We knew we had to be able to move multiple volumes at one time and the work would have to be done in the upstream Velero project.  After much deliberation and collaboration in the upstream the OADP team completed it's design for handling <a href=https://github.com/vmware-tanzu/velero/blob/main/design/Implemented/general-progress-monitoring.md>asynchronous operations (BIA/RIA V2)</a> for backups and restores.  This design laid the foundations for Data Mover in both OADP 1.2.0 and OADP 1.3.0.

The Data Mover released in OADP 1.2.0 was performant for production workloads and was on average five times faster in uploading and downloading volumes than OADP 1.1.0.  Around the same time OADP 1.2.0 was released <a href=https://github.com/vmware-tanzu/velero/blob/main/design/Implemented/unified-repo-and-kopia-integration/unified-repo-and-kopia-integration.md>Kopia</a> was introduced and supported in Velero.  Kopia and asynchronous operations opened the door to a built in Data Mover in Velero itself.  A built in Data Mover allows for a more simplified workflow by not having the complexity of integrating an additional component like Volsync.  A design for <a href=https://github.com/vmware-tanzu/velero/blob/main/design/volume-snapshot-data-movement/volume-snapshot-data-movement.md>a built Data Mover</a> was proposed and accepted in the Velero project.  Thus far Red Hat engineering has found this new design for Data Mover to be reliable, performant and easier to maintain for future releases of OADP.  OADP 1.3.0 will bring this new design for a Data Mover to our customers as tech preview and we expect full support of the feature in OADP 1.3.2 to be released early in 2024.

<h2>What Is CSI?</h2>

<p dir="auto">One of the more important components of Data Mover to understand is CSI, or Container Storage Interface. CSI provides a layer of abstraction between container orchestration tools and storage systems such that storage vendors can develop a plugin once and have it work across a number of container orchestration systems. CSI defines an API for storage plugins to enable creation of a snapshot to provides point-in-time snapshotting of volumes.</p>

<p dir="auto">CSI compliant storage plugins are now the industry standard and are the preferred storage plugin type for most container orchestrators including Kubernetes. Most of Kubernetes "in-tree" drivers developed prior to CSI all have a target removal date as most storage vendors move towards deprecating non CSI plugins. However, issues concerning CSI volumes still remain. Some volumes have vendor-specific requirements, and can prevent proper portability and durability. Data Mover works to solve this case, which will be discussed more in the next section.</p>

<p dir="auto">You can read more about CSI<span>&nbsp;</span><a href="https://kubernetes-csi.github.io/docs/">here</a>.</p>

<h2>Why We Need Data Mover</h2>

<p dir="auto">During a backup using Velero with CSI, CSI snapshotting is performed. This snapshot is created on the storage provider where the snapshot was taken. This means that for some providers, such as ODF (OpenShift Data Foundation), the snapshot lives on the cluster. Due to this poor durability, in the case of a disaster scenario, the snapshot is also subjected to disaster.</p>

<h3><a href="https://github.com/openshift/oadp-operator">OADP OPERATOR</a>:</h3>

<p dir="auto">OADP is the OpenShift API for Data Protection operator. This open source operator sets up and installs Velero on the OpenShift platform, allowing users to backup and restore applications. We will be installing Velero alongside the CSI plugin (modified version).</p>

<h3><a href="https://github.com/vmware-tanzu/velero-plugin-for-csi">CSI PLUGIN</a>:</h3>

<p dir="auto">The collection of Velero plugins for snapshotting CSI backed PVCs using the <a href="https://kubernetes.io/docs/concepts/storage/volume-snapshots/">CSI snapshot APIs</a>.</p>

<h3><a href="https://github.com/migtools/kopia">Kopia</a>:</h3>

<p>Kopia is a fast and secure open-source backup/restore tool that allows you to create encrypted snapshots of your data and save the snapshots to remote or cloud storage of your choice, to network-attached storage or server, or locally on your machine.</p>

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
<p>Exposer is to expose the snapshot/target volume as a path/device name/endpoint that are recognizable by Velero generic data path. For different snapshot types/snapshot accesses, the Exposer may be different. This isolation guarantees that when we want to support other snapshot types/snapshot accesses, we only need to replace with a new Exposer and keep other components as is.</p>



<h1>Below are the data movement actions and sequences during backup</h1>
<p dir="auto"><img alt="data-mover-13-backup-sequence" src="data-mover-backup-sequence.png" width="850" /></p>

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
<td>SnapshotType is the type of the snapshot to be backed up.</td>
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

<h3>Note: For additional specification information please see the <a href=https://github.com/openshift/oadp-operator/blob/master/docs/API_ref.md>API reference documentation</a>


<h2>Backup Process</h2>
<div>
	
to do
</div>


<p dir="auto"><img alt="data-mover-backup" src="data-mover-backup.png" width="850" /></p>


<h2>Restore Process</h2>
<div>
Previously mentioned, during the backup process, a VSB custom resource is stored as a backup object that contains essential details for performing a volumeSnapshotMover restore. When a VSB CR is encountered, the VSM plugin generates a VSR CR. The VSM controller then begins to reconcile on the VSR CR. Furthermore, the VSM controller creates a VolSync ReplicationDestination CR in the OADP Operator namespace, which facilitates the recovery of the VolumeSnapshot stored in the object storage location during the backup.<br><br>

After the completion of the VolSync restore step, the Velero restore process continues as usual. However, the CSI plugin utilizes the snapHandle of the VolSync VolumeSnapshot as the data source for its corresponding PVC.
</div>

<p dir="auto"><img alt="data-mover-restore" src="data-mover-restore.png" width="850" /></p>


<h2>Thank you!</h2>
The source of this blog post can be found in the <a href="https://github.com/openshift/oadp-operator/tree/master/blogs/data-mover">oadp-operator repository</a>