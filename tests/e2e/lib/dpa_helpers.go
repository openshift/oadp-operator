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
	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	buildv1 "github.com/openshift/api/build/v1"
	"github.com/openshift/oadp-operator/controllers"
	"github.com/openshift/oadp-operator/pkg/common"

	volsync "github.com/backube/volsync/api/v1alpha1"
	volumesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	appsv1 "github.com/openshift/api/apps/v1"
	security "github.com/openshift/api/security/v1"
	templatev1 "github.com/openshift/api/template/v1"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	utils "github.com/openshift/oadp-operator/tests/e2e/utils"
	operators "github.com/operator-framework/api/pkg/operators/v1alpha1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	velero "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

type BackupRestoreType string

const (
	CSI          BackupRestoreType = "csi"
	CSIDataMover BackupRestoreType = "csi-datamover"
	RESTIC       BackupRestoreType = "restic"
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
				Restic: &oadpv1alpha1.ResticConfig{
					PodConfig: &oadpv1alpha1.PodConfig{},
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
	dpaInstance.Spec.Features = &oadpv1alpha1.Features{DataMover: &oadpv1alpha1.DataMover{Enable: false}}
	switch backupRestoreType {
	case RESTIC:
		dpaInstance.Spec.Configuration.Restic.Enable = pointer.Bool(true)
		delete(defaultPlugins, oadpv1alpha1.DefaultPluginCSI)
		delete(veleroFeatureFlags, "EnableCSI")
	case CSI:
		dpaInstance.Spec.Configuration.Restic.Enable = pointer.Bool(false)
		defaultPlugins[oadpv1alpha1.DefaultPluginCSI] = emptyStruct{}
		veleroFeatureFlags["EnableCSI"] = emptyStruct{}
	case CSIDataMover:
		dpaInstance.Spec.Configuration.Restic.Enable = pointer.Bool(false)
		defaultPlugins[oadpv1alpha1.DefaultPluginCSI] = emptyStruct{}
		veleroFeatureFlags["EnableCSI"] = emptyStruct{}
		dpaInstance.Spec.Features.DataMover.Enable = true
		dpaInstance.Spec.Features.DataMover.CredentialName = controllers.ResticsecretName
		dpaInstance.Spec.Features.DataMover.Timeout = "40m"
		// annotate namespace for volsync privileged movers
		ns := &corev1.Namespace{}
		if err := v.Client.Get(context.Background(), types.NamespacedName{Name: v.Namespace}, ns); err != nil {
			return err
		}
		nsOriginal := ns.DeepCopy()
		if ns.Annotations == nil {
			ns.Annotations = make(map[string]string)
		}
		ns.Annotations[volsync.PrivilegedMoversNamespaceAnnotation] = "true"
		if err := v.Client.Patch(context.Background(), ns, client.StrategicMergeFrom(nsOriginal)); err != nil {
			fmt.Printf("failed to annotate namespace: %s for volsync privileged movers\n", v.Namespace)
			return err
		}
		gomega.Eventually(func() error {
			if err := v.Client.Get(context.Background(), types.NamespacedName{Name: v.Namespace}, ns); err != nil {
				return err
			}
			if ns.Annotations[volsync.PrivilegedMoversNamespaceAnnotation] != "true" {
				return errors.New("failed to annotate namespace for volsync privileged movers")
			}
			return nil
		}, 5*time.Minute, 1*time.Second).Should(gomega.Succeed())
		fmt.Printf("successfully validated annotated namespace: %s for volsync privileged movers\n", v.Namespace)
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
		// oadpv1alpha1.DataMoverImageKey: "quay.io/emcmulla/data-mover:latest",
		// oadpv1alpha1.CSIPluginImageKey: "quay.io/emcmulla/csi-plugin:latest",
	}
	v.CustomResource = &dpaInstance
	return nil
}

func (v *DpaCustomResource) Create() error {
	err := v.SetClient()
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

func (v *DpaCustomResource) Get() (*oadpv1alpha1.DataProtectionApplication, error) {
	err := v.SetClient()
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

func (v *DpaCustomResource) GetNoErr() *oadpv1alpha1.DataProtectionApplication {
	Dpa, _ := v.Get()
	return Dpa
}

func (v *DpaCustomResource) CreateOrUpdate(spec *oadpv1alpha1.DataProtectionApplicationSpec) error {
	return v.CreateOrUpdateWithRetries(spec, 3)
}
func (v *DpaCustomResource) CreateOrUpdateWithRetries(spec *oadpv1alpha1.DataProtectionApplicationSpec, retries int) error {
	var (
		err error
		cr  *oadpv1alpha1.DataProtectionApplication
	)
	for i := 0; i < retries; i++ {
		if cr, err = v.Get(); apierrors.IsNotFound(err) {
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
			return v.Create()
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

func (v *DpaCustomResource) Delete() error {
	err := v.SetClient()
	if err != nil {
		return err
	}
	err = v.Client.Delete(context.Background(), v.CustomResource)
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func (v *DpaCustomResource) SetClient() error {
	client, err := client.New(config.GetConfigOrDie(), client.Options{})
	if err != nil {
		return err
	}
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
	volsync.AddToScheme(client.Scheme())

	v.Client = client
	return nil
}

func GetVeleroPods(namespace string) (*corev1.PodList, error) {
	clientset, err := setUpClient()
	if err != nil {
		return nil, err
	}
	// select Velero pod with this label
	veleroOptions := metav1.ListOptions{
		LabelSelector: "component=velero",
	}
	veleroOptionsDeploy := metav1.ListOptions{
		LabelSelector: "deploy=velero",
	}
	// get pods in test namespace with labelSelector
	var podList *corev1.PodList
	if podList, err = clientset.CoreV1().Pods(namespace).List(context.TODO(), veleroOptions); err != nil {
		return nil, err
	}
	if len(podList.Items) == 0 {
		// handle some oadp versions where label was deploy=velero
		if podList, err = clientset.CoreV1().Pods(namespace).List(context.TODO(), veleroOptionsDeploy); err != nil {
			return nil, err
		}
	}
	return podList, nil
}

func AreVeleroPodsRunning(namespace string) wait.ConditionFunc {
	return func() (bool, error) {
		podList, err := GetVeleroPods(namespace)
		if err != nil {
			return false, err
		}
		if podList.Items == nil || len(podList.Items) == 0 {
			GinkgoWriter.Println("velero pods not found")
			return false, nil
		}
		for _, podInfo := range (*podList).Items {
			if podInfo.Status.Phase != corev1.PodRunning {
				log.Printf("pod: %s is not yet running with status: %v", podInfo.Name, podInfo.Status)
				return false, nil
			}
		}
		return true, nil
	}
}

func GetOpenShiftADPLogs(namespace string) (string, error) {
	return GetPodWithPrefixContainerLogs(namespace, "openshift-adp-controller-manager-", "manager")
}

// Returns logs from velero container on velero pod
func GetVeleroContainerLogs(namespace string) (string, error) {
	return GetPodWithPrefixContainerLogs(namespace, "velero-", "velero")
}

func GetVeleroContainerFailureLogs(namespace string) []string {
	containerLogs, err := GetVeleroContainerLogs(namespace)
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

func (v *DpaCustomResource) IsDeleted() wait.ConditionFunc {
	return func() (bool, error) {
		err := v.SetClient()
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
func VerifyVeleroTolerations(namespace string, t []corev1.Toleration) wait.ConditionFunc {
	return func() (bool, error) {
		clientset, err := setUpClient()
		if err != nil {
			return false, err
		}

		veldep, _ := clientset.AppsV1().Deployments(namespace).Get(context.Background(), "velero", metav1.GetOptions{})

		if !reflect.DeepEqual(t, veldep.Spec.Template.Spec.Tolerations) {
			return false, errors.New("given Velero tolerations does not match the deployed velero tolerations")
		}
		return true, nil
	}
}

// check for velero resource requests
func VerifyVeleroResourceRequests(namespace string, requests corev1.ResourceList) wait.ConditionFunc {
	return func() (bool, error) {
		clientset, err := setUpClient()
		if err != nil {
			return false, err
		}
		veldep, _ := clientset.AppsV1().Deployments(namespace).Get(context.Background(), "velero", metav1.GetOptions{})

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
func VerifyVeleroResourceLimits(namespace string, limits corev1.ResourceList) wait.ConditionFunc {
	return func() (bool, error) {
		clientset, err := setUpClient()
		if err != nil {
			return false, err
		}
		veldep, _ := clientset.AppsV1().Deployments(namespace).Get(context.Background(), "velero", metav1.GetOptions{})

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
