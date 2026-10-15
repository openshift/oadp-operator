package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	velero "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/uuid"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/tests/e2e/lib"
)

const networkPolicyFixtureLabel = "e2e.oadp.openshift.io/network-policy-test"

type networkPolicyFixture struct {
	ctx    context.Context
	client client.Client
	id     string
}

func (f *networkPolicyFixture) create(object client.Object) {
	gomega.ExpectWithOffset(1, f.client.Create(f.ctx, object)).To(gomega.Succeed())
	ginkgo.DeferCleanup(func(ctx ginkgo.SpecContext) {
		gomega.Expect(client.IgnoreNotFound(f.client.Delete(ctx, object))).To(gomega.Succeed())
	})
}

func (f *networkPolicyFixture) metadata(name, ns string) metav1.ObjectMeta {
	return metav1.ObjectMeta{Name: name, Namespace: ns, Labels: map[string]string{networkPolicyFixtureLabel: f.id}}
}

func (f *networkPolicyFixture) createNamespace(monitoring bool) string {
	ns := &corev1.Namespace{ObjectMeta: f.metadata("np-"+string(uuid.NewUUID())[:8], "")}
	if monitoring {
		ns.Labels["network.openshift.io/policy-group"] = "monitoring"
	}
	f.create(ns)
	return ns.Name
}

func (f *networkPolicyFixture) probePod(ns string, podLabels map[string]string) *corev1.Pod {
	name := "np-probe-" + string(uuid.NewUUID())[:8]
	owner := &corev1.ConfigMap{ObjectMeta: f.metadata(name, ns)}
	f.create(owner)
	pod := &corev1.Pod{
		ObjectMeta: f.metadata(name, ns),
		Spec: corev1.PodSpec{
			RestartPolicy:                 corev1.RestartPolicyNever,
			TerminationGracePeriodSeconds: ptr.To(int64(1)),
			SecurityContext: &corev1.PodSecurityContext{
				RunAsNonRoot: ptr.To(true), SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
			},
			// Keep generic partner labels out of existing Service ready endpoints.
			ReadinessGates: []corev1.PodReadinessGate{{ConditionType: "e2e.oadp.openshift.io/probe"}},
			Containers: []corev1.Container{{
				Name: "curl-tool", Image: "registry.k8s.io/e2e-test-images/agnhost:2.39", Command: []string{"/agnhost", "netexec", "--http-port=8080"},
				SecurityContext: &corev1.SecurityContext{
					AllowPrivilegeEscalation: ptr.To(false), Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
				},
			}},
		},
	}
	for key, value := range podLabels {
		pod.Labels[key] = value
	}
	// Prevent a real ReplicaSet from adopting a probe with generic labels.
	pod.OwnerReferences = []metav1.OwnerReference{{APIVersion: "v1", Kind: "ConfigMap", Name: owner.Name, UID: owner.UID, Controller: ptr.To(true)}}
	f.create(&corev1.ServiceAccount{ObjectMeta: f.metadata(name, ns)})
	pod.Spec.ServiceAccountName = name
	f.create(pod)
	gomega.Eventually(func() (bool, error) {
		err := f.client.Get(f.ctx, client.ObjectKeyFromObject(pod), pod)
		return pod.Status.Phase == corev1.PodRunning && len(pod.Status.ContainerStatuses) == 1 && pod.Status.ContainerStatuses[0].State.Running != nil, err
	}, 3*time.Minute, 2*time.Second).Should(gomega.BeTrue(), "probe pod must start")
	return pod
}

func (f *networkPolicyFixture) apiProbe(podLabels map[string]string) (*corev1.Pod, string) {
	pod := f.probePod(namespace, podLabels)
	cm := &corev1.ConfigMap{ObjectMeta: f.metadata(pod.Name+"-marker", namespace), Data: map[string]string{"marker": f.id}}
	f.create(cm)
	role := &rbacv1.Role{ObjectMeta: f.metadata(pod.Name, namespace), Rules: []rbacv1.PolicyRule{{
		APIGroups: []string{""}, Resources: []string{"configmaps"}, ResourceNames: []string{cm.Name}, Verbs: []string{"get"},
	}}}
	f.create(role)
	f.create(&rbacv1.RoleBinding{
		ObjectMeta: f.metadata(pod.Name, namespace), RoleRef: rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: role.Name},
		Subjects: []rbacv1.Subject{{Kind: "ServiceAccount", Name: pod.Spec.ServiceAccountName, Namespace: namespace}},
	})
	return pod, fmt.Sprintf("https://kubernetes.default.svc/api/v1/namespaces/%s/configmaps/%s", namespace, cm.Name)
}

func (f *networkPolicyFixture) readyPod(selector string) *corev1.Pod {
	var found *corev1.Pod
	gomega.Eventually(func() (bool, error) {
		pods, err := kubernetesClientForSuiteRun.CoreV1().Pods(namespace).List(f.ctx, metav1.ListOptions{LabelSelector: selector})
		if err != nil {
			return false, err
		}
		for _, pod := range pods.Items {
			if pod.DeletionTimestamp != nil || pod.Status.PodIP == "" {
				continue
			}
			for _, condition := range pod.Status.Conditions {
				if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
					found = pod.DeepCopy()
					return true, nil
				}
			}
		}
		return false, nil
	}, 3*time.Minute, 2*time.Second).Should(gomega.BeTrue(), "expected a ready pod for %s", selector)
	return found
}

func (f *networkPolicyFixture) policy(name string, pod *corev1.Pod) {
	dpa, err := dpaCR.Get()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Eventually(func() (bool, error) {
		np := &networkingv1.NetworkPolicy{}
		if err := f.client.Get(f.ctx, client.ObjectKey{Name: name, Namespace: namespace}, np); err != nil {
			return false, err
		}
		if !metav1.IsControlledBy(np, dpa) {
			return false, nil
		}
		if pod != nil {
			selector, err := metav1.LabelSelectorAsSelector(&np.Spec.PodSelector)
			return err == nil && selector.Matches(labels.Set(pod.Labels)), err
		}
		return true, nil
	}, time.Minute, 2*time.Second).Should(gomega.BeTrue(), "expected DPA policy %s to select the intended pod", name)
}

func networkPolicyExec(pod *corev1.Pod, container, script string) error {
	stdout, stderr, err := lib.ExecuteShellCommandInPod(lib.ProxyPodParameters{
		KubeClient: kubernetesClientForSuiteRun, KubeConfig: kubeConfig, Namespace: pod.Namespace, PodName: pod.Name, ContainerName: container,
	}, script)
	if err != nil {
		return fmt.Errorf("probe %s/%s: %w; stdout=%s; stderr=%s", pod.Name, container, err, strings.TrimSpace(stdout), strings.TrimSpace(stderr))
	}
	return nil
}

func networkPolicyRequest(url, options, assertion string) string {
	return fmt.Sprintf(`status=$(curl --silent --show-error --noproxy '*' --connect-timeout 2 --max-time 4 %s --output /tmp/np-response --write-out '%%{http_code}' '%s')
result=$?
printf 'http=%%s curl_exit=%%s\n' "$status" "$result"
%s`, options, url, assertion)
}

const networkPolicyAPIAuth = `--cacert /var/run/secrets/kubernetes.io/serviceaccount/ca.crt --header "Authorization: Bearer $(cat /var/run/secrets/kubernetes.io/serviceaccount/token)"`
const networkPolicyHTTPAllowed = `[ "$result" -eq 0 ] && [ "$status" = 200 ]`
const networkPolicyHTTPDenied = `[ "$result" -eq 28 ] && [ "$status" = 000 ]`

func expectNetworkPolicyRequest(pod *corev1.Pod, container, url, options, assertion string) {
	ginkgo.GinkgoHelper()
	gomega.Eventually(func() error {
		return networkPolicyExec(pod, container, networkPolicyRequest(url, options, assertion))
	}, time.Minute, 2*time.Second).Should(gomega.Succeed())
}

func networkPolicyPodURL(pod *corev1.Pod, port int, path string) string {
	return "http://" + net.JoinHostPort(pod.Status.PodIP, fmt.Sprint(port)) + path
}

var _ = ginkgo.Describe("NetworkPolicy", ginkgo.Label("network-policy"), func() {
	var fixture *networkPolicyFixture
	ginkgo.BeforeEach(func(ctx ginkgo.SpecContext) {
		probeClient, err := client.New(kubeConfig, client.Options{Scheme: clientgoscheme.Scheme})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		fixture = &networkPolicyFixture{ctx: ctx, client: probeClient, id: "np-" + string(uuid.NewUUID())[:8]}
		gomega.Expect(dpaCR.CreateOrUpdate(createTestDPASpec(TestDPASpec{BSLSecretName: bslSecretName}))).To(gomega.Succeed())
		gomega.Eventually(dpaCR.IsReconciledTrue(), 2*time.Minute, 2*time.Second).Should(gomega.BeTrue())
		fixture.policy("oadp-operator-network-policy", nil)
		fixture.policy("velero-network-policy", nil)
	})

	ginkgo.DescribeTable("preserves API access for a partner sharing OADP's namespace",
		func(ctx ginkgo.SpecContext, podLabels map[string]string) {
			fixture.ctx = ctx
			pod, url := fixture.apiProbe(podLabels)
			// The token is read inside the pod and never appears in test output.
			expectNetworkPolicyRequest(pod, "curl-tool", url, networkPolicyAPIAuth,
				networkPolicyHTTPAllowed+fmt.Sprintf(` && grep -q '%s' /tmp/np-response`, fixture.id))
			// API access alone cannot detect an accidentally broad selector on
			// a policy with unrestricted egress. Check inbound access as well.
			caller := fixture.probePod(namespace, nil)
			expectNetworkPolicyRequest(caller, "curl-tool", networkPolicyPodURL(pod, 8080, "/hostname"), "",
				networkPolicyHTTPAllowed+fmt.Sprintf(` && grep -Fx '%s' /tmp/np-response`, pod.Name))
		},
		ginkgo.Entry("ACM backup controller", map[string]string{"app": "cluster-backup-chart", "component": "clusterbackup"}),
		ginkgo.Entry("generic controller-manager", map[string]string{"control-plane": "controller-manager"}),
		ginkgo.Entry("generic Velero", map[string]string{"component": "velero"}),
		ginkgo.Entry("another operator", map[string]string{"control-plane": "controller-manager", "app.kubernetes.io/name": "partner-operator"}),
		ginkgo.Entry("another Velero installation", map[string]string{"component": "velero", "app.kubernetes.io/managed-by": "partner-operator"}),
		ginkgo.Entry("unlabelled partner", map[string]string{}),
	)

	ginkgo.It("allows monitoring metrics and downloads while blocking forbidden traffic", func(ctx ginkgo.SpecContext) {
		fixture.ctx = ctx
		ordinary := fixture.probePod(namespace, nil)
		monitoring := fixture.probePod(fixture.createNamespace(true), nil)
		veleroPod := fixture.readyPod("deploy=velero,component=velero,!job-name")
		fixture.policy("velero-network-policy", veleroPod)
		metricsURL := networkPolicyPodURL(veleroPod, 8085, "/metrics")
		ginkgo.By("checking the same metrics listener from monitoring and ordinary namespaces")
		expectNetworkPolicyRequest(monitoring, "curl-tool", metricsURL, "", networkPolicyHTTPAllowed)
		expectNetworkPolicyRequest(ordinary, "curl-tool", metricsURL, "", networkPolicyHTTPDenied)
		apiService, err := kubernetesClientForSuiteRun.CoreV1().Services("default").Get(fixture.ctx, "kubernetes", metav1.GetOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		apiURL := "https://" + net.JoinHostPort(apiService.Spec.ClusterIP, "443") + "/api"
		// Prove the destination is reachable before expecting an egress timeout.
		expectNetworkPolicyRequest(ordinary, "curl-tool", apiURL, "--insecure", `[ "$result" -eq 0 ] && [ "$status" != 000 ]`)
		for _, component := range []struct{ app, policy string }{
			{"oadp-cli", "oadp-cli-server-network-policy"}, {"oadp-vmdp", "oadp-vmdp-server-network-policy"},
		} {
			server := fixture.readyPod("app=" + component.app)
			fixture.policy(component.policy, server)
			ginkgo.By("checking downloads and denied egress for " + component.app)
			expectNetworkPolicyRequest(ordinary, "curl-tool", networkPolicyPodURL(server, 8080, "/"), "", networkPolicyHTTPAllowed)
			expectNetworkPolicyRequest(server, server.Spec.Containers[0].Name, apiURL, "--insecure", networkPolicyHTTPDenied)
		}
	}, ginkgo.SpecTimeout(8*time.Minute))

	ginkgo.It("allows non-admin DNS and API access while restricting other traffic", func(ctx ginkgo.SpecContext) {
		fixture.ctx = ctx
		original, err := dpaCR.Get()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		ginkgo.DeferCleanup(func() {
			gomega.Expect(dpaCR.CreateOrUpdate(original.Spec.DeepCopy())).To(gomega.Succeed())
			gomega.Eventually(dpaCR.IsReconciledTrue(), 2*time.Minute, 2*time.Second).Should(gomega.BeTrue())
			if original.Spec.NonAdmin == nil || !ptr.Deref(original.Spec.NonAdmin.Enable, false) {
				gomega.Eventually(func() bool {
					pods, err := kubernetesClientForSuiteRun.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "control-plane=non-admin-controller"})
					return err == nil && len(pods.Items) == 0
				}, time.Minute, 2*time.Second).Should(gomega.BeTrue(), "temporary non-admin controller and diagnostics must be removed")
			}
		})
		spec := original.Spec.DeepCopy()
		spec.NonAdmin = &oadpv1alpha1.NonAdmin{Enable: ptr.To(true)}
		gomega.Expect(dpaCR.CreateOrUpdate(spec)).To(gomega.Succeed())
		gomega.Eventually(dpaCR.IsReconciledTrue(), 2*time.Minute, 2*time.Second).Should(gomega.BeTrue())
		controller := fixture.readyPod("control-plane=non-admin-controller")
		fixture.policy("non-admin-controller-network-policy", controller)
		diagnostics := addNetworkPolicyDiagnostics(ctx, controller)
		ordinary := fixture.probePod(namespace, nil)
		monitoring := fixture.probePod(fixture.createNamespace(true), nil)
		apiService, err := kubernetesClientForSuiteRun.CoreV1().Services("default").Get(ctx, "kubernetes", metav1.GetOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		dnsService, err := kubernetesClientForSuiteRun.CoreV1().Services("openshift-dns").Get(ctx, "dns-default", metav1.GetOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		slices, err := kubernetesClientForSuiteRun.DiscoveryV1().EndpointSlices("openshift-dns").List(ctx, metav1.ListOptions{LabelSelector: discoveryv1.LabelServiceName + "=" + dnsService.Name})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		var endpointIP string
		for _, slice := range slices.Items {
			for _, endpoint := range slice.Endpoints {
				if (endpoint.Conditions.Ready == nil || *endpoint.Conditions.Ready) && len(endpoint.Addresses) > 0 {
					endpointIP = endpoint.Addresses[0]
					break
				}
			}
			if endpointIP != "" {
				break
			}
		}
		gomega.Expect(endpointIP).NotTo(gomega.BeEmpty(), "expected a ready OpenShift DNS endpoint")
		recordType := "A"
		if net.ParseIP(apiService.Spec.ClusterIP).To4() == nil {
			recordType = "AAAA"
		}
		for _, destination := range []struct {
			name, ip string
			port     int
		}{{"Service", dnsService.Spec.ClusterIP, 53}, {"endpoint", endpointIP, 5353}} {
			for _, protocol := range []string{"+notcp +ignore", "+tcp"} {
				ginkgo.By(fmt.Sprintf("resolving the API through DNS %s port %d using %s", destination.name, destination.port, protocol))
				script := fmt.Sprintf("dig +short +time=2 +tries=1 %s @%s -p %d kubernetes.default.svc.cluster.local %s | grep -Fx '%s'", protocol, destination.ip, destination.port, recordType, apiService.Spec.ClusterIP)
				gomega.Eventually(func() error { return networkPolicyExec(controller, diagnostics, script) }, time.Minute, 2*time.Second).Should(gomega.Succeed())
			}
		}
		expectNetworkPolicyRequest(controller, diagnostics, "https://kubernetes.default.svc/api", networkPolicyAPIAuth, networkPolicyHTTPAllowed)
		server := fixture.readyPod("app=oadp-cli")
		downloadURL := networkPolicyPodURL(server, 8080, "/")
		expectNetworkPolicyRequest(ordinary, "curl-tool", downloadURL, "", networkPolicyHTTPAllowed)
		expectNetworkPolicyRequest(controller, diagnostics, downloadURL, "", networkPolicyHTTPDenied)
		metricsURL := networkPolicyPodURL(controller, 8080, "/metrics")
		expectNetworkPolicyRequest(monitoring, "curl-tool", metricsURL, "", networkPolicyHTTPAllowed)
		expectNetworkPolicyRequest(ordinary, "curl-tool", metricsURL, "", networkPolicyHTTPDenied)
	}, ginkgo.SpecTimeout(10*time.Minute))

	ginkgo.It("allows a partner service account to submit a backup and restore through the API", func(ctx ginkgo.SpecContext) {
		fixture.ctx = ctx
		pod, url := fixture.apiProbe(map[string]string{"app": "partner-backup-controller"})
		expectNetworkPolicyRequest(pod, "curl-tool", url, networkPolicyAPIAuth, networkPolicyHTTPAllowed)
		role := &rbacv1.Role{ObjectMeta: fixture.metadata(fixture.id, namespace), Rules: []rbacv1.PolicyRule{{
			APIGroups: []string{"velero.io"}, Resources: []string{"backups", "restores"}, Verbs: []string{"create"},
		}}}
		fixture.create(role)
		fixture.create(&rbacv1.RoleBinding{
			ObjectMeta: fixture.metadata(fixture.id, namespace), RoleRef: rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: role.Name},
			Subjects: []rbacv1.Subject{{Kind: "ServiceAccount", Name: pod.Spec.ServiceAccountName, Namespace: namespace}},
		})
		sourceNS := fixture.createNamespace(false)
		marker := &corev1.ConfigMap{ObjectMeta: fixture.metadata("marker", sourceNS), Data: map[string]string{"marker": fixture.id}}
		fixture.create(marker)
		backup := &velero.Backup{
			TypeMeta: metav1.TypeMeta{APIVersion: "velero.io/v1", Kind: "Backup"}, ObjectMeta: fixture.metadata(fixture.id, namespace),
			Spec: velero.BackupSpec{IncludedNamespaces: []string{sourceNS}, IncludedResources: []string{"configmaps"}, IncludeClusterResources: ptr.To(false), LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{networkPolicyFixtureLabel: fixture.id}}},
		}
		var restoreName string
		ginkgo.DeferCleanup(func() {
			_, err := lib.GetBackup(dpaCR.Client, namespace, backup.Name)
			if apierrors.IsNotFound(err) {
				return
			}
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(lib.DeleteVeleroBackupAndRestore(dpaCR.Client, kubernetesClientForSuiteRun, kubeConfig, namespace, backup.Name, restoreName)).To(gomega.Succeed())
		})
		post := func(resource string, object any) {
			body, err := json.Marshal(object)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			options := networkPolicyAPIAuth + " --request POST --header 'Content-Type: application/json' --data-binary '" + strings.ReplaceAll(string(body), "'", "'\\''") + "'"
			expectNetworkPolicyRequest(pod, "curl-tool", fmt.Sprintf("https://kubernetes.default.svc/apis/velero.io/v1/namespaces/%s/%s", namespace, resource), options, `[ "$result" -eq 0 ] && [ "$status" = 201 ]`)
		}
		ginkgo.By("submitting the backup from the partner pod using its own service account")
		gomega.Eventually(dpaCR.BSLsAreAvailable(), 3*time.Minute, 2*time.Second).Should(gomega.BeTrue())
		post("backups", backup)
		gomega.Eventually(lib.IsBackupDone(dpaCR.Client, namespace, backup.Name), 3*time.Minute, 2*time.Second).Should(gomega.BeTrue())
		completedBackup, err := lib.GetBackup(dpaCR.Client, namespace, backup.Name)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(completedBackup.Status.Phase).To(gomega.Equal(velero.BackupPhaseCompleted))
		gomega.Expect(fixture.client.Delete(fixture.ctx, marker)).To(gomega.Succeed())
		restore := &velero.Restore{
			TypeMeta: metav1.TypeMeta{APIVersion: "velero.io/v1", Kind: "Restore"}, ObjectMeta: fixture.metadata(fixture.id, namespace),
			Spec: velero.RestoreSpec{BackupName: backup.Name},
		}
		ginkgo.By("submitting a restore and verifying the deleted ConfigMap's contents")
		post("restores", restore)
		restoreName = restore.Name
		gomega.Eventually(lib.IsRestoreDone(dpaCR.Client, namespace, restore.Name), 3*time.Minute, 2*time.Second).Should(gomega.BeTrue())
		completedRestore, err := lib.GetRestore(dpaCR.Client, namespace, restore.Name)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(completedRestore.Status.Phase).To(gomega.Equal(velero.RestorePhaseCompleted))
		restored := &corev1.ConfigMap{}
		gomega.Expect(fixture.client.Get(fixture.ctx, client.ObjectKeyFromObject(marker), restored)).To(gomega.Succeed())
		gomega.Expect(restored.Data).To(gomega.Equal(marker.Data))
	}, ginkgo.SpecTimeout(10*time.Minute))
})

// The non-admin image has no shell. A restricted diagnostic container shares
// the actual controller pod's network namespace and service-account mount.
func addNetworkPolicyDiagnostics(ctx context.Context, pod *corev1.Pod) string {
	name := "np-diagnostics-" + string(uuid.NewUUID())[:8]
	sc := pod.Spec.Containers[0].SecurityContext.DeepCopy()
	if sc == nil {
		sc = &corev1.SecurityContext{}
	}
	sc.AllowPrivilegeEscalation = ptr.To(false)
	sc.RunAsNonRoot = ptr.To(true)
	sc.Capabilities = &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}}
	sc.SeccompProfile = &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}
	var mounts []corev1.VolumeMount
	for _, mount := range pod.Spec.Containers[0].VolumeMounts {
		if mount.MountPath == "/var/run/secrets/kubernetes.io/serviceaccount" {
			mounts = append(mounts, mount)
		}
	}
	gomega.Expect(mounts).To(gomega.HaveLen(1), "controller service-account mount is required")
	pod.Spec.EphemeralContainers = append(pod.Spec.EphemeralContainers, corev1.EphemeralContainer{EphemeralContainerCommon: corev1.EphemeralContainerCommon{
		Name: name, Image: "registry.k8s.io/e2e-test-images/agnhost:2.39", Command: []string{"sh", "-c", "trap 'exit 0' TERM; while :; do sleep 30 & wait $!; done"}, SecurityContext: sc, VolumeMounts: mounts,
	}})
	_, err := kubernetesClientForSuiteRun.CoreV1().Pods(pod.Namespace).UpdateEphemeralContainers(ctx, pod.Name, pod, metav1.UpdateOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Eventually(func() (bool, error) {
		current, err := kubernetesClientForSuiteRun.CoreV1().Pods(pod.Namespace).Get(ctx, pod.Name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		for _, status := range current.Status.EphemeralContainerStatuses {
			if status.Name == name && status.State.Running != nil {
				return true, nil
			}
		}
		return false, nil
	}, 3*time.Minute, 2*time.Second).Should(gomega.BeTrue(), "network diagnostic container must start")
	return name
}
