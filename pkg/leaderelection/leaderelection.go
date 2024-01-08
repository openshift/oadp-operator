package leaderelection

import (
	"context"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/library-go/pkg/config/clusterstatus"
	"github.com/openshift/library-go/pkg/config/leaderelection"
	"k8s.io/client-go/rest"
	"k8s.io/klog"
	"time"
)

// GetLeaderElectionConfig returns leader election config defaults based on the cluster topology
func GetLeaderElectionConfig(restConfig *rest.Config, enabled bool) configv1.LeaderElection {

	// Defaults follow conventions
	// https://github.com/openshift/enhancements/blob/master/CONVENTIONS.md#high-availability
	defaultLeaderElection := leaderelection.LeaderElectionDefaulting(
		configv1.LeaderElection{
			Disable: !enabled,
		},
		"",
		"",
	)

	if enabled {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
		defer cancel()
		if infra, err := clusterstatus.GetClusterInfraStatus(ctx, restConfig); err == nil && infra != nil {
			// check if the cluster is a SNO (Single Node Openshift) Cluster
			if infra.ControlPlaneTopology == configv1.SingleReplicaTopologyMode {
				return leaderelection.LeaderElectionSNOConfig(defaultLeaderElection)
			}
		} else {
			klog.Warning("unable to get cluster infrastructure status, using HA cluster values for leader election: %v", err)
		}
	}
	return defaultLeaderElection
}
