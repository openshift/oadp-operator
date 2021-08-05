package controllers

import (
	"fmt"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"

	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// Registry Env var keys
const (
	// AWS registry env vars
	RegistryStorageEnvVarKey                 = "REGISTRY_STORAGE"
	RegistryStorageS3AccesskeyEnvVarKey      = "REGISTRY_STORAGE_S3_ACCESSKEY"
	RegistryStorageS3BucketEnvVarKey         = "REGISTRY_STORAGE_S3_BUCKET"
	RegistryStorageS3RegionEnvVarKey         = "REGISTRY_STORAGE_S3_REGION"
	RegistryStorageS3SecretkeyEnvVarKey      = "REGISTRY_STORAGE_S3_SECRETKEY"
	RegistryStorageS3RegionendpointEnvVarKey = "REGISTRY_STORAGE_S3_REGIONENDPOINT"
	RegistryStorageS3RootdirectoryEnvVarKey  = "REGISTRY_STORAGE_S3_ROOTDIRECTORY"
	RegistryStorageS3SkipverifyEnvVarKey     = "REGISTRY_STORAGE_S3_SKIPVERIFY"
	// Azure registry env vars
	RegistryStorageAzureContainerEnvVarKey   = "REGISTRY_STORAGE_AZURE_CONTAINER"
	RegistryStorageAzureAccountnameEnvVarKey = "REGISTRY_STORAGE_AZURE_ACCOUNTNAME"
	RegistryStorageAzureAccountkeyEnvVarKey  = "REGISTRY_STORAGE_AZURE_ACCOUNTKEY"
	// GCP registry env vars
	RegistryStorageGCSBucket        = "REGISTRY_STORAGE_GCS_BUCKET"
	RegistryStorageGCSKeyfile       = "REGISTRY_STORAGE_GCS_KEYFILE"
	RegistryStorageGCSRootdirectory = "REGISTRY_STORAGE_GCS_ROOTDIRECTORY"
)

// provider specific object storage
const (
	S3                    = "s3"
	Azure                 = "azure"
	GCS                   = "gcs"
	AWSProvider           = "aws"
	AzureProvider         = "azure"
	GCPProvider           = "gcp"
	Region                = "region"
	S3URL                 = "s3Url"
	RootDirectory         = "rootDirectory"
	InsecureSkipTLSVerify = "insecureSkipTLSVerify"
	StorageAccount        = "storageAccount"
)

// creating skeleton for provider based env var map
var cloudProviderEnvVarMap = map[string][]corev1.EnvVar{
	"aws": {
		{
			Name:  RegistryStorageEnvVarKey,
			Value: S3,
		},
		{
			Name:  RegistryStorageS3AccesskeyEnvVarKey,
			Value: "",
		},
		{
			Name:  RegistryStorageS3BucketEnvVarKey,
			Value: "",
		},
		{
			Name:  RegistryStorageS3RegionEnvVarKey,
			Value: "",
		},
		{
			Name:  RegistryStorageS3SecretkeyEnvVarKey,
			Value: "",
		},
		{
			Name:  RegistryStorageS3RegionendpointEnvVarKey,
			Value: "",
		},
		{
			Name:  RegistryStorageS3RootdirectoryEnvVarKey,
			Value: "",
		},
		{
			Name:  RegistryStorageS3SkipverifyEnvVarKey,
			Value: "",
		},
	},
	"azure": {
		{
			Name:  RegistryStorageEnvVarKey,
			Value: Azure,
		},
		{
			Name:  RegistryStorageAzureContainerEnvVarKey,
			Value: "",
		},
		{
			Name:  RegistryStorageAzureAccountnameEnvVarKey,
			Value: "",
		},
		{
			Name:  RegistryStorageAzureAccountkeyEnvVarKey,
			Value: "",
		},
	},
	"gcp": {
		{
			Name:  RegistryStorageEnvVarKey,
			Value: GCS,
		},
		{
			Name:  RegistryStorageGCSBucket,
			Value: "",
		},
		{
			Name:  RegistryStorageGCSKeyfile,
			Value: "",
		},
		{
			Name:  RegistryStorageGCSRootdirectory,
			Value: "",
		},
	},
}

func (r *VeleroReconciler) ReconcileRegistries(log logr.Logger) (bool, error) {

	bslLabels := map[string]string{
		"app.kubernetes.io/name":       "oadp-operator-velero",
		"app.kubernetes.io/managed-by": "oadp-operator",
		"app.kubernetes.io/component":  "bsl",
	}
	bslListOptions := client.MatchingLabels(bslLabels)
	backupStorageLocationList := velerov1.BackupStorageLocationList{}

	// Fetch the configured backupstoragelocations
	if err := r.List(r.Context, &backupStorageLocationList, bslListOptions); err != nil {
		return false, err
	}

	// Loop through all the configured BSLs and create registry for each of them
	for _, bsl := range backupStorageLocationList.Items {
		registryDeployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      constructBSLRegistryName(&bsl),
				Namespace: VeleoNamespace,
			},
		}

		op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, registryDeployment, func() error {

			// Setting Registry Deployment selector if a new object is created as it is immutable
			if registryDeployment.ObjectMeta.CreationTimestamp.IsZero() {
				registryDeployment.Spec.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"component": constructBSLRegistryName(&bsl),
					},
				}
			}

			// update the Registry Deployment template
			err := r.buildRegistryDeployment(registryDeployment, &bsl)
			return err
		})

		if err != nil {
			return false, err
		}

		//TODO: Review registry deployment status and report errors and conditions

		if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
			// Trigger event to indicate registry deployment was created or updated
			r.EventRecorder.Event(registryDeployment,
				corev1.EventTypeNormal,
				"RegistryDeploymentReconciled",
				fmt.Sprintf("performed %s on registry deployment %s/%s", op, registryDeployment.Namespace, registryDeployment.Name),
			)
		}

	}

	return true, nil
}

// Construct and update the registry deployment for a bsl
func (r *VeleroReconciler) buildRegistryDeployment(registryDeployment *appsv1.Deployment, bsl *velerov1.BackupStorageLocation) error {

	// Setting controller owner reference on the registry deployment
	err := controllerutil.SetControllerReference(bsl, registryDeployment, r.Scheme)
	if err != nil {
		return err
	}

	registryDeployment.Labels = r.getRegistryBSLLabels(bsl)

	registryDeployment.Spec = appsv1.DeploymentSpec{
		Replicas: pointer.Int32(1),
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"component": constructBSLRegistryName(bsl),
				},
			},
			Spec: corev1.PodSpec{
				RestartPolicy: corev1.RestartPolicyAlways,
				Containers:    r.buildRegistryContainer(bsl),
			},
		},
	}
	return nil
}

func (r *VeleroReconciler) getRegistryBSLLabels(bsl *velerov1.BackupStorageLocation) map[string]string {
	labels := map[string]string{
		"app.kubernetes.io/name":       OADPOperatorVelero,
		"app.kubernetes.io/instance":   constructBSLRegistryName(bsl),
		"app.kubernetes.io/managed-by": OADPOperator,
		"app.kubernetes.io/component":  Registry,
	}
	return labels
}

func constructBSLRegistryName(bsl *velerov1.BackupStorageLocation) string {
	return "oadp-" + bsl.Name + "-" + bsl.Spec.Provider + "-registry"
}

func (r *VeleroReconciler) buildRegistryContainer(bsl *velerov1.BackupStorageLocation) []corev1.Container {
	containers := []corev1.Container{
		{
			Image: RegistryImage,
			Name:  constructBSLRegistryName(bsl) + "-container",
			Ports: []corev1.ContainerPort{
				{
					ContainerPort: 5000,
					Protocol:      corev1.ProtocolTCP,
				},
			},
			Env: r.getRegistryEnvVars(bsl),
			LivenessProbe: &corev1.Probe{
				Handler: corev1.Handler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/v2/_catalog?n=5",
						Port: intstr.IntOrString{IntVal: 5000},
					},
				},
				PeriodSeconds:       5,
				TimeoutSeconds:      3,
				InitialDelaySeconds: 15,
			},
			ReadinessProbe: &corev1.Probe{
				Handler: corev1.Handler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/v2/_catalog?n=5",
						Port: intstr.IntOrString{IntVal: 5000},
					},
				},
				PeriodSeconds:       5,
				TimeoutSeconds:      3,
				InitialDelaySeconds: 15,
			},
		},
	}

	return containers
}

func (r *VeleroReconciler) getRegistryEnvVars(bsl *velerov1.BackupStorageLocation) []corev1.EnvVar {
	envVar := []corev1.EnvVar{}
	provider := bsl.Spec.Provider
	switch provider {
	case AWSProvider:
		envVar = r.getAWSRegistryEnvVars(bsl, cloudProviderEnvVarMap[AWSProvider])

	case AzureProvider:
		envVar = r.getAzureRegistryEnvVars(bsl, cloudProviderEnvVarMap[AzureProvider])

	case GCPProvider:
		envVar = r.getGCPRegistryEnvVars(bsl, cloudProviderEnvVarMap[GCPProvider])
	}
	return envVar
}

func (r *VeleroReconciler) getAWSRegistryEnvVars(bsl *velerov1.BackupStorageLocation, awsEnvVars []corev1.EnvVar) []corev1.EnvVar {
	for i, _ := range awsEnvVars {
		//TODO: This needs to be fetched from the provider secret
		if awsEnvVars[i].Name == RegistryStorageS3AccesskeyEnvVarKey {
			awsEnvVars[i].Value = ""
		}

		if awsEnvVars[i].Name == RegistryStorageS3BucketEnvVarKey {
			awsEnvVars[i].Value = bsl.Spec.StorageType.ObjectStorage.Bucket
		}

		if awsEnvVars[i].Name == RegistryStorageS3RegionEnvVarKey {
			awsEnvVars[i].Value = bsl.Spec.Config[Region]
		}
		//TODO: This needs to be fetched from the provider secret
		if awsEnvVars[i].Name == RegistryStorageS3SecretkeyEnvVarKey {
			awsEnvVars[i].Value = ""
		}

		if awsEnvVars[i].Name == RegistryStorageS3RegionendpointEnvVarKey && bsl.Spec.Config[S3URL] != "" {
			awsEnvVars[i].Value = bsl.Spec.Config[S3URL]
		}

		if awsEnvVars[i].Name == RegistryStorageS3RootdirectoryEnvVarKey && bsl.Spec.Config[RootDirectory] != "" {
			awsEnvVars[i].Value = bsl.Spec.Config[RootDirectory]
		}

		if awsEnvVars[i].Name == RegistryStorageS3SkipverifyEnvVarKey && bsl.Spec.Config[InsecureSkipTLSVerify] != "" {
			awsEnvVars[i].Value = bsl.Spec.Config[InsecureSkipTLSVerify]
		}
	}
	return awsEnvVars
}

func (r *VeleroReconciler) getAzureRegistryEnvVars(bsl *velerov1.BackupStorageLocation, azureEnvVars []corev1.EnvVar) []corev1.EnvVar {
	for i, _ := range azureEnvVars {
		if azureEnvVars[i].Name == RegistryStorageAzureContainerEnvVarKey {
			azureEnvVars[i].Value = bsl.Spec.StorageType.ObjectStorage.Bucket
		}

		if azureEnvVars[i].Name == RegistryStorageAzureAccountnameEnvVarKey {
			azureEnvVars[i].Value = bsl.Spec.Config[StorageAccount]
		}
		//TODO: This needs to be fetched from the provider secret
		if azureEnvVars[i].Name == RegistryStorageAzureAccountkeyEnvVarKey {
			azureEnvVars[i].Value = ""
		}
	}
	return azureEnvVars
}

func (r *VeleroReconciler) getGCPRegistryEnvVars(bsl *velerov1.BackupStorageLocation, gcpEnvVars []corev1.EnvVar) []corev1.EnvVar {
	for i, _ := range gcpEnvVars {
		if gcpEnvVars[i].Name == RegistryStorageGCSBucket {
			gcpEnvVars[i].Value = bsl.Spec.StorageType.ObjectStorage.Bucket
		}
		//TODO: This needs to be fetched from the provider secret
		if gcpEnvVars[i].Name == RegistryStorageGCSKeyfile {
			gcpEnvVars[i].Value = ""
		}
		if gcpEnvVars[i].Name == RegistryStorageGCSRootdirectory && bsl.Spec.Config[RootDirectory] != "" {
			gcpEnvVars[i].Value = bsl.Spec.Config[RootDirectory]
		}
	}
	return gcpEnvVars
}
