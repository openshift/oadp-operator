package lib

import (
	"context"
	"fmt"
	"log"
	"time"

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
	return GetRouteEndpointURLWithTimeout(ocClient, namespace, routeName, 10*time.Second)
}

// GetRouteEndpointURLWithTimeout is GetRouteEndpointURL with a caller-supplied
// per-attempt timeout bounding both the route API lookup and the HTTP
// reachability probe -- so a single stalled attempt can't block a retry
// loop's own bound indefinitely.
func GetRouteEndpointURLWithTimeout(ocClient client.Client, namespace, routeName string, timeout time.Duration) (string, error) {
	log.Println("Verifying if the service is accessible via route")
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	route := &routev1.Route{}
	err := ocClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: routeName}, route)
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
	resp, err := IsURLReachableWithTimeout(routeEndpoint, timeout)
	if err != nil || resp == false {
		return "", fmt.Errorf("Route endpoint not accessible: %v", err)
	}

	return routeEndpoint, nil
}
