package lib

import (
	"context"
	"errors"
	"log"
	"strings"

	operators "github.com/operator-framework/api/pkg/operators/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Subscription struct {
	*operators.Subscription
}

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

func (v *VirtOperator) getOperatorSubscription() (*Subscription, error) {
	label := "operators.coreos.com/kubevirt-hyperconverged.openshift-cnv"
	if v.Upstream {
		label = "operators.coreos.com/community-kubevirt-hyperconverged.kubevirt-hyperconverged"
	}
	return getOperatorSubscription(v.Client, v.Namespace, label)
}

func (s *Subscription) getCSV(c client.Client) (*operators.ClusterServiceVersion, error) {
	err := c.Get(context.Background(), types.NamespacedName{Namespace: s.Namespace, Name: s.Name}, s.Subscription)
	if err != nil {
		return nil, err
	}

	if s.Status.InstallPlanRef == nil {
		return nil, errors.New("no install plan found in subscription")
	}

	var installPlan operators.InstallPlan
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

func (s *Subscription) CsvIsReady(c client.Client) wait.ConditionFunc {
	return func() (bool, error) {
		csv, err := s.getCSV(c)
		if err != nil {
			return false, err
		}
		log.Printf("CSV %s status phase: %v", csv.Name, csv.Status.Phase)
		return csv.Status.Phase == operators.CSVPhaseSucceeded, nil
	}
}

func (s *Subscription) CsvIsInstalling(c client.Client) wait.ConditionFunc {
	return func() (bool, error) {
		csv, err := s.getCSV(c)
		if err != nil {
			return false, err
		}
		return csv.Status.Phase == operators.CSVPhaseInstalling, nil
	}
}

func (s *Subscription) Delete(c client.Client) error {
	err := c.Get(context.Background(), types.NamespacedName{Namespace: s.Namespace, Name: s.Name}, &operators.Subscription{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return c.Delete(context.Background(), s.Subscription)
}

func GetManagerPodLogs(c *kubernetes.Clientset, namespace string) (string, error) {
	controllerManagerPod, err := GetPodWithLabel(c, namespace, "control-plane=controller-manager")
	if err != nil {
		return "", err
	}
	logs, err := GetPodContainerLogs(c, namespace, controllerManagerPod.Name, "manager")
	if err != nil {
		return "", err
	}
	return logs, nil
}

// TODO doc
func ManagerPodIsUp(c *kubernetes.Clientset, namespace string) wait.ConditionFunc {
	return func() (bool, error) {
		logs, err := GetManagerPodLogs(c, namespace)
		if err != nil {
			return false, err
		}
		log.Print("waiting for leader election")
		return strings.Contains(logs, "successfully acquired lease"), nil
	}
}
