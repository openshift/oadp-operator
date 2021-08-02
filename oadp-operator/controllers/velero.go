package controllers

import (
	"context"
	"github.com/go-logr/logr"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"reflect"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/utils/pointer"
)

const (
	Velero          = "velero"
	VeleroNamespace = "oadp-operator"
	//TODO: Check for default secret names
	VELERO_AWS_SECRET_NAME   = "cloud-credentials"
	VELERO_AZURE_SECRET_NAME = "cloud-credentials-azure"
	VELERO_GCP_SECRET_NAME   = "cloud-credentials-gcp"
)

// Environment Vars
const (
	LD_LIBRARY_PATH               = "LD_LIBRARY_PATH"
	VELERO_NAMESPACE              = "VELERO_NAMESPACE"
	VELERO_SCRATCH_DIR            = "VELERO_SCRATCH_DIR"
	AWS_SHARED_CREDENTIALS_FILE   = "AWS_SHARED_CREDENTIALS_FILE"
	AZURE_SHARED_CREDENTIALS_FILE = "AZURE_SHARED_CREDENTIALS_FILE"
	GCP_SHARED_CREDENTIALS_FILE   = "GCP_SHARED_CREDENTIALS_FILE"
	//TODO: Add Proxy env vars
	HTTP_PROXY  = "HTTP_PROXY"
	HTTPS_PROXY = "HTTPS_PROXY"
	NO_PROXY    = "NO_PROXY"
)

//TODO: Add Image customization options
// Images
const (
	VELERO_IMAGE           = "quay.io/konveyor/velero:latest"
	OPENSHIFT_PLUGIN_IMAGE = "quay.io/konveyor/openshift-velero-plugins:latest"
	AWS_PLUGIN_IMAGE       = "quay.io/konveyor/velero-plugin-for-aws:latest"
	AZURE_PLUGIN_IMAGE     = "quay.io/konveyor/velero-plugin-for-microsoft-azure:latest"
	GCP_PLUGIN_IMAGE       = "quay.io/konveyor/velero-plugin-for-gcp:latest"
	CSI_PLUGIN_IMAGE       = "quay.io/konveyor/velero-plugin-for-csi:latest"
)

// Plugin names
const (
	VELERO_PLUGIN_FOR_AWS       = "velero-plugin-for-aws"
	VELERO_PLUGIN_FOR_AZURE     = "velero-plugin-for-microsoft-azure"
	VELERO_PLUGIN_FOR_GCP       = "velero-plugin-for-gcp"
	VELERO_PLUGIN_FOR_CSI       = "velero-plugin-for-csi"
	VELERO_PLUGIN_FOR_OPENSHIFT = "openshift-velero-plugin"
)

func (r *VeleroReconciler) ReconcileVeleroDeployment(log logr.Logger) (bool, error) {
	velero := oadpv1alpha1.Velero{}
	if err := r.Get(r.Context, r.NamespacedName, &velero); err != nil {
		return false, err
	}

	veleroVolumeMounts := r.getVeleroVolumeMounts(&velero)
	veleroVolumes := r.getVeleroVolumes(&velero)
	veleroEnv := r.getVeleroEnv(&velero)
	veleroInitContainers := r.getVeleroInitContainers(&velero)
	veleroResourceReqs := r.getVeleroResourceReqs(&velero)
	veleroTolerations := velero.Spec.VeleroTolerations

	// Build Velero Deployment
	newVeleroDeployment := r.buildVeleroDeployment(veleroVolumeMounts, veleroVolumes, veleroEnv, veleroInitContainers, veleroResourceReqs, veleroTolerations)

	// Get existing Velero Deployment if any
	existingVeleroDeployment, err := GetVeleroDeployment(r.Client)

	if err != nil {
		return false, err
	}

	// If no Velero Deployment exists then create one
	if existingVeleroDeployment == nil {
		err = r.Client.Create(context.TODO(), newVeleroDeployment)
		if err != nil {
			return false, err
		}
		return true, nil
	}

	// If Velero Deployment exists check if it meets the requirements specified
	if r.EqualsDeployment(existingVeleroDeployment, newVeleroDeployment) {
		return true, nil
	}

	// Update the Velero Deployment if it already exists
	err = r.Client.Update(context.TODO(), existingVeleroDeployment)
	if err != nil {
		return false, err
	}

	return true, nil
}

func GetVeleroDeployment(client k8sclient.Client) (*appsv1.Deployment, error) {
	deploymentList := appsv1.DeploymentList{}
	labels := map[string]string{
		"component": Velero,
	}

	err := client.List(context.TODO(), &deploymentList, k8sclient.MatchingLabels(labels))

	if err != nil {
		return nil, err
	}

	if len(deploymentList.Items) > 0 {
		return &deploymentList.Items[0], nil
	}

	return nil, nil
}

// Checks if two Deployments are equal
func (r *VeleroReconciler) EqualsDeployment(a, b *appsv1.Deployment) bool {
	if !(reflect.DeepEqual(a.Spec.Replicas, b.Spec.Replicas) &&
		reflect.DeepEqual(a.Spec.Selector, b.Spec.Selector) &&
		reflect.DeepEqual(a.Spec.Template.ObjectMeta, b.Spec.Template.ObjectMeta) &&
		reflect.DeepEqual(a.Spec.Template.Spec.Volumes, b.Spec.Template.Spec.Volumes) &&
		len(a.Spec.Template.Spec.Containers) == len(b.Spec.Template.Spec.Containers)) {
		return false
	}
	for i, container := range a.Spec.Template.Spec.Containers {
		if !(reflect.DeepEqual(container.Env, b.Spec.Template.Spec.Containers[i].Env) &&
			reflect.DeepEqual(container.Name, b.Spec.Template.Spec.Containers[i].Name) &&
			reflect.DeepEqual(container.Ports, b.Spec.Template.Spec.Containers[i].Ports) &&
			reflect.DeepEqual(container.LivenessProbe, b.Spec.Template.Spec.Containers[i].LivenessProbe) &&
			reflect.DeepEqual(container.ReadinessProbe, b.Spec.Template.Spec.Containers[i].ReadinessProbe) &&
			reflect.DeepEqual(container.TerminationMessagePolicy, b.Spec.Template.Spec.Containers[i].TerminationMessagePolicy) &&
			reflect.DeepEqual(container.Image, b.Spec.Template.Spec.Containers[i].Image) &&
			reflect.DeepEqual(container.VolumeMounts, b.Spec.Template.Spec.Containers[i].VolumeMounts)) {
			return false
		}
	}
	return true
}

// Build Velero Deployment
func (r *VeleroReconciler) buildVeleroDeployment(veleroVolumeMounts []corev1.VolumeMount, veleroVolumes []corev1.Volume, veleroEnv []corev1.EnvVar, veleroInitContainers []corev1.Container, veleroResourceReqs corev1.ResourceRequirements, veleroTolerations []corev1.Toleration) *appsv1.Deployment {

	deployment := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      Velero,
			Namespace: VeleroNamespace,
		},
		Spec: appsv1.DeploymentSpec{
			//TODO: add velero nodeselector, needs to be added to the Velero CR first
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"component": Velero,
				},
			},
			Replicas: pointer.Int32(1),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"component": Velero,
					},
					Annotations: map[string]string{
						"prometheus.io/scrape": "true",
						"prometheus.io/port":   "8085",
						"prometheus.io/path":   "/metrics",
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy:      corev1.RestartPolicyAlways,
					ServiceAccountName: Velero,
					Tolerations:        veleroTolerations,
					Containers: []corev1.Container{
						{
							Name:  Velero,
							Image: VELERO_IMAGE,
							//TODO: Make the image policy parametrized
							ImagePullPolicy: corev1.PullAlways,
							Ports: []corev1.ContainerPort{
								{
									Name:          "metrics",
									ContainerPort: 8085,
								},
							},
							Resources: veleroResourceReqs,
							Command:   []string{"/velero"},
							//TODO: Parametrize restic timeout, Features flag as well as Velero debug flag
							Args:         []string{"server", "--restic-timeout", "1h"},
							VolumeMounts: veleroVolumeMounts,
							Env:          veleroEnv,
						},
					},
					Volumes:        veleroVolumes,
					InitContainers: veleroInitContainers,
				},
			},
		},
	}
	return &deployment
}

// Get Velero Resource Requirements
func (r *VeleroReconciler) getVeleroResourceReqs(velero *oadpv1alpha1.Velero) corev1.ResourceRequirements {

	ResourcesReqs := corev1.ResourceRequirements{}
	ResourceReqsLimits := corev1.ResourceList{}
	ResourceReqsRequests := corev1.ResourceList{}

	if velero != nil {

		// Set custom limits and requests values if defined on Velero Spec
		if velero.Spec.VeleroResourceAllocations.Requests != nil {
			ResourceReqsRequests[corev1.ResourceCPU] = resource.MustParse(velero.Spec.VeleroResourceAllocations.Requests.Cpu().String())
			ResourceReqsRequests[corev1.ResourceMemory] = resource.MustParse(velero.Spec.VeleroResourceAllocations.Requests.Memory().String())
		}

		if velero.Spec.VeleroResourceAllocations.Limits != nil {
			ResourceReqsLimits[corev1.ResourceCPU] = resource.MustParse(velero.Spec.VeleroResourceAllocations.Limits.Cpu().String())
			ResourceReqsLimits[corev1.ResourceMemory] = resource.MustParse(velero.Spec.VeleroResourceAllocations.Limits.Memory().String())
		}
		ResourcesReqs.Requests = ResourceReqsRequests
		ResourcesReqs.Limits = ResourceReqsLimits

		return ResourcesReqs

	}

	// Set default values
	ResourcesReqs = corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		},
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		},
	}
	return ResourcesReqs
}

func (r *VeleroReconciler) getVeleroVolumeMounts(velero *oadpv1alpha1.Velero) []corev1.VolumeMount {

	defaultVeleroPluginsList := velero.Spec.DefaultVeleroPlugins
	awsPluginVolumeMount := corev1.VolumeMount{
		Name:      VELERO_AWS_SECRET_NAME,
		MountPath: "/credentials",
	}
	azurePluginVolumeMount := corev1.VolumeMount{
		Name:      VELERO_AZURE_SECRET_NAME,
		MountPath: "/credentials-azure",
	}
	gcpPluginVolumeMount := corev1.VolumeMount{
		Name:      VELERO_GCP_SECRET_NAME,
		MountPath: "/credentials-gcp",
	}

	// add default volumemounts
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "plugins",
			MountPath: "/plugins",
		},
		{
			Name:      "scratch",
			MountPath: "/scratch",
		},
		{
			Name:      "certs",
			MountPath: "/etc/ssl/certs",
		},
	}
	// add default plugin based volumemounts
	if defaultVeleroPluginsList != nil {
		for _, plugin := range defaultVeleroPluginsList {
			if plugin == oadpv1alpha1.DefaultPluginAWS {
				volumeMounts = append(volumeMounts, awsPluginVolumeMount)
			}
			if plugin == oadpv1alpha1.DefaultPluginMicrosoftAzure {
				volumeMounts = append(volumeMounts, azurePluginVolumeMount)
			}
			if plugin == oadpv1alpha1.DefaultPluginGCP {
				volumeMounts = append(volumeMounts, gcpPluginVolumeMount)
			}
		}
	}
	return volumeMounts
}

func (r *VeleroReconciler) getVeleroEnv(velero *oadpv1alpha1.Velero) []corev1.EnvVar {

	// add default Env vars
	envVars := []corev1.EnvVar{
		{
			Name:  LD_LIBRARY_PATH,
			Value: "/plugins",
		},
		{
			Name:  VELERO_NAMESPACE,
			Value: VeleroNamespace,
		},
		{
			Name:  VELERO_SCRATCH_DIR,
			Value: "/scratch",
		},
		//TODO: Add the PROXY VARS
	}

	awsPluginEnvVar := corev1.EnvVar{
		Name:  AWS_SHARED_CREDENTIALS_FILE,
		Value: "/credentials/cloud",
	}
	azurePluginEnvVar := corev1.EnvVar{
		Name:  AZURE_SHARED_CREDENTIALS_FILE,
		Value: "/credentials-azure/cloud",
	}
	gcpPluginEnvVar := corev1.EnvVar{
		Name:  GCP_SHARED_CREDENTIALS_FILE,
		Value: "/credentials-gcp/cloud",
	}

	// add default plugin based Env vars
	defaultVeleroPluginsList := velero.Spec.DefaultVeleroPlugins

	if defaultVeleroPluginsList != nil {
		for _, plugin := range defaultVeleroPluginsList {
			if plugin == oadpv1alpha1.DefaultPluginAWS {
				envVars = append(envVars, awsPluginEnvVar)
			}
			if plugin == oadpv1alpha1.DefaultPluginMicrosoftAzure {
				envVars = append(envVars, azurePluginEnvVar)
			}
			if plugin == oadpv1alpha1.DefaultPluginGCP {
				envVars = append(envVars, gcpPluginEnvVar)
			}
		}
	}

	return envVars
}

func (r *VeleroReconciler) getVeleroVolumes(velero *oadpv1alpha1.Velero) []corev1.Volume {
	// add default volumes
	volumes := []corev1.Volume{
		{
			Name: "plugins",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: "scratch",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: "certs",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}

	// add default plugin based volumes
	awsPluginVolume := corev1.Volume{
		Name: VELERO_AWS_SECRET_NAME,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: VELERO_AWS_SECRET_NAME,
			},
		},
	}
	azurePluginVolume := corev1.Volume{
		Name: VELERO_AZURE_SECRET_NAME,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: VELERO_AZURE_SECRET_NAME,
			},
		},
	}
	gcpPluginVolume := corev1.Volume{
		Name: VELERO_GCP_SECRET_NAME,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: VELERO_GCP_SECRET_NAME,
			},
		},
	}

	defaultVeleroPluginsList := velero.Spec.DefaultVeleroPlugins

	if defaultVeleroPluginsList != nil {
		for _, plugin := range defaultVeleroPluginsList {
			if plugin == oadpv1alpha1.DefaultPluginAWS {
				volumes = append(volumes, awsPluginVolume)
			}
			if plugin == oadpv1alpha1.DefaultPluginMicrosoftAzure {
				volumes = append(volumes, azurePluginVolume)
			}
			if plugin == oadpv1alpha1.DefaultPluginGCP {
				volumes = append(volumes, gcpPluginVolume)
			}
		}
	}

	return volumes

}

func (r *VeleroReconciler) getVeleroInitContainers(velero *oadpv1alpha1.Velero) []corev1.Container {

	// add default volumemounts
	volumeMounts := []corev1.VolumeMount{
		{
			MountPath: "/certs",
			Name:      "certs",
		},
	}

	defaultVeleroPluginsList := velero.Spec.DefaultVeleroPlugins

	//TODO: Check why this is only done for AWS and not for other providers
	if defaultVeleroPluginsList != nil {
		for _, plugin := range defaultVeleroPluginsList {
			if plugin == oadpv1alpha1.DefaultPluginAWS {
				volumeMounts = append(volumeMounts, corev1.VolumeMount{
					MountPath: "/credentials",
					Name:      VELERO_AWS_SECRET_NAME,
				})
			}
		}
	}

	// add default initcontainers
	initContainers := []corev1.Container{
		{
			//TODO: Check this image as well as pull policy
			Image:                    VELERO_IMAGE,
			ImagePullPolicy:          corev1.PullAlways,
			Name:                     "setup-certificate-secret",
			Command:                  []string{"sh", "'-ec'", "cp /etc/ssl/certs/* /certs/; ln -sf /credentials/ca_bundle.pem /certs/ca_bundle.pem;"},
			Resources:                corev1.ResourceRequirements{},
			TerminationMessagePath:   "/dev/termination-log",
			TerminationMessagePolicy: "File",
			VolumeMounts:             volumeMounts,
		},
	}

	// add default plugin based initcontainers
	if defaultVeleroPluginsList != nil {
		for _, plugin := range defaultVeleroPluginsList {
			if plugin == oadpv1alpha1.DefaultPluginAWS {
				awsInitContainer := buildPluginInitContainer(VELERO_PLUGIN_FOR_AWS, AWS_PLUGIN_IMAGE, corev1.PullAlways)
				initContainers = append(initContainers, awsInitContainer)
			}
			if plugin == oadpv1alpha1.DefaultPluginMicrosoftAzure {
				azureInitContainer := buildPluginInitContainer(VELERO_PLUGIN_FOR_AZURE, AZURE_PLUGIN_IMAGE, corev1.PullAlways)
				initContainers = append(initContainers, azureInitContainer)
			}
			if plugin == oadpv1alpha1.DefaultPluginGCP {
				gcpInitContainer := buildPluginInitContainer(VELERO_PLUGIN_FOR_GCP, GCP_PLUGIN_IMAGE, corev1.PullAlways)
				initContainers = append(initContainers, gcpInitContainer)
			}
			if plugin == oadpv1alpha1.DefaultPluginCSI {
				csiInitContainer := buildPluginInitContainer(VELERO_PLUGIN_FOR_CSI, CSI_PLUGIN_IMAGE, corev1.PullAlways)
				initContainers = append(initContainers, csiInitContainer)
			}
			if plugin == oadpv1alpha1.DefaultPluginOpenShift {
				openshiftInitContainer := buildPluginInitContainer(VELERO_PLUGIN_FOR_OPENSHIFT, OPENSHIFT_PLUGIN_IMAGE, corev1.PullAlways)
				initContainers = append(initContainers, openshiftInitContainer)
			}
			//TODO: check if vsphere is needed
		}
	}

	//add custom plugin based initcontainers
	customVeleroPluginList := velero.Spec.CustomVeleroPlugins

	if customVeleroPluginList != nil {
		for _, customPlugin := range customVeleroPluginList {
			customPluginInitContainer := buildPluginInitContainer(customPlugin.Name, customPlugin.Image, corev1.PullAlways)
			initContainers = append(initContainers, customPluginInitContainer)
		}
	}

	return initContainers
}

func buildPluginInitContainer(initContainerName string, initContainerImage string, imagePullPolicy corev1.PullPolicy) corev1.Container {
	initContainer := corev1.Container{
		Image:                    initContainerImage,
		Name:                     initContainerName,
		ImagePullPolicy:          imagePullPolicy,
		Resources:                corev1.ResourceRequirements{},
		TerminationMessagePath:   "/dev/termination-log",
		TerminationMessagePolicy: "File",
		VolumeMounts: []corev1.VolumeMount{
			{
				MountPath: "/target",
				Name:      "plugins",
			},
		},
	}

	return initContainer
}
