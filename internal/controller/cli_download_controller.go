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
	cliServerDeploymentName = "openshift-adp-oadp-cli-server"
	cliServerServiceName    = "openshift-adp-cli-server"
	cliServerRouteName      = "oadp-cli-server-route"
	cliDownloadName         = "openshift-adp-oadp-cli"
	managedByLabel          = "app.kubernetes.io/managed-by"
	operatorName            = "oadp-operator"
)

// CLIDownloadSetup is a runnable that sets up CLI download resources when the operator starts
type CLIDownloadSetup struct {
	Client            client.Client
	Namespace         string
	OperatorName      string
	OperatorNamespace string
	Log               logr.Logger
}

// Start implements the Runnable interface
func (c *CLIDownloadSetup) Start(ctx context.Context) error {
	c.Log = ctrl.Log.WithName("cli-download-setup")
	c.Log.Info("Starting CLI download setup")

	if available, err := isConsoleCRDAvailable(c.Client.RESTMapper(), c.Log); !available {
		return err
	}

	// Get the CLI server image from environment variable
	cliServerImage := os.Getenv("RELATED_IMAGE_CONSOLE_CLI_DOWNLOAD")
	if cliServerImage == "" {
		cliServerImage = "quay.io/konveyor/oadp-cli-binaries" // fallback default
		c.Log.Info("Using default CLI server image", "image", cliServerImage)
	}

	// Get the operator deployment to use as owner for namespaced resources
	operatorDeployment := &appsv1.Deployment{}
	err := c.Client.Get(ctx, types.NamespacedName{
		Name:      c.OperatorName,
		Namespace: c.OperatorNamespace,
	}, operatorDeployment)
	if err != nil {
		c.Log.Error(err, "Failed to get operator deployment")
		return err
	}

	// Create CLI resources (idempotent - will reuse existing ConsoleCLIDownload if present)
	if err := c.reconcileCLIResources(ctx, operatorDeployment, cliServerImage); err != nil {
		return err
	}

	c.Log.Info("CLI download setup completed successfully")
	return nil
}

// reconcileCLIResources creates or updates all CLI-related resources
// This is idempotent - it will reuse existing resources if they already exist
func (c *CLIDownloadSetup) reconcileCLIResources(ctx context.Context, operatorDeployment *appsv1.Deployment, cliServerImage string) error {
	// 1. Create or update the deployment
	deployment := &appsv1.Deployment{}
	err := c.Client.Get(ctx, client.ObjectKey{
		Name:      cliServerDeploymentName,
		Namespace: c.Namespace,
	}, deployment)

	if errors.IsNotFound(err) {
		deployment = buildCLIServerDeployment(c.Namespace, cliServerImage)
		// Set operator deployment as owner for automatic garbage collection
		// Use SetOwnerReference instead of SetControllerReference to avoid needing finalizer permissions
		if err := controllerutil.SetOwnerReference(operatorDeployment, deployment, c.Client.Scheme()); err != nil {
			return fmt.Errorf("failed to set owner reference on deployment: %w", err)
		}
		err = c.Client.Create(ctx, deployment)
		if err != nil && !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create CLI server deployment: %w", err)
		}
		c.Log.Info("Created CLI server deployment", "image", cliServerImage)
	} else if err != nil {
		return fmt.Errorf("failed to get CLI server deployment: %w", err)
	}

	// 2. Create or update the service
	service := &corev1.Service{}
	err = c.Client.Get(ctx, client.ObjectKey{
		Name:      cliServerServiceName,
		Namespace: c.Namespace,
	}, service)

	if errors.IsNotFound(err) {
		service = buildCLIServerService(c.Namespace)
		// Set operator deployment as owner for automatic garbage collection
		// Use SetOwnerReference instead of SetControllerReference to avoid needing finalizer permissions
		if err := controllerutil.SetOwnerReference(operatorDeployment, service, c.Client.Scheme()); err != nil {
			return fmt.Errorf("failed to set owner reference on service: %w", err)
		}
		err = c.Client.Create(ctx, service)
		if err != nil && !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create CLI server service: %w", err)
		}
		c.Log.Info("Created CLI server service")
	} else if err != nil {
		return fmt.Errorf("failed to get CLI server service: %w", err)
	}

	// 3. Create or get the route
	route := &routev1.Route{}
	err = c.Client.Get(ctx, client.ObjectKey{
		Name:      cliServerRouteName,
		Namespace: c.Namespace,
	}, route)

	if errors.IsNotFound(err) {
		route = buildCLIServerRoute(c.Namespace)
		// Set operator deployment as owner for automatic garbage collection
		// Use SetOwnerReference instead of SetControllerReference to avoid needing finalizer permissions
		if err := controllerutil.SetOwnerReference(operatorDeployment, route, c.Client.Scheme()); err != nil {
			return fmt.Errorf("failed to set owner reference on route: %w", err)
		}
		err = c.Client.Create(ctx, route)
		if err != nil && !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create CLI server route: %w", err)
		}
		c.Log.Info("Created CLI server route, waiting for hostname assignment")
		// Wait a bit for hostname to be assigned
		time.Sleep(2 * time.Second)
		// Re-fetch to get the hostname
		if err := c.Client.Get(ctx, client.ObjectKey{
			Name:      cliServerRouteName,
			Namespace: c.Namespace,
		}, route); err != nil {
			return fmt.Errorf("failed to get route after creation: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to get CLI server route: %w", err)
	}

	// Check if route has hostname - retry with backoff if not assigned
	hostname := route.Spec.Host
	if hostname == "" {
		c.Log.Info("Route hostname not yet assigned, retrying with backoff")
		maxRetries := 5
		backoff := 2 * time.Second

		for attempt := 1; attempt <= maxRetries; attempt++ {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
				// Re-fetch route to check for hostname
				if err := c.Client.Get(ctx, client.ObjectKey{
					Name:      cliServerRouteName,
					Namespace: c.Namespace,
				}, route); err != nil {
					c.Log.Error(err, "Failed to get route on retry", "attempt", attempt)
					continue
				}

				if route.Spec.Host != "" {
					hostname = route.Spec.Host
					c.Log.Info("Route hostname assigned", "hostname", hostname, "attempt", attempt)
					break
				}

				c.Log.Info("Route hostname still not assigned, will retry", "attempt", attempt, "maxRetries", maxRetries)
				backoff *= 2 // Exponential backoff
			}
		}

		if hostname == "" {
			c.Log.Info("Route hostname not assigned after max retries, ConsoleCLIDownload will not be created. It will be created on next reconciliation when hostname becomes available.")
			return nil
		}
	}

	// 4. Create or update ConsoleCLIDownload (cluster-scoped)
	// Note: This is idempotent. If the ConsoleCLIDownload already exists from a previous
	// installation, we'll reuse it and update the URL to point to our route.
	// We use a fixed name to prevent duplicates across operator reinstalls.
	downloadURL := fmt.Sprintf("https://%s/", hostname)

	consoleCLIDownload := &consolev1.ConsoleCLIDownload{}
	err = c.Client.Get(ctx, client.ObjectKey{Name: cliDownloadName}, consoleCLIDownload)

	desiredSpec := consolev1.ConsoleCLIDownloadSpec{
		Description: "OADP operator Command Line Interface (CLI)",
		DisplayName: "oadp - OADP operator Command Line Interface (CLI)",
		Links: []consolev1.CLIDownloadLink{
			{
				Href: downloadURL,
				Text: "Download OADP CLI",
			},
		},
	}

	if errors.IsNotFound(err) {
		// ConsoleCLIDownload doesn't exist, create it
		consoleCLIDownload = &consolev1.ConsoleCLIDownload{
			ObjectMeta: metav1.ObjectMeta{
				Name: cliDownloadName,
				Labels: map[string]string{
					managedByLabel:               operatorName,
					"app.kubernetes.io/instance": c.OperatorNamespace,
				},
			},
			Spec: desiredSpec,
		}
		err = c.Client.Create(ctx, consoleCLIDownload)
		if err != nil && !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create ConsoleCLIDownload: %w", err)
		}
		c.Log.Info("Created ConsoleCLIDownload", "url", downloadURL)
	} else if err != nil {
		return fmt.Errorf("failed to get ConsoleCLIDownload: %w", err)
	} else {
		// ConsoleCLIDownload exists (possibly from previous install), update if needed
		needsUpdate := false

		// Update labels if missing
		if consoleCLIDownload.Labels == nil {
			consoleCLIDownload.Labels = make(map[string]string)
		}
		if consoleCLIDownload.Labels[managedByLabel] != operatorName {
			consoleCLIDownload.Labels[managedByLabel] = operatorName
			consoleCLIDownload.Labels["app.kubernetes.io/instance"] = c.OperatorNamespace
			needsUpdate = true
		}

		// Update spec if URL changed
		if len(consoleCLIDownload.Spec.Links) == 0 || consoleCLIDownload.Spec.Links[0].Href != downloadURL {
			consoleCLIDownload.Spec = desiredSpec
			needsUpdate = true
		}

		if needsUpdate {
			err = c.Client.Update(ctx, consoleCLIDownload)
			if err != nil {
				return fmt.Errorf("failed to update ConsoleCLIDownload: %w", err)
			}
			c.Log.Info("Updated ConsoleCLIDownload", "url", downloadURL)
		} else {
			c.Log.Info("ConsoleCLIDownload already exists and is up-to-date", "url", downloadURL)
		}
	}

	return nil
}

func buildCLIServerDeployment(namespace, image string) *appsv1.Deployment {
	replicas := int32(1)
	runAsNonRoot := true
	allowPrivilegeEscalation := false
	readOnlyRootFilesystem := true

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cliServerDeploymentName,
			Namespace: namespace,
			Labels: map[string]string{
				"app":          "oadp-cli",
				managedByLabel: operatorName,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "oadp-cli",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "oadp-cli",
					},
				},
				Spec: corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: &runAsNonRoot,
					},
					Containers: []corev1.Container{
						{
							Name:  "oadp-cli-server",
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
						},
					},
					TerminationGracePeriodSeconds: int64Ptr(10),
				},
			},
		},
	}
}

func buildCLIServerService(namespace string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cliServerServiceName,
			Namespace: namespace,
			Labels: map[string]string{
				"app":          "oadp-cli",
				managedByLabel: operatorName,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": "oadp-cli",
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

func buildCLIServerRoute(namespace string) *routev1.Route {
	return &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cliServerRouteName,
			Namespace: namespace,
			Labels: map[string]string{
				"app":          "oadp-cli",
				managedByLabel: operatorName,
			},
		},
		Spec: routev1.RouteSpec{
			To: routev1.RouteTargetReference{
				Kind: "Service",
				Name: cliServerServiceName,
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

func int64Ptr(i int64) *int64 {
	return &i
}
