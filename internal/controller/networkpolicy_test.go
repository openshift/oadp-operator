package controller

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/common"
)

var _ = ginkgo.Describe("ReconcileNetworkPolicies", func() {
	var (
		ctx       context.Context
		namespace *corev1.Namespace
		dpa       *oadpv1alpha1.DataProtectionApplication
		r         *DataProtectionApplicationReconciler
	)

	ginkgo.BeforeEach(func() {
		ctx = context.Background()
		namespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-np-",
			},
		}
		gomega.Expect(k8sClient.Create(ctx, namespace)).To(gomega.Succeed())

		dpa = &oadpv1alpha1.DataProtectionApplication{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-dpa",
				Namespace: namespace.Name,
			},
			Spec: oadpv1alpha1.DataProtectionApplicationSpec{
				Configuration: &oadpv1alpha1.ApplicationConfig{
					Velero: &oadpv1alpha1.VeleroConfig{},
				},
			},
		}
		gomega.Expect(k8sClient.Create(ctx, dpa)).To(gomega.Succeed())

		r = &DataProtectionApplicationReconciler{
			Client:         k8sClient,
			Scheme:         k8sClient.Scheme(),
			Log:            logr.Discard(),
			Context:        ctx,
			NamespacedName: types.NamespacedName{Name: dpa.Name, Namespace: dpa.Namespace},
			EventRecorder:  record.NewFakeRecorder(10),
			dpa:            dpa,
		}
	})

	ginkgo.AfterEach(func() {
		gomega.Expect(k8sClient.Delete(ctx, dpa)).To(gomega.Succeed())
		gomega.Expect(k8sClient.Delete(ctx, namespace)).To(gomega.Succeed())
	})

	ginkgo.It("should create default-deny NetworkPolicy", func() {
		success, err := r.ReconcileNetworkPolicies(r.Log)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(success).To(gomega.BeTrue())

		np := &networkingv1.NetworkPolicy{}
		err = k8sClient.Get(ctx, types.NamespacedName{
			Name:      defaultDenyNetworkPolicyName,
			Namespace: namespace.Name,
		}, np)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(np.Spec.PodSelector).To(gomega.Equal(metav1.LabelSelector{}))
		gomega.Expect(np.Spec.PolicyTypes).To(gomega.ConsistOf(networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress))
		gomega.Expect(np.Spec.Ingress).To(gomega.BeEmpty())
		gomega.Expect(np.Spec.Egress).To(gomega.BeEmpty())
	})

	ginkgo.It("should create velero NetworkPolicy with correct port", func() {
		success, err := r.ReconcileNetworkPolicies(r.Log)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(success).To(gomega.BeTrue())

		np := &networkingv1.NetworkPolicy{}
		err = k8sClient.Get(ctx, types.NamespacedName{
			Name:      veleroNetworkPolicyName,
			Namespace: namespace.Name,
		}, np)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// Verify pod selector
		gomega.Expect(np.Spec.PodSelector.MatchLabels).To(gomega.HaveKeyWithValue("component", common.Velero))

		// Verify ingress rules
		gomega.Expect(np.Spec.Ingress).To(gomega.HaveLen(1))
		ingress := np.Spec.Ingress[0]

		// Verify no 'from' restriction (metrics from anywhere)
		gomega.Expect(ingress.From).To(gomega.BeNil())

		// Verify port 8085
		gomega.Expect(ingress.Ports).To(gomega.HaveLen(1))
		gomega.Expect(*ingress.Ports[0].Port).To(gomega.Equal(intstr.FromInt32(8085)))
		gomega.Expect(*ingress.Ports[0].Protocol).To(gomega.Equal(corev1.ProtocolTCP))

		// Verify unrestricted egress (Velero needs to reach arbitrary cloud/object-storage endpoints)
		gomega.Expect(np.Spec.PolicyTypes).To(gomega.ContainElement(networkingv1.PolicyTypeEgress))
		gomega.Expect(np.Spec.Egress).To(gomega.Equal(unrestrictedEgressRule()))
	})

	ginkgo.It("should create velero-mover NetworkPolicy with unrestricted egress and no ingress", func() {
		success, err := r.ReconcileNetworkPolicies(r.Log)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(success).To(gomega.BeTrue())

		np := &networkingv1.NetworkPolicy{}
		err = k8sClient.Get(ctx, types.NamespacedName{
			Name:      veleroMoverNetworkPolicyName,
			Namespace: namespace.Name,
		}, np)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// Verify pod selector matches the dedicated mover label
		gomega.Expect(np.Spec.PodSelector.MatchLabels).To(gomega.HaveKeyWithValue(networkPolicyMoverLabel, networkPolicyMoverLabelValue))

		// Egress-only policy: no ingress rules/type, unrestricted egress
		gomega.Expect(np.Spec.PolicyTypes).To(gomega.ConsistOf(networkingv1.PolicyTypeEgress))
		gomega.Expect(np.Spec.Ingress).To(gomega.BeEmpty())
		gomega.Expect(np.Spec.Egress).To(gomega.Equal(unrestrictedEgressRule()))
	})

	ginkgo.It("should create non-admin NetworkPolicy when enabled", func() {
		// Enable non-admin
		dpa.Spec.NonAdmin = &oadpv1alpha1.NonAdmin{
			Enable: ptr.To(true),
		}
		gomega.Expect(k8sClient.Update(ctx, dpa)).To(gomega.Succeed())

		// Refresh DPA in reconciler
		gomega.Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      dpa.Name,
			Namespace: dpa.Namespace,
		}, r.dpa)).To(gomega.Succeed())

		success, err := r.ReconcileNetworkPolicies(r.Log)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(success).To(gomega.BeTrue())

		np := &networkingv1.NetworkPolicy{}
		err = k8sClient.Get(ctx, types.NamespacedName{
			Name:      nonAdminNetworkPolicyName,
			Namespace: namespace.Name,
		}, np)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// Verify pod selector
		gomega.Expect(np.Spec.PodSelector.MatchLabels).To(gomega.HaveKeyWithValue("app.kubernetes.io/component", "manager"))
		gomega.Expect(np.Spec.PodSelector.MatchLabels).To(gomega.HaveKeyWithValue("control-plane", "non-admin-controller"))

		// Verify ports 8081 and 8080 (in that order per implementation)
		gomega.Expect(np.Spec.Ingress).To(gomega.HaveLen(1))
		ingress := np.Spec.Ingress[0]
		gomega.Expect(ingress.Ports).To(gomega.HaveLen(2))

		ports := []int32{8081, 8080}
		for i, port := range ingress.Ports {
			gomega.Expect(*port.Port).To(gomega.Equal(intstr.FromInt32(ports[i])))
			gomega.Expect(*port.Protocol).To(gomega.Equal(corev1.ProtocolTCP))
		}

		// Verify scoped egress (DNS + API-server only)
		gomega.Expect(np.Spec.PolicyTypes).To(gomega.ContainElement(networkingv1.PolicyTypeEgress))
		gomega.Expect(np.Spec.Egress).To(gomega.Equal(scopedEgressRules()))
	})

	ginkgo.It("should NOT create non-admin NetworkPolicy when disabled", func() {
		// Non-admin disabled by default (nil or false)
		success, err := r.ReconcileNetworkPolicies(r.Log)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(success).To(gomega.BeTrue())

		np := &networkingv1.NetworkPolicy{}
		err = k8sClient.Get(ctx, types.NamespacedName{
			Name:      nonAdminNetworkPolicyName,
			Namespace: namespace.Name,
		}, np)
		gomega.Expect(err).To(gomega.HaveOccurred())
	})

	ginkgo.It("should delete non-admin NetworkPolicy when disabled after being enabled", func() {
		// First enable and create NP
		dpa.Spec.NonAdmin = &oadpv1alpha1.NonAdmin{
			Enable: ptr.To(true),
		}
		gomega.Expect(k8sClient.Update(ctx, dpa)).To(gomega.Succeed())
		gomega.Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      dpa.Name,
			Namespace: dpa.Namespace,
		}, r.dpa)).To(gomega.Succeed())

		success, err := r.ReconcileNetworkPolicies(r.Log)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(success).To(gomega.BeTrue())

		// Verify it exists
		np := &networkingv1.NetworkPolicy{}
		err = k8sClient.Get(ctx, types.NamespacedName{
			Name:      nonAdminNetworkPolicyName,
			Namespace: namespace.Name,
		}, np)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// Now disable non-admin
		dpa.Spec.NonAdmin.Enable = ptr.To(false)
		gomega.Expect(k8sClient.Update(ctx, dpa)).To(gomega.Succeed())
		gomega.Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      dpa.Name,
			Namespace: dpa.Namespace,
		}, r.dpa)).To(gomega.Succeed())

		success, err = r.ReconcileNetworkPolicies(r.Log)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(success).To(gomega.BeTrue())

		// Verify it's deleted
		err = k8sClient.Get(ctx, types.NamespacedName{
			Name:      nonAdminNetworkPolicyName,
			Namespace: namespace.Name,
		}, np)
		gomega.Expect(err).To(gomega.HaveOccurred())
	})

	ginkgo.It("should create VM file restore NetworkPolicy when enabled", func() {
		// Enable VM file restore
		dpa.Spec.VMFileRestore = &oadpv1alpha1.VMFileRestore{
			Enable: ptr.To(true),
		}
		gomega.Expect(k8sClient.Update(ctx, dpa)).To(gomega.Succeed())
		gomega.Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      dpa.Name,
			Namespace: dpa.Namespace,
		}, r.dpa)).To(gomega.Succeed())

		success, err := r.ReconcileNetworkPolicies(r.Log)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(success).To(gomega.BeTrue())

		np := &networkingv1.NetworkPolicy{}
		err = k8sClient.Get(ctx, types.NamespacedName{
			Name:      vmFileRestoreNetworkPolicyName,
			Namespace: namespace.Name,
		}, np)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// Verify pod selector
		gomega.Expect(np.Spec.PodSelector.MatchLabels).To(gomega.HaveKeyWithValue("control-plane", "oadp-vm-file-restore-controller"))

		// Verify ports 8081 and 8443
		gomega.Expect(np.Spec.Ingress).To(gomega.HaveLen(1))
		ingress := np.Spec.Ingress[0]
		gomega.Expect(ingress.Ports).To(gomega.HaveLen(2))

		ports := []int32{8081, 8443}
		for i, port := range ingress.Ports {
			gomega.Expect(*port.Port).To(gomega.Equal(intstr.FromInt32(ports[i])))
			gomega.Expect(*port.Protocol).To(gomega.Equal(corev1.ProtocolTCP))
		}

		// Verify scoped egress (DNS + API-server only)
		gomega.Expect(np.Spec.PolicyTypes).To(gomega.ContainElement(networkingv1.PolicyTypeEgress))
		gomega.Expect(np.Spec.Egress).To(gomega.Equal(scopedEgressRules()))
	})

	ginkgo.It("should create KubeVirt datamover NetworkPolicy when enabled", func() {
		// Enable KubeVirt datamover via default plugin
		dpa.Spec.Configuration.Velero.DefaultPlugins = []oadpv1alpha1.DefaultPlugin{
			oadpv1alpha1.DefaultPluginKubeVirtDataMover,
		}
		gomega.Expect(k8sClient.Update(ctx, dpa)).To(gomega.Succeed())
		gomega.Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      dpa.Name,
			Namespace: dpa.Namespace,
		}, r.dpa)).To(gomega.Succeed())

		success, err := r.ReconcileNetworkPolicies(r.Log)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(success).To(gomega.BeTrue())

		np := &networkingv1.NetworkPolicy{}
		err = k8sClient.Get(ctx, types.NamespacedName{
			Name:      kubevirtDatamoverNetworkPolicyName,
			Namespace: namespace.Name,
		}, np)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// Verify pod selector
		gomega.Expect(np.Spec.PodSelector.MatchLabels).To(gomega.HaveKeyWithValue("control-plane", "oadp-kubevirt-datamover-controller"))

		// Verify ports 8081 and 8443
		gomega.Expect(np.Spec.Ingress).To(gomega.HaveLen(1))
		ingress := np.Spec.Ingress[0]
		gomega.Expect(ingress.Ports).To(gomega.HaveLen(2))

		ports := []int32{8081, 8443}
		for i, port := range ingress.Ports {
			gomega.Expect(*port.Port).To(gomega.Equal(intstr.FromInt32(ports[i])))
			gomega.Expect(*port.Protocol).To(gomega.Equal(corev1.ProtocolTCP))
		}

		// Verify unrestricted egress (KubeVirt datamover reaches arbitrary cluster/registry endpoints)
		gomega.Expect(np.Spec.PolicyTypes).To(gomega.ContainElement(networkingv1.PolicyTypeEgress))
		gomega.Expect(np.Spec.Egress).To(gomega.Equal(unrestrictedEgressRule()))
	})

	ginkgo.It("should set controller reference on NetworkPolicies", func() {
		success, err := r.ReconcileNetworkPolicies(r.Log)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(success).To(gomega.BeTrue())

		// Check default-deny NP has controller reference
		np := &networkingv1.NetworkPolicy{}
		err = k8sClient.Get(ctx, types.NamespacedName{
			Name:      defaultDenyNetworkPolicyName,
			Namespace: namespace.Name,
		}, np)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// Verify controller reference exists
		gomega.Expect(np.OwnerReferences).To(gomega.HaveLen(1))
		gomega.Expect(np.OwnerReferences[0].Name).To(gomega.Equal(dpa.Name))
		gomega.Expect(np.OwnerReferences[0].Kind).To(gomega.Equal("DataProtectionApplication"))
		gomega.Expect(*np.OwnerReferences[0].Controller).To(gomega.BeTrue())
	})
})
