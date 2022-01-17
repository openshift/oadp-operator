package e2e

import (
	"context"
	"errors"
	"log"

	operators "github.com/operator-framework/api/pkg/operators/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

type Subscription struct {
	*operators.Subscription
}

func (d *dpaCustomResource) getOperatorSubscription() (*Subscription, error) {
	err := d.SetClient()
	if err != nil {
		return nil, err
	}
	sl := operators.SubscriptionList{}
	err = d.Client.List(context.Background(), &sl, client.InNamespace(d.Namespace), client.MatchingLabels(map[string]string{"operators.coreos.com/oadp-operator." + d.Namespace: ""}))
	if err != nil {
		return nil, err
	}
	if len(sl.Items) == 0 {
		return nil, errors.New("no subscription found")
	}
	if len(sl.Items) > 1 {
		return nil, errors.New("more than one subscription found")
	}
	return &Subscription{&sl.Items[0]}, nil
}

func (s *Subscription) getCSV() (*operators.ClusterServiceVersion, error) {
	client, err := client.New(config.GetConfigOrDie(), client.Options{})
	if err != nil {
		return nil, err
	}
	var installPlan operators.InstallPlan
	err = client.Get(context.Background(), types.NamespacedName{Namespace: s.Namespace, Name: s.Status.InstallPlanRef.Name}, &installPlan)
	if err != nil {
		return nil, err
	}
	var csv operators.ClusterServiceVersion
	err = client.Get(context.Background(), types.NamespacedName{Namespace: installPlan.Namespace, Name: installPlan.Spec.ClusterServiceVersionNames[0]}, &csv)
	if err != nil {
		return nil, err
	}
	return &csv, nil
}

func (s *Subscription) csvIsReady() bool {
	csv, err := s.getCSV()
	if err != nil {
		log.Printf("Error getting CSV: %v", err)
		return false
	}
	return csv.Status.Phase == operators.CSVPhaseSucceeded
}
func (s *Subscription) csvIsInstalling() bool {
	csv, err := s.getCSV()
	if err != nil {
		log.Printf("Error getting CSV: %v", err)
		return false
	}
	return csv.Status.Phase == operators.CSVPhaseInstalling
}

func (s *Subscription) CreateOrUpdate() error {
	client, err := client.New(config.GetConfigOrDie(), client.Options{})
	if err != nil {
		return err
	}
	log.Printf(s.APIVersion)
	var currentSubscription operators.Subscription
	err = client.Get(context.Background(), types.NamespacedName{Namespace: s.Namespace, Name: s.Name}, &currentSubscription)
	if apierrors.IsNotFound(err) {
		return client.Create(context.Background(), s.Subscription)
	}
	if err != nil {
		return err
	}
	return client.Update(context.Background(), s.Subscription)
}
