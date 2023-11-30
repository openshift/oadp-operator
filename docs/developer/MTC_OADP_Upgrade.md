<hr style="height:1px;border:none;color:#333;">
<h1 align="center">MTC-OADP Integration Upgrade</h1>

MTC 1.7 onwards, MTC will be depending on OADP for the provision of Velero and Restic. The following steps help in
simulating the MTC-OADP Integration upgrade scenario via OLM.

Prerequisites:
- MTC 1.6.3 installed
- OPM
- export ORG="your org name"
- export TAG="your tag name"
- Also take care of replacing the $ORG and $TAG values wherever necessary in the steps (specs/files/commands).

Before you follow the steps, install MTC `1.6.3` in the cluster and also make the required changes to MTC and OADP.

**MTC changes are as follows:**

_**bundle.Dockerfile:**_

```diff
- LABEL operators.operatorframework.io.bundle.package.v1=crane-operator
- LABEL operators.operatorframework.io.bundle.channels.v1=development
- LABEL operators.operatorframework.io.bundle.channel.default.v1=development
+ LABEL operators.operatorframework.io.bundle.package.v1=mtc-oadp-operator
+ LABEL operators.operatorframework.io.bundle.channels.v1=release-v1.7
+ LABEL operators.operatorframework.io.bundle.channel.default.v1=release-v1.7
```

_**deploy/olm-catalog/bundle/manifests/mtc-oadp-operator.v1.7.0.clusterserviceversion.yaml:**_
(observe that the csv file name is changed to `mtc-oadp-operator.v1.7.0.clusterserviceversion.yaml`)

```diff
-  name: crane-operator.v99.0.0
+  name: mtc-oadp-operator.v1.7.0
[...]
-  olm.skipRange: '>=0.0.0 <99.0.0'
+  olm.skipRange: '>=0.0.0 <1.7.0'
[...]
_  containerImage: quay.io/konveyor/mig-operator-container:latest
+  containerImage: quay.io/$ORG/mig-operator-container:$TAG
[...]
skips:
-  - crane-operator.v1.6.3
-  - crane-operator.v1.6.2
-  - crane-operator.v1.6.1
-  - crane-operator.v1.6.0
-  - crane-operator.v1.5.4
-  - crane-operator.v1.5.3
-  - crane-operator.v1.5.2
-  - crane-operator.v1.5.1
-  - crane-operator.v1.5.0
-  - crane-operator.v1.4.7
-  - crane-operator.v1.4.6
-  - crane-operator.v1.4.5
-  - crane-operator.v1.4.4
-  - crane-operator.v1.4.3
-  - crane-operator.v1.4.2
-  - crane-operator.v1.4.1
-  - crane-operator.v1.4.0
-  - crane-operator.v1.3.2
-  - crane-operator.v1.3.1
-  - crane-operator.v1.3.0
-  - crane-operator.v1.2.5
-  - crane-operator.v1.2.4
-  - crane-operator.v1.2.3
-  - crane-operator.v1.2.2
-  - crane-operator.v1.2.1
-  - crane-operator.v1.2.0
+  - mtc-operator.v1.6.3
[...]
  containers:
  - name: operator
-    image: quay.io/konveyor/mig-operator-container:latest
+    image: quay.io/$ORG/mig-operator-container:$TAG
[...]
- version: 99.0.0
+ version: 1.7.0
```

_**deploy/olm-catalog/bundle/metadata/annotations.yaml:**_
```diff
-  operators.operatorframework.io.bundle.channel.default.v1: development
-  operators.operatorframework.io.bundle.channels.v1: development
-  operators.operatorframework.io.bundle.package.v1: crane-operator
+  operators.operatorframework.io.bundle.channel.default.v1: release-v1.7
+  operators.operatorframework.io.bundle.channels.v1: release-v1.7
+  operators.operatorframework.io.bundle.package.v1: mtc-oadp-operator
```

_**deploy/olm-catalog/bundle/metadata/dependencies.yaml:**_
```diff
-    packageName: oadp-operator
-    version: ">=0.5.0"
+    packageName: mtc2-oadp-operator
+    version: ">=1.0.0"
```

**OADP changes are as follows:**

**_bundle.Dockerfile:_**
```diff
- LABEL operators.operatorframework.io.bundle.package.v1=oadp-operator
- LABEL operators.operatorframework.io.bundle.channels.v1=stable
- LABEL operators.operatorframework.io.bundle.channel.default.v1=stable
+ LABEL operators.operatorframework.io.bundle.package.v1=mtc2-oadp-operator
+ LABEL operators.operatorframework.io.bundle.channels.v1=release-v1.0
+ LABEL operators.operatorframework.io.bundle.channel.default.v1=release-v1.0
```

**_bundle/manifests/mtc2-oadp-operator.v1.0.0.clusterserviceversion.yaml:_**
(observe that the csv file name is changed to `mtc2-oadp-operator.v1.0.0.clusterserviceversion.yaml`)
```diff
- containerImage: quay.io/konveyor/oadp-operator:latest
+ containerImage: quay.io/$ORG/oadp-operator:$TAG
[...]
- olm.skipRange: '>=0.0.0 <99.0.0'
+ olm.skipRange: '>=0.0.0 <1.0.0'
[...]
- name: oadp-operator.v99.0.0
+ name: mtc2-oadp-operator.v1.0.0
[...]
- image: quay.io/konveyor/oadp-operator:latest
+ image: quay.io/$ORG/oadp-operator:$TAG
[...]
- version: 99.0.0
+ version: 1.0.0
```

**_bundle/metadata/annotations.yaml:_**
```diff
-  operators.operatorframework.io.bundle.package.v1: oadp-operator
-  operators.operatorframework.io.bundle.channels.v1: stable
-  operators.operatorframework.io.bundle.channel.default.v1: stable
+  operators.operatorframework.io.bundle.package.v1: mtc2-oadp-operator
+  operators.operatorframework.io.bundle.channels.v1: release-v1.0
+  operators.operatorframework.io.bundle.channel.default.v1: release-v1.0
```

Upgrade simulation steps:

1. Build the MTC bundle using the following command (from MTC root directory)
```
podman build -f bundle.Dockerfile -t quay.io/$ORG/mig-operator-bundle:$TAG .  
```
2. Push the MTC bundle image
```
podman push quay.io/$ORG/mig-operator-bundle:$TAG
```
3. Build the OADP Operator bundle using the following command (from OADP root directory)
```
podman build -f bundle.Dockerfile -t quay.io/$ORG/oadp-operator-bundle:$TAG .
```
4. Push the OADP bundle image
```
podman push quay.io/$ORG/oadp-operator-bundle:$TAG
```
5. Now, create an index image and the bundles of MRC and OADP to this index image
```
opm index add --container-tool podman --bundles quay.io/$ORG/mig-operator-bundle:$TAG,quay.io/$ORG/oadp-operator-bundle:$TAG --tag quay.io/$ORG/mig-operator-index:$TAG
```
6. Push the index image
```
podman push quay.io/$ORG/mig-operator-index:$TAG
```
7. Create a catalog source with this index image
```
apiVersion: operators.coreos.com/v1alpha1
kind: CatalogSource
metadata:
  name: mtc-oadp-operator
  namespace: openshift-marketplace
spec:
  sourceType: grpc
  image: quay.io/$ORG/mig-operator-index:$TAG
```
8. Now, edit the subscription of existing MTC instance (MTC `1.6.3`) and this should update the `installPlan` for MTC.
```diff
spec:
  channel: release-v1.7
  installPlanApproval: Automatic
  name: mtc-oadp-operator
  source: mtc-oadp-operator
  sourceNamespace: openshift-marketplace
  startingCSV: mtc-operator.v1.6.3
```
Debugging tips:
- Always stream the pod logs `catalog-operator` pod from the `openshift-operator-lifecycle-manager` namespace when you edit
the MTC operator subscription
- Lookout for the status of `installPlan`
- Also, check up on the health of the catalog source pods in `openshift-marketplace` namespace.