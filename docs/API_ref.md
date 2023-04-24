<h1>API References</h1>

OADP install CRDs for the following resources
```
❯ ls  bundle/manifests/*oadp.openshift.io* bundle/manifests/velero.io* | xargs -I {} sh -c 'yq ".metadata.name" {} && echo "shortName: $(yq .spec.names.shortNames {})"'
volumesnapshotbackups.datamover.oadp.openshift.io
shortName: - vsb
volumesnapshotrestores.datamover.oadp.openshift.io
shortName: - vsr
cloudstorages.oadp.openshift.io
shortName: null
dataprotectionapplications.oadp.openshift.io
shortName: - dpa
backuprepositories.velero.io
shortName: null
backups.velero.io
shortName: null
backupstoragelocations.velero.io
shortName: - bsl
deletebackuprequests.velero.io
shortName: null
downloadrequests.velero.io
shortName: null
podvolumebackups.velero.io
shortName: null
podvolumerestores.velero.io
shortName: null
restores.velero.io
shortName: null
schedules.velero.io
shortName: null
serverstatusrequests.velero.io
shortName: - ssr
volumesnapshotlocations.velero.io
shortName: - vsl
```

You can use `oc explain <full-name|short-name>.<fields>` to explore available APIs

eg.
```
❯ oc explain dpa.spec.features
KIND:     DataProtectionApplication
VERSION:  oadp.openshift.io/v1alpha1

RESOURCE: features <Object>

DESCRIPTION:
     features defines the configuration for the DPA to enable the OADP tech
     preview features

FIELDS:
   dataMover	<Object>
     Contains data mover specific configurations
```

See also [![Go Reference](https://pkg.go.dev/badge/github.com/openshift/oadp-operator.svg)](https://pkg.go.dev/github.com/openshift/oadp-operator@master) for a deeper dive.
