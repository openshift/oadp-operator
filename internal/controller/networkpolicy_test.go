package controller

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

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

	ginkgo.It("should isolate OADP workloads without selecting unrelated pods in the namespace", func() {
		dpa.Spec.NonAdmin = &oadpv1alpha1.NonAdmin{Enable: ptr.To(true)}
		dpa.Spec.VMFileRestore = &oadpv1alpha1.VMFileRestore{Enable: ptr.To(true)}
		dpa.Spec.Configuration.Velero.DefaultPlugins = []oadpv1alpha1.DefaultPlugin{
			oadpv1alpha1.DefaultPluginKubeVirtDataMover,
		}
		gomega.Expect(k8sClient.Update(ctx, dpa)).To(gomega.Succeed())
		gomega.Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name: dpa.Name, Namespace: dpa.Namespace,
		}, r.dpa)).To(gomega.Succeed())

		success, err := r.ReconcileNetworkPolicies(r.Log)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(success).To(gomega.BeTrue())

		workloads := map[string]labels.Set{
			operatorNetworkPolicyName:              {"control-plane": "controller-manager", "app.kubernetes.io/name": "oadp-operator"},
			veleroNetworkPolicyName:                {"component": "velero", "app.kubernetes.io/managed-by": "oadp-operator"},
			veleroMoverNetworkPolicyName:           {"oadp.openshift.io/network-policy": "velero"},
			cliServerNetworkPolicyName:             {"app": "oadp-cli"},
			vmdpServerNetworkPolicyName:            {"app": "oadp-vmdp"},
			nonAdminNetworkPolicyName:              {"control-plane": "non-admin-controller"},
			vmFileRestoreNetworkPolicyName:         {"control-plane": "oadp-vm-file-restore-controller"},
			kubevirtDatamoverNetworkPolicyName:     {"control-plane": "oadp-kubevirt-datamover-controller"},
			kubevirtDatamoverPodsNetworkPolicyName: {"kubevirt-datamover.io/pod-type": "upload"},
		}
		policies := &networkingv1.NetworkPolicyList{}
		gomega.Expect(k8sClient.List(ctx, policies, client.InNamespace(namespace.Name))).To(gomega.Succeed())
		gomega.Expect(policies.Items).To(gomega.HaveLen(len(workloads)))
		for _, policy := range policies.Items {
			gomega.Expect(workloads).To(gomega.HaveKey(policy.Name))
			selector, selectorErr := metav1.LabelSelectorAsSelector(&policy.Spec.PodSelector)
			gomega.Expect(selectorErr).NotTo(gomega.HaveOccurred())
			gomega.Expect(selector.Matches(workloads[policy.Name])).To(gomega.BeTrue(), policy.Name)
			gomega.Expect(selector.Matches(labels.Set{
				"app": "cluster-backup-chart", "component": "clusterbackup",
			})).To(gomega.BeFalse(), "policy %s must not isolate the ACM controller", policy.Name)
			gomega.Expect(selector.Matches(labels.Set{})).To(gomega.BeFalse(), policy.Name)
			gomega.Expect(selector.Matches(labels.Set{
				"control-plane": "controller-manager", "app.kubernetes.io/name": "another-operator",
			})).To(gomega.BeFalse(), "policy %s must not select another operator", policy.Name)
			gomega.Expect(selector.Matches(labels.Set{
				"component": "velero", "app.kubernetes.io/managed-by": "another-operator",
			})).To(gomega.BeFalse(), "policy %s must not select another Velero installation", policy.Name)
			gomega.Expect(policy.Spec.PolicyTypes).To(gomega.ConsistOf(
				networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress,
			), policy.Name)
		}
	})

	ginkgo.It("should remove an existing default-deny controlled by the DPA without recreating it", func() {
		np := &networkingv1.NetworkPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name: defaultDenyNetworkPolicyName, Namespace: namespace.Name,
				OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(dpa, oadpv1alpha1.GroupVersion.WithKind("DataProtectionApplication"))},
			},
			Spec: networkingv1.NetworkPolicySpec{
				PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress},
			},
		}
		gomega.Expect(k8sClient.Create(ctx, np)).To(gomega.Succeed())
		for range 2 {
			success, err := r.ReconcileNetworkPolicies(r.Log)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(success).To(gomega.BeTrue())
			err = k8sClient.Get(ctx, types.NamespacedName{Name: np.Name, Namespace: np.Namespace}, np)
			gomega.Expect(apierrors.IsNotFound(err)).To(gomega.BeTrue(), "expected legacy policy to be removed, got: %v", err)
		}
	})

	ginkgo.It("should retain the legacy default-deny if a component policy cannot be reconciled", func() {
		legacy := &networkingv1.NetworkPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name: defaultDenyNetworkPolicyName, Namespace: namespace.Name,
				OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(dpa, oadpv1alpha1.GroupVersion.WithKind("DataProtectionApplication"))},
			},
			Spec: networkingv1.NetworkPolicySpec{
				PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress},
			},
		}
		gomega.Expect(k8sClient.Create(ctx, legacy)).To(gomega.Succeed())
		original := legacy.DeepCopy()

		// An existing component policy controlled by another DPA prevents OADP
		// from applying the replacement policies for this DPA.
		owner := metav1.NewControllerRef(dpa, oadpv1alpha1.GroupVersion.WithKind("DataProtectionApplication"))
		owner.Name = "another-dpa"
		owner.UID = types.UID("another-dpa")
		conflict := &networkingv1.NetworkPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name: veleroNetworkPolicyName, Namespace: namespace.Name,
				OwnerReferences: []metav1.OwnerReference{*owner},
			},
			Spec: networkingv1.NetworkPolicySpec{PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress}},
		}
		gomega.Expect(k8sClient.Create(ctx, conflict)).To(gomega.Succeed())
		success, err := r.ReconcileNetworkPolicies(r.Log)
		gomega.Expect(err).To(gomega.HaveOccurred())
		gomega.Expect(success).To(gomega.BeFalse())
		gomega.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: legacy.Name, Namespace: legacy.Namespace}, legacy)).To(gomega.Succeed())
		gomega.Expect(legacy).To(gomega.Equal(original))
	})

	ginkgo.DescribeTable("should preserve a default-deny not controlled by the DPA", func(ownership string) {
		np := &networkingv1.NetworkPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: defaultDenyNetworkPolicyName, Namespace: namespace.Name},
			Spec: networkingv1.NetworkPolicySpec{
				PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress},
			},
		}
		switch ownership {
		case "another-controller":
			owner := metav1.NewControllerRef(dpa, oadpv1alpha1.GroupVersion.WithKind("DataProtectionApplication"))
			owner.UID = types.UID("another-dpa")
			np.OwnerReferences = []metav1.OwnerReference{*owner}
		case "non-controller":
			owner := metav1.NewControllerRef(dpa, oadpv1alpha1.GroupVersion.WithKind("DataProtectionApplication"))
			owner.Controller = ptr.To(false)
			np.OwnerReferences = []metav1.OwnerReference{*owner}
		}
		gomega.Expect(k8sClient.Create(ctx, np)).To(gomega.Succeed())
		original := np.DeepCopy()
		success, err := r.ReconcileNetworkPolicies(r.Log)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(success).To(gomega.BeTrue())
		gomega.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: np.Name, Namespace: np.Namespace}, np)).To(gomega.Succeed())
		gomega.Expect(np).To(gomega.Equal(original))
	},
		ginkgo.Entry("when user-managed", "unowned"),
		ginkgo.Entry("when controlled by another DPA", "another-controller"),
		ginkgo.Entry("when the DPA is only a non-controller owner", "non-controller"),
	)

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
		gomega.Expect(np.Spec.PodSelector.MatchLabels).To(gomega.Equal(map[string]string{
			"component":                    common.Velero,
			"app.kubernetes.io/managed-by": common.OADPOperator,
		}))

		// Verify ingress rules
		gomega.Expect(np.Spec.Ingress).To(gomega.HaveLen(1))
		ingress := np.Spec.Ingress[0]

		// Verify metrics scrape is scoped to the monitoring namespace
		gomega.Expect(ingress.From).To(gomega.HaveLen(1))
		gomega.Expect(ingress.From[0].NamespaceSelector.MatchLabels).To(gomega.HaveKeyWithValue("network.openshift.io/policy-group", "monitoring"))

		// Verify port 8085
		gomega.Expect(ingress.Ports).To(gomega.HaveLen(1))
		gomega.Expect(*ingress.Ports[0].Port).To(gomega.Equal(intstr.FromInt32(8085)))
		gomega.Expect(*ingress.Ports[0].Protocol).To(gomega.Equal(corev1.ProtocolTCP))

		// Verify unrestricted egress (Velero needs to reach arbitrary cloud/object-storage endpoints)
		gomega.Expect(np.Spec.PolicyTypes).To(gomega.ContainElement(networkingv1.PolicyTypeEgress))
		gomega.Expect(np.Spec.Egress).To(gomega.Equal(unrestrictedEgressRule()))
	})

	ginkgo.It("should create operator NetworkPolicy allowing metrics and health-probe ports", func() {
		success, err := r.ReconcileNetworkPolicies(r.Log)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(success).To(gomega.BeTrue())

		np := &networkingv1.NetworkPolicy{}
		err = k8sClient.Get(ctx, types.NamespacedName{
			Name:      operatorNetworkPolicyName,
			Namespace: namespace.Name,
		}, np)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// Verify pod selector
		gomega.Expect(np.Spec.PodSelector.MatchLabels).To(gomega.Equal(map[string]string{
			"control-plane":          "controller-manager",
			"app.kubernetes.io/name": common.OADPOperator,
		}))

		// Verify ingress rules: metrics (8443, scoped to the monitoring namespace) and kubelet
		// health-probe (8081, open cluster-wide since the kubelet is not a pod) must both be
		// allowed, otherwise the kubelet's liveness/readiness/startup probes get blocked and
		// the operator pod is marked unhealthy and restarted.
		gomega.Expect(np.Spec.Ingress).To(gomega.HaveLen(2))

		metricsIngress := np.Spec.Ingress[0]
		gomega.Expect(metricsIngress.From).To(gomega.HaveLen(1))
		gomega.Expect(metricsIngress.From[0].NamespaceSelector.MatchLabels).To(gomega.HaveKeyWithValue("network.openshift.io/policy-group", "monitoring"))
		gomega.Expect(metricsIngress.Ports).To(gomega.HaveLen(1))
		gomega.Expect(*metricsIngress.Ports[0].Port).To(gomega.Equal(intstr.FromInt32(8443)))
		gomega.Expect(*metricsIngress.Ports[0].Protocol).To(gomega.Equal(corev1.ProtocolTCP))

		healthProbeIngress := np.Spec.Ingress[1]
		gomega.Expect(healthProbeIngress.From).To(gomega.BeNil())
		gomega.Expect(healthProbeIngress.Ports).To(gomega.HaveLen(1))
		gomega.Expect(*healthProbeIngress.Ports[0].Port).To(gomega.Equal(intstr.FromInt32(8081)))
		gomega.Expect(*healthProbeIngress.Ports[0].Protocol).To(gomega.Equal(corev1.ProtocolTCP))

		// Verify unrestricted egress: the DataProtectionTest controller runs inside this
		// pod and connects directly to admin-configured BSL endpoints (S3, GCS, etc.) to
		// measure upload speed, so DNS+API-server-only egress is insufficient.
		gomega.Expect(np.Spec.PolicyTypes).To(gomega.ContainElement(networkingv1.PolicyTypeEgress))
		gomega.Expect(np.Spec.Egress).To(gomega.Equal(unrestrictedEgressRule()))
	})

	ginkgo.It("should create velero-mover NetworkPolicy with unrestricted egress and denied ingress", func() {
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

		// Isolate both directions, allowing only outbound connections.
		gomega.Expect(np.Spec.PolicyTypes).To(gomega.ConsistOf(networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress))
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

		// Verify the complete pod selector to prevent accidental extra labels.
		gomega.Expect(np.Spec.PodSelector.MatchLabels).To(gomega.Equal(map[string]string{
			"control-plane": "non-admin-controller",
		}))

		// Verify ports 8081 and 8080 (in that order per implementation)
		gomega.Expect(np.Spec.Ingress).To(gomega.HaveLen(1))
		ingress := np.Spec.Ingress[0]
		gomega.Expect(ingress.Ports).To(gomega.HaveLen(2))

		ports := []int32{8081, 8080}
		for i, port := range ingress.Ports {
			gomega.Expect(*port.Port).To(gomega.Equal(intstr.FromInt32(ports[i])))
			gomega.Expect(*port.Protocol).To(gomega.Equal(corev1.ProtocolTCP))
		}

		// Verify scoped egress (DNS + API server only)
		gomega.Expect(np.Spec.PolicyTypes).To(gomega.ContainElement(networkingv1.PolicyTypeEgress))
		// Assert the actual DNS service and endpoint ports independently of the
		// generator, so a missing post-DNAT port cannot pass this test.
		gomega.Expect(np.Spec.Egress).To(gomega.HaveLen(2))
		dnsEgress := np.Spec.Egress[0]
		gomega.Expect(dnsEgress.To).To(gomega.Equal([]networkingv1.NetworkPolicyPeer{
			{
				NamespaceSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"kubernetes.io/metadata.name": "openshift-dns",
					},
				},
			},
		}))
		gomega.Expect(dnsEgress.Ports).To(gomega.ConsistOf(
			networkingv1.NetworkPolicyPort{Protocol: ptr.To(corev1.ProtocolTCP), Port: ptr.To(intstr.FromInt32(53))},
			networkingv1.NetworkPolicyPort{Protocol: ptr.To(corev1.ProtocolUDP), Port: ptr.To(intstr.FromInt32(53))},
			networkingv1.NetworkPolicyPort{Protocol: ptr.To(corev1.ProtocolTCP), Port: ptr.To(intstr.FromInt32(5353))},
			networkingv1.NetworkPolicyPort{Protocol: ptr.To(corev1.ProtocolUDP), Port: ptr.To(intstr.FromInt32(5353))},
		))
		apiEgress := np.Spec.Egress[1]
		gomega.Expect(apiEgress.To).To(gomega.BeEmpty())
		gomega.Expect(apiEgress.Ports).To(gomega.Equal([]networkingv1.NetworkPolicyPort{
			{Protocol: ptr.To(corev1.ProtocolTCP), Port: ptr.To(intstr.FromInt32(6443))},
		}))
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
		gomega.Expect(apierrors.IsNotFound(err)).To(gomega.BeTrue(), "expected NotFound, got: %v", err)
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
		gomega.Expect(apierrors.IsNotFound(err)).To(gomega.BeTrue(), "expected NotFound, got: %v", err)
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

		// Verify unrestricted egress: VMFR downloads backup contents directly from
		// arbitrary, admin-configured S3-compatible BackupStorageLocation endpoints.
		gomega.Expect(np.Spec.PolicyTypes).To(gomega.ContainElement(networkingv1.PolicyTypeEgress))
		gomega.Expect(np.Spec.Egress).To(gomega.Equal(unrestrictedEgressRule()))
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

	ginkgo.It("should create KubeVirt datamover pods NetworkPolicy when enabled", func() {
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
			Name:      kubevirtDatamoverPodsNetworkPolicyName,
			Namespace: namespace.Name,
		}, np)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// Verify pod selector matches on the dynamically-spawned datamover pod label
		gomega.Expect(np.Spec.PodSelector.MatchExpressions).To(gomega.ContainElement(metav1.LabelSelectorRequirement{
			Key:      kubevirtDatamoverPodTypeLabel,
			Operator: metav1.LabelSelectorOpExists,
		}))

		// Isolate ingress without allowing unsolicited inbound connections.
		gomega.Expect(np.Spec.PolicyTypes).To(gomega.ConsistOf(networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress))
		gomega.Expect(np.Spec.Ingress).To(gomega.BeEmpty())

		// Verify unrestricted egress (these pods move VM disk data directly to/from
		// admin-configured, arbitrary S3-compatible endpoints)
		gomega.Expect(np.Spec.PolicyTypes).To(gomega.ContainElement(networkingv1.PolicyTypeEgress))
		gomega.Expect(np.Spec.Egress).To(gomega.Equal(unrestrictedEgressRule()))
	})

	ginkgo.It("should set controller reference on NetworkPolicies", func() {
		success, err := r.ReconcileNetworkPolicies(r.Log)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(success).To(gomega.BeTrue())

		// Check a component policy has the DPA controller reference.
		np := &networkingv1.NetworkPolicy{}
		err = k8sClient.Get(ctx, types.NamespacedName{
			Name:      veleroNetworkPolicyName,
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
