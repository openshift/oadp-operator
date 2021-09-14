package e2e

import (
	"context"
	"errors"

	appsv1 "github.com/openshift/api/apps/v1"
	security "github.com/openshift/api/security/v1"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	velero "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

type veleroCustomResource struct {
	Name           string
	Namespace      string
	SecretName     string
	Bucket         string
	Region         string
	Provider       string
	CustomResource *oadpv1alpha1.Velero
	Client         client.Client
}

func (v *veleroCustomResource) Build() error {
	// Velero Instance creation spec with backupstorage location default to AWS. Would need to parameterize this later on to support multiple plugins.
	veleroSpec := oadpv1alpha1.Velero{
		ObjectMeta: metav1.ObjectMeta{
			Name:      v.Name,
			Namespace: v.Namespace,
		},
		Spec: oadpv1alpha1.VeleroSpec{
			OlmManaged:   pointer.Bool(false),
			EnableRestic: pointer.Bool(true),
			BackupStorageLocations: []velero.BackupStorageLocationSpec{
				{
					Provider: v.Provider,
					Config: map[string]string{
						"region": v.Region,
					},
					Default: true,
					StorageType: velero.StorageType{
						ObjectStorage: &velero.ObjectStorageLocation{
							Bucket: v.Bucket,
							Prefix: "velero",
						},
					},
				},
			},
			DefaultVeleroPlugins: []oadpv1alpha1.DefaultPlugin{
				oadpv1alpha1.DefaultPluginOpenShift,
				oadpv1alpha1.DefaultPluginAWS,
			},
		},
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
		v.Build()
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

func isVeleroPodRunning(namespace string) wait.ConditionFunc {
	return func() (bool, error) {
		clientset, err := setUpClient()
		if err != nil {
			return false, err
		}
		// select Velero pod with this label
		veleroOptions := metav1.ListOptions{
			LabelSelector: "component=velero",
		}
		// get pods in test namespace with labelSelector
		podList, err := clientset.CoreV1().Pods(namespace).List(context.TODO(), veleroOptions)
		if err != nil {
			return false, nil
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
