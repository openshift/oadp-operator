package lib

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"
	volumesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	"github.com/onsi/ginkgo/v2"
	ocpappsv1 "github.com/openshift/api/apps/v1"
	buildv1 "github.com/openshift/api/build/v1"
	routev1 "github.com/openshift/api/route/v1"
	security "github.com/openshift/api/security/v1"
	templatev1 "github.com/openshift/api/template/v1"
	"github.com/vmware-tanzu/velero/pkg/label"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

const e2eAppLabelKey = "e2e-app"
const e2eAppLabelValue = "true"

var (
	e2eAppLabelRequirement, _ = labels.NewRequirement(e2eAppLabelKey, selection.Equals, []string{e2eAppLabelValue})
	e2eAppLabelSelector       = labels.NewSelector().Add(*e2eAppLabelRequirement)
)

func InstallApplication(ocClient client.Client, file string) error {
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
		labels := resource.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}
		labels[e2eAppLabelKey] = "true"
		resource.SetLabels(labels)
		err = ocClient.Create(context.Background(), &resource)
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
			}
			needsUpdate := false
			for key := range clusterResource.Object {
				if key == "status" {
					continue
				}
				if !reflect.DeepEqual(clusterResource.Object[key], resource.Object[key]) {
					fmt.Println("diff found for key:", key)
					ginkgo.GinkgoWriter.Println(cmp.Diff(clusterResource.Object[key], resource.Object[key]))
					needsUpdate = true
					clusterResource.Object[key] = resource.Object[key]
				}
			}
			if needsUpdate {
				fmt.Printf("updating resource: %s; name: %s\n", resource.GetKind(), resource.GetName())
				err = ocClient.Update(context.Background(), &clusterResource)
				if err != nil {
					return err
				}
			}
		} else if err != nil {
			return err
		}
	}
	return nil
}

func DoesSCCExist(ocClient client.Client, sccName string) (bool, error) {
	scc := security.SecurityContextConstraints{}
	err := ocClient.Get(context.Background(), client.ObjectKey{
		Name: sccName,
	}, &scc)
	if err != nil {
		return false, err
	}
	return true, nil

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
		// vListBeta := &volumesnapshotv1beta1.VolumeSnapshotList{}
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
			if condition.Type == ocpappsv1.DeploymentAvailable {
				if condition.Status == corev1.ConditionFalse {
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
		for _, trigger := range dc.Spec.Triggers {
			if trigger.Type == ocpappsv1.DeploymentTriggerOnImageChange {
				if trigger.ImageChangeParams.Automatic {
					return areAppBuildsReady(ocClient, namespace)
				}
			}
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

func AreAppBuildsReady(ocClient client.Client, namespace string) wait.ConditionFunc {
	return func() (bool, error) {
		return areAppBuildsReady(ocClient, namespace)
	}
}

func areAppBuildsReady(ocClient client.Client, namespace string) (bool, error) {
	buildList := &buildv1.BuildList{}
	err := ocClient.List(context.Background(), buildList, &client.ListOptions{Namespace: namespace, LabelSelector: e2eAppLabelSelector})
	if err != nil && !apierrors.IsNotFound(err) {
		return false, err
	}
	if buildList.Items != nil {
		for _, build := range buildList.Items {
			if build.Status.Phase == buildv1.BuildPhaseNew ||
				build.Status.Phase == buildv1.BuildPhasePending ||
				build.Status.Phase == buildv1.BuildPhaseRunning {
				log.Println("Build is not ready: " + build.Name)
				return false, nil
			}
			if build.Status.Phase == buildv1.BuildPhaseFailed || build.Status.Phase == buildv1.BuildPhaseError {
				ginkgo.GinkgoWriter.Println("Build failed/error: " + build.Name)
				ginkgo.GinkgoWriter.Println(fmt.Sprintf("status: %v", build.Status))
				return false, errors.New("found build failed or error")
			}
			if build.Status.Phase == buildv1.BuildPhaseComplete {
				log.Println("Build is complete: " + build.Name + " patching build pod label to exclude from backup")
				podName := build.GetAnnotations()["openshift.io/build.pod-name"]
				pod := corev1.Pod{}
				err := ocClient.Get(context.Background(), client.ObjectKey{
					Namespace: namespace,
					Name:      podName,
				}, &pod)
				if err != nil {
					return false, err
				}
				pod.Labels["velero.io/exclude-from-backup"] = "true"
				err = ocClient.Update(context.Background(), &pod)
				if err != nil {
					log.Println("Error patching build pod label to exclude from backup: " + err.Error())
					return false, err
				}
			}
		}
	}
	return true, nil
}

func AreApplicationPodsRunning(namespace string) wait.ConditionFunc {
	return func() (bool, error) {
		clientset, err := setUpClient()
		if err != nil {
			return false, err
		}
		// select Velero pod with this label
		veleroOptions := metav1.ListOptions{
			LabelSelector: e2eAppLabelSelector.String(),
		}
		// get pods in test namespace with labelSelector
		podList, err := clientset.CoreV1().Pods(namespace).List(context.TODO(), veleroOptions)
		if err != nil {
			return false, nil
		}
		if len(podList.Items) == 0 {
			return false, nil
		}
		// get pod name and status with specified label selector
		for _, podInfo := range podList.Items {
			phase := podInfo.Status.Phase
			if phase != corev1.PodRunning && phase != corev1.PodSucceeded {
				ginkgo.GinkgoWriter.Write([]byte(fmt.Sprintf("Pod %v not yet succeeded", podInfo.Name)))
				ginkgo.GinkgoWriter.Write([]byte(fmt.Sprintf("status: %v", podInfo.Status)))
				return false, nil
			}
		}
		return true, err
	}
}

func PrintNamespaceEventsAfterTime(namespace string, startTime time.Time) {
	log.Println("Printing events for namespace: ", namespace)
	clientset, err := setUpClient()
	if err != nil {
		ginkgo.GinkgoWriter.Write([]byte(fmt.Sprintf("Error getting client: %v\n", err)))
		return
	}
	events, err := clientset.CoreV1().Events(namespace).List(context.TODO(), metav1.ListOptions{})
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
	logCmdPmt := "--"
	logCmd := "gather_logs_without_zip"

	cmd := exec.Command(ocClient, ocAdmin, mustGatherCmd, mustGatherImg, destDir, logCmdPmt, logCmd)
	_, err := cmd.Output()
	return err
}

func makeRequest(request string, api string, todo string) {
	params := url.Values{}
	params.Add("description", todo)
	body := strings.NewReader(params.Encode())
	req, err := http.NewRequest(request, api, body)
	if err != nil {
		log.Printf("Error making post request to todo app before Prebackup  %s", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("Response of todo POST REQUEST  %s", resp.Status)
	}
	defer resp.Body.Close()
}

// VerifyBackupRestoreData verifies if app ready before backup and after restore to compare data.
func VerifyBackupRestoreData(artifact_dir string, namespace string, routeName string, app string, prebackupState bool, backupRestoretype BackupRestoreType) error {
	log.Printf("Verifying backup/restore data of %s", app)
	appRoute := &routev1.Route{}
	clientv1, err := client.New(config.GetConfigOrDie(), client.Options{})
	if err != nil {
		return err
	}
	backupFile := artifact_dir + "/backup-data.txt"
	routev1.AddToScheme(clientv1.Scheme())
	err = clientv1.Get(context.Background(), client.ObjectKey{
		Namespace: namespace,
		Name:      routeName,
	}, appRoute)
	if err != nil {
		return err
	}
	appApi := "http://" + appRoute.Spec.Host

	//if this is prebackstate = true, add items via makeRequest function. We only want to make request before backup
	//and ignore post restore checks.
	if prebackupState {
		// delete backupFile if it exists
		if _, err := os.Stat(backupFile); err == nil {
			os.Remove(backupFile)
		}
		//data before curl request
		dataBeforeCurl, err := getResponseData(appApi + "/todo-incomplete")
		if err != nil {
			return err
		} else {
			fmt.Printf("Data before the curl request: \n %s\n", dataBeforeCurl)
		}
		//make post request to given api
		switch app {
		case "todolist":
			fmt.Printf("PrebackState: %t, so make a curl request\n", prebackupState)
			makeRequest("POST", appApi+"/todo", time.Now().String())
			makeRequest("POST", appApi+"/todo", time.Now().Weekday().String())
		}
	}
	switch app {
	case "todolist":
		appApi += "/todo-incomplete"
	}

	//get response Data if response status is 200
	respData, err := getResponseData(appApi)
	if err != nil {
		return err
	}
	//Verifying backup-restore data only for CSI as of now.
	if backupRestoretype == CSI || backupRestoretype == RESTIC {
		//check if backupfile exists. If true { compare data response with data from file} (post restore step)
		//else write data to backup-data.txt (prebackup step)
		if _, err := os.Stat(backupFile); err == nil {
			backupData, err := os.ReadFile(backupFile)
			if err != nil {
				return err
			}
			os.Remove(backupFile)
			fmt.Printf("Data came from backup-file\n %s\n", backupData)
			fmt.Printf("Data from the response after restore\n %s\n", respData)
			backDataIsEqual := false
			for i := 0; i < 5 && !backDataIsEqual; i++ {
				respData, err = getResponseData(appApi)
				if err != nil {
					return err
				}
				backDataIsEqual = bytes.Equal(backupData, respData)
				if backDataIsEqual != true {
					fmt.Printf("Backup and Restore Data are not equal, retry %d\n", i)
					fmt.Printf("Data from the response after restore\n %s\n", respData)
					time.Sleep(10 * time.Second)
				}
			}
			if backDataIsEqual != true {
				return errors.New("Backup and Restore Data are not the same")
			}

		} else if errors.Is(err, os.ErrNotExist) {
			fmt.Printf("Writing data to backupFile (backup-data.txt): \n %s\n", respData)
			err := os.WriteFile(backupFile, respData, 0644)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func getResponseData(appApi string) ([]byte, error) {
	resp, err := http.Get(appApi)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 { // # TODO: NEED TO FIND A BETTER WAY TO DEBUG RESPONSE
		var retrySchedule = []time.Duration{
			15 * time.Second,
			1 * time.Minute,
			2 * time.Minute,
		}
		for _, backoff := range retrySchedule {
			resp, err = http.Get(appApi)
			if resp.StatusCode != 200 {
				log.Printf("Request error: %+v\n", err)
				log.Printf("Retrying in %v\n", backoff)
				time.Sleep(backoff)
			}
		}
	}
	return ioutil.ReadAll(resp.Body)

}
