package lib

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"
	volumesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	ocpappsv1 "github.com/openshift/api/apps/v1"
	openshiftconfigv1 "github.com/openshift/api/config/v1"
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
	log.Printf("Installing application with retries: %s", template)
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
			log.Printf("waiting for volume snapshot contents %s to be ready", v.Name)
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
		log.Printf("Deployment %s status: %v", dName, deployment.Status)
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

// IsStatefulSetReady checks if a StatefulSet is ready
func IsStatefulSetReady(ocClient client.Client, namespace, name string) wait.ConditionFunc {
	return func() (bool, error) {
		sts := &appsv1.StatefulSet{}
		err := ocClient.Get(context.Background(), client.ObjectKey{
			Namespace: namespace,
			Name:      name,
		}, sts)
		if err != nil {
			return false, err
		}
		log.Printf("StatefulSet %s status: %v", name, sts.Status)
		if sts.Status.ReadyReplicas != sts.Status.Replicas || sts.Status.Replicas == 0 {
			for _, condition := range sts.Status.Conditions {
				if len(condition.Message) > 0 {
					ginkgo.GinkgoWriter.Write([]byte(fmt.Sprintf("statefulset not available with condition: %s\n", condition.Message)))
				}
			}
			return false, errors.New("statefulset is not in a ready state")
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

// GetNamespaceEventMessages returns "Reason: Message" for every event
// currently in namespace, for feeding into CheckIfFlakeOccurred alongside
// pod logs -- some known-flake signatures (e.g. a controller's own emitted
// Kubernetes Event on an object it owns) only ever show up as an Event, not
// in any pod's log output.
func GetNamespaceEventMessages(c *kubernetes.Clientset, namespace string) []string {
	events, err := c.CoreV1().Events(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		log.Printf("could not list events in namespace %s for flake-detection: %v", namespace, err)
		return nil
	}
	messages := make([]string, 0, len(events.Items))
	for _, event := range events.Items {
		messages = append(messages, fmt.Sprintf("%s: %s", event.Reason, event.Message))
	}
	return messages
}

func RunMustGather(artifact_dir string, clusterClient client.Client) error {
	mustGatherImage := os.Getenv("MUST_GATHER_IMAGE")
	if mustGatherImage == "" {
		mustGatherImage = "quay.io/konveyor/oadp-must-gather:latest"
	}

	log.Printf("Running must-gather with image: %s", mustGatherImage)
	cmd := exec.Command("oc", "adm", "must-gather",
		"--image="+mustGatherImage,
		"--dest-dir="+artifact_dir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("must-gather command failed: %w, output: %s", err, string(output))
	}

	clusterVersionList := &openshiftconfigv1.ClusterVersionList{}
	err = clusterClient.List(context.Background(), clusterVersionList)
	if err != nil {
		return err
	}
	if len(clusterVersionList.Items) == 0 {
		return errors.New("no ClusterVersion found in cluster")
	}
	clusterVersion := &clusterVersionList.Items[0]
	clusterID := string(clusterVersion.Spec.ClusterID)
	if len(clusterID) > 8 {
		clusterID = clusterID[:8]
	}

	pattern := filepath.Join(artifact_dir, "*", "clusters", clusterID, "oadp-must-gather-summary.md")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("error finding must-gather summary: %w", err)
	}
	if len(matches) == 0 {
		dirs, _ := filepath.Glob(filepath.Join(artifact_dir, "*"))
		return fmt.Errorf("no must-gather summary found at pattern: %s\nDirectories in artifact_dir: %v", pattern, dirs)
	}

	sort.Strings(matches)
	summaryPath := matches[len(matches)-1]
	log.Printf("Reading must-gather summary from: %s", summaryPath)
	mustGatherSummaryContent, err := os.ReadFile(summaryPath)
	if err != nil {
		return err
	}

	mustGatherSummaryText := string(mustGatherSummaryContent)

	if !strings.Contains(mustGatherSummaryText, "No errors happened or were found while running OADP must-gather") {
		if !mustGatherErrorsAreOnlyKnownBenignWarnings(mustGatherSummaryText) {
			return errors.New("expected no errors in must-gather Errors section")
		}
	}

	return nil
}

// mustGatherKnownBenignErrorPatterns are must-gather "Errors" section lines
// that are purely informational -- the summary generator flags any DPA using
// spec.unsupportedOverrides at all, regardless of which key or why, since
// that field is inherently "unsupported" by design. A test intentionally
// using it (e.g. to override kdm-controller's image with a candidate fix
// build before it merges upstream) is not itself an error condition, and
// treating it as one broke every e2e job's must-gather check the moment any
// suite's DPA carried an UnsupportedOverrides entry -- confirmed live on
// oadp-operator PR#2404, where a single test-time override in
// e2e_suite_test.go's shared dpaCR broke unrelated CLI e2e jobs across
// multiple OCP versions. Any OTHER line in the Errors section still fails
// the check below -- only this specific, expected caveat is tolerated.
var mustGatherKnownBenignErrorPatterns = []*regexp.Regexp{
	regexp.MustCompile(`is using \*\*unsupportedOverrides\*\*`),
}

// mustGatherErrorsAreOnlyKnownBenignWarnings extracts the "## Errors" section
// of the must-gather summary and reports whether every non-blank line in it
// matches a known-benign pattern above. Returns false (i.e. a real error is
// present) if the section can't be found at all, to fail safe.
func mustGatherErrorsAreOnlyKnownBenignWarnings(summary string) bool {
	lines := strings.Split(summary, "\n")
	start := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "## Errors" {
			start = i + 1
			break
		}
	}
	if start == -1 {
		return false
	}

	for _, line := range lines[start:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "## ") {
			break
		}
		matched := false
		for _, p := range mustGatherKnownBenignErrorPatterns {
			if p.MatchString(trimmed) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

// VerifyBackupRestoreData verifies if app ready before backup and after restore to compare data.
// skipReadyz skips the post-restore readyz endpoint check (use for VM-based tests where the
// app route is not directly reachable from the test harness).
func VerifyBackupRestoreData(ocClient client.Client, kubeClient *kubernetes.Clientset, kubeConfig *rest.Config, artifactDir string, namespace string, routeName string, serviceName string, app string, prebackupState bool, twoVol bool, skipReadyz ...bool) error {
	log.Printf("Verifying backup/restore data of %s", app)
	appEndpointURL, proxyPodParams, err := getAppEndpointURLAndProxyParams(ocClient, kubeClient, kubeConfig, namespace, serviceName, routeName)
	log.Printf("App endpoint URL: %s", appEndpointURL)
	if err != nil {
		return err
	}

	respData := ""
	var errResp string

	if prebackupState {
		// Clean up existing backup file
		RemoveFileIfExists(artifactDir + "/backup-data.txt")

		// todolist
		if namespace == "mysql-persistent" || namespace == "mongo-persistent" {
			// Construct request parameters for the "todo-incomplete" endpoint
			responseParams := getRequestParameters(appEndpointURL+"/todo-incomplete", proxyPodParams, GET, nil)
			dataBeforeCurl, errResp, err := MakeRequest(*responseParams)
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
			log.Printf("requestParams: %v\n", requestParams)
			MakeRequest(*requestParams)

			postPayload = `{"description": "` + time.Now().Weekday().String() + `"}`
			requestParams = getRequestParameters(appEndpointURL+"/todo", proxyPodParams, POST, &postPayload)
			MakeRequest(*requestParams)
			respData, errResp, err = MakeRequest(*responseParams)
			if err != nil {
				if errResp != "" {
					log.Printf("Request response error msg: %s\n", errResp)
				}
				return err
			}
		}
		// parks-app
		if namespace == "parks-app" {
			// POST with no body - equivalent to: curl -X POST http://restify/clicks
			postParams := getRequestParameters(appEndpointURL+"/clicks", proxyPodParams, POST, nil)
			responseParams := getRequestParameters(appEndpointURL+"/clicks", proxyPodParams, GET, nil)
			log.Printf("postParams: %v\n", postParams)
			// 2 clicks
			MakeRequest(*postParams)
			MakeRequest(*postParams)
			respData, errResp, err = MakeRequest(*responseParams)
			if err != nil {
				if errResp != "" {
					log.Printf("Request response error msg: %s\n", errResp)
				}
				return err
			}
		}

		// Write data to backup file
		log.Printf("Writing data to backupFile %s /backup-data.txt: \n %s\n", artifactDir, respData)
		if err := os.WriteFile(artifactDir+"/backup-data.txt", []byte(respData), 0644); err != nil {
			return err
		}
	} else {
		// --- Restore verification ---
		// After a Velero restore, verify that the application is serving the same data
		// that was captured before the backup (stored in backup-data.txt).
		//
		// Flow for todo apps (mysql-persistent / mongo-persistent):
		//   1. If !shouldSkipReadyz: poll /healthz to confirm the app is alive.
		//   2. Fetch /todo-incomplete to get the current data for comparison.
		//      - For VM-based tests (shouldSkipReadyz=true) the route is not directly
		//        reachable until the VM finishes cloud-init, so we poll with retries.
		//      - For container-based tests a single request suffices after healthz passes.
		// Flow for parks-app: single GET /clicks.
		// Finally, compare the fetched data against backup-data.txt.

		shouldSkipReadyz := len(skipReadyz) > 0 && skipReadyz[0]
		isTodoApp := namespace == "mysql-persistent" || namespace == "mongo-persistent"

		// Step 1: healthz gate (container-based todo apps only).
		// Polls /healthz to confirm the app is alive and the HTTP server is responding.
		// The todo2-go app exposes /healthz (used by all K8s probes) and /readyz (returns
		// 503 until DB is connected). We use /healthz here because it matches the probe
		// configuration in the app manifests and becomes available immediately on startup.
		// Skipped for VM tests where the app runs inside a Fedora/CentOS VM and the
		// OpenShift route proxies to a different service topology.
		if isTodoApp && !shouldSkipReadyz {
			// MakeRequest can return err == nil for HTTP 5xx when using the proxy (curl),
			// so we validate the response body and errResp via isHealthzAlive.
			requestParams := getRequestParameters(appEndpointURL+"/healthz", proxyPodParams, GET, nil)
			const maxHealthzAttempts = 5
			for attempt := 1; attempt <= maxHealthzAttempts; attempt++ {
				log.Printf("healthz check attempt %d/%d: GET %s/healthz\n", attempt, maxHealthzAttempts, appEndpointURL)
				respData, errResp, err = MakeRequest(*requestParams)
				if err == nil && isHealthzAlive(respData, errResp) {
					log.Printf("healthz endpoint is alive (attempt %d/%d): %s\n", attempt, maxHealthzAttempts, respData)
					break
				}
				if err != nil {
					if errResp != "" {
						log.Printf("Request response error msg: %s\n", errResp)
					}
				} else {
					log.Printf("healthz attempt %d/%d: response not healthy (body=%q, errResp=%q)\n", attempt, maxHealthzAttempts, respData, errResp)
				}
				if attempt == maxHealthzAttempts {
					log.Printf("healthz endpoint did not become alive after %d attempts: %v\n", maxHealthzAttempts, err)
					if err != nil {
						return err
					}
					return fmt.Errorf("healthz did not return healthy response after %d attempts (last body=%q, errResp=%q)", maxHealthzAttempts, respData, errResp)
				}
				backoff := time.Duration(attempt) * 5 * time.Second
				log.Printf("healthz attempt %d/%d failed, retrying in %s: %v\n", attempt, maxHealthzAttempts, backoff, err)
				time.Sleep(backoff)
			}
		}

		// Step 2: fetch /todo-incomplete data for todo apps.
		// In the VM (shouldSkipReadyz) case we skipped the readyz gate above, so the
		// app may not be ready yet. Poll with retries and increasing backoff.
		// In the container case healthz already passed, so one attempt is enough.
		if isTodoApp {
			requestParamsTodoIncomplete := getRequestParameters(appEndpointURL+"/todo-incomplete", proxyPodParams, GET, nil)
			maxTodoAttempts := 1
			todoBackoffSec := 0
			if shouldSkipReadyz {
				maxTodoAttempts = 10
				todoBackoffSec = 10
			}
			for attempt := 1; attempt <= maxTodoAttempts; attempt++ {
				if maxTodoAttempts > 1 {
					log.Printf("Polling app endpoint attempt %d/%d: GET %s/todo-incomplete", attempt, maxTodoAttempts, appEndpointURL)
				}
				respData, errResp, err = MakeRequest(*requestParamsTodoIncomplete)
				success := err == nil && (maxTodoAttempts == 1 || len(bytes.TrimSpace([]byte(respData))) > 0)
				if success {
					if maxTodoAttempts > 1 {
						log.Printf("VIRT App endpoint responded with data (attempt %d/%d): %s", attempt, maxTodoAttempts, respData)
					}
					break
				}
				if attempt == maxTodoAttempts {
					if err != nil {
						if errResp != "" {
							log.Printf("Request response error msg: %s\n", errResp)
						}
						return err
					}
					if maxTodoAttempts > 1 {
						log.Printf("VIRT App endpoint returned empty data after %d attempts", maxTodoAttempts)
						return errors.New("VIRT App endpoint returned empty data after max attempts")
					}
					if errResp != "" {
						log.Printf("Request response error msg: %s\n", errResp)
					}
					return err
				}
				backoff := time.Duration(attempt) * time.Duration(todoBackoffSec) * time.Second
				log.Printf("VIRT Attempt %d/%d: no data yet, retrying in %s (err=%v, resp=%q)", attempt, maxTodoAttempts, backoff, err, respData)
				time.Sleep(backoff)
			}
		}

		if namespace == "parks-app" {
			// Make request to the "clicks" endpoint
			responseParams := getRequestParameters(appEndpointURL+"/clicks", proxyPodParams, GET, nil)
			respData, errResp, err = MakeRequest(*responseParams)
			if err != nil {
				if errResp != "" {
					log.Printf("Request response error msg: %s\n", errResp)
				}
				return err
			}
		}

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

// errRespIndicatesHTTPError returns true when errResp contains HTTP error indicators (e.g. 5xx from MakeRequest).
func errRespIndicatesHTTPError(errResp string) bool {
	if errResp == "" {
		return false
	}
	return strings.Contains(errResp, "HTTP request failed") ||
		strings.Contains(errResp, "status code") ||
		strings.Contains(errResp, "500") ||
		strings.Contains(errResp, "502") ||
		strings.Contains(errResp, "503")
}

// isHealthzAlive returns true when the /healthz response indicates the app is
// alive: the response body is non-empty (any content is fine) and errResp
// does not contain HTTP error indicators.
func isHealthzAlive(respData, errResp string) bool {
	return strings.TrimSpace(respData) != "" &&
		!errRespIndicatesHTTPError(errResp)
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
