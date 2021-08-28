package controllers

import (
	"fmt"
	"time"

	"github.com/go-logr/logr"

	// noobaav1alpha1 "github.com/noobaa/noobaa-operator/v2/pkg/apis/noobaa/v1alpha1"
	// ocsv1 "github.com/openshift/ocs-operator/api/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	// "k8s.io/client-go/tools/clientcmd"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"golang.org/x/net/context"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	// "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// "k8s.io/apimachinery/pkg/types"
	// coreV1Types "k8s.io/client-go/kubernetes/typed/core/v1"
	// k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	// obv1 "github.com/kube-object-storage/lib-bucket-provisioner/pkg/apis/objectbucket.io/v1alpha1"
	// "github.com/kube-object-storage/lib-bucket-provisioner/pkg/provisioner/api"
)

func (r *VeleroReconciler) ValidateNoobaa(log logr.Logger) (bool, error) {
	velero := oadpv1alpha1.Velero{}
	if err := r.Get(r.Context, r.NamespacedName, &velero); err != nil {
		return false, err
	}

	//Validation logic for noobaa
	//check if noobaa:true is present, if present proceed
	if velero.Spec.Noobaa {

		fmt.Println("Noobaa is true, perform validation below.")

		//check no vsl or bsl are present

		//default plugins should only consist of aws plugin

		//enable_restic:true flag present

		//check if ocs and noobaa is up and running
	}

	return true, nil
}

func (r *VeleroReconciler) ReconcileNoobaa(log logr.Logger) (bool, error) {
	//We will need 4 things from noobaa, ie. AWS Access Key, AWS Secret Access Key, S3 Endpoint, and Bucket Name to setup this feature. This feature sets up Velero without the need for user specified backupstoragelocations. As of now, this
	//The workflow is as below:
	//1. Create StorageCluster resource, this installs all the noobaa pods, compute and storage resources. This step also sets up all the AWS Creds and the S3 endpoint that can be plugged into the Velero CR in the next steps.
	//2. Create ObjectBucketClaim, which then creates ObjectBucket resource, which creates an S3 backed bucket. The objectbucketclaim, objectbucket and bucketname are unique, and the bucketname is to be plugged into the Velero CR along with the other information in step 2.
	//3. The creds are present in data object in the noobaa-admin pod, in the openshift-storage namespace. We read them from there and add them as a secret in the oadp-operator-system namespace.
	//4. The S3 endpoint and Bucket Name are then plugged into the BSL field in the Velero CR.

	fmt.Println("Step 1, get kubeconfig")

	kubeConf := getKubeConfig()
	kubeClient, err := dynamic.NewForConfig(kubeConf)
	if err != nil {
		return false, err
	}

	clientset, err := kubernetes.NewForConfig(kubeConf)
	if err != nil {
		return true, err
	}

	openshiftStorageNamespace := "openshift-storage"
	velero := oadpv1alpha1.Velero{}
	if err := r.Get(r.Context, r.NamespacedName, &velero); err != nil {
		return false, err
	}

	// noobaa := noobaav1alpha1.NooBaa{}
	// if err := r.Get(r.Context, r.NamespacedName, &noobaa); err != nil {
	// 	return false, err
	// }

	//Reconcile logic for Noobaa

	//check if noobaa:true flag is present, if present proceed
	if velero.Spec.Noobaa {
		fmt.Println("Step 3, inside the loop")
		fmt.Println("Entering Noobaa reconciler")

		//TODO: Logic to check if there is a current storage cluster present. Currently the client creates a storagecluster and errors out, as it keeps trying to create another one, and it aleady exists.
		//TODO: Implement same logic using CreateOrUpdate functionality, did not implement as was short on time. 
		storageClusterRes := schema.GroupVersionResource{Group: "ocs.openshift.io", Version: "v1", Resource: "storageclusters"}
		_, err := kubeClient.Resource(storageClusterRes).Namespace(openshiftStorageNamespace).Get(context.TODO(), "oadp-ocs-storagecluster",metav1.GetOptions{})
		if err != nil && !apierrors.IsAlreadyExists(err){

		//creating the storageClusterRes and storageCluster object if it does not exist
		//referenced from the ocs operator README.md "https://github.com/openshift/ocs-operator/blob/master/README.md"
		storageCluster := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "ocs.openshift.io/v1",
				"kind":       "StorageCluster",
				"metadata": map[string]interface{}{
					"namespace": "openshift-storage",
					"name":      "oadp-ocs-storagecluster",
				},
				"spec": map[string]interface{}{
					"manageNodes": false,
					"monPVCTemplate": map[string]interface{}{
						"spec": map[string]interface{}{
							"storageClassName": "gp2",
							"accessModes": []string{
								"ReadWriteOnce",
							},
							"resources": map[string]interface{}{

								"requests": map[string]interface{}{

									"storage": "10Gi",
								},
							},
						},
					},
					"storageDeviceSets": []map[string]interface{}{
						{
							"name":     "oadp-ocs-deviceset",
							"count":    3,
							"portable": true,
							"dataPVCTemplate": map[string]interface{}{
								"spec": map[string]interface{}{
									"storageClassName": "gp2",
									"accessModes": []string{
										"ReadWriteOnce",
									},
									"volumeMode": "Block",
									"resources": map[string]interface{}{

										"requests": map[string]interface{}{

											"storage": "1Ti",
										},
									},
								},
							},
						},
					},
				},
			},
		}

		// Create StorageCluster
		fmt.Println("Creating OADP OCS StorageCluster...")
		_, err = kubeClient.Resource(storageClusterRes).Namespace(openshiftStorageNamespace).Create(context.TODO(), storageCluster, metav1.CreateOptions{})
		if err != nil {
			return false, err
			}
		}

		//TODO: Track the state of the StorageCluster, and when it transitions from Progressing to Ready, initiate creation of the ObjectBucketClaim. This implementation right now is a hacky and undesirable way of achieving this and was done due to lack of time.
		//The ObjectBucketClaim depends on the StorageClass to initialize, which is created after the StorageCluster is created above.
		time.Sleep(8 * time.Minute)



		//create an objectbucketclaim (which creates a bucket)
		//referenced from here. https://access.redhat.com/documentation/en-us/red_hat_openshift_container_storage/4.2/html/managing_openshift_container_storage/configure-storage-for-openshift-container-platform-services_rhocs#object-bucket-claim
		//actually creating the obc
		objectBucketClaimRes := schema.GroupVersionResource{Group: "objectbucket.io", Version: "v1alpha1", Resource: "objectbucketclaims"}
		bucketName := "oadp-noobaa-bucket"

		//check if obc exists
		//TODO: Implement same logic using CreateOrUpdate functionality
		_, err = kubeClient.Resource(objectBucketClaimRes).Namespace(openshiftStorageNamespace).Get(context.TODO(), "oadp-noobaa-obc",metav1.GetOptions{})
		if err != nil && !apierrors.IsAlreadyExists(err){

		//if it does not exist, then create a new one
		noobaaObjectBucketClaim := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "objectbucket.io/v1alpha1",
				"kind":       "ObjectBucketClaim",
				"metadata": map[string]interface{}{
					"name": "oadp-noobaa-obc",
				},
				"spec": map[string]interface{}{
					"bucketName":       bucketName,
					"objectBucketName": "oadp-noobaa-objectbucket",
					"storageClassName": "openshift-storage.noobaa.io",
				},
			},
		}
		fmt.Println("Step 6")
		// Creating ObjectBucketClaim using the client
		fmt.Println("Creating ObjectBucketClaim...")
		_, err = kubeClient.Resource(objectBucketClaimRes).Namespace(openshiftStorageNamespace).Create(context.TODO(), noobaaObjectBucketClaim, metav1.CreateOptions{})
		if err != nil {
			return false, err
		}
		}
		// if err != nil {
		// 	return false, err
		// }


		// fmt.Printf("Created ObjectBucketClaim %q.\n", result.GetName())

		fmt.Println("Step 7")
		//fetch the secret from openshift-storage namespace
		noobaaSecret := &corev1.Secret{}
		noobaaSecret, err = clientset.CoreV1().Secrets(openshiftStorageNamespace).Get(context.TODO(), "noobaa-admin", metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		//getting the value of access key and secret access key from Data object in noobaa-admin pod
		AccessKey := noobaaSecret.Data["AWS_ACCESS_KEY_ID"]
		SecretAccessKey := noobaaSecret.Data["AWS_ACCESS_KEY_ID"]

		//formatting the info for use as a secret
		var secretDataString string = "[default]" + "\n" +
			"aws_access_key_id=" + string(AccessKey) + "\n" +
			"aws_secret_access_key=" + string(SecretAccessKey)

		var secretData = map[string][]byte{
			"cloud": []byte(secretDataString),
		}

		fmt.Println("Step 8")

		//creating new secret for oadp-operator-system namespace
		oadpSecret := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cloud-credentials",
				Namespace: "oadp-operator-system",
			},
			TypeMeta: metav1.TypeMeta{
				Kind:       "Secret",
				APIVersion: metav1.SchemeGroupVersion.String(),
			},
			Data: secretData,
			Type: corev1.SecretTypeOpaque,
		}

		// Creating oadpSecret using the client
		fmt.Println("Creating OADP Secret...")
		_, err = clientset.CoreV1().Secrets(openshiftStorageNamespace).Create(context.TODO(), &oadpSecret, metav1.CreateOptions{})
		if err != nil {
			return false, err
		}
		if apierrors.IsAlreadyExists(err) {

			return false, err
		}

		fmt.Println("Step 9")

		//OADP creates a BackupStorageLocation that points to this bucket
		bsl := velerov1.BackupStorageLocation{
			ObjectMeta: metav1.ObjectMeta{
				// TODO: Use a hash instead of i
				Name:      r.NamespacedName.Name,
				Namespace: r.NamespacedName.Namespace,
			},
			Spec: velerov1.BackupStorageLocationSpec{
				Provider: "velero.io/aws",
				StorageType: velerov1.StorageType{
					ObjectStorage: &velerov1.ObjectStorageLocation{
						Bucket: bucketName,
						Prefix: "velero",
					},
				},
				//TODO: Add S3 url from noobaa status field as mentioned below from unstructured
				// Config: map[string]string{
				// 	S3URL: noobaa.Status.Services.ServiceS3.ExternalDNS[0],
				// },
			},
		}

		//TODO: Add logic to create programmatically create BSL

		fmt.Println("Step 10")

		op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, &bsl, func() error {
			// TODO: Velero may be setting controllerReference as
			// well and taking ownership. If so move this to
			// SetOwnerReference instead

			//TODO: add logic to reconcile all of above using controller runtime reconcile loop

			return err
		})
		if err != nil {
			return false, err
		}

		fmt.Println("Step 11")

		if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
			// Trigger event to indicate BSL was created or updated
			r.EventRecorder.Event(&bsl,
				corev1.EventTypeNormal,
				"BackupStorageLocationReconciled",
				fmt.Sprintf("performed %s on backupstoragelocation %s/%s", op, bsl.Namespace, bsl.Name),
			)
		}

	}

	return true, nil
}

// func createOCSClient(client dynamic.Interface, namespace string) (dynamic.ResourceInterface, error) {
// 	resourceClient := client.Resource(schema.GroupVersionResource{
// 		Group:    "ocs.openshift.io",
// 		Version:  "v1",
// 		Resource: "storageclusters",
// 	})
// 	namespaceResClient := resourceClient.Namespace(namespace)

// 	return namespaceResClient, nil
// }

// func createNoobaaClient(client dynamic.Interface, namespace string) (dynamic.ResourceInterface, error) {
// 	resourceClient := client.Resource(schema.GroupVersionResource{
// 		Group:    "noobaa.io",
// 		Version:  "v1alpha1",
// 		Resource: "noobaas",
// 	})
// 	namespaceResClient := resourceClient.Namespace(namespace)

// 	return namespaceResClient, nil
// }

func getKubeConfig() *rest.Config {
	return config.GetConfigOrDie()
}
