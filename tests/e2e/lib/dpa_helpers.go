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
	velero "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
)

type BackupRestoreType string

const (
	CSI             BackupRestoreType = "csi"
	CSIDataMover    BackupRestoreType = "csi-datamover"
	RESTIC          BackupRestoreType = "restic"
	KOPIA           BackupRestoreType = "kopia"
	NativeSnapshots BackupRestoreType = "native-snapshots"
)

type DpaCustomResource struct {
	Name                 string
	Namespace            string
	Client               client.Client
	VSLSecretName        string
	BSLSecretName        string
	BSLConfig            map[string]string
	BSLProvider          string
	BSLBucket            string
	BSLBucketPrefix      string
	VeleroDefaultPlugins []oadpv1alpha1.DefaultPlugin
	SnapshotLocations    []oadpv1alpha1.SnapshotLocation
	UnsupportedOverrides map[oadpv1alpha1.UnsupportedImageKey]string
}

func LoadDpaSettingsFromJson(settings string) (*oadpv1alpha1.DataProtectionApplication, error) {
	file, err := ReadFile(settings)
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

func (v *DpaCustomResource) Build(backupRestoreType BackupRestoreType) *oadpv1alpha1.DataProtectionApplicationSpec {
	dpaSpec := oadpv1alpha1.DataProtectionApplicationSpec{
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
		UnsupportedOverrides: v.UnsupportedOverrides,
	}
	switch backupRestoreType {
	case RESTIC, KOPIA:
		dpaSpec.Configuration.NodeAgent.Enable = ptr.To(true)
		dpaSpec.Configuration.NodeAgent.UploaderType = string(backupRestoreType)
		dpaSpec.SnapshotLocations = nil
	case CSI:
		dpaSpec.Configuration.NodeAgent.Enable = ptr.To(false)
		dpaSpec.Configuration.Velero.DefaultPlugins = append(dpaSpec.Configuration.Velero.DefaultPlugins, oadpv1alpha1.DefaultPluginCSI)
		dpaSpec.Configuration.Velero.FeatureFlags = append(dpaSpec.Configuration.Velero.FeatureFlags, velero.CSIFeatureFlag)
		dpaSpec.SnapshotLocations = nil
	case CSIDataMover:
		// We don't need to have restic use case, kopia is enough
		dpaSpec.Configuration.NodeAgent.Enable = ptr.To(true)
		dpaSpec.Configuration.NodeAgent.UploaderType = "kopia"
		dpaSpec.Configuration.Velero.DefaultPlugins = append(dpaSpec.Configuration.Velero.DefaultPlugins, oadpv1alpha1.DefaultPluginCSI)
		dpaSpec.Configuration.Velero.FeatureFlags = append(dpaSpec.Configuration.Velero.FeatureFlags, velero.CSIFeatureFlag)
		dpaSpec.SnapshotLocations = nil
	case NativeSnapshots:
		dpaSpec.SnapshotLocations[0].Velero.Credential = &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: v.VSLSecretName,
			},
			Key: "cloud",
		}
	}

	return &dpaSpec
}

func (v *DpaCustomResource) Create(dpa *oadpv1alpha1.DataProtectionApplication) error {
	err := v.Client.Create(context.Background(), dpa)
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
	// for debugging
	// prettyPrint, _ := json.MarshalIndent(spec, "", "  ")
	// log.Printf("DPA with spec\n%s\n", prettyPrint)
	dpa, err := v.Get()
	if err != nil {
		if apierrors.IsNotFound(err) {
			dpa = &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      v.Name,
					Namespace: v.Namespace,
				},
				Spec: *spec.DeepCopy(),
			}
			dpa.Spec.UnsupportedOverrides = v.UnsupportedOverrides
			return v.Create(dpa)
		}
		return err
	}
	dpaPatch := dpa.DeepCopy()
	spec.DeepCopyInto(&dpaPatch.Spec)
	dpaPatch.ObjectMeta.ManagedFields = nil
	dpaPatch.Spec.UnsupportedOverrides = v.UnsupportedOverrides
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
	dpa, err := v.Get()
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	err = v.Client.Delete(context.Background(), dpa)
	if err != nil && apierrors.IsNotFound(err) {
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

		for _, bsl := range bsls.Items {
			if !bsl.Status.LastValidationTime.After(updateTime) {
				return false, nil
			}
		}
		return true, nil
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
				for key, value := range dpaBSLSpec.Config {
					configWithChecksumAlgorithm[key] = value
				}
				configWithChecksumAlgorithm["checksumAlgorithm"] = ""
				dpaBSLSpec.Config = configWithChecksumAlgorithm
			}
		}
		if !reflect.DeepEqual(dpaBSLSpec, bslReal.Spec) {
			log.Println(cmp.Diff(dpaBSLSpec, bslReal.Spec))
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

// check if vsl matches the spec
func (v *DpaCustomResource) DoesVSLSpecMatchesDpa(namespace string, dpaVSLSpec velero.VolumeSnapshotLocationSpec) (bool, error) {
	vsls, err := v.ListVSLs()
	if err != nil {
		return false, err
	}
	for _, vslReal := range vsls.Items {
		if !reflect.DeepEqual(dpaVSLSpec, vslReal.Spec) {
			log.Println(cmp.Diff(dpaVSLSpec, vslReal.Spec))
			return false, errors.New("given DPA VSL spec does not match the deployed VSL")
		}
	}
	return true, nil
}
