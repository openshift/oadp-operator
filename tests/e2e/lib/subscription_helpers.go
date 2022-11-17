package lib

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

func (d *DpaCustomResource) GetOperatorSubscription() (*Subscription, error) {
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

func (s *Subscription) Refresh() error {
	cfg, err := config.GetConfig()
	if err != nil {
		return err
	}
	c, err := client.New(cfg, client.Options{})
	if err != nil {
		return err
	}
	return c.Get(context.Background(), types.NamespacedName{Namespace: s.Namespace, Name: s.Name}, s.Subscription)
}

func (s *Subscription) getCSV() (*operators.ClusterServiceVersion, error) {
	err := s.Refresh()
	if err != nil {
		return nil, err
	}

	client, err := client.New(config.GetConfigOrDie(), client.Options{})
	if err != nil {
		return nil, err
	}
	var installPlan operators.InstallPlan

	if s.Status.InstallPlanRef == nil {
		return nil, errors.New("no install plan found in subscription")
	}
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

func (s *Subscription) CsvIsReady() bool {
	csv, err := s.getCSV()
	if err != nil {
		log.Printf("Error getting CSV: %v", err)
		return false
	}
	log.Default().Printf("CSV status phase: %v", csv.Status.Phase)
	return csv.Status.Phase == operators.CSVPhaseSucceeded
}
func (s *Subscription) CsvIsInstalling() bool {
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
