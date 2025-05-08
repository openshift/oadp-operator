package lib

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/onsi/gomega"
	"github.com/openshift/oadp-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CreateUploadTestOnlyDPT(c client.Client, namespace, bslName string) error {
	dpt := &v1alpha1.DataProtectionTest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("e2e-uploadtest-dpt-%d", time.Now().Unix()),
			Namespace: namespace,
		},
		Spec: v1alpha1.DataProtectionTestSpec{
			BackupLocationName: bslName,
			UploadSpeedTestConfig: &v1alpha1.UploadSpeedTestConfig{
				FileSize: "5MB",
				Timeout:  metav1.Duration{Duration: 120 * time.Second},
			},
		},
	}

	if err := c.Create(context.TODO(), dpt); err != nil {
		return fmt.Errorf("creating DataProtectionTest: %w", err)
	}

	// Wait until DPT completes
	gomega.Eventually(func() bool {
		_ = c.Get(context.TODO(), types.NamespacedName{
			Name:      dpt.Name,
			Namespace: namespace,
		}, dpt)
		return dpt.Status.Phase == "Complete" || dpt.Status.Phase == "Failed"
	}, time.Minute*3, time.Second*10).Should(gomega.BeTrue())

	log.Printf("✅ DPT %s completed with phase: %s", dpt.Name, dpt.Status.Phase)
	return nil
}
