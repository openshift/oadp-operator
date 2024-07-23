package lib

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"reflect"
	"sort"
	"time"

	"github.com/google/go-cmp/cmp"
	volumesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	ocpappsv1 "github.com/openshift/api/apps/v1"
	security "github.com/openshift/api/security/v1"
	templatev1 "github.com/openshift/api/template/v1"
	"github.com/vmware-tanzu/velero/pkg/label"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	e2eAppLabelKey   = "e2e-app"
	e2eAppLabelValue = "true"
)

var e2eAppLabel = fmt.Sprintf("%s=%s", e2eAppLabelKey, e2eAppLabelValue)

func InstallApplication(ocClient client.Client, file string) error {
	return InstallApplicationWithRetries(ocClient, file, 3)
}

func InstallApplicationWithRetries(ocClient client.Client, file string, retries int) error {
	template, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	obj := &unstructured.UnstructuredList{}

	dec := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	_, _, err = dec.Decode([]byte(template), nil, obj)
	if err != nil {
		return err
	}
	for _, resource := range obj.Items {
		resourceLabels := resource.GetLabels()
		if resourceLabels == nil {
			resourceLabels = make(map[string]string)
		}
		resourceLabels[e2eAppLabelKey] = e2eAppLabelValue
		resource.SetLabels(resourceLabels)
		resourceCreate := resource.DeepCopy()
		err = nil // reset error for each resource
		for i := 0; i < retries; i++ {
			err = nil // reset error for each retry
			err = ocClient.Create(context.Background(), resourceCreate)
			if apierrors.IsAlreadyExists(err) {
				// if spec has changed for following kinds, update the resource
				clusterResource := unstructured.Unstructured{
					Object: resource.Object,
				}
				err = ocClient.Get(context.Background(), types.NamespacedName{Name: resource.GetName(), Namespace: resource.GetNamespace()}, &clusterResource)
				if err != nil {
					return err
				}
				if _, metadataExists := clusterResource.Object["metadata"]; metadataExists {
					// copy generation, resourceVersion, and annotations from the existing resource
					resource.SetGeneration(clusterResource.GetGeneration())
					resource.SetResourceVersion(clusterResource.GetResourceVersion())
					resource.SetUID(clusterResource.GetUID())
					resource.SetManagedFields(clusterResource.GetManagedFields())
					resource.SetCreationTimestamp(clusterResource.GetCreationTimestamp())
					resource.SetDeletionTimestamp(clusterResource.GetDeletionTimestamp())
					resource.SetFinalizers(clusterResource.GetFinalizers())
					// append cluster labels to existing labels if they don't already exist
					resourceLabels := resource.GetLabels()
					if resourceLabels == nil {
						resourceLabels = make(map[string]string)
					}
					for k, v := range clusterResource.GetLabels() {
						if _, exists := resourceLabels[k]; !exists {
							resourceLabels[k] = v
						}
					}
				}
				needsUpdate := false
				for key := range clusterResource.Object {
					if key == "status" {
						// check we aren't hitting pending deletion finalizers
						ginkgo.GinkgoWriter.Printf("%s has status %v", clusterResource.GroupVersionKind(), clusterResource.Object[key])
						continue
					}
					if !reflect.DeepEqual(clusterResource.Object[key], resource.Object[key]) {
						log.Println("diff found for key:", key)
						ginkgo.GinkgoWriter.Println(cmp.Diff(clusterResource.Object[key], resource.Object[key]))
						needsUpdate = true
						clusterResource.Object[key] = resource.Object[key]
					}
				}
				if needsUpdate {
					log.Printf("updating resource: %s; name: %s\n", resource.GroupVersionKind(), resource.GetName())
					err = ocClient.Update(context.Background(), &clusterResource)
				}
			}
			// if no error, stop retrying
			if err == nil {
				break
			}
			// if error, retry
			log.Printf("error creating or updating resource: %s; name: %s; error: %s; retrying for %d more times\n", resource.GroupVersionKind(), resource.GetName(), err, retries-i)
		}
		// if still error on this resource, return error
		if err != nil {
			return err
		}
		// next resource
	}
	return nil
}

func DoesSCCExist(ocClient client.Client, sccName string) error {
	return ocClient.Get(
		context.Background(),
		client.ObjectKey{Name: sccName},
		&security.SecurityContextConstraints{},
	)
}

func UninstallApplication(ocClient client.Client, file string) error {
	template, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	obj := &unstructured.UnstructuredList{}

	dec := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	_, _, err = dec.Decode([]byte(template), nil, obj)
	if err != nil {
		return err
	}
	for _, resource := range obj.Items {
		err = ocClient.Delete(context.Background(), &resource)
		if apierrors.IsNotFound(err) {
			continue
		} else if err != nil {
			return err
		}
	}
	return nil
}

func HasDCsInNamespace(ocClient client.Client, namespace string) (bool, error) {
	dcList := &ocpappsv1.DeploymentConfigList{}
	err := ocClient.List(context.Background(), dcList, client.InNamespace(namespace))
	if err != nil {
		return false, err
	}
	if len(dcList.Items) == 0 {
		return false, nil
	}
	return true, nil
}

func HasReplicationControllersInNamespace(ocClient client.Client, namespace string) (bool, error) {
	rcList := &corev1.ReplicationControllerList{}
	err := ocClient.List(context.Background(), rcList, client.InNamespace(namespace))
	if err != nil {
		return false, err
	}
	if len(rcList.Items) == 0 {
		return false, nil
	}
	return true, nil
}

func HasTemplateInstancesInNamespace(ocClient client.Client, namespace string) (bool, error) {
	tiList := &templatev1.TemplateInstanceList{}
	err := ocClient.List(context.Background(), tiList, client.InNamespace(namespace))
	if err != nil {
		return false, err
	}
	if len(tiList.Items) == 0 {
		return false, nil
	}
	return true, nil
}

func NamespaceRequiresResticDCWorkaround(ocClient client.Client, namespace string) (bool, error) {
	hasDC, err := HasDCsInNamespace(ocClient, namespace)
	if err != nil {
		return false, err
	}
	hasRC, err := HasReplicationControllersInNamespace(ocClient, namespace)
	if err != nil {
		return false, err
	}
	hasTI, err := HasTemplateInstancesInNamespace(ocClient, namespace)
	if err != nil {
		return false, err
	}
	return hasDC || hasRC || hasTI, nil
}

func AreVolumeSnapshotsReady(ocClient client.Client, backupName string) wait.ConditionFunc {
	return func() (bool, error) {
		vList := &volumesnapshotv1.VolumeSnapshotContentList{}
		err := ocClient.List(context.Background(), vList, &client.ListOptions{LabelSelector: label.NewSelectorForBackup(backupName)})
		if err != nil {
			return false, err
		}
		if len(vList.Items) == 0 {
			ginkgo.GinkgoWriter.Println("No VolumeSnapshotContents found")
			return false, nil
		}
		for _, v := range vList.Items {
			log.Println(fmt.Sprintf("waiting for volume snapshot contents %s to be ready", v.Name))
			if v.Status.ReadyToUse == nil {
				ginkgo.GinkgoWriter.Println("VolumeSnapshotContents Ready status not found for " + v.Name)
				ginkgo.GinkgoWriter.Println(fmt.Sprintf("status: %v", v.Status))
				return false, nil
			}
			if !*v.Status.ReadyToUse {
				ginkgo.GinkgoWriter.Println("VolumeSnapshotContents Ready status is false " + v.Name)
				return false, nil
			}
		}
		return true, nil
	}
}

func IsDCReady(ocClient client.Client, namespace, dcName string) wait.ConditionFunc {
	return func() (bool, error) {
		dc := ocpappsv1.DeploymentConfig{}
		err := ocClient.Get(context.Background(), client.ObjectKey{
			Namespace: namespace,
			Name:      dcName,
		}, &dc)
		if err != nil {
			return false, err
		}
		// check dc for false availability condition which occurs when a new replication controller is created (after a new build completed) even if there are satisfactory available replicas
		for _, condition := range dc.Status.Conditions {
			log.Printf("DeploymentConfig %s Status.Conditions:\n%#v", dc.Name, condition)
			if condition.Type == ocpappsv1.DeploymentAvailable {
				if condition.Status == corev1.ConditionFalse {
					log.Printf("DeploymentConfig %s has condition.Status False", dc.Name)
					return false, nil
				}
				break
			}
		}
		if dc.Status.AvailableReplicas != dc.Status.Replicas || dc.Status.Replicas == 0 {
			for _, condition := range dc.Status.Conditions {
				if len(condition.Message) > 0 {
					ginkgo.GinkgoWriter.Write([]byte(fmt.Sprintf("DC not available with condition: %s\n", condition.Message)))
				}
			}
			return false, errors.New("DC is not in a ready state")
		}
		return true, nil
	}
}

func IsDeploymentReady(ocClient client.Client, namespace, dName string) wait.ConditionFunc {
	return func() (bool, error) {
		deployment := appsv1.Deployment{}
		err := ocClient.Get(context.Background(), client.ObjectKey{
			Namespace: namespace,
			Name:      dName,
		}, &deployment)
		if err != nil {
			return false, err
		}
		if deployment.Status.AvailableReplicas != deployment.Status.Replicas || deployment.Status.Replicas == 0 {
			for _, condition := range deployment.Status.Conditions {
				if len(condition.Message) > 0 {
					ginkgo.GinkgoWriter.Write([]byte(fmt.Sprintf("deployment not available with condition: %s\n", condition.Message)))
				}
			}
			return false, errors.New("deployment is not in a ready state")
		}
		return true, nil
	}
}

func AreApplicationPodsRunning(c *kubernetes.Clientset, namespace string) wait.ConditionFunc {
	return func() (bool, error) {
		podList, err := GetAllPodsWithLabel(c, namespace, e2eAppLabel)
		if err != nil {
			return false, err
		}

		for _, pod := range podList.Items {
			if pod.DeletionTimestamp != nil {
				log.Printf("Pod %v is terminating", pod.Name)
				return false, fmt.Errorf("Pod is terminating")
			}

			phase := pod.Status.Phase
			if phase != corev1.PodRunning && phase != corev1.PodSucceeded {
				log.Printf("Pod %v not yet succeeded: phase is %v", pod.Name, phase)
				return false, fmt.Errorf("Pod not yet succeeded")
			}

			for _, condition := range pod.Status.Conditions {
				log.Printf("Pod %v condition:\n%#v", pod.Name, condition)
				if condition.Type == corev1.ContainersReady && condition.Status != corev1.ConditionTrue {
					log.Printf("Pod %v not yet succeeded: condition is: %v", pod.Name, condition.Status)
					return false, fmt.Errorf("Pod not yet succeeded")
				}
			}
		}
		return true, nil
	}
}

func PrintNamespaceEventsAfterTime(c *kubernetes.Clientset, namespace string, startTime time.Time) {
	log.Println("Printing events for namespace: ", namespace)
	events, err := c.CoreV1().Events(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		ginkgo.GinkgoWriter.Write([]byte(fmt.Sprintf("Error getting events: %v\n", err)))
		return
	}
	eventItems := events.Items
	// sort events by timestamp
	sort.Slice(eventItems, func(i, j int) bool {
		return eventItems[i].LastTimestamp.Before(&eventItems[j].LastTimestamp)
	})
	for _, event := range eventItems {
		// only get events after time
		if event.LastTimestamp.After(startTime) {
			ginkgo.GinkgoWriter.Println(fmt.Sprintf("Event: %v, Type: %v, Count: %v, Src: %v, Reason: %v", event.Message, event.Type, event.Count, event.InvolvedObject, event.Reason))
		}
	}
}

func RunMustGather(oc_cli string, artifact_dir string) error {
	ocClient := oc_cli
	ocAdmin := "adm"
	mustGatherCmd := "must-gather"
	mustGatherImg := "--image=quay.io/konveyor/oadp-must-gather:latest"
	destDir := "--dest-dir=" + artifact_dir

	cmd := exec.Command(ocClient, ocAdmin, mustGatherCmd, mustGatherImg, destDir)
	_, err := cmd.Output()
	return err
}

// VerifyBackupRestoreData verifies if app ready before backup and after restore to compare data.
func VerifyBackupRestoreData(ocClient client.Client, kubeClient *kubernetes.Clientset, kubeConfig *rest.Config, artifactDir string, namespace string, routeName string, serviceName string, app string, prebackupState bool, twoVol bool) error {
	log.Printf("Verifying backup/restore data of %s", app)
	appEndpointURL, proxyPodParams, err := getAppEndpointURLAndProxyParams(ocClient, kubeClient, kubeConfig, namespace, serviceName, routeName)
	if err != nil {
		return err
	}

	// Construct request parameters for the "todo-incomplete" endpoint
	requestParamsTodoIncomplete := getRequestParameters(appEndpointURL+"/todo-incomplete", proxyPodParams, GET, nil)

	if prebackupState {
		// Clean up existing backup file
		RemoveFileIfExists(artifactDir + "/backup-data.txt")

		// Make requests and update data before backup
		dataBeforeCurl, errResp, err := MakeRequest(*requestParamsTodoIncomplete)
		if err != nil {
			if errResp != "" {
				log.Printf("Request response error msg: %s\n", errResp)
			}
			return err
		}
		log.Printf("Data before the curl request: \n %s\n", dataBeforeCurl)

		// Make two post requests to the "todo" endpoint
		postPayload := `{"description": "` + time.Now().String() + `"}`
		requestParams := getRequestParameters(appEndpointURL+"/todo", proxyPodParams, POST, &postPayload)
		MakeRequest(*requestParams)

		postPayload = `{"description": "` + time.Now().Weekday().String() + `"}`
		requestParams = getRequestParameters(appEndpointURL+"/todo", proxyPodParams, POST, &postPayload)
		MakeRequest(*requestParams)
	}

	// Make request to the "todo-incomplete" endpoint
	respData, errResp, err := MakeRequest(*requestParamsTodoIncomplete)
	if err != nil {
		if errResp != "" {
			log.Printf("Request response error msg: %s\n", errResp)
		}
		return err
	}

	if prebackupState {
		// Write data to backup file
		log.Printf("Writing data to backupFile (backup-data.txt): \n %s\n", respData)
		if err := os.WriteFile(artifactDir+"/backup-data.txt", []byte(respData), 0644); err != nil {
			return err
		}
	} else {
		// Compare data with backup file after restore
		backupData, err := os.ReadFile(artifactDir + "/backup-data.txt")
		if err != nil {
			return err
		}
		log.Printf("Data came from backup-file\n %s\n", backupData)
		log.Printf("Data came from response\n %s\n", respData)
		trimmedBackup := bytes.TrimSpace(backupData)
		trimmedResp := bytes.TrimSpace([]byte(respData))
		if !bytes.Equal(trimmedBackup, trimmedResp) {
			return errors.New("Backup and Restore Data are not the same")
		}
	}

	if twoVol {
		// Verify volume data if needed
		requestParamsVolume := getRequestParameters(appEndpointURL+"/log", proxyPodParams, GET, nil)
		volumeFile := artifactDir + "/volume-data.txt"
		return verifyVolume(requestParamsVolume, volumeFile, prebackupState)
	}

	return nil
}

func getRequestParameters(url string, proxyPodParams *ProxyPodParameters, method HTTPMethod, payload *string) *RequestParameters {
	return &RequestParameters{
		ProxyPodParams: proxyPodParams,
		RequestMethod:  &method,
		URL:            url,
		Payload:        payload,
	}
}

func getAppEndpointURLAndProxyParams(ocClient client.Client, kubeClient *kubernetes.Clientset, kubeConfig *rest.Config, namespace, serviceName, routeName string) (string, *ProxyPodParameters, error) {
	appEndpointURL, err := GetRouteEndpointURL(ocClient, namespace, routeName)
	// Something wrong with standard endpoint, try with proxy pod.
	if err != nil {
		log.Println("Can not connect to the application endpoint with route:", err)
		log.Println("Trying to get to the service via proxy POD")

		pod, podErr := GetFirstPodByLabel(kubeClient, namespace, "curl-tool=true")
		if podErr != nil {
			return "", nil, fmt.Errorf("Error getting pod for the proxy command: %v", podErr)
		}

		proxyPodParams := &ProxyPodParameters{
			KubeClient:    kubeClient,
			KubeConfig:    kubeConfig,
			Namespace:     namespace,
			PodName:       pod.ObjectMeta.Name,
			ContainerName: "curl-tool",
		}

		appEndpointURL = GetInternalServiceEndpointURL(namespace, serviceName)

		return appEndpointURL, proxyPodParams, nil
	}

	return appEndpointURL, nil, nil
}

// VerifyVolumeData for application with two volumes
func verifyVolume(requestParams *RequestParameters, volumeFile string, prebackupState bool) error {
	var volData string
	var err error

	// Function to get response
	getResponseFromVolumeCall := func() bool {
		// Attempt to make the request
		var errResp string
		volData, errResp, err = MakeRequest(*requestParams)
		if err != nil {
			if errResp != "" {
				log.Printf("Request response error msg: %s\n", errResp)
			}
			log.Printf("Request errored out: %v\n", err)
			return false
		}
		return true
	}

	if success := gomega.Eventually(getResponseFromVolumeCall, time.Minute*4, time.Second*15).Should(gomega.BeTrue()); !success {
		return fmt.Errorf("Failed to get response for volume: %v", err)
	}

	if prebackupState {
		// delete volumeFile if it exists
		RemoveFileIfExists(volumeFile)

		log.Printf("Writing data to volumeFile (volume-data.txt): \n %s", volData)
		err := os.WriteFile(volumeFile, []byte(volData), 0644)
		if err != nil {
			return err
		}

	} else {
		volumeBackupData, err := os.ReadFile(volumeFile)
		if err != nil {
			return err
		}
		log.Printf("Data came from volume-file\n %s", volumeBackupData)
		log.Printf("Volume Data after restore\n %s", volData)
		dataIsIn := bytes.Contains([]byte(volData), volumeBackupData)
		if dataIsIn != true {
			return errors.New("Backup data is not in Restore Data")
		}
	}
	return nil
}
