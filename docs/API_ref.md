<h1>API References</h1>

Pre-requisites: Install OADP to your cluster. before proceeding to the next steps.

You can use `oc explain <full-name|kind|short-name>.<fields>` to explore available APIs

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
