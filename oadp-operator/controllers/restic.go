package controllers

import (
	"fmt"
	"os"

	"github.com/go-logr/logr"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

/**
- name: "Enable restic"
  k8s:
    state: "{{ velero_state }}"
    definition: "{{ lookup('template', 'restic.yml.j2')}}"
  when: enable_restic == true
*/

const (
	veleroSAName     = "velero"
	resticPvHostPath = "/var/lib/kubelet/pods"
	cloudFieldPath   = "cloud"
)

// const mountPropagationMode = v1.MountPropagationMode
var (
	mountPropagationToHostContainer = v1.MountPropagationHostToContainer
	resticLabelMap                  = map[string]string{
		"name": "restic",
	}
	cloudProviderConst = map[oadpv1alpha1.DefaultPlugin]map[string]string{
		oadpv1alpha1.DefaultPluginAWS: {
			"secret-name":        "cloud-credentials",
			"mountPath":          "/credentials",
			"envCredentialsFile": "AWS_SHARED_CREDENTIALS_FILE",
		},
		oadpv1alpha1.DefaultPluginGCP: {
			"secret-name":        "cloud-credentials-gcp",
			"mountPath":          "/credentials-gcp",
			"envCredentialsFile": "GOOGLE_APPLICATION_CREDENTIALS",
		},
		oadpv1alpha1.DefaultPluginMicrosoftAzure: {
			"secret-name":        "cloud-credentials-azure",
			"mountPath":          "/credentials-azure",
			"envCredentialsFile": "AZURE_CREDENTIALS_FILE",
		},
	}
)

func (r *VeleroReconciler) ReconcileResticDaemonset(log logr.Logger) (bool, error) {
	velero := oadpv1alpha1.Velero{}
	if err := r.Get(r.Context, r.NamespacedName, &velero); err != nil {
		return false, err
	}

	// Define "static" portion of daemonset
	ds := r.buildResticDaemonset(&velero)

	if velero.Spec.EnableRestic != nil && !*velero.Spec.EnableRestic {
		// If velero Spec enableRestic exists and is false, attempt to delete.
		r.Delete(r.Context, &ds) //TODO: delete fail logic?
		return true, nil
	}

	// Check if Daemonset already exists
	//make a copy of ds to be used in r.Get
	existingDS := *ds.DeepCopy()
	if err := r.Get(r.Context, r.NamespacedName, &existingDS); err != nil {
		if errors.IsNotFound(err) { // Daemonset not found so create Daemonset
			if err := r.Create(r.Context, &ds); err != nil {
				return false, err
			}
		}
	} else {
		// Daemonset found, update it.
		if err := r.Update(r.Context, &ds); err != nil { // Update daemonset
			return false, err
		}
	}

	return true, nil
}

func (r *VeleroReconciler) buildResticDaemonset(velero *oadpv1alpha1.Velero) appsv1.DaemonSet {
	ds := appsv1.DaemonSet{

		ObjectMeta: metav1.ObjectMeta{
			Name:      resticLabelMap["name"],
			Namespace: velero.Namespace,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: resticLabelMap,
			},
			UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
				Type: appsv1.RollingUpdateDaemonSetStrategyType,
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: resticLabelMap,
				},
				Spec: v1.PodSpec{
					NodeSelector:       velero.Spec.ResticNodeSelector,
					ServiceAccountName: veleroSAName,
					SecurityContext: &v1.PodSecurityContext{
						RunAsUser:          pointer.Int64(0),
						SupplementalGroups: []int64{},
					},
					Volumes: []v1.Volume{
						// Cloud Provider volumes are dynamically added in the for loop below
						{
							Name: "host-pods",
							VolumeSource: v1.VolumeSource{
								HostPath: &v1.HostPathVolumeSource{
									Path: resticPvHostPath,
								},
							},
						},
						{
							Name: "scratch",
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{},
							},
						},
						{
							Name: "certs",
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{},
							},
						},
					},
					Tolerations: velero.Spec.ResticTolerations,
					Containers: []v1.Container{
						{
							Name: "velero",
							SecurityContext: &v1.SecurityContext{
								Privileged: pointer.Bool(true),
							},
							Image: fmt.Sprintf("%v/%v/%v:%v", os.Getenv("REGISTRY"), os.Getenv("PROJECT"), os.Getenv("VELERO_REPO"), os.Getenv("VELERO_TAG")),
							// velero_image_fqin: "{{ velero_image }}:{{ velero_version }}"
							// velero_image: "{{ registry }}/{{ project }}/{{ velero_repo }}"
							// velero_version: "{{ lookup( 'env', 'VELERO_TAG') }}"
							ImagePullPolicy: "Always",
							Resources:       getVeleroResourceReqs(velero), //setting default.
							Command: []string{
								"/velero",
							},
							Args: []string{
								"restic",
								"server",
							},
							VolumeMounts: []v1.VolumeMount{
								{
									Name:             "host-pods",
									MountPath:        "/host_pods",
									MountPropagation: &mountPropagationToHostContainer,
								},
								{
									Name:      "scratch",
									MountPath: "/scratch",
								},
								{
									Name:      "certs",
									MountPath: "/etc/ssl/certs",
								},
							},
							Env: []v1.EnvVar{
								{
									Name:  "HTTP_PROXY",
									Value: os.Getenv("HTTP_PROXY"),
								},
								{
									Name:  "HTTPS_PROXY",
									Value: os.Getenv("HTTPS_PROXY"),
								},
								{
									Name:  "NO_PROXY",
									Value: os.Getenv("NO_PROXY"),
								},
								{
									Name: "NODE_NAME",
									ValueFrom: &v1.EnvVarSource{
										FieldRef: &v1.ObjectFieldSelector{
											FieldPath: "spec.nodeName",
										},
									},
								},
								{
									Name: "POD_NAME",
									ValueFrom: &v1.EnvVarSource{
										FieldRef: &v1.ObjectFieldSelector{
											FieldPath: "metadata.name"},
									},
								},
								{
									Name: "VELERO_NAMESPACE",
									ValueFrom: &v1.EnvVarSource{
										FieldRef: &v1.ObjectFieldSelector{
											FieldPath: "metadata.namespace",
										},
									},
								},
								{
									Name: "VELERO_SCRATCH_DIR",
									ValueFrom: &v1.EnvVarSource{
										FieldRef: &v1.ObjectFieldSelector{
											FieldPath: "/scratch",
										},
									},
								},
							},
						},
					},
					InitContainers: []v1.Container{
						{
							Name: "setup-certificate-secret",
							Command: []string{
								"sh",
								"-ec",
								">-",
								"cp /etc/ssl/certs/* /certs/; ln -sf /credentials/ca_bundle.pem",
								"/certs/ca_bundle.pem;",
							},
							Resources:                v1.ResourceRequirements{},
							TerminationMessagePath:   "/dev/termination-log",
							TerminationMessagePolicy: v1.TerminationMessagePolicy("File"),
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "certs",
									MountPath: "/certs",
								},
								{
									Name:      string(oadpv1alpha1.DefaultPluginAWS),
									MountPath: "/credentials",
								},
							},
						},
					},
				},
			},
		},
	}
	// Dynamically add to daemonset definition
	// If the default velero plugins contains cloud provider, attach VolumeSource
	var veleroContainer *v1.Container
	for _, container := range ds.Spec.Template.Spec.Containers {
		if container.Name == "velero" {
			veleroContainer = &container
		}
	}
	for provider, cloudProviderMap := range cloudProviderConst {
		if contains(provider, velero.Spec.DefaultVeleroPlugins) {
			ds.Spec.Template.Spec.Volumes = append(
				ds.Spec.Template.Spec.Volumes,
				v1.Volume{
					Name: string(provider),
					VolumeSource: v1.VolumeSource{
						Secret: &v1.SecretVolumeSource{
							SecretName: cloudProviderMap["secret-name"],
						},
					},
				},
			)
			// Find Velero container
			veleroContainer.VolumeMounts = append(
				veleroContainer.VolumeMounts,
				v1.VolumeMount{
					Name:             cloudProviderMap["secret-name"],
					MountPath:        cloudProviderMap["mountPath"],
					MountPropagation: &mountPropagationToHostContainer,
				},
			)
			veleroContainer.Env = append(
				veleroContainer.Env,
				v1.EnvVar{
					Name:  cloudProviderMap["envCredentialsFile"],
					Value: cloudProviderMap["mountPath"] + "/" + cloudFieldPath,
				},
			)
		}
	}
	return ds
}

func getVeleroResourceReqs(velero *oadpv1alpha1.Velero) v1.ResourceRequirements {

	ResourcesReqs := v1.ResourceRequirements{}
	ResourceReqsLimits := v1.ResourceList{}
	ResourceReqsRequests := v1.ResourceList{}

	if velero != nil {

		// Set custom limits and requests values if defined on Velero Spec
		if velero.Spec.VeleroResourceAllocations.Requests != nil {
			ResourceReqsRequests[v1.ResourceCPU] = resource.MustParse(velero.Spec.VeleroResourceAllocations.Requests.Cpu().String())
			ResourceReqsRequests[v1.ResourceMemory] = resource.MustParse(velero.Spec.VeleroResourceAllocations.Requests.Memory().String())
		}

		if velero.Spec.VeleroResourceAllocations.Limits != nil {
			ResourceReqsLimits[v1.ResourceCPU] = resource.MustParse(velero.Spec.VeleroResourceAllocations.Limits.Cpu().String())
			ResourceReqsLimits[v1.ResourceMemory] = resource.MustParse(velero.Spec.VeleroResourceAllocations.Limits.Memory().String())
		}
		ResourcesReqs.Requests = ResourceReqsRequests
		ResourcesReqs.Limits = ResourceReqsLimits

		return ResourcesReqs

	}

	// Set default values
	ResourcesReqs = v1.ResourceRequirements{
		Limits: v1.ResourceList{
			v1.ResourceCPU:    resource.MustParse("1"),
			v1.ResourceMemory: resource.MustParse("512Mi"),
		},
		Requests: v1.ResourceList{
			v1.ResourceCPU:    resource.MustParse("500m"),
			v1.ResourceMemory: resource.MustParse("128Mi"),
		},
	}
	return ResourcesReqs
}

func contains(thisString oadpv1alpha1.DefaultPlugin, thisArray []oadpv1alpha1.DefaultPlugin) bool {
	for _, thisOne := range thisArray {
		if thisOne == thisString {
			return true
		}
	}
	return false
}
