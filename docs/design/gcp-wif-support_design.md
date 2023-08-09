# GCP WIF Support for OADP

<!-- _Note_: The preferred style for design documents is one sentence per line.
*Do not wrap lines*.
This aids in review of the document as changes to a line are not obscured by the reflowing those changes caused and has a side effect of avoiding debate about one or two space after a period.

_Note_: The name of the file should follow the name pattern `<short meaningful words joined by '-'>_design.md`, e.g:
`listener-design.md`. -->

## Abstract
Support Google Cloud's WIF (Workload Identity Federation) for OADP.

## Background
In currently released versions of OADP, the only way to authenticate to GCP is via a long lived service account credentials.
This is not ideal for customers who are using GCP's WIF ([Workload Identity](https://cloud.google.com/kubernetes-engine/docs/concepts/workload-identity)) feature to authenticate to GCP.
This proposal aims to add support for WIF to OADP.

## Goals
- GCP WIF support for OADP and Velero for backup and restore of applications backed by GCP resources.
- Using OpenShift's Cloud Credentials Operator to generate a short-lived token for authentication to GCP.
- ImageStreamTag backup and restore

## Non Goals
- [Standardized update flow for OLM-managed operators leveraging short-lived token authentication](https://issues.redhat.com/browse/OCPSTRAT-95) (follow up design/implementation)
- Allowing customers to use another long lived tokens separate from the one used by the Cloud Credentials Operator to generate short-lived tokens.


## High-Level Design
<!-- One to two paragraphs that describe the high level changes that will be made to implement this proposal. -->
A wiki will be made available to customers to follow the steps to configure GCP WIF for OADP. The wiki will also include steps to configure the Cloud Credentials Operator to generate a short-lived token for authentication to GCP. Updates Velero GCP plugin to use the short-lived token for authentication to GCP.

## Detailed Design


### Prerequisites
- Cluster installed in manual mode [with GCP Workload Identity configured](https://access.redhat.com/documentation/en-us/openshift_container_platform/4.12/html-single/authentication_and_authorization/index#gcp-workload-identity-mode-installing).
    - This means you should now have access to `ccoctl` CLI from this step and access to associated workload-identity-pool.

### Create Credential Request for OADP Operator
- Create oadp-credrequest dir
    ```bash
    mkdir -p oadp-credrequest
    ```
- Create credrequest.yaml
    ```bash
    echo 'apiVersion: cloudcredential.openshift.io/v1
    kind: CredentialsRequest
    metadata:
      name: oadp-operator-credentials
      namespace: openshift-cloud-credential-operator
    spec:
      providerSpec:
        apiVersion: cloudcredential.openshift.io/v1
        kind: GCPProviderSpec
        permissions:
        - compute.disks.get
        - compute.disks.create
        - compute.disks.createSnapshot
        - compute.snapshots.get
        - compute.snapshots.create
        - compute.snapshots.useReadOnly
        - compute.snapshots.delete
        - compute.zones.get
        - storage.objects.create
        - storage.objects.delete
        - storage.objects.get
        - storage.objects.list
        - iam.serviceAccounts.signBlob
        skipServiceCheck: true
      secretRef:
        name: cloud-credentials-gcp
        namespace: <OPERATOR_INSTALL_NS>
      serviceAccountNames:
      - velero
    ' > oadp-credrequest/credrequest.yaml
    ```
- Use ccoctl to create the credrequest poiting to dir `oadp-credrequest`
    ```bash
    ccoctl gcp create-service-accounts --name=<name> \
        --project=<gcp-project-id> \
        --credentials-requests-dir=oadp-credrequest \
        --workload-identity-pool=<pool-id> \
        --workload-identity-provider=<provider-id>
    ```
    [ccoctl reference](https://github.com/openshift/cloud-credential-operator/blob/master/docs/ccoctl.md#creating-iam-service-accounts)
    This should generate `manifests/openshift-adp-cloud-credentials-gcp-credentials.yaml` to use in the next step.

### Apply credentials secret to openshift-adp namespace
```bash
oc create namespace openshift-adp
oc apply -f manifests/openshift-adp-cloud-credentials-gcp-credentials.yaml
```

- [4.3.4.1. Installing the OADP Operator](https://access.redhat.com/documentation/en-us/openshift_container_platform/4.10/html-single/backup_and_restore/index#oadp-installing-operator_installing-oadp-gcp)
- Skip to [4.3.4.5. Installing the Data Protection Application
](https://access.redhat.com/documentation/en-us/openshift_container_platform/4.10/html-single/backup_and_restore/index#oadp-installing-dpa_installing-oadp-gcp) to create Data Protection Application
    
    Note that the key for credentials should be `service_account.json` instead of `cloud` in the official documentation example.
    ```yaml
    apiVersion: oadp.openshift.io/v1alpha1
    kind: DataProtectionApplication
    metadata:
      name: <dpa_sample>
      namespace: openshift-adp
    spec:
      configuration:
        velero:
          defaultPlugins:
          - openshift
          - gcp
      backupLocations:
        - velero:
            provider: gcp
            default: true
            credential:
              key: service_account.json
              name: cloud-credentials-gcp
            objectStorage:
              bucket: <bucket_name>
              prefix: <prefix>
      # Temporary image override while https://github.com/vmware-tanzu/velero-plugin-for-gcp/pull/142 not cherry-picked to Openshift
      unsupportedOverrides:
        gcpPluginImageFqin: ghcr.io/kaovilai/velero-plugin-for-gcp:file-wif
    ```

## Alternatives Considered
- Using Google Config Connector on OpenShift to manage short-lived tokens.
    - This would require another long lived token to be created and managed by the administrator, increasing the attack surface.
    - There are pull requests put up during investigation of this alternative.
        - https://github.com/GoogleCloudPlatform/k8s-config-connector/pull/797
        - https://github.com/GoogleCloudPlatform/k8s-config-connector/pull/801

## Security Considerations
This proposal allows OADP Operator to depend on short lived credentials generated by the Cloud Credentials Operator. This is a more secure way to authenticate to GCP than using a long lived service account key.

## Compatibility
<!-- A discussion of any compatibility issues that need to be considered -->

## Implementation
<!-- A description of the implementation, timelines, and any resources that have agreed to contribute. -->

velero-plugin-for-gcp update to support (stop panicking on) external_account (WIF) credentials https://github.com/vmware-tanzu/velero-plugin-for-gcp/pull/142

OADP Operator will be updated to bind openshift service account token when WIF credentials is used. The following is done today for AWS STS credentials and will be extended to GCP WIF credentials.
```go
		veleroContainer.VolumeMounts = append(veleroContainer.VolumeMounts,
			corev1.VolumeMount{
				Name:      "bound-sa-token",
				MountPath: "/var/run/secrets/openshift/serviceaccount",
				ReadOnly:  true,
			}),
```

## Open Issues
<!-- A discussion of issues relating to this proposal for which the author does not know the solution. This section may be omitted if there are none. -->
- [Standardized update flow for OLM-managed operators leveraging short-lived token authentication](https://issues.redhat.com/browse/OCPSTRAT-95) do not yet have support for WIF. We will have to follow up with this work later.

