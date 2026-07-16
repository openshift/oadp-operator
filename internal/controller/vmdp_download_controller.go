package controller

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/go-logr/logr"
	consolev1 "github.com/openshift/api/console/v1"
	routev1 "github.com/openshift/api/route/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	vmdpServerDeploymentName = "openshift-adp-oadp-vmdp-server"
	vmdpServerServiceName    = "openshift-adp-vmdp-server"
	vmdpServerRouteName      = "oadp-vmdp-server-route"
	vmdpDownloadName         = "openshift-adp-oadp-vmdp"
)

// VMDPDownloadSetup is a runnable that sets up VMDP download resources when the operator starts
type VMDPDownloadSetup struct {
	Client            client.Client
	Namespace         string
	OperatorName      string
	OperatorNamespace string
	Log               logr.Logger
}

// Start implements the Runnable interface
func (v *VMDPDownloadSetup) Start(ctx context.Context) error {
	v.Log = ctrl.Log.WithName("vmdp-download-setup")
	v.Log.Info("Starting VMDP download setup")

	// Get the VMDP server image from environment variable
	vmdpServerImage := os.Getenv("RELATED_IMAGE_VMDP_CLI_DOWNLOAD")
	if vmdpServerImage == "" {
		vmdpServerImage = "quay.io/konveyor/oadp-vmdp-binaries:oadp-1.6" // fallback default
		v.Log.Info("Using default VMDP server image", "image", vmdpServerImage)
	}

	// Get the operator deployment to use as owner for namespaced resources
	operatorDeployment := &appsv1.Deployment{}
	err := v.Client.Get(ctx, types.NamespacedName{
		Name:      v.OperatorName,
		Namespace: v.OperatorNamespace,
	}, operatorDeployment)
	if err != nil {
		v.Log.Error(err, "Failed to get operator deployment")
		return err
	}

	// Create VMDP resources (idempotent - will reuse existing ConsoleCLIDownload if present)
	if err := v.reconcileVMDPResources(ctx, operatorDeployment, vmdpServerImage); err != nil {
		return err
	}

	v.Log.Info("VMDP download setup completed successfully")
	return nil
}

// reconcileVMDPResources creates or updates all VMDP download-related resources
func (v *VMDPDownloadSetup) reconcileVMDPResources(ctx context.Context, operatorDeployment *appsv1.Deployment, vmdpServerImage string) error {
	// 1. Create or update the deployment
	deployment := &appsv1.Deployment{}
	err := v.Client.Get(ctx, client.ObjectKey{
		Name:      vmdpServerDeploymentName,
		Namespace: v.Namespace,
	}, deployment)

	if errors.IsNotFound(err) {
		deployment = buildVMDPServerDeployment(v.Namespace, vmdpServerImage)
		if err := controllerutil.SetOwnerReference(operatorDeployment, deployment, v.Client.Scheme()); err != nil {
			return fmt.Errorf("failed to set owner reference on deployment: %w", err)
		}
		err = v.Client.Create(ctx, deployment)
		if err != nil && !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create VMDP server deployment: %w", err)
		}
		v.Log.Info("Created VMDP server deployment", "image", vmdpServerImage)
	} else if err != nil {
		return fmt.Errorf("failed to get VMDP server deployment: %w", err)
	} else if len(deployment.Spec.Template.Spec.Containers) > 0 &&
		deployment.Spec.Template.Spec.Containers[0].ReadinessProbe == nil {
		// Deployment exists from a version before probes were added; backfill them.
		desired := buildVMDPServerDeployment(v.Namespace, vmdpServerImage)
		deployment.Spec.Template.Spec.Containers[0].ReadinessProbe = desired.Spec.Template.Spec.Containers[0].ReadinessProbe
		deployment.Spec.Template.Spec.Containers[0].LivenessProbe = desired.Spec.Template.Spec.Containers[0].LivenessProbe
		if err := v.Client.Update(ctx, deployment); err != nil {
			return fmt.Errorf("failed to update VMDP server deployment with probes: %w", err)
		}
		v.Log.Info("Updated VMDP server deployment with readiness/liveness probes")
	}

	// 2. Create or update the service
	service := &corev1.Service{}
	err = v.Client.Get(ctx, client.ObjectKey{
		Name:      vmdpServerServiceName,
		Namespace: v.Namespace,
	}, service)

	if errors.IsNotFound(err) {
		service = buildVMDPServerService(v.Namespace)
		if err := controllerutil.SetOwnerReference(operatorDeployment, service, v.Client.Scheme()); err != nil {
			return fmt.Errorf("failed to set owner reference on service: %w", err)
		}
		err = v.Client.Create(ctx, service)
		if err != nil && !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create VMDP server service: %w", err)
		}
		v.Log.Info("Created VMDP server service")
	} else if err != nil {
		return fmt.Errorf("failed to get VMDP server service: %w", err)
	}

	// 3. Create or get the route
	route := &routev1.Route{}
	err = v.Client.Get(ctx, client.ObjectKey{
		Name:      vmdpServerRouteName,
		Namespace: v.Namespace,
	}, route)

	if errors.IsNotFound(err) {
		route = buildVMDPServerRoute(v.Namespace)
		if err := controllerutil.SetOwnerReference(operatorDeployment, route, v.Client.Scheme()); err != nil {
			return fmt.Errorf("failed to set owner reference on route: %w", err)
		}
		err = v.Client.Create(ctx, route)
		if err != nil && !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create VMDP server route: %w", err)
		}
		v.Log.Info("Created VMDP server route, waiting for hostname assignment")
		time.Sleep(2 * time.Second)
		if err := v.Client.Get(ctx, client.ObjectKey{
			Name:      vmdpServerRouteName,
			Namespace: v.Namespace,
		}, route); err != nil {
			return fmt.Errorf("failed to get route after creation: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to get VMDP server route: %w", err)
	}

	// Check if route has hostname - retry with backoff if not assigned
	hostname := route.Spec.Host
	if hostname == "" {
		v.Log.Info("Route hostname not yet assigned, retrying with backoff")
		maxRetries := 5
		backoff := 2 * time.Second

		for attempt := 1; attempt <= maxRetries; attempt++ {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
				if err := v.Client.Get(ctx, client.ObjectKey{
					Name:      vmdpServerRouteName,
					Namespace: v.Namespace,
				}, route); err != nil {
					v.Log.Error(err, "Failed to get route on retry", "attempt", attempt)
					continue
				}

				if route.Spec.Host != "" {
					hostname = route.Spec.Host
					v.Log.Info("Route hostname assigned", "hostname", hostname, "attempt", attempt)
					break
				}

				v.Log.Info("Route hostname still not assigned, will retry", "attempt", attempt, "maxRetries", maxRetries)
				backoff *= 2
			}
		}

		if hostname == "" {
			v.Log.Info("Route hostname not assigned after max retries, ConsoleCLIDownload will not be created. It will be created on next reconciliation when hostname becomes available.")
			return nil
		}
	}

	// 4. Create or update ConsoleCLIDownload (cluster-scoped)
	downloadURL := fmt.Sprintf("https://%s/", hostname)

	consoleCLIDownload := &consolev1.ConsoleCLIDownload{}
	err = v.Client.Get(ctx, client.ObjectKey{Name: vmdpDownloadName}, consoleCLIDownload)

	desiredSpec := consolev1.ConsoleCLIDownloadSpec{
		Description: "OADP VM Data Protection CLI - back up and restore data inside OpenShift Virtualization guest VMs",
		DisplayName: "oadp-vmdp - OADP VM Data Protection CLI",
		Links: []consolev1.CLIDownloadLink{
			{
				Href: downloadURL,
				Text: "Download OADP VMDP CLI",
			},
		},
	}

	if errors.IsNotFound(err) {
		consoleCLIDownload = &consolev1.ConsoleCLIDownload{
			ObjectMeta: metav1.ObjectMeta{
				Name: vmdpDownloadName,
				Labels: map[string]string{
					managedByLabel:               operatorName,
					"app.kubernetes.io/instance": v.OperatorNamespace,
				},
			},
			Spec: desiredSpec,
		}
		err = v.Client.Create(ctx, consoleCLIDownload)
		if err != nil && !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create ConsoleCLIDownload for VMDP: %w", err)
		}
		v.Log.Info("Created ConsoleCLIDownload for VMDP", "url", downloadURL)
	} else if err != nil {
		return fmt.Errorf("failed to get ConsoleCLIDownload for VMDP: %w", err)
	} else {
		// ConsoleCLIDownload exists, update if needed
		needsUpdate := false

		if consoleCLIDownload.Labels == nil {
			consoleCLIDownload.Labels = make(map[string]string)
		}
		if consoleCLIDownload.Labels[managedByLabel] != operatorName {
			consoleCLIDownload.Labels[managedByLabel] = operatorName
			consoleCLIDownload.Labels["app.kubernetes.io/instance"] = v.OperatorNamespace
			needsUpdate = true
		}

		if len(consoleCLIDownload.Spec.Links) == 0 || consoleCLIDownload.Spec.Links[0].Href != downloadURL {
			consoleCLIDownload.Spec = desiredSpec
			needsUpdate = true
		}

		if needsUpdate {
			err = v.Client.Update(ctx, consoleCLIDownload)
			if err != nil {
				return fmt.Errorf("failed to update ConsoleCLIDownload for VMDP: %w", err)
			}
			v.Log.Info("Updated ConsoleCLIDownload for VMDP", "url", downloadURL)
		} else {
			v.Log.Info("ConsoleCLIDownload for VMDP already exists and is up-to-date", "url", downloadURL)
		}
	}

	return nil
}

func buildVMDPServerDeployment(namespace, image string) *appsv1.Deployment {
	replicas := int32(1)
	runAsNonRoot := true
	allowPrivilegeEscalation := false
	readOnlyRootFilesystem := true

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vmdpServerDeploymentName,
			Namespace: namespace,
			Labels: map[string]string{
				"app":          "oadp-vmdp",
				managedByLabel: operatorName,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "oadp-vmdp",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "oadp-vmdp",
					},
				},
				Spec: corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: &runAsNonRoot,
					},
					Containers: []corev1.Container{
						{
							Name:  "oadp-vmdp-server",
							Image: image,
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: 8080,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("64Mi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("50m"),
									corev1.ResourceMemory: resource.MustParse("32Mi"),
								},
							},
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: &allowPrivilegeEscalation,
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
								ReadOnlyRootFilesystem: &readOnlyRootFilesystem,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/",
										Port: intstr.FromString("http"),
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       10,
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/",
										Port: intstr.FromString("http"),
									},
								},
								InitialDelaySeconds: 15,
								PeriodSeconds:       20,
							},
						},
					},
					TerminationGracePeriodSeconds: int64Ptr(10),
				},
			},
		},
	}
}

func buildVMDPServerService(namespace string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vmdpServerServiceName,
			Namespace: namespace,
			Labels: map[string]string{
				"app":          "oadp-vmdp",
				managedByLabel: operatorName,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": "oadp-vmdp",
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromInt(8080),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}
}

func buildVMDPServerRoute(namespace string) *routev1.Route {
	return &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vmdpServerRouteName,
			Namespace: namespace,
			Labels: map[string]string{
				"app":          "oadp-vmdp",
				managedByLabel: operatorName,
			},
		},
		Spec: routev1.RouteSpec{
			To: routev1.RouteTargetReference{
				Kind: "Service",
				Name: vmdpServerServiceName,
			},
			Port: &routev1.RoutePort{
				TargetPort: intstr.FromString("http"),
			},
			TLS: &routev1.TLSConfig{
				Termination:                   routev1.TLSTerminationEdge,
				InsecureEdgeTerminationPolicy: routev1.InsecureEdgeTerminationPolicyRedirect,
			},
		},
	}
}
