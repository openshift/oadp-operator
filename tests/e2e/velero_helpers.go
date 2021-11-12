package e2e

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"reflect"
	"strings"

	appsv1 "github.com/openshift/api/apps/v1"
	security "github.com/openshift/api/security/v1"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	velero "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

type BackupRestoreType string

const (
	csi    BackupRestoreType = "csi"
	restic BackupRestoreType = "restic"
)

type veleroCustomResource struct {
	Name              string
	Namespace         string
	SecretName        string
	Bucket            string
	BslRegion         string
	VslRegion         string
	BslProfile        string
	Provider          string
	credentials       string
	credSecretRef     string
	backupRestoreType BackupRestoreType
	CustomResource    *oadpv1alpha1.Velero
	Client            client.Client
}

var veleroPrefix = "velero-e2e-" + string(uuid.NewUUID())

func (v *veleroCustomResource) Build(backupRestoreType BackupRestoreType) error {
	// Velero Instance creation spec with backupstorage location default to AWS. Would need to parameterize this later on to support multiple plugins.

	veleroSpec := oadpv1alpha1.Velero{
		ObjectMeta: metav1.ObjectMeta{
			Name:      v.Name,
			Namespace: v.Namespace,
		},
		Spec: oadpv1alpha1.VeleroSpec{
			BackupStorageLocations: []velero.BackupStorageLocationSpec{
				{
					Provider: v.Provider,
					Default:  true,
					Credential: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: v.credSecretRef,
						},
						Key: "cloud",
					},
					StorageType: velero.StorageType{
						ObjectStorage: &velero.ObjectStorageLocation{
							Bucket: v.Bucket,
							Prefix: veleroPrefix,
						},
					},
				},
			},
			DefaultVeleroPlugins: []oadpv1alpha1.DefaultPlugin{
				oadpv1alpha1.DefaultPluginOpenShift,
			},
			VeleroFeatureFlags: []string{},
		},
	}
	v.backupRestoreType = backupRestoreType
	switch backupRestoreType {
	case restic:
		veleroSpec.Spec.EnableRestic = pointer.Bool(true)
	case csi:
		veleroSpec.Spec.EnableRestic = pointer.Bool(false)
		veleroSpec.Spec.DefaultVeleroPlugins = append(veleroSpec.Spec.DefaultVeleroPlugins, oadpv1alpha1.DefaultPluginCSI)
		veleroSpec.Spec.VeleroFeatureFlags = append(veleroSpec.Spec.VeleroFeatureFlags, "EnableCSI")
	}
	switch v.Provider {
	case "aws":
		veleroSpec.Spec.BackupStorageLocations[0].Config = map[string]string{
			"region":  v.BslRegion,
			"profile": v.BslProfile,
		}
		veleroSpec.Spec.VolumeSnapshotLocations = []velero.VolumeSnapshotLocationSpec{
			{
				Provider: v.Provider,
				Config: map[string]string{
					"region":  v.VslRegion,
					"profile": "default",
				},
			},
		}
		veleroSpec.Spec.DefaultVeleroPlugins = append(veleroSpec.Spec.DefaultVeleroPlugins, oadpv1alpha1.DefaultPluginAWS) // case "gcp":
		// 	config["serviceAccount"] = v.Region
	}
	v.CustomResource = &veleroSpec
	return nil
}

func (v *veleroCustomResource) Create() error {
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

func (v *veleroCustomResource) Get() (*oadpv1alpha1.Velero, error) {
	err := v.SetClient()
	if err != nil {
		return nil, err
	}
	vel := oadpv1alpha1.Velero{}
	err = v.Client.Get(context.Background(), client.ObjectKey{
		Namespace: v.Namespace,
		Name:      v.Name,
	}, &vel)
	if err != nil {
		return nil, err
	}
	return &vel, nil
}

func (v *veleroCustomResource) CreateOrUpdate(spec *oadpv1alpha1.VeleroSpec) error {
	cr, err := v.Get()
	if apierrors.IsNotFound(err) {
		v.Build(v.backupRestoreType)
		v.CustomResource.Spec = *spec
		return v.Create()
	}
	if err != nil {
		return err
	}
	cr.Spec = *spec
	err = v.Client.Update(context.Background(), cr)
	if err != nil {
		return err
	}
	return nil
}

func (v *veleroCustomResource) Delete() error {
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

func (v *veleroCustomResource) SetClient() error {
	client, err := client.New(config.GetConfigOrDie(), client.Options{})
	if err != nil {
		return err
	}
	oadpv1alpha1.AddToScheme(client.Scheme())
	velero.AddToScheme(client.Scheme())
	appsv1.AddToScheme(client.Scheme())
	security.AddToScheme(client.Scheme())

	v.Client = client
	return nil
}

func getVeleroPods(namespace string) (*corev1.PodList, error) {
	clientset, err := setUpClient()
	if err != nil {
		return nil, err
	}
	// select Velero pod with this label
	veleroOptions := metav1.ListOptions{
		LabelSelector: "component=velero",
	}
	// get pods in test namespace with labelSelector
	podList, err := clientset.CoreV1().Pods(namespace).List(context.TODO(), veleroOptions)
	if err != nil {
		return nil, err
	}
	return podList, nil
}

func isVeleroPodRunning(namespace string) wait.ConditionFunc {
	return func() (bool, error) {
		podList, err := getVeleroPods(namespace)
		if err != nil {
			return false, err
		}
		// get pod name and status with specified label selector
		var status string
		for _, podInfo := range (*podList).Items {
			status = string(podInfo.Status.Phase)
		}
		if status == "Running" {
			return true, nil
		}
		return false, err
	}
}

// Returns logs from velero container on velero pod
func getVeleroContainerLogs(namespace string) (string, error) {
	podList, err := getVeleroPods(namespace)
	if err != nil {
		return "", err
	}
	clientset, err := setUpClient()
	if err != nil {
		return "", err
	}
	var logs string
	for _, podInfo := range (*podList).Items {
		if !strings.HasPrefix(podInfo.ObjectMeta.Name, "velero-") {
			continue
		}
		podLogOpts := corev1.PodLogOptions{
			Container: "velero",
		}
		req := clientset.CoreV1().Pods(podInfo.Namespace).GetLogs(podInfo.Name, &podLogOpts)
		podLogs, err := req.Stream(context.TODO())
		if err != nil {
			return "", err
		}
		defer podLogs.Close()
		buf := new(bytes.Buffer)
		_, err = io.Copy(buf, podLogs)
		if err != nil {
			return "", err
		}
		logs = buf.String()
	}
	return logs, nil
}

func getVeleroContainerFailureLogs(namespace string) []string {
	containerLogs, err := getVeleroContainerLogs(namespace)
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

func (v *veleroCustomResource) IsDeleted() wait.ConditionFunc {
	return func() (bool, error) {
		err := v.SetClient()
		if err != nil {
			return false, err
		}
		// Check for velero CR in cluster
		vel := oadpv1alpha1.Velero{}
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

//check if bsl matches the spec
func doesBSLExist(namespace string, bsl velero.BackupStorageLocationSpec, spec *oadpv1alpha1.VeleroSpec) wait.ConditionFunc {
	return func() (bool, error) {
		if len(spec.BackupStorageLocations) == 0 {
			return false, errors.New("no backup storage location configured. Expected BSL to be configured")
		}
		for _, b := range spec.BackupStorageLocations {
			if b.Provider == bsl.Provider {
				if !reflect.DeepEqual(bsl, b) {
					return false, errors.New("given Velero bsl does not match the deployed velero bsl")
				}
			}
		}
		return true, nil
	}
}

//check if vsl matches the spec
func doesVSLExist(namespace string, vslspec velero.VolumeSnapshotLocationSpec, spec *oadpv1alpha1.VeleroSpec) wait.ConditionFunc {
	return func() (bool, error) {

		if len(spec.VolumeSnapshotLocations) == 0 {
			return false, errors.New("no volume storage location configured. Expected VSL to be configured")
		}
		for _, v := range spec.VolumeSnapshotLocations {
			if v.Provider == vslspec.Provider {
				if !reflect.DeepEqual(vslspec, v) {
					return false, errors.New("given Velero vslspec does not match the deployed velero vslspec")
				}
			}
		}
		return true, nil
	}
}

//check velero tolerations
func verifyVeleroTolerations(namespace string, t []corev1.Toleration) wait.ConditionFunc {
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
