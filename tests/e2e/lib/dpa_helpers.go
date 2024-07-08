package lib

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"reflect"
	"time"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo/v2"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	utils "github.com/openshift/oadp-operator/tests/e2e/utils"
	velero "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/wait"
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

var (
	VeleroPrefix = "velero-e2e-" + string(uuid.NewUUID())
	Dpa          *oadpv1alpha1.DataProtectionApplication
)

type DpaCustomResource struct {
	Name              string
	Namespace         string
	backupRestoreType BackupRestoreType
	CustomResource    *oadpv1alpha1.DataProtectionApplication
	Client            client.Client
	Provider          string
	BSLSecretName     string
}

func LoadDpaSettingsFromJson(settings string) error {
	file, err := utils.ReadFile(settings)
	if err != nil {
		return fmt.Errorf("Error decoding json file: %v", err)
	}

	Dpa = &oadpv1alpha1.DataProtectionApplication{}
	err = json.Unmarshal(file, &Dpa)
	if err != nil {
		return fmt.Errorf("Error getting settings json file: %v", err)
	}
	return nil
}

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
								Name: v.BSLSecretName,
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

func (v *DpaCustomResource) Create() error {
	err := v.Client.Create(context.Background(), v.CustomResource)
	if apierrors.IsAlreadyExists(err) {
		return errors.New("found unexpected existing Velero CR")
	} else if err != nil {
		return err
	}
	return nil
}

func (v *DpaCustomResource) Get() (*oadpv1alpha1.DataProtectionApplication, error) {
	dpa := oadpv1alpha1.DataProtectionApplication{}
	err := v.Client.Get(context.Background(), client.ObjectKey{
		Namespace: v.Namespace,
		Name:      v.Name,
	}, &dpa)
	if err != nil {
		return nil, err
	}
	return &dpa, nil
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
	err := v.Client.Delete(context.Background(), v.CustomResource)
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func (v *DpaCustomResource) IsDeleted() wait.ConditionFunc {
	return func() (bool, error) {
		// Check for velero CR in cluster
		vel := oadpv1alpha1.DataProtectionApplication{}
		err := v.Client.Get(context.Background(), client.ObjectKey{
			Namespace: v.Namespace,
			Name:      v.Name,
		}, &vel)
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}
}

func (v *DpaCustomResource) IsReconciled() wait.ConditionFunc {
	return func() (bool, error) {
		dpa, err := v.Get()
		if err != nil {
			return false, err
		}
		if len(dpa.Status.Conditions) == 0 {
			return false, nil
		}
		log.Printf("DPA status is %s", dpa.Status.Conditions[0].Type)
		return dpa.Status.Conditions[0].Type == oadpv1alpha1.ConditionReconciled, nil
	}
}

func (v *DpaCustomResource) IsNotReconciled(message string) wait.ConditionFunc {
	return func() (bool, error) {
		dpa, err := v.Get()
		if err != nil {
			return false, err
		}
		if len(dpa.Status.Conditions) == 0 {
			return false, nil
		}
		log.Printf("DPA status is %s; %s; %s", dpa.Status.Conditions[0].Status, dpa.Status.Conditions[0].Reason, dpa.Status.Conditions[0].Message)
		return dpa.Status.Conditions[0].Status == metav1.ConditionFalse &&
			dpa.Status.Conditions[0].Reason == oadpv1alpha1.ReconciledReasonError &&
			dpa.Status.Conditions[0].Message == message, nil
	}
}

// func (v *DpaCustomResource) IsUpdated(updateTime time.Time) bool {
// 	dpa, err := v.Get()
// 	if err != nil {
// 		return false
// 	}
// 	if len(dpa.Status.Conditions) == 0 {
// 		return false
// 	}
// 	return dpa.Status.Conditions[0].LastTransitionTime.After(updateTime)
// }

func (v *DpaCustomResource) ListBSLs() (*velero.BackupStorageLocationList, error) {
	bsls := &velero.BackupStorageLocationList{}
	err := v.Client.List(context.Background(), bsls, client.InNamespace(v.Namespace))
	if err != nil {
		return nil, err
	}
	if len(bsls.Items) == 0 {
		return nil, fmt.Errorf("no BSL in %s namespace", v.Namespace)
	}
	return bsls, nil
}

func (v *DpaCustomResource) BSLsAreAvailable() wait.ConditionFunc {
	return func() (bool, error) {
		bsls, err := v.ListBSLs()
		if err != nil {
			return false, err
		}
		areAvailable := true
		for _, bsl := range bsls.Items {
			phase := bsl.Status.Phase
			if len(phase) > 0 {
				log.Printf("BSL %s phase is %s", bsl.Name, phase)
				if phase != velero.BackupStorageLocationPhaseAvailable {
					areAvailable = false
				}
			} else {
				log.Printf("BSL %s phase is not yet set", bsl.Name)
				areAvailable = false
			}
		}

		return areAvailable, nil
	}
}

func (v *DpaCustomResource) BSLsAreUpdated(updateTime time.Time) wait.ConditionFunc {
	return func() (bool, error) {
		bsls, err := v.ListBSLs()
		if err != nil {
			return false, err
		}
		areUpdated := true
		for _, bsl := range bsls.Items {
			if !bsl.Status.LastValidationTime.After(updateTime) {
				areUpdated = false
			}
		}

		return areUpdated, nil
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

func (v *DpaCustomResource) ListVSLs() (*velero.VolumeSnapshotLocationList, error) {
	vsls := &velero.VolumeSnapshotLocationList{}
	err := v.Client.List(context.Background(), vsls, client.InNamespace(v.Namespace))
	if err != nil {
		return nil, err
	}
	if len(vsls.Items) == 0 {
		return nil, fmt.Errorf("no VSL in %s namespace", v.Namespace)
	}
	return vsls, nil
}

func (v *DpaCustomResource) VSLsAreAvailable() wait.ConditionFunc {
	return func() (bool, error) {
		vsls, err := v.ListVSLs()
		if err != nil {
			return false, err
		}
		areAvailable := true
		for _, vsl := range vsls.Items {
			phase := vsl.Status.Phase
			if len(phase) > 0 {
				log.Printf("VSL %s phase is %s", vsl.Name, phase)
				if phase != velero.VolumeSnapshotLocationPhaseAvailable {
					areAvailable = false
				}
			} else {
				log.Printf("VSL %s phase is not yet set", vsl.Name)
				areAvailable = false
			}
		}

		return areAvailable, nil
	}
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
