package gather

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

func AllResources(clusterClient client.Client, clusterResource client.ObjectList) error {
	return clusterClient.List(context.Background(), clusterResource)
}
