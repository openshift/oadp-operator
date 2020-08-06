***
## Cleanup OADP Operator and NooBaa
***

Please follow the following steps in order to cleanup the resources deployed:

1. At first delete the deployment, role, role binding, service account and the cluster role binding, velero CRD and Velero CR using the commands:
    ```
    oc delete -f deploy/crds/konveyor.openshift.io_v1alpha1_velero_cr.yaml
    oc delete -f deploy/crds/konveyor.openshift.io_veleros_crd.yaml   
    oc delete -f deploy/
    ```
2. Now check if there are any instances present for the CRDs namely - backingstores, bucketclass, objectbucketclaim and objectbucket.objectbucket.io, if there are any instances present then delete them and if the deletion gets stuck then try removing the finalizers, it should work.
3. Once the instances of NooBaa CRDs are deleted, you need to remove the CRDs from cluster, you can use the following commands (also if the deletion of any CRD gets stuck, it implies that there are some instances remaining for that particular CRD and you will have to delete them):
    ```
    oc delete crd $(oc get crds | grep velero.io | awk -F ' ' '{print $1}')
    oc delete crd $(oc get crds | grep object | awk -F ' ' '{print $1}')
    oc delete crd $(oc get crds | grep rook.io | awk -F ' ' '{print $1}')
    oc delete crd $(oc get crds | grep noobaa.io | awk -F ' ' '{print $1}')
    ```
4. After this you need to delete the statefulset as well, do oc get statefulset and delete it, the pv-pool-backingstore pods must get deleted.
5. Finally, check for the presence of pvc(s) created by noobaa, if they are present delete them as well, once the pvc(s) are deleted, the pv(s) will also be cleaned up and you should be able to delete the oadp-operator namespace without any issues.
