package lib

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/oadp-operator/api/v1alpha1"
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

// CreateDPTAndAssertComplete creates a DataProtectionTest for the given BSL,
// waits for it to reach a terminal phase, and returns an error if it did not
// complete successfully. The DPT is deleted after the check regardless of outcome.
func CreateDPTAndAssertComplete(c client.Client, namespace, bslName string) error {
	dpt := &v1alpha1.DataProtectionTest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("e2e-dpt-%d", time.Now().Unix()),
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
	defer func() {
		if err := c.Delete(context.TODO(), dpt); err != nil {
			log.Printf("warning: could not delete DPT %s: %v", dpt.Name, err)
		}
	}()

	gomega.Eventually(func() bool {
		_ = c.Get(context.TODO(), types.NamespacedName{
			Name:      dpt.Name,
			Namespace: namespace,
		}, dpt)
		return dpt.Status.Phase == "Complete" || dpt.Status.Phase == "Failed"
	}, time.Minute*3, time.Second*10).Should(gomega.BeTrue())

	log.Printf("✅ DPT %s completed with phase: %s", dpt.Name, dpt.Status.Phase)

	if dpt.Status.Phase != "Complete" {
		return fmt.Errorf("DataProtectionTest %s reached phase %q (error: %s)", dpt.Name, dpt.Status.Phase, dpt.Status.ErrorMessage)
	}
	// The reconciler marks the DPT Complete even when the upload itself failed
	// and records the outcome in status.uploadTest, so phase alone is not
	// enough: a TLS failure reaching the bucket would otherwise pass here.
	if !dpt.Status.UploadTest.Success {
		return fmt.Errorf("DataProtectionTest %s completed but the upload test failed: %s", dpt.Name, dpt.Status.UploadTest.ErrorMessage)
	}
	return nil
}
