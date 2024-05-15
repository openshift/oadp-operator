package e2e_test

import (
	"context"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/openshift/oadp-operator/tests/e2e/lib"
)

type NonAdminCRDCase struct {
	spec map[string]any
}

const nonAdminCRDNamespace = "test-non-admin-controller"

var nonAdminRestoreResource = schema.GroupVersionResource{
	Group:    "nac.oadp.openshift.io",
	Version:  "v1alpha1",
	Resource: "nonadminrestores",
}

var _ = ginkgo.Describe("Test NonAdminRestore in cluster validation", func() {
	var _ = ginkgo.BeforeAll(func() {
		err := lib.CreateNamespace(kubernetesClientForSuiteRun, nonAdminCRDNamespace)
		gomega.Expect(err).To(gomega.BeNil())
	})

	var _ = ginkgo.AfterAll(func() {
		err := lib.DeleteNamespace(kubernetesClientForSuiteRun, nonAdminCRDNamespace)
		gomega.Expect(err).To(gomega.BeNil())
	})

	ginkgo.DescribeTable("Validation is false",
		func(scenario NonAdminCRDCase) {
			nonAdminRestore := &unstructured.Unstructured{Object: map[string]any{
				"apiVersion": "nac.oadp.openshift.io/v1alpha1",
				"kind":       "NonAdminRestore",
				"metadata": map[string]string{
					"name":      "wrong-spec",
					"namespace": nonAdminCRDNamespace,
				},
				"spec": scenario.spec,
			}}
			_, err := dynamicClientForSuiteRun.Resource(nonAdminRestoreResource).Namespace(nonAdminCRDNamespace).Create(context.Background(), nonAdminRestore, v1.CreateOptions{})
			gomega.Expect(err).To(gomega.Not(gomega.BeNil()))
			gomega.Expect(err.Error()).To(gomega.ContainSubstring("Required value"))
		},
		ginkgo.Entry("Should NOT create NonAdminRestore without spec.restoreSpec", NonAdminCRDCase{
			spec: map[string]any{},
		}),
		ginkgo.Entry("Should NOT create NonAdminRestore without spec.restoreSpec.backupName", NonAdminCRDCase{
			spec: map[string]any{
				"restoreSpec": map[string]any{},
			},
		}),
	)
})
