<hr style="height:1px;border:none;color:#333;">
<h1 align="center">Cleanup Noobaa</h1>
<hr style="height:1px;border:none;color:#333;">

1. At first delete the deployment, role, role binding, service account and the 
cluster role binding, Velero CRD and Velero CR using the commands:
    ```
    oc delete -f deploy/crds/konveyor.openshift.io_v1alpha1_velero_cr.yaml
    oc delete -f deploy/crds/konveyor.openshift.io_veleros_crd.yaml   
    oc delete -f deploy/
    ```
2. Now check if there are any instances present for the CRDs namely 
- backingstores, bucketclass, objectbucketclaim and objectbuckets.objectbucket.io.

3. Run `oc api-resources --verbs=list --namespaced -o name | xargs -n 1 oc get --show-kind --ignore-not-found -n oadp-operator-system` 
and `oc api-resources --verbs=list --namespaced -o name | xargs -n 1 oc get --show-kind --ignore-not-found -n openshift-storage` 
to see if there are any instances present then delete them and if the deletion 
gets stuck, then try removing the finalizers. It should work.

4. Once the instances of NooBaa CRDs are deleted, you need to remove the CRDs 
from the cluster. You can use the following commands (also if the deletion of 
any CRD gets stuck, it implies that there are some instances remaining for that 
particular CRD, and you will have to delete them):

    ```
    oc delete crd $(oc get crds | grep velero.io | awk -F ' ' '{print $1}')
    oc delete crd $(oc get crds | grep object | awk -F ' ' '{print $1}')
    oc delete crd $(oc get crds | grep rook.io | awk -F ' ' '{print $1}')
    oc delete crd $(oc get crds | grep noobaa.io | awk -F ' ' '{print $1}')
    ```
5. After this step, you need to delete the statefulset as well: 
`oc get statefulset` and delete it. The pv-pool-backingstore pods must get deleted.
6. Finally, check for the presence of pvc(s) created by Noobaa. If they are 
present, delete them as well. Once the pvc(s) are deleted, the pv(s) will also 
be cleaned up, and you should be able to delete the oadp-operator namespace 
without any issues.
