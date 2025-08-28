package lib

import (
	"context"
	"fmt"
	"log"

	routev1 "github.com/openshift/api/route/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// getRouteEndpointURL retrieves and verifies the accessibility of a Kubernetes route HOST endpoint
//
// Parameters:
//   - ocClient:     An instance of the OpenShift client.
//   - namespace:    The Kubernetes namespace in which the service route is located.
//   - routeName:    The name of the Kubernetes route.
//
// Returns:
//   - string:       The full route endpoint URL if the service route is accessible.
//   - error:        An error message if the service route is not accessible, if the route is not found, or if there is an issue with the HTTP request.
func GetRouteEndpointURL(ocClient client.Client, namespace, routeName string) (string, error) {
	log.Println("Verifying if the service is accessible via route")
	route := &routev1.Route{}
	err := ocClient.Get(context.Background(), client.ObjectKey{Namespace: namespace, Name: routeName}, route)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return "", fmt.Errorf("Service route not found: %v", err)
		}
		return "", err
	}
	// Construct the route endpoint
	routeEndpoint := "http://" + route.Spec.Host

	// Check if the route is accessible
	log.Printf("Verifying if the service is accessible via: %s", routeEndpoint)
	resp, err := IsURLReachable(routeEndpoint)
	if err != nil || resp == false {
		return "", fmt.Errorf("Route endpoint not accessible: %v", err)
	}

	return routeEndpoint, nil
}
