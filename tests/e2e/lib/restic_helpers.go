package lib

import (
	"context"
	"log"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func HasCorrectNumResticPods(namespace string) wait.ConditionFunc {
	return func() (bool, error) {
		client, err := setUpClient()
		if err != nil {
			return false, err
		}
		resticOptions := metav1.ListOptions{
			FieldSelector: "metadata.name=restic",
		}
		resticDaemeonSet, err := client.AppsV1().DaemonSets(namespace).List(context.TODO(), resticOptions)
		if err != nil {
			return false, err
		}
		var numScheduled int32
		var numDesired int32

		for _, daemonSetInfo := range (*resticDaemeonSet).Items {
			numScheduled = daemonSetInfo.Status.CurrentNumberScheduled
			numDesired = daemonSetInfo.Status.DesiredNumberScheduled
		}
		// check correct num of Restic pods are initialized
		if numScheduled != 0 && numDesired != 0 {
			if numScheduled == numDesired {
				return true, nil
			}
		}
		if numDesired == 0 {
			return true, nil
		}
		return false, err
	}
}

func AreResticDaemonsetUpdatedAndReady(namespace string) wait.ConditionFunc {
	log.Printf("Checking if Restic daemonset is ready...")
	return func() (bool, error) {
		rds, err := GetResticDaemonSet(namespace, "restic")
		if err != nil {
			return false, err
		}
		if rds.Status.UpdatedNumberScheduled == rds.Status.DesiredNumberScheduled &&
			rds.Status.NumberUnavailable == 0 {
			return true, nil
		}
		log.Printf("Restic daemonset is not ready with condition: %v", rds.Status.Conditions)
		return false, nil
	}
}

func DoesDaemonSetExists(namespace string, resticName string) wait.ConditionFunc {
	log.Printf("Checking if restic daemonset exists...")
	return func() (bool, error) {
		_, err := GetResticDaemonSet(namespace, resticName)
		if err != nil {
			return false, err
		}
		return true, nil
	}
}

func GetResticDaemonSet(namespace, resticName string) (*appsv1.DaemonSet, error) {
	clientset, err := setUpClient()
	if err != nil {
		return nil, err
	}
	return clientset.AppsV1().DaemonSets(namespace).Get(context.Background(), resticName, metav1.GetOptions{})
}

// keep for now
func IsResticDaemonsetDeleted(namespace string) wait.ConditionFunc {
	log.Printf("Checking if Restic daemonset has been deleted...")
	return func() (bool, error) {
		_, err := GetResticDaemonSet(namespace, "restic")
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}
}

func (v *DpaCustomResource) DisableRestic(namespace string, instanceName string) error {
	err := v.SetClient()
	if err != nil {
		return err
	}
	dpa := &oadpv1alpha1.DataProtectionApplication{}
	err = v.Client.Get(context.Background(), client.ObjectKey{
		Namespace: v.Namespace,
		Name:      v.Name,
	}, dpa)
	if err != nil {
		return err
	}
	dpa.Spec.Configuration.Restic.Enable = pointer.Bool(false)

	err = v.Client.Update(context.Background(), dpa)
	if err != nil {
		return err
	}
	log.Printf("spec 'enable_restic' has been updated to false")
	return nil
}

func (v *DpaCustomResource) EnableResticNodeSelector(namespace string, s3Bucket string, credSecretRef string, instanceName string) error {
	err := v.SetClient()
	if err != nil {
		return err
	}
	dpa := &oadpv1alpha1.DataProtectionApplication{}
	err = v.Client.Get(context.Background(), client.ObjectKey{
		Namespace: v.Namespace,
		Name:      v.Name,
	}, dpa)
	if err != nil {
		return err
	}
	nodeSelector := map[string]string{"foo": "bar"}
	dpa.Spec.Configuration.Restic.PodConfig.NodeSelector = nodeSelector

	err = v.Client.Update(context.Background(), dpa)
	if err != nil {
		return err
	}
	log.Printf("spec 'restic_node_selector' has been updated")
	return nil
}

func ResticDaemonSetHasNodeSelector(namespace, key, value string) wait.ConditionFunc {
	return func() (bool, error) {
		client, err := setUpClient()
		if err != nil {
			return false, nil
		}
		ds, err := client.AppsV1().DaemonSets(namespace).Get(context.TODO(), "restic", metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		// verify daemonset has nodeSelector "foo": "bar"
		selector := ds.Spec.Template.Spec.NodeSelector[key]

		if selector == value {
			return true, nil
		}
		return false, err
	}
}

func GetResticDaemonsetList(namespace string) (*appsv1.DaemonSetList, error) {
	client, err := setUpClient()
	if err != nil {
		return nil, err
	}
	registryListOptions := metav1.ListOptions{
		LabelSelector: "component=velero",
	}
	// get pods in the oadp-operator-e2e namespace with label selector
	deploymentList, err := client.AppsV1().DaemonSets(namespace).List(context.TODO(), registryListOptions)
	if err != nil {
		return nil, err
	}
	return deploymentList, nil
}
