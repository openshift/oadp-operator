package lib

import (
	"context"
	"fmt"

	"github.com/onsi/ginkgo/v2"
	velero "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type restoreOpts func(*velero.Restore)

func WithIncludedResources(resources []string) restoreOpts {
	return func(restore *velero.Restore) {
		restore.Spec.IncludedResources = resources
	}
}

func WithExcludedResources(resources []string) restoreOpts {
	return func(restore *velero.Restore) {
		restore.Spec.ExcludedResources = resources
	}
}

func WithIncludedNamespaces(namespaces []string) restoreOpts {
	return func(restore *velero.Restore) {
		restore.Spec.IncludedNamespaces = namespaces
	}
}

func WithExcludedNamespaces(namespaces []string) restoreOpts {
	return func(restore *velero.Restore) {
		restore.Spec.ExcludedNamespaces = namespaces
	}
}

func CreateRestoreFromBackup(ocClient client.Client, veleroNamespace, backupName, restoreName string, opts ...restoreOpts) error {
	restore := velero.Restore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      restoreName,
			Namespace: veleroNamespace,
		},
		Spec: velero.RestoreSpec{
			BackupName: backupName,
		},
	}
	for _, opt := range opts {
		opt(&restore)
	}
	err := ocClient.Create(context.Background(), &restore)
	return restore, err
}

func IsRestoreDone(ocClient client.Client, veleroNamespace, name string) wait.ConditionFunc {
	return func() (bool, error) {
		restore := velero.Restore{}
		err := ocClient.Get(context.Background(), client.ObjectKey{
			Namespace: veleroNamespace,
			Name:      name,
		}, &restore)
		if err != nil {
			return false, err
		}
		if len(restore.Status.Phase) > 0 {
			ginkgo.GinkgoWriter.Write([]byte(fmt.Sprintf("restore phase: %s\n", restore.Status.Phase)))
		}
		if restore.Status.Phase != "" && restore.Status.Phase != velero.RestorePhaseNew && restore.Status.Phase != velero.RestorePhaseInProgress {
			return true, nil
		}
		return false, nil
	}
}

func IsRestoreCompletedSuccessfully(ocClient client.Client, veleroNamespace, name string) (bool, error) {
	restore := velero.Restore{}
	err := ocClient.Get(context.Background(), client.ObjectKey{
		Namespace: veleroNamespace,
		Name:      name,
	}, &restore)
	if err != nil {
		return false, err
	}
	if restore.Status.Phase == velero.RestorePhaseCompleted {
		return true, nil
	}
	return false, fmt.Errorf("restore phase is: %s; expected: %s\nfailure reason: %s\nvalidation errors: %v\nvelero failure logs: %v", restore.Status.Phase, velero.RestorePhaseCompleted, restore.Status.FailureReason, restore.Status.ValidationErrors, GetVeleroContainerFailureLogs(veleroNamespace))
}
