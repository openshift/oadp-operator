package controller

import (
	"fmt"
	"os"

	"github.com/go-logr/logr"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/common"
)

const (
	kubevirtAnnotationsRemoverName        = "kubevirt-velero-annotations-remover"
	kubevirtAnnotationsRemoverTLSSecret   = "kubevirt-annotations-remover-tls"
	kubevirtAnnotationsRemoverWebhookPort = int32(8443)
)

var (
	kubevirtAnnotationsRemoverAppLabel = map[string]string{
		"app": kubevirtAnnotationsRemoverName,
	}
	kubevirtAnnotationsRemoverDeploymentLabels = map[string]string{
		"app":                          kubevirtAnnotationsRemoverName,
		"app.kubernetes.io/component":  "webhook",
		"app.kubernetes.io/created-by": common.OADPOperator,
		"app.kubernetes.io/instance":   kubevirtAnnotationsRemoverName,
		"app.kubernetes.io/managed-by": common.OADPOperator,
		"app.kubernetes.io/name":       kubevirtAnnotationsRemoverName,
		"app.kubernetes.io/part-of":    common.OADPOperator,
	}
)

//+kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=mutatingwebhookconfigurations,verbs=get;list;watch;create;update;patch;delete

// ReconcileKubevirtAnnotationsRemover manages the kubevirt-velero-annotations-remover
// webhook deployment, service, and MutatingWebhookConfiguration.
func (r *DataProtectionApplicationReconciler) ReconcileKubevirtAnnotationsRemover(log logr.Logger) (bool, error) {
	if !r.checkKubevirtAnnotationsRemoverEnabled() {
		return r.cleanupKubevirtAnnotationsRemover()
	}

	// Reconcile Service first (triggers TLS secret creation via OpenShift Service CA)
	if err := r.reconcileKubevirtAnnotationsRemoverService(); err != nil {
		return false, err
	}

	// Reconcile Deployment
	if err := r.reconcileKubevirtAnnotationsRemoverDeployment(); err != nil {
		return false, err
	}

	// Reconcile MutatingWebhookConfiguration (cluster-scoped)
	if err := r.reconcileKubevirtAnnotationsRemoverWebhook(); err != nil {
		return false, err
	}

	return true, nil
}

// cleanupKubevirtAnnotationsRemover removes all resources when the feature is disabled.
func (r *DataProtectionApplicationReconciler) cleanupKubevirtAnnotationsRemover() (bool, error) {
	// Delete MutatingWebhookConfiguration (cluster-scoped, must be deleted by name)
	webhookConfig := &admissionregistrationv1.MutatingWebhookConfiguration{}
	if err := r.Get(
		r.Context,
		types.NamespacedName{Name: kubevirtAnnotationsRemoverName},
		webhookConfig,
	); err != nil {
		if !k8serror.IsNotFound(err) {
			return false, err
		}
	} else {
		if err := r.Delete(r.Context, webhookConfig); err != nil && !k8serror.IsNotFound(err) {
			r.EventRecorder.Event(
				webhookConfig,
				corev1.EventTypeWarning,
				"KubevirtAnnotationsRemoverWebhookDeleteFailed",
				fmt.Sprintf("Could not delete MutatingWebhookConfiguration %s: %s", kubevirtAnnotationsRemoverName, err),
			)
			return false, err
		}
		r.EventRecorder.Event(
			webhookConfig,
			corev1.EventTypeNormal,
			"KubevirtAnnotationsRemoverWebhookDeleteSucceed",
			fmt.Sprintf("MutatingWebhookConfiguration %s deleted", kubevirtAnnotationsRemoverName),
		)
	}

	// Delete Deployment (namespace-scoped)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubevirtAnnotationsRemoverName,
			Namespace: r.NamespacedName.Namespace,
		},
	}
	if err := r.Get(
		r.Context,
		types.NamespacedName{
			Name:      deployment.Name,
			Namespace: deployment.Namespace,
		},
		deployment,
	); err != nil {
		if !k8serror.IsNotFound(err) {
			return false, err
		}
	} else {
		if err := r.Delete(
			r.Context,
			deployment,
			&client.DeleteOptions{PropagationPolicy: ptr.To(metav1.DeletePropagationForeground)},
		); err != nil && !k8serror.IsNotFound(err) {
			r.EventRecorder.Event(
				deployment,
				corev1.EventTypeWarning,
				"KubevirtAnnotationsRemoverDeploymentDeleteFailed",
				fmt.Sprintf("Could not delete deployment %s/%s: %s", deployment.Namespace, deployment.Name, err),
			)
			return false, err
		}
		r.EventRecorder.Event(
			deployment,
			corev1.EventTypeNormal,
			"KubevirtAnnotationsRemoverDeploymentDeleteSucceed",
			fmt.Sprintf("Deployment %s/%s deleted", deployment.Namespace, deployment.Name),
		)
	}

	// Delete Service (namespace-scoped)
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubevirtAnnotationsRemoverName,
			Namespace: r.NamespacedName.Namespace,
		},
	}
	if err := r.Get(
		r.Context,
		types.NamespacedName{
			Name:      svc.Name,
			Namespace: svc.Namespace,
		},
		svc,
	); err != nil {
		if !k8serror.IsNotFound(err) {
			return false, err
		}
	} else {
		if err := r.Delete(r.Context, svc); err != nil && !k8serror.IsNotFound(err) {
			r.EventRecorder.Event(
				svc,
				corev1.EventTypeWarning,
				"KubevirtAnnotationsRemoverServiceDeleteFailed",
				fmt.Sprintf("Could not delete service %s/%s: %s", svc.Namespace, svc.Name, err),
			)
			return false, err
		}
		r.EventRecorder.Event(
			svc,
			corev1.EventTypeNormal,
			"KubevirtAnnotationsRemoverServiceDeleteSucceed",
			fmt.Sprintf("Service %s/%s deleted", svc.Namespace, svc.Name),
		)
	}

	return true, nil
}

// reconcileKubevirtAnnotationsRemoverService creates or updates the Service
// with OpenShift Service CA annotation for TLS certificate generation.
func (r *DataProtectionApplicationReconciler) reconcileKubevirtAnnotationsRemoverService() error {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubevirtAnnotationsRemoverName,
			Namespace: r.NamespacedName.Namespace,
		},
	}

	op, err := controllerutil.CreateOrPatch(r.Context, r.Client, svc, func() error {
		// Set annotations for OpenShift Service CA TLS certificate generation
		if svc.Annotations == nil {
			svc.Annotations = make(map[string]string)
		}
		svc.Annotations["service.beta.openshift.io/serving-cert-secret-name"] = kubevirtAnnotationsRemoverTLSSecret

		// Set labels
		if svc.Labels == nil {
			svc.Labels = make(map[string]string)
		}
		for k, v := range kubevirtAnnotationsRemoverAppLabel {
			svc.Labels[k] = v
		}

		// Set spec
		svc.Spec.Selector = kubevirtAnnotationsRemoverAppLabel
		svc.Spec.Type = corev1.ServiceTypeClusterIP
		svc.Spec.Ports = []corev1.ServicePort{
			{
				Protocol:   corev1.ProtocolTCP,
				Port:       443,
				TargetPort: intstr.FromInt32(kubevirtAnnotationsRemoverWebhookPort),
			},
		}

		// Apply user-provided resource labels and annotations
		svc.Labels = applyResourceLabels(r.dpa, svc.Labels)
		svc.Annotations = applyResourceAnnotations(r.dpa, svc.Annotations)

		return controllerutil.SetControllerReference(r.dpa, svc, r.Scheme)
	})
	if err != nil {
		return err
	}

	if op != controllerutil.OperationResultNone {
		r.EventRecorder.Event(
			svc,
			corev1.EventTypeNormal,
			"KubevirtAnnotationsRemoverServiceReconciled",
			fmt.Sprintf("Service %s/%s %s", svc.Namespace, svc.Name, op),
		)
	}

	return nil
}

// reconcileKubevirtAnnotationsRemoverDeployment creates or updates the webhook Deployment.
func (r *DataProtectionApplicationReconciler) reconcileKubevirtAnnotationsRemoverDeployment() error {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubevirtAnnotationsRemoverName,
			Namespace: r.NamespacedName.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, deployment, func() error {
		if err := r.buildKubevirtAnnotationsRemoverDeployment(deployment); err != nil {
			return err
		}
		return controllerutil.SetControllerReference(r.dpa, deployment, r.Scheme)
	})
	if err != nil {
		return err
	}

	if op != controllerutil.OperationResultNone {
		r.EventRecorder.Event(
			deployment,
			corev1.EventTypeNormal,
			"KubevirtAnnotationsRemoverDeploymentReconciled",
			fmt.Sprintf("Deployment %s/%s %s", deployment.Namespace, deployment.Name, op),
		)
	}

	return nil
}

// buildKubevirtAnnotationsRemoverDeployment populates the Deployment spec.
func (r *DataProtectionApplicationReconciler) buildKubevirtAnnotationsRemoverDeployment(deploymentObject *appsv1.Deployment) error {
	image := r.getKubevirtAnnotationsRemoverImage()
	imagePullPolicy, err := common.GetImagePullPolicy(r.dpa.Spec.ImagePullPolicy, image)
	if err != nil {
		r.Log.Error(err, "imagePullPolicy regex failed")
	}

	// Set labels
	deploymentObjectLabels := deploymentObject.GetLabels()
	if deploymentObjectLabels == nil {
		deploymentObject.SetLabels(make(map[string]string))
		deploymentObjectLabels = deploymentObject.GetLabels()
	}
	for k, v := range kubevirtAnnotationsRemoverDeploymentLabels {
		deploymentObjectLabels[k] = v
	}
	deploymentObject.SetLabels(deploymentObjectLabels)

	// Set spec
	deploymentObject.Spec.Replicas = ptr.To(int32(1))
	deploymentObject.Spec.Selector = &metav1.LabelSelector{
		MatchLabels: kubevirtAnnotationsRemoverAppLabel,
	}

	// Template labels
	templateLabels := deploymentObject.Spec.Template.GetLabels()
	if templateLabels == nil {
		templateLabels = make(map[string]string)
	}
	for k, v := range kubevirtAnnotationsRemoverAppLabel {
		templateLabels[k] = v
	}
	deploymentObject.Spec.Template.SetLabels(templateLabels)

	// Pod security context
	if deploymentObject.Spec.Template.Spec.SecurityContext == nil {
		deploymentObject.Spec.Template.Spec.SecurityContext = &corev1.PodSecurityContext{
			RunAsNonRoot: ptr.To(true),
			RunAsUser:    ptr.To(int64(1001)),
			FSGroup:      ptr.To(int64(1001)),
			SeccompProfile: &corev1.SeccompProfile{
				Type: corev1.SeccompProfileTypeRuntimeDefault,
			},
		}
	}

	// Container spec
	containerSpec := corev1.Container{
		Name:            "webhook",
		Image:           image,
		ImagePullPolicy: imagePullPolicy,
		Ports: []corev1.ContainerPort{
			{
				Name:          "https",
				ContainerPort: kubevirtAnnotationsRemoverWebhookPort,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "tls",
				MountPath: "/tls",
				ReadOnly:  true,
			},
		},
		SecurityContext: &corev1.SecurityContext{
			RunAsNonRoot:             ptr.To(true),
			AllowPrivilegeEscalation: ptr.To(false),
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
		},
		TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
	}

	webhookContainerFound := false
	if len(deploymentObject.Spec.Template.Spec.Containers) == 0 {
		deploymentObject.Spec.Template.Spec.Containers = []corev1.Container{containerSpec}
		webhookContainerFound = true
	} else {
		for index, container := range deploymentObject.Spec.Template.Spec.Containers {
			if container.Name == "webhook" {
				c := &deploymentObject.Spec.Template.Spec.Containers[index]
				c.Image = image
				c.ImagePullPolicy = imagePullPolicy
				c.Ports = containerSpec.Ports
				c.VolumeMounts = containerSpec.VolumeMounts
				c.SecurityContext = containerSpec.SecurityContext
				c.TerminationMessagePolicy = containerSpec.TerminationMessagePolicy
				webhookContainerFound = true
				break
			}
		}
	}
	if !webhookContainerFound {
		return fmt.Errorf("could not find webhook container in kubevirt-velero-annotations-remover Deployment")
	}

	// Volumes
	deploymentObject.Spec.Template.Spec.Volumes = []corev1.Volume{
		{
			Name: "tls",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: kubevirtAnnotationsRemoverTLSSecret,
				},
			},
		},
	}

	deploymentObject.Spec.Template.Spec.RestartPolicy = corev1.RestartPolicyAlways
	deploymentObject.Spec.Template.Spec.ServiceAccountName = common.Velero

	// Apply user-provided resource labels (protected labels are filtered)
	deploymentObject.Labels = applyResourceLabels(r.dpa, deploymentObject.Labels)
	deploymentObject.Spec.Template.Labels = applyResourceLabels(r.dpa, deploymentObject.Spec.Template.Labels)

	// Re-assert selector labels to ensure template labels match selector
	if deploymentObject.Spec.Selector != nil && deploymentObject.Spec.Selector.MatchLabels != nil {
		if deploymentObject.Spec.Template.Labels == nil {
			deploymentObject.Spec.Template.Labels = make(map[string]string)
		}
		for k, v := range deploymentObject.Spec.Selector.MatchLabels {
			deploymentObject.Spec.Template.Labels[k] = v
		}
	}

	// Apply user-provided resource annotations
	deploymentObject.Annotations = applyResourceAnnotations(r.dpa, deploymentObject.Annotations)
	deploymentObject.Spec.Template.Annotations = applyResourceAnnotations(r.dpa, deploymentObject.Spec.Template.Annotations)

	return nil
}

// reconcileKubevirtAnnotationsRemoverWebhook creates or updates the MutatingWebhookConfiguration.
// This is a cluster-scoped resource, so it cannot have an owner reference to the namespace-scoped DPA.
// Instead, it is managed by label-based tracking with a fixed name.
func (r *DataProtectionApplicationReconciler) reconcileKubevirtAnnotationsRemoverWebhook() error {
	webhookConfig := &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: kubevirtAnnotationsRemoverName,
		},
	}

	failurePolicy := admissionregistrationv1.Ignore
	sideEffects := admissionregistrationv1.SideEffectClassNone
	webhookPath := "/mutate"

	op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, webhookConfig, func() error {
		// Set labels for tracking (cannot use owner reference for cluster-scoped resources)
		if webhookConfig.Labels == nil {
			webhookConfig.Labels = make(map[string]string)
		}
		webhookConfig.Labels[oadpv1alpha1.OadpOperatorLabel] = "True"
		webhookConfig.Labels["app.kubernetes.io/managed-by"] = common.OADPOperator
		webhookConfig.Labels["app.kubernetes.io/instance"] = kubevirtAnnotationsRemoverName

		// Set annotation for OpenShift Service CA bundle injection
		if webhookConfig.Annotations == nil {
			webhookConfig.Annotations = make(map[string]string)
		}
		webhookConfig.Annotations["service.beta.openshift.io/inject-cabundle"] = "true"

		webhookConfig.Webhooks = []admissionregistrationv1.MutatingWebhook{
			{
				Name:                    fmt.Sprintf("%s.%s.svc", kubevirtAnnotationsRemoverName, r.NamespacedName.Namespace),
				AdmissionReviewVersions: []string{"v1"},
				FailurePolicy:           &failurePolicy,
				SideEffects:             &sideEffects,
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					Service: &admissionregistrationv1.ServiceReference{
						Name:      kubevirtAnnotationsRemoverName,
						Namespace: r.NamespacedName.Namespace,
						Path:      &webhookPath,
						Port:      ptr.To(int32(443)),
					},
				},
				Rules: []admissionregistrationv1.RuleWithOperations{
					{
						Operations: []admissionregistrationv1.OperationType{
							admissionregistrationv1.Create,
							admissionregistrationv1.Update,
						},
						Rule: admissionregistrationv1.Rule{
							APIGroups:   []string{""},
							APIVersions: []string{"v1"},
							Resources:   []string{"pods"},
						},
					},
				},
				ObjectSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"kubevirt.io": "virt-launcher",
					},
				},
			},
		}

		return nil
	})
	if err != nil {
		return err
	}

	if op != controllerutil.OperationResultNone {
		r.EventRecorder.Event(
			webhookConfig,
			corev1.EventTypeNormal,
			"KubevirtAnnotationsRemoverWebhookReconciled",
			fmt.Sprintf("MutatingWebhookConfiguration %s %s", kubevirtAnnotationsRemoverName, op),
		)
	}

	return nil
}

// checkKubevirtAnnotationsRemoverEnabled returns true if the feature is enabled in the DPA spec.
func (r *DataProtectionApplicationReconciler) checkKubevirtAnnotationsRemoverEnabled() bool {
	if r.dpa.Spec.KubevirtAnnotationsRemover != nil && r.dpa.Spec.KubevirtAnnotationsRemover.Enable != nil {
		return *r.dpa.Spec.KubevirtAnnotationsRemover.Enable
	}
	return false
}

// getKubevirtAnnotationsRemoverImage returns the image to use for the webhook,
// following the standard UnsupportedOverrides -> RELATED_IMAGE -> default pattern.
func (r *DataProtectionApplicationReconciler) getKubevirtAnnotationsRemoverImage() string {
	dpa := r.dpa
	unsupportedOverride := dpa.Spec.UnsupportedOverrides[oadpv1alpha1.KubevirtAnnotationsRemoverImageKey]
	if unsupportedOverride != "" {
		return unsupportedOverride
	}

	environmentVariable := os.Getenv("RELATED_IMAGE_KUBEVIRT_ANNOTATIONS_REMOVER")
	if environmentVariable != "" {
		return environmentVariable
	}

	return "quay.io/migtools/kubevirt-velero-annotations-remover-go:latest"
}
