package lib

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"reflect"
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"
	volumesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	. "github.com/onsi/ginkgo/v2"
	appsv1 "github.com/openshift/api/apps/v1"
	buildv1 "github.com/openshift/api/build/v1"
	security "github.com/openshift/api/security/v1"
	templatev1 "github.com/openshift/api/template/v1"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/common"
	utils "github.com/openshift/oadp-operator/tests/e2e/utils"
	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	operators "github.com/operator-framework/api/pkg/operators/v1alpha1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	velero "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

type BackupRestoreType string

const (
	CSI          BackupRestoreType = "csi"
	CSIDataMover BackupRestoreType = "csi-datamover"
	RESTIC       BackupRestoreType = "restic"
	KOPIA        BackupRestoreType = "kopia"
)

type DpaCustomResource struct {
	Name              string
	Namespace         string
	backupRestoreType BackupRestoreType
	CustomResource    *oadpv1alpha1.DataProtectionApplication
	Client            client.Client
	Provider          string
}

var VeleroPrefix = "velero-e2e-" + string(uuid.NewUUID())
var Dpa *oadpv1alpha1.DataProtectionApplication

func (v *DpaCustomResource) Build(backupRestoreType BackupRestoreType) error {
	// Velero Instance creation spec with backupstorage location default to AWS. Would need to parameterize this later on to support multiple plugins.
	dpaInstance := oadpv1alpha1.DataProtectionApplication{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DataProtectionApplication",
			APIVersion: "oadp.openshift.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      v.Name,
			Namespace: v.Namespace,
		},
		Spec: oadpv1alpha1.DataProtectionApplicationSpec{
			Configuration: &oadpv1alpha1.ApplicationConfig{
				Velero: &oadpv1alpha1.VeleroConfig{
					LogLevel:       "debug",
					DefaultPlugins: v.CustomResource.Spec.Configuration.Velero.DefaultPlugins,
				},
				NodeAgent: &oadpv1alpha1.NodeAgentConfig{
					UploaderType: "kopia",
					NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
						PodConfig: &oadpv1alpha1.PodConfig{},
					},
				},
			},
			SnapshotLocations: v.CustomResource.Spec.SnapshotLocations,
			BackupLocations: []oadpv1alpha1.BackupLocation{
				{
					Velero: &velero.BackupStorageLocationSpec{
						Provider: v.CustomResource.Spec.BackupLocations[0].Velero.Provider,
						Default:  true,
						Config:   v.CustomResource.Spec.BackupLocations[0].Velero.Config,
						Credential: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "bsl-cloud-credentials-" + v.Provider,
							},
							Key: "cloud",
						},
						StorageType: velero.StorageType{
							ObjectStorage: &velero.ObjectStorageLocation{
								Bucket: v.CustomResource.Spec.BackupLocations[0].Velero.ObjectStorage.Bucket,
								Prefix: VeleroPrefix,
							},
						},
					},
				},
			},
		},
	}
	v.backupRestoreType = backupRestoreType
	type emptyStruct struct{}
	// default plugins map
	defaultPlugins := make(map[oadpv1alpha1.DefaultPlugin]emptyStruct)
	for _, plugin := range dpaInstance.Spec.Configuration.Velero.DefaultPlugins {
		defaultPlugins[plugin] = emptyStruct{}
	}
	veleroFeatureFlags := make(map[string]emptyStruct)
	for _, flag := range dpaInstance.Spec.Configuration.Velero.FeatureFlags {
		veleroFeatureFlags[flag] = emptyStruct{}
	}
	switch backupRestoreType {
	case RESTIC, KOPIA:
		dpaInstance.Spec.Configuration.NodeAgent.Enable = pointer.Bool(true)
		dpaInstance.Spec.Configuration.NodeAgent.UploaderType = string(backupRestoreType)
		delete(defaultPlugins, oadpv1alpha1.DefaultPluginCSI)
		delete(veleroFeatureFlags, "EnableCSI")
		dpaInstance.Spec.SnapshotLocations = nil
	case CSI:
		dpaInstance.Spec.Configuration.NodeAgent.Enable = pointer.Bool(false)
		defaultPlugins[oadpv1alpha1.DefaultPluginCSI] = emptyStruct{}
		veleroFeatureFlags["EnableCSI"] = emptyStruct{}
		dpaInstance.Spec.SnapshotLocations = nil
	case CSIDataMover:
		// We don't need to have restic use case, kopia is enough
		dpaInstance.Spec.Configuration.NodeAgent.Enable = pointer.Bool(true)
		dpaInstance.Spec.Configuration.NodeAgent.UploaderType = "kopia"
		defaultPlugins[oadpv1alpha1.DefaultPluginCSI] = emptyStruct{}
		veleroFeatureFlags["EnableCSI"] = emptyStruct{}
		dpaInstance.Spec.SnapshotLocations = nil
	}
	dpaInstance.Spec.Configuration.Velero.DefaultPlugins = make([]oadpv1alpha1.DefaultPlugin, 0)
	for k := range defaultPlugins {
		dpaInstance.Spec.Configuration.Velero.DefaultPlugins = append(dpaInstance.Spec.Configuration.Velero.DefaultPlugins, k)
	}
	dpaInstance.Spec.Configuration.Velero.FeatureFlags = make([]string, 0)
	for k := range veleroFeatureFlags {
		dpaInstance.Spec.Configuration.Velero.FeatureFlags = append(dpaInstance.Spec.Configuration.Velero.FeatureFlags, k)
	}
	// Uncomment to override plugin images to use
	dpaInstance.Spec.UnsupportedOverrides = map[oadpv1alpha1.UnsupportedImageKey]string{
		// oadpv1alpha1.VeleroImageKey: "quay.io/konveyor/velero:oadp-1.1",
	}
	v.CustomResource = &dpaInstance
	return nil
}

// if e2e, test/e2e is "." since context is tests/e2e/
// for unit-test, test/e2e is ".." since context is tests/e2e/lib/
func (v *DpaCustomResource) ProviderStorageClassName(e2eRoot string) (string, error) {
	pvcFile := fmt.Sprintf("%s/sample-applications/%s/pvc/%s.yaml", e2eRoot, "mongo-persistent", v.Provider)
	pvcList := corev1.PersistentVolumeClaimList{}
	pvcBytes, err := utils.ReadFile(pvcFile)
	if err != nil {
		return "", err
	}
	err = yaml.Unmarshal(pvcBytes, &pvcList)
	if err != nil {
		return "", err
	}
	if pvcList.Items == nil || len(pvcList.Items) == 0 {
		return "", errors.New("pvc not found")
	}
	if pvcList.Items[0].Spec.StorageClassName == nil {
		return "", errors.New("storage class name not found in pvc")
	}
	return *pvcList.Items[0].Spec.StorageClassName, nil
}

func (v *DpaCustomResource) Create(c client.Client) error {
	err := v.SetClient(c)
	if err != nil {
		return err
	}
	err = v.Client.Create(context.Background(), v.CustomResource)
	if apierrors.IsAlreadyExists(err) {
		return errors.New("found unexpected existing Velero CR")
	} else if err != nil {
		return err
	}
	return nil
}

func (v *DpaCustomResource) Get(c client.Client) (*oadpv1alpha1.DataProtectionApplication, error) {
	err := v.SetClient(c)
	if err != nil {
		return nil, err
	}
	vel := oadpv1alpha1.DataProtectionApplication{}
	err = v.Client.Get(context.Background(), client.ObjectKey{
		Namespace: v.Namespace,
		Name:      v.Name,
	}, &vel)
	if err != nil {
		return nil, err
	}
	return &vel, nil
}

func (v *DpaCustomResource) GetNoErr(c client.Client) *oadpv1alpha1.DataProtectionApplication {
	Dpa, _ := v.Get(c)
	return Dpa
}

func (v *DpaCustomResource) CreateOrUpdate(c client.Client, spec *oadpv1alpha1.DataProtectionApplicationSpec) error {
	return v.CreateOrUpdateWithRetries(c, spec, 3)
}
func (v *DpaCustomResource) CreateOrUpdateWithRetries(c client.Client, spec *oadpv1alpha1.DataProtectionApplicationSpec, retries int) error {
	var (
		err error
		cr  *oadpv1alpha1.DataProtectionApplication
	)
	for i := 0; i < retries; i++ {
		if cr, err = v.Get(c); apierrors.IsNotFound(err) {
			v.CustomResource = &oadpv1alpha1.DataProtectionApplication{
				TypeMeta: metav1.TypeMeta{
					Kind:       "DataProtectionApplication",
					APIVersion: "oadp.openshift.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      v.Name,
					Namespace: v.Namespace,
				},
				Spec: *spec.DeepCopy(),
			}
			return v.Create(c)
		} else if err != nil {
			return err
		}
		crPatch := cr.DeepCopy()
		spec.DeepCopyInto(&crPatch.Spec)
		crPatch.ObjectMeta.ManagedFields = nil
		if err = v.Client.Patch(context.Background(), crPatch, client.MergeFrom(cr), &client.PatchOptions{}); err != nil {
			log.Println("error patching velero cr", err)
			if apierrors.IsConflict(err) && i < retries-1 {
				log.Println("conflict detected during DPA CreateOrUpdate, retrying for ", retries-i-1, " more times")
				time.Sleep(time.Second * 2)
				continue
			}
			return err
		}
		return nil
	}
	return err
}

func (v *DpaCustomResource) Delete(c client.Client) error {
	err := v.SetClient(c)
	if err != nil {
		return err
	}
	err = v.Client.Delete(context.Background(), v.CustomResource)
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func (v *DpaCustomResource) SetClient(client client.Client) error {
	oadpv1alpha1.AddToScheme(client.Scheme())
	velero.AddToScheme(client.Scheme())
	appsv1.AddToScheme(client.Scheme())
	corev1.AddToScheme(client.Scheme())
	templatev1.AddToScheme(client.Scheme())
	security.AddToScheme(client.Scheme())
	operators.AddToScheme(client.Scheme())
	volumesnapshotv1.AddToScheme(client.Scheme())
	buildv1.AddToScheme(client.Scheme())
	operatorsv1alpha1.AddToScheme(client.Scheme())
	operatorsv1.AddToScheme(client.Scheme())

	v.Client = client
	return nil
}

func GetVeleroPods(c *kubernetes.Clientset, namespace string) (*corev1.PodList, error) {
	// select Velero pod with this label
	veleroOptions := metav1.ListOptions{
		LabelSelector: "component=velero,!job-name",
	}
	veleroOptionsDeploy := metav1.ListOptions{
		LabelSelector: "deploy=velero,!job-name",
	}
	// get pods in test namespace with labelSelector
	var podList *corev1.PodList
	var err error
	if podList, err = c.CoreV1().Pods(namespace).List(context.Background(), veleroOptions); err != nil {
		return nil, err
	}
	if len(podList.Items) == 0 {
		// handle some oadp versions where label was deploy=velero
		if podList, err = c.CoreV1().Pods(namespace).List(context.Background(), veleroOptionsDeploy); err != nil {
			return nil, err
		}
	}
	return podList, nil
}

// TODO duplications with AreApplicationPodsRunning form apps.go
func AreVeleroPodsRunning(c *kubernetes.Clientset, namespace string) wait.ConditionFunc {
	return func() (bool, error) {
		podList, err := GetVeleroPods(c, namespace)
		if err != nil {
			return false, err
		}
		if podList.Items == nil || len(podList.Items) == 0 {
			log.Println("velero pods not found")
			return false, nil
		}
		for _, podInfo := range (*podList).Items {
			if podInfo.Status.Phase != corev1.PodRunning {
				log.Printf("pod: %s is not yet running: phase is %v", podInfo.Name, podInfo.Status.Phase)
				return false, nil
			}
		}
		log.Println("velero pods are running")
		return true, nil
	}
}

func GetOpenShiftADPLogs(c *kubernetes.Clientset, namespace string) (string, error) {
	return GetPodWithPrefixContainerLogs(c, namespace, "openshift-adp-controller-manager-", "manager")
}

// Returns logs from velero container on velero pod
func GetVeleroContainerLogs(c *kubernetes.Clientset, namespace string) (string, error) {
	return GetPodWithPrefixContainerLogs(c, namespace, "velero-", "velero")
}

func GetVeleroContainerFailureLogs(c *kubernetes.Clientset, namespace string) []string {
	containerLogs, err := GetVeleroContainerLogs(c, namespace)
	if err != nil {
		log.Printf("cannot get velero container logs")
		return nil
	}
	containerLogsArray := strings.Split(containerLogs, "\n")
	var failureArr = []string{}
	for i, line := range containerLogsArray {
		if strings.Contains(line, "level=error") {
			failureArr = append(failureArr, fmt.Sprintf("velero container error line#%d: "+line+"\n", i))
		}
	}
	return failureArr
}

func (v *DpaCustomResource) IsDeleted(c client.Client) wait.ConditionFunc {
	return func() (bool, error) {
		err := v.SetClient(c)
		if err != nil {
			return false, err
		}
		// Check for velero CR in cluster
		vel := oadpv1alpha1.DataProtectionApplication{}
		err = v.Client.Get(context.Background(), client.ObjectKey{
			Namespace: v.Namespace,
			Name:      v.Name,
		}, &vel)
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}
}

// check if bsl matches the spec
func DoesBSLSpecMatchesDpa(namespace string, bsl velero.BackupStorageLocationSpec, spec *oadpv1alpha1.DataProtectionApplicationSpec) (bool, error) {
	if len(spec.BackupLocations) == 0 {
		return false, errors.New("no backup storage location configured. Expected BSL to be configured")
	}
	for _, b := range spec.BackupLocations {
		if b.Velero.Provider == bsl.Provider {
			if b.Velero.Config == nil {
				b.Velero.Config = make(map[string]string)
			}
			if bsl.Config == nil {
				bsl.Config = make(map[string]string)
			}
			if !reflect.DeepEqual(bsl, *b.Velero) {
				GinkgoWriter.Print(cmp.Diff(bsl, *b.Velero))
				return false, errors.New("given Velero bsl does not match the deployed velero bsl")
			}
		}
	}
	return true, nil
}

// check if vsl matches the spec
func DoesVSLSpecMatchesDpa(namespace string, vslspec velero.VolumeSnapshotLocationSpec, spec *oadpv1alpha1.DataProtectionApplicationSpec) (bool, error) {
	if len(spec.SnapshotLocations) == 0 {
		return false, errors.New("no volume storage location configured. Expected VSL to be configured")
	}
	for _, v := range spec.SnapshotLocations {
		if v.Velero.Config == nil {
			v.Velero.Config = make(map[string]string)
		}
		if vslspec.Config == nil {
			vslspec.Config = make(map[string]string)
		}
		if reflect.DeepEqual(vslspec, *v.Velero) {
			GinkgoWriter.Print(cmp.Diff(vslspec, *v.Velero))
			return true, nil
		}
	}
	return false, errors.New("did not find expected VSL")
}

// check velero tolerations
func VerifyVeleroTolerations(c *kubernetes.Clientset, namespace string, t []corev1.Toleration) wait.ConditionFunc {
	return func() (bool, error) {
		veldep, _ := c.AppsV1().Deployments(namespace).Get(context.Background(), "velero", metav1.GetOptions{})

		if !reflect.DeepEqual(t, veldep.Spec.Template.Spec.Tolerations) {
			return false, errors.New("given Velero tolerations does not match the deployed velero tolerations")
		}
		return true, nil
	}
}

// check for velero resource requests
func VerifyVeleroResourceRequests(c *kubernetes.Clientset, namespace string, requests corev1.ResourceList) wait.ConditionFunc {
	return func() (bool, error) {
		veldep, _ := c.AppsV1().Deployments(namespace).Get(context.Background(), "velero", metav1.GetOptions{})

		for _, c := range veldep.Spec.Template.Spec.Containers {
			if c.Name == common.Velero {
				if !reflect.DeepEqual(requests, c.Resources.Requests) {
					return false, errors.New("given Velero resource requests do not match the deployed velero resource requests")
				}
			}
		}
		return true, nil
	}
}

// check for velero resource limits
func VerifyVeleroResourceLimits(c *kubernetes.Clientset, namespace string, limits corev1.ResourceList) wait.ConditionFunc {
	return func() (bool, error) {
		veldep, _ := c.AppsV1().Deployments(namespace).Get(context.Background(), "velero", metav1.GetOptions{})

		for _, c := range veldep.Spec.Template.Spec.Containers {
			if c.Name == common.Velero {
				if !reflect.DeepEqual(limits, c.Resources.Limits) {
					return false, errors.New("given Velero resource limits do not match the deployed velero resource limits")
				}
			}
		}
		return true, nil
	}
}

func LoadDpaSettingsFromJson(settings string) string {
	file, err := utils.ReadFile(settings)
	if err != nil {
		return fmt.Sprintf("Error decoding json file: %v", err)
	}

	Dpa = &oadpv1alpha1.DataProtectionApplication{}
	err = json.Unmarshal(file, &Dpa)
	if err != nil {
		return fmt.Sprintf("Error getting settings json file: %v", err)
	}
	return ""
}
