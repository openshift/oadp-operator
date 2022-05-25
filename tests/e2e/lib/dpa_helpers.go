package lib

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"reflect"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	buildv1 "github.com/openshift/api/build/v1"
	"github.com/openshift/oadp-operator/pkg/common"

	volumesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	appsv1 "github.com/openshift/api/apps/v1"
	security "github.com/openshift/api/security/v1"
	templatev1 "github.com/openshift/api/template/v1"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	utils "github.com/openshift/oadp-operator/tests/e2e/utils"
	operators "github.com/operator-framework/api/pkg/operators/v1alpha1"
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
	CSI    BackupRestoreType = "csi"
	RESTIC BackupRestoreType = "restic"
)

type DpaCustomResource struct {
	Name              string
	Namespace         string
	SecretName        string
	backupRestoreType BackupRestoreType
	CustomResource    *oadpv1alpha1.DataProtectionApplication
	Client            client.Client
	Credentials       string
	CredSecretRef     string
	Provider          string
}

type DpaCROption func(*oadpv1alpha1.DataProtectionApplication) error

func WithConfiguration(configuration *oadpv1alpha1.ApplicationConfig) DpaCROption {
	return func(cr *oadpv1alpha1.DataProtectionApplication) error {
		cr.Spec.Configuration = configuration
		return nil
	}
}

func WithVeleroConfig(config *oadpv1alpha1.VeleroConfig) DpaCROption {
	return func(cr *oadpv1alpha1.DataProtectionApplication) error {
		cr.Spec.Configuration.Velero = config
		return nil
	}
}

func WithResticConfig(config *oadpv1alpha1.ResticConfig) DpaCROption {
	return func(cr *oadpv1alpha1.DataProtectionApplication) error {
		cr.Spec.Configuration.Restic = config
		return nil
	}
}

func WithBackupImages(backupImage bool) DpaCROption {
	return func(cr *oadpv1alpha1.DataProtectionApplication) error {
		cr.Spec.BackupImages = pointer.Bool(backupImage)
		return nil
	}
}

var VeleroPrefix = "velero-e2e-" + string(uuid.NewUUID())
var Dpa *oadpv1alpha1.DataProtectionApplication

func (v *DpaCustomResource) VeleroBSL() *velero.BackupStorageLocationSpec {
	return &velero.BackupStorageLocationSpec{
		Provider:   v.CustomResource.Spec.BackupLocations[0].Velero.Provider,
		Default:    true,
		Config:     v.CustomResource.Spec.BackupLocations[0].Velero.Config,
		Credential: v.CustomResource.Spec.BackupLocations[0].Velero.Credential,
		StorageType: velero.StorageType{
			ObjectStorage: &velero.ObjectStorageLocation{
				Bucket: v.CustomResource.Spec.BackupLocations[0].Velero.ObjectStorage.Bucket,
				Prefix: VeleroPrefix,
			},
		},
	}
}

func (v *DpaCustomResource) Build(backupRestoreType BackupRestoreType, dpaCrOpts ...DpaCROption) error {
	// Velero Instance creation spec with backupstorage location default to AWS. Would need to parameterize this later on to support multiple plugins.
	dpaInstance := oadpv1alpha1.DataProtectionApplication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      v.Name,
			Namespace: v.Namespace,
		},
		Spec: oadpv1alpha1.DataProtectionApplicationSpec{
			Configuration: &oadpv1alpha1.ApplicationConfig{
				Velero: &oadpv1alpha1.VeleroConfig{
					DefaultPlugins: v.CustomResource.Spec.Configuration.Velero.DefaultPlugins,
				},
				Restic: &oadpv1alpha1.ResticConfig{
					PodConfig: &oadpv1alpha1.PodConfig{},
				},
			},
			SnapshotLocations: v.CustomResource.Spec.SnapshotLocations,
			BackupLocations: []oadpv1alpha1.BackupLocation{
				{
					Velero: v.VeleroBSL(),
				},
			},
		},
	}
	if dpaInstance.Spec.BackupLocations[0].Velero.Config != nil {
		dpaInstance.Spec.BackupLocations[0].Velero.Config["credentialsFile"] = "bsl-cloud-credentials-" + v.Provider + "/cloud"
	}
	v.backupRestoreType = backupRestoreType
	switch backupRestoreType {
	case RESTIC:
		dpaInstance.Spec.Configuration.Restic.Enable = pointer.Bool(true)
	case CSI:
		dpaInstance.Spec.Configuration.Restic.Enable = pointer.Bool(false)
		dpaInstance.Spec.Configuration.Velero.DefaultPlugins = append(dpaInstance.Spec.Configuration.Velero.DefaultPlugins, oadpv1alpha1.DefaultPluginCSI)
		dpaInstance.Spec.Configuration.Velero.FeatureFlags = append(dpaInstance.Spec.Configuration.Velero.FeatureFlags, "EnableCSI")
	}

	for _, opt := range dpaCrOpts {
		if err := opt(&dpaInstance); err != nil {
			return err
		}
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
			v.Build(v.backupRestoreType)
			v.CustomResource.Spec = *spec
			return v.Create()
		} else if err != nil {
			return err
		}
		cr.Spec = *spec
		if err = v.Client.Update(context.Background(), cr); err != nil {
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

	v.Client = client
	return nil
}

func (dpa *DpaCustomResource) DPAReconcileError() error {
	if dpa.GetNoErr().Status.Conditions == nil {
		return errors.New("no status conditions yet")
	}
	if dpa.GetNoErr().Status.Conditions[0].Type != ("Reconciled") {
		return errors.New("Expected Reconciled condition on DPA, got " + dpa.GetNoErr().Status.Conditions[0].Type)
	}
	if dpa.GetNoErr().Status.Conditions[0].Status != metav1.ConditionTrue {
		return fmt.Errorf("expected reconciled condition on DPA to be true, got %v", dpa.GetNoErr().Status.Conditions[0].Status)
	}
	if dpa.GetNoErr().Status.Conditions[0].Reason !="Error" {
		return nil
	}
	return errors.New(dpa.GetNoErr().Status.Conditions[0].Message)
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

func AreVeleroDeploymentReplicasReady(namespace string) wait.ConditionFunc {
	return func() (bool, error) {
		deploymentList, err := GetVeleroDeploymentList(namespace)
		if err != nil {
			return false, err
		}
		if deploymentList.Items == nil || len(deploymentList.Items) == 0 {
			GinkgoWriter.Println("velero deployments not found")
			return false, nil
		}
		for _, deploymentInfo := range (*deploymentList).Items {
			if deploymentInfo.Status.UpdatedReplicas != deploymentInfo.Status.Replicas || 
				deploymentInfo.Status.AvailableReplicas != deploymentInfo.Status.Replicas || 
				deploymentInfo.Status.UnavailableReplicas != 0 {
				log.Printf("deployment: %s does not have desired updated replicas: %v", deploymentInfo.Name, deploymentInfo.Status)
				log.Printf("deployment has conditions: %v", deploymentInfo.Status.Conditions)
				return false, nil
			}
		}
		return true, nil
	}
}

// Returns logs from velero container on velero pod
func getVeleroContainerLogs(namespace string) (string, error) {
	podList, err := GetVeleroPods(namespace)
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

func GetVeleroContainerFailureLogs(namespace string) []string {
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

//check if bsl matches the spec
func DoesBSLExist(namespace string, bsl velero.BackupStorageLocationSpec, spec *oadpv1alpha1.DataProtectionApplicationSpec) wait.ConditionFunc {
	return func() (bool, error) {
		if len(spec.BackupLocations) == 0 {
			return false, errors.New("no backup storage location configured. Expected BSL to be configured")
		}
		for _, b := range spec.BackupLocations {
			if b.Velero.Provider == bsl.Provider {
				if !reflect.DeepEqual(bsl, *b.Velero) {
					return false, errors.New("given Velero bsl does not match the deployed velero bsl")
				}
			}
		}
		return true, nil
	}
}

//check if vsl matches the spec
func DoesVSLExist(namespace string, vslspec velero.VolumeSnapshotLocationSpec, spec *oadpv1alpha1.DataProtectionApplicationSpec) wait.ConditionFunc {
	return func() (bool, error) {

		if len(spec.SnapshotLocations) == 0 {
			return false, errors.New("no volume storage location configured. Expected VSL to be configured")
		}
		for _, v := range spec.SnapshotLocations {
			if reflect.DeepEqual(vslspec, *v.Velero) {
				return true, nil
			}
		}
		return false, errors.New("did not find expected VSL")

	}
}

//check velero tolerations
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

func GetSecretRef(credSecretRef string) string {
	if Dpa.Spec.BackupLocations[0].Velero.Credential == nil {
		return credSecretRef
	} else {
		return Dpa.Spec.BackupLocations[0].Velero.Credential.Name
	}
}
