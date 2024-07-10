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

type DpaCustomResource struct {
	Name                 string
	Namespace            string
	CustomResource       *oadpv1alpha1.DataProtectionApplication
	Client               client.Client
	Provider             string
	BSLSecretName        string
	BSLConfig            map[string]string
	BSLProvider          string
	BSLBucket            string
	BSLBucketPrefix      string
	VeleroDefaultPlugins []oadpv1alpha1.DefaultPlugin
	SnapshotLocations    []oadpv1alpha1.SnapshotLocation
}

func LoadDpaSettingsFromJson(settings string) (*oadpv1alpha1.DataProtectionApplication, error) {
	file, err := utils.ReadFile(settings)
	if err != nil {
		return nil, fmt.Errorf("Error getting settings json file: %v", err)
	}

	dpa := &oadpv1alpha1.DataProtectionApplication{}
	err = json.Unmarshal(file, &dpa)
	if err != nil {
		return nil, fmt.Errorf("Error decoding json file: %v", err)
	}
	return dpa, nil
}

func (v *DpaCustomResource) Build(backupRestoreType BackupRestoreType) error {
	dpaInstance := oadpv1alpha1.DataProtectionApplication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      v.Name,
			Namespace: v.Namespace,
		},
		Spec: oadpv1alpha1.DataProtectionApplicationSpec{
			Configuration: &oadpv1alpha1.ApplicationConfig{
				Velero: &oadpv1alpha1.VeleroConfig{
					LogLevel:       "debug",
					DefaultPlugins: v.VeleroDefaultPlugins,
				},
				NodeAgent: &oadpv1alpha1.NodeAgentConfig{
					UploaderType: "kopia",
					NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
						PodConfig: &oadpv1alpha1.PodConfig{},
					},
				},
			},
			SnapshotLocations: v.SnapshotLocations,
			BackupLocations: []oadpv1alpha1.BackupLocation{
				{
					Velero: &velero.BackupStorageLocationSpec{
						Provider: v.BSLProvider,
						Default:  true,
						Config:   v.BSLConfig,
						Credential: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: v.BSLSecretName,
							},
							Key: "cloud",
						},
						StorageType: velero.StorageType{
							ObjectStorage: &velero.ObjectStorageLocation{
								Bucket: v.BSLBucket,
								Prefix: v.BSLBucketPrefix,
							},
						},
					},
				},
			},
		},
	}
	switch backupRestoreType {
	case RESTIC, KOPIA:
		dpaInstance.Spec.Configuration.NodeAgent.Enable = pointer.Bool(true)
		dpaInstance.Spec.Configuration.NodeAgent.UploaderType = string(backupRestoreType)
		dpaInstance.Spec.SnapshotLocations = nil
	case CSI:
		dpaInstance.Spec.Configuration.NodeAgent.Enable = pointer.Bool(false)
		dpaInstance.Spec.Configuration.Velero.DefaultPlugins = append(dpaInstance.Spec.Configuration.Velero.DefaultPlugins, oadpv1alpha1.DefaultPluginCSI)
		dpaInstance.Spec.Configuration.Velero.FeatureFlags = append(dpaInstance.Spec.Configuration.Velero.FeatureFlags, velero.CSIFeatureFlag)
		dpaInstance.Spec.SnapshotLocations = nil
	case CSIDataMover:
		// We don't need to have restic use case, kopia is enough
		dpaInstance.Spec.Configuration.NodeAgent.Enable = pointer.Bool(true)
		dpaInstance.Spec.Configuration.NodeAgent.UploaderType = "kopia"
		dpaInstance.Spec.Configuration.Velero.DefaultPlugins = append(dpaInstance.Spec.Configuration.Velero.DefaultPlugins, oadpv1alpha1.DefaultPluginCSI)
		dpaInstance.Spec.Configuration.Velero.FeatureFlags = append(dpaInstance.Spec.Configuration.Velero.FeatureFlags, velero.CSIFeatureFlag)
		dpaInstance.Spec.SnapshotLocations = nil
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
	dpa, err := v.Get()
	if err != nil {
		if apierrors.IsNotFound(err) {
			v.CustomResource = &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      v.Name,
					Namespace: v.Namespace,
				},
				Spec: *spec.DeepCopy(),
			}
			return v.Create()
		}
		return err
	}
	dpaPatch := dpa.DeepCopy()
	spec.DeepCopyInto(&dpaPatch.Spec)
	dpaPatch.ObjectMeta.ManagedFields = nil
	err = v.Client.Patch(context.Background(), dpaPatch, client.MergeFrom(dpa), &client.PatchOptions{})
	if err != nil {
		log.Printf("error patching DPA: %s", err)
		if apierrors.IsConflict(err) {
			log.Println("conflict detected during DPA CreateOrUpdate")
		}
		return err
	}
	return nil
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

func (v *DpaCustomResource) IsReconciledTrue() wait.ConditionFunc {
	return func() (bool, error) {
		dpa, err := v.Get()
		if err != nil {
			return false, err
		}
		if len(dpa.Status.Conditions) == 0 {
			return false, nil
		}
		dpaType := dpa.Status.Conditions[0].Type
		dpaStatus := dpa.Status.Conditions[0].Status
		dpaReason := dpa.Status.Conditions[0].Reason
		dpaMessage := dpa.Status.Conditions[0].Message
		log.Printf("DPA status is %s: %s, reason %s: %s", dpaType, dpaStatus, dpaReason, dpaMessage)
		return dpaType == oadpv1alpha1.ConditionReconciled &&
			dpaStatus == metav1.ConditionTrue &&
			dpaReason == oadpv1alpha1.ReconciledReasonComplete &&
			dpaMessage == oadpv1alpha1.ReconcileCompleteMessage, nil
	}
}

func (v *DpaCustomResource) IsReconciledFalse(message string) wait.ConditionFunc {
	return func() (bool, error) {
		dpa, err := v.Get()
		if err != nil {
			return false, err
		}
		if len(dpa.Status.Conditions) == 0 {
			return false, nil
		}
		dpaType := dpa.Status.Conditions[0].Type
		dpaStatus := dpa.Status.Conditions[0].Status
		dpaReason := dpa.Status.Conditions[0].Reason
		dpaMessage := dpa.Status.Conditions[0].Message
		log.Printf("DPA status is %s: %s, reason %s: %s", dpaType, dpaStatus, dpaReason, dpaMessage)
		return dpaType == oadpv1alpha1.ConditionReconciled &&
			dpaStatus == metav1.ConditionFalse &&
			dpaReason == oadpv1alpha1.ReconciledReasonError &&
			dpaMessage == message, nil
	}
}

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
func (v *DpaCustomResource) DoesBSLSpecMatchesDpa(namespace string, dpaBSLSpec velero.BackupStorageLocationSpec) (bool, error) {
	bsls, err := v.ListBSLs()
	if err != nil {
		return false, err
	}
	for _, bslReal := range bsls.Items {
		if bslReal.Spec.Provider == "aws" {
			if _, exists := dpaBSLSpec.Config["checksumAlgorithm"]; !exists {
				configWithChecksumAlgorithm := map[string]string{}
				for key, value := range v.BSLConfig {
					configWithChecksumAlgorithm[key] = value
				}
				configWithChecksumAlgorithm["checksumAlgorithm"] = ""
				dpaBSLSpec.Config = configWithChecksumAlgorithm
			}
		}
		if !reflect.DeepEqual(dpaBSLSpec, bslReal.Spec) {
			GinkgoWriter.Print(cmp.Diff(dpaBSLSpec, bslReal.Spec))
			return false, errors.New("given DPA BSL spec does not match the deployed BSL")
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
func (v *DpaCustomResource) DoesVSLSpecMatchesDpa(namespace string, dpaVSLSpec velero.VolumeSnapshotLocationSpec) (bool, error) {
	vsls, err := v.ListBSLs()
	if err != nil {
		return false, err
	}
	for _, vslReal := range vsls.Items {
		if !reflect.DeepEqual(dpaVSLSpec, vslReal.Spec) {
			GinkgoWriter.Print(cmp.Diff(dpaVSLSpec, vslReal.Spec))
			return false, errors.New("given DPA VSL spec does not match the deployed VSL")
		}
	}
	return true, nil
}
