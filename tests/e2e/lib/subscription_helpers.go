package lib

import (
	"context"
	"errors"
	"log"

	operators "github.com/operator-framework/api/pkg/operators/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Subscription struct {
	*operators.Subscription
}

type StreamSource string

const (
	UPSTREAM   StreamSource = "up"
	DOWNSTREAM StreamSource = "down"
)

func getOperatorSubscription(c client.Client, namespace, label string) (*Subscription, error) {
	sl := operators.SubscriptionList{}
	err := c.List(context.Background(), &sl, client.InNamespace(namespace), client.MatchingLabels(map[string]string{label: ""}))
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

func (d *DpaCustomResource) GetOperatorSubscription(c client.Client, stream StreamSource) (*Subscription, error) {
	err := d.SetClient(c)
	if err != nil {
		return nil, err
	}

	label := ""
	if stream == UPSTREAM {
		label = "operators.coreos.com/oadp-operator." + d.Namespace
	}
	if stream == DOWNSTREAM {
		label = "operators.coreos.com/redhat-oadp-operator." + d.Namespace
	}
	return getOperatorSubscription(c, d.Namespace, label)
}

func (v *VirtOperator) getOperatorSubscription() (*Subscription, error) {
	label := "operators.coreos.com/kubevirt-hyperconverged.openshift-cnv"
	return getOperatorSubscription(v.Client, v.Namespace, label)
}

func (s *Subscription) Refresh(c client.Client) error {
	return c.Get(context.Background(), types.NamespacedName{Namespace: s.Namespace, Name: s.Name}, s.Subscription)
}

func (s *Subscription) getCSV(c client.Client) (*operators.ClusterServiceVersion, error) {
	err := s.Refresh(c)
	if err != nil {
		return nil, err
	}

	var installPlan operators.InstallPlan

	if s.Status.InstallPlanRef == nil {
		return nil, errors.New("no install plan found in subscription")
	}
	err = c.Get(context.Background(), types.NamespacedName{Namespace: s.Namespace, Name: s.Status.InstallPlanRef.Name}, &installPlan)
	if err != nil {
		return nil, err
	}
	var csv operators.ClusterServiceVersion
	err = c.Get(context.Background(), types.NamespacedName{Namespace: installPlan.Namespace, Name: installPlan.Spec.ClusterServiceVersionNames[0]}, &csv)
	if err != nil {
		return nil, err
	}
	return &csv, nil
}

func (s *Subscription) CsvIsReady(c client.Client) bool {
	csv, err := s.getCSV(c)
	if err != nil {
		log.Printf("Error getting CSV: %v", err)
		return false
	}
	log.Default().Printf("CSV status phase: %v", csv.Status.Phase)
	return csv.Status.Phase == operators.CSVPhaseSucceeded
}

func (s *Subscription) CsvIsInstalling(c client.Client) bool {
	csv, err := s.getCSV(c)
	if err != nil {
		log.Printf("Error getting CSV: %v", err)
		return false
	}
	return csv.Status.Phase == operators.CSVPhaseInstalling
}

func (s *Subscription) CreateOrUpdate(c client.Client) error {
	log.Printf(s.APIVersion)
	var currentSubscription operators.Subscription
	err := c.Get(context.Background(), types.NamespacedName{Namespace: s.Namespace, Name: s.Name}, &currentSubscription)
	if apierrors.IsNotFound(err) {
		return c.Create(context.Background(), s.Subscription)
	}
	if err != nil {
		return err
	}
	return c.Update(context.Background(), s.Subscription)
}

func (s *Subscription) Delete(c client.Client) error {
	var currentSubscription operators.Subscription
	err := c.Get(context.Background(), types.NamespacedName{Namespace: s.Namespace, Name: s.Name}, &currentSubscription)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return c.Delete(context.Background(), s.Subscription)
}
