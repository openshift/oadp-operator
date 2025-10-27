package controller

import (
	"context"
	"fmt"
	"time"

	consolev1 "github.com/openshift/api/console/v1"
	routev1 "github.com/openshift/api/route/v1"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch
//+kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=console.openshift.io,resources=consoleclidownloads,verbs=get;list;watch;create;update;patch;delete

type OADPCLIReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Namespace string
}

const (
	cliServerDeploymentName = "openshift-adp-oadp-cli-server"
	cliServerRouteName      = "oadp-cli-server-route"
	cliServerServiceName    = "openshift-adp-cli-server"
)

func (r *OADPCLIReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("Reconciling OADP CLI download resources", "triggered_by", req.NamespacedName)

	// 1. Check if CLI server deployment exists
	deployment := &appsv1.Deployment{}
	err := r.Get(ctx, client.ObjectKey{
		Name:      cliServerDeploymentName,
		Namespace: r.Namespace,
	}, deployment)

	if errors.IsNotFound(err) {
		log.V(1).Info("CLI server deployment not found, nothing to reconcile")
		return ctrl.Result{}, nil
	}
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get CLI server deployment: %w", err)
	}

	// 2. Check if CLI server route exists
	route := &routev1.Route{}
	err = r.Get(ctx, client.ObjectKey{
		Name:      cliServerRouteName,
		Namespace: r.Namespace,
	}, route)

	if err != nil && !errors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("failed to get CLI server route: %w", err)
	}

	// Route not found, create it
	if errors.IsNotFound(err) {
		route = &routev1.Route{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cliServerRouteName,
				Namespace: r.Namespace,
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
		err = r.Create(ctx, route)
		if err != nil && !errors.IsAlreadyExists(err) {
			return ctrl.Result{}, fmt.Errorf("failed to create CLI server route: %w", err)
		}

		// If AlreadyExists, just continue - another reconcile loop created it
		// Wait for route to get hostname assigned
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// At this point, route with hostname exists. Grab it
	hostname := route.Spec.Host
	if hostname == "" {
		return ctrl.Result{}, fmt.Errorf("CLI server route has no hostname")
	}

	// 3. Create or update ConsoleCLIDownload
	downloadURL := fmt.Sprintf("https://%s/", hostname)

	consoleCLIDownload := &consolev1.ConsoleCLIDownload{}
	err = r.Get(ctx, client.ObjectKey{Name: "openshift-adp-oadp-cli"}, consoleCLIDownload)

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
		// Create new ConsoleCLIDownload
		consoleCLIDownload = &consolev1.ConsoleCLIDownload{
			ObjectMeta: metav1.ObjectMeta{
				Name: "openshift-adp-oadp-cli",
			},
			Spec: desiredSpec,
		}
		err = r.Create(ctx, consoleCLIDownload)
		if err != nil && !errors.IsAlreadyExists(err) {
			return ctrl.Result{}, fmt.Errorf("failed to create ConsoleCLIDownload: %w", err)
		}
		log.Info("Created ConsoleCLIDownload", "url", downloadURL)
	} else if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get ConsoleCLIDownload: %w", err)
	} else {
		// Update existing ConsoleCLIDownload if URL changed
		if len(consoleCLIDownload.Spec.Links) == 0 || consoleCLIDownload.Spec.Links[0].Href != downloadURL {
			consoleCLIDownload.Spec = desiredSpec
			err = r.Update(ctx, consoleCLIDownload)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to update ConsoleCLIDownload: %w", err)
			}
			log.Info("Updated ConsoleCLIDownload with new URL", "url", downloadURL)
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *OADPCLIReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1.Deployment{}).
		WithEventFilter(cliServerPredicate()).
		Complete(r)
}

func cliServerPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return e.Object.GetName() == cliServerDeploymentName
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return e.ObjectNew.GetName() == cliServerDeploymentName
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return e.Object.GetName() == cliServerDeploymentName
		},
	}
}
