package olmv1_test

import (
	"context"
	"flag"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

var (
	namespace          string
	packageName        string
	channel            string
	version            string
	upgradeVersion     string
	catalogName        string
	catalogImage       string
	serviceAccountName string
	artifactDir        string
	migrate            bool

	createdCatalog bool

	kubeClient    *kubernetes.Clientset
	dynamicClient dynamic.Interface

	clusterExtensionGVR = schema.GroupVersionResource{
		Group:    "olm.operatorframework.io",
		Version:  "v1",
		Resource: "clusterextensions",
	}

	clusterCatalogGVR = schema.GroupVersionResource{
		Group:    "olm.operatorframework.io",
		Version:  "v1",
		Resource: "clustercatalogs",
	}
)

func init() {
	flag.StringVar(&namespace, "namespace", "openshift-adp", "Namespace to install the operator into")
	flag.StringVar(&packageName, "package", "oadp-operator", "OLM package name for the operator")
	flag.StringVar(&channel, "channel", "", "Catalog channel (optional)")
	flag.StringVar(&version, "version", "", "Version to install (optional, e.g. '1.5.1' or '1.5.x')")
	flag.StringVar(&upgradeVersion, "upgrade-version", "", "Version to upgrade to (optional)")
	flag.StringVar(&catalogName, "catalog", "oadp-olmv1-test-catalog", "ClusterCatalog name to create or reference")
	flag.StringVar(&catalogImage, "catalog-image", "", "Catalog image to use for creating a ClusterCatalog (required when package is not in default catalogs)")
	flag.StringVar(&serviceAccountName, "service-account", "oadp-operator-installer", "ServiceAccount name for ClusterExtension")
	flag.StringVar(&artifactDir, "artifact_dir", "/tmp", "Directory for test artifacts")
	flag.BoolVar(&migrate, "migrate", false, "Run OLMv0-to-OLMv1 migration tests (expects pre-existing OLMv0 install)")
}

func TestOADPOLMv1(t *testing.T) {
	flag.Parse()
	gomega.RegisterFailHandler(ginkgo.Fail)

	kubeConfig := config.GetConfigOrDie()
	kubeConfig.QPS = 50
	kubeConfig.Burst = 100

	var err error
	kubeClient, err = kubernetes.NewForConfig(kubeConfig)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	dynamicClient, err = dynamic.NewForConfig(kubeConfig)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	ginkgo.RunSpecs(t, "OADP OLMv1 Suite")
}

// --- Helpers ---

func ensureNamespace(ctx context.Context, name string) {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	_, err := kubeClient.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return
	}
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	log.Printf("Created namespace %s", name)
}

func ensureServiceAccount(ctx context.Context, name, ns string) {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
	}
	_, err := kubeClient.CoreV1().ServiceAccounts(ns).Create(ctx, sa, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return
	}
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	log.Printf("Created ServiceAccount %s/%s", ns, name)
}

// ensureClusterAdminBinding grants cluster-admin to the installer SA.
// This is intentionally broad for testing; production should use least-privilege RBAC.
func ensureClusterAdminBinding(ctx context.Context, saName, ns string) {
	bindingName := saName + "-binding"
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: bindingName},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "cluster-admin",
		},
		Subjects: []rbacv1.Subject{
			{Kind: "ServiceAccount", Name: saName, Namespace: ns},
		},
	}
	_, err := kubeClient.RbacV1().ClusterRoleBindings().Create(ctx, crb, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return
	}
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	log.Printf("Created ClusterRoleBinding %s", bindingName)
}

func buildClusterExtension(name, pkg, ns, sa string) *unstructured.Unstructured {
	spec := map[string]interface{}{
		"namespace": ns,
		"serviceAccount": map[string]interface{}{
			"name": sa,
		},
		"source": map[string]interface{}{
			"sourceType": "Catalog",
			"catalog": map[string]interface{}{
				"packageName": pkg,
			},
		},
		// OwnNamespace operators require watchNamespace to tell OLMv1
		// which namespace the operator should watch. Set it to the
		// install namespace so it mirrors OLMv0 OwnNamespace behavior.
		"config": map[string]interface{}{
			"configType": "Inline",
			"inline": map[string]interface{}{
				"watchNamespace": ns,
			},
		},
	}

	ce := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "olm.operatorframework.io/v1",
			"kind":       "ClusterExtension",
			"metadata": map[string]interface{}{
				"name": name,
			},
			"spec": spec,
		},
	}

	catalogSpec := spec["source"].(map[string]interface{})["catalog"].(map[string]interface{})
	if catalogImage != "" {
		catalogSpec["selector"] = map[string]interface{}{
			"matchLabels": map[string]interface{}{
				"olm.operatorframework.io/metadata.name": catalogName,
			},
		}
	}
	if channel != "" {
		catalogSpec["channels"] = []interface{}{channel}
	}
	if version != "" {
		catalogSpec["version"] = version
	}

	return ce
}

func getClusterExtension(ctx context.Context, name string) (*unstructured.Unstructured, error) {
	return dynamicClient.Resource(clusterExtensionGVR).Get(ctx, name, metav1.GetOptions{})
}

func deleteClusterExtension(ctx context.Context, name string) error {
	err := dynamicClient.Resource(clusterExtensionGVR).Delete(ctx, name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func getCondition(obj *unstructured.Unstructured, condType string) (map[string]interface{}, bool) {
	conditions, found, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if err != nil || !found {
		return nil, false
	}
	for _, c := range conditions {
		cond, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		if cond["type"] == condType {
			return cond, true
		}
	}
	return nil, false
}

func logAllConditions(obj *unstructured.Unstructured) {
	conditions, found, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if err != nil || !found {
		log.Print("  No conditions present yet")
		return
	}
	for _, c := range conditions {
		cond, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		condType, _ := cond["type"].(string)
		status, _ := cond["status"].(string)
		reason, _ := cond["reason"].(string)
		message, _ := cond["message"].(string)
		if len(message) > 120 {
			message = message[:120] + "..."
		}
		log.Printf("  %s: status=%s reason=%s message=%s", condType, status, reason, message)
	}
}

func getInstalledBundle(obj *unstructured.Unstructured) (name string, ver string, found bool) {
	bundleName, _, _ := unstructured.NestedString(obj.Object, "status", "install", "bundle", "name")
	bundleVersion, _, _ := unstructured.NestedString(obj.Object, "status", "install", "bundle", "version")
	if bundleName != "" {
		return bundleName, bundleVersion, true
	}
	return "", "", false
}

func crdExists(ctx context.Context, name string) (bool, error) {
	crdGVR := schema.GroupVersionResource{
		Group:    apiextensionsv1.SchemeGroupVersion.Group,
		Version:  apiextensionsv1.SchemeGroupVersion.Version,
		Resource: "customresourcedefinitions",
	}
	_, err := dynamicClient.Resource(crdGVR).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func cleanupClusterRoleBinding(ctx context.Context, saName string) {
	bindingName := saName + "-binding"
	err := kubeClient.RbacV1().ClusterRoleBindings().Delete(ctx, bindingName, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		log.Printf("Warning: failed to delete ClusterRoleBinding %s: %v", bindingName, err)
	}
}

// cleanupOrphanedCRDs deletes any OADP or Velero CRDs left behind by a
// previous OLMv0 deployment or a prior test run. OLMv1 cannot adopt CRDs
// it did not create, so these must be removed before a fresh install.
func cleanupOrphanedCRDs(ctx context.Context) {
	crdGVR := schema.GroupVersionResource{
		Group:    apiextensionsv1.SchemeGroupVersion.Group,
		Version:  apiextensionsv1.SchemeGroupVersion.Version,
		Resource: "customresourcedefinitions",
	}
	crdList, err := dynamicClient.Resource(crdGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Printf("Warning: failed to list CRDs: %v", err)
		return
	}
	var deleted int
	for _, crd := range crdList.Items {
		name := crd.GetName()
		if strings.HasSuffix(name, ".oadp.openshift.io") || strings.HasSuffix(name, ".velero.io") {
			if err := dynamicClient.Resource(crdGVR).Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
				log.Printf("Warning: failed to delete CRD %s: %v", name, err)
			} else {
				deleted++
			}
		}
	}
	if deleted > 0 {
		log.Printf("Deleted %d orphaned OADP/Velero CRDs", deleted)
	}
}

func ensureClusterCatalog(ctx context.Context, name, image string) {
	cc := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "olm.operatorframework.io/v1",
			"kind":       "ClusterCatalog",
			"metadata": map[string]interface{}{
				"name": name,
			},
			"spec": map[string]interface{}{
				"source": map[string]interface{}{
					"type": "Image",
					"image": map[string]interface{}{
						"ref": image,
					},
				},
			},
		},
	}
	_, err := dynamicClient.Resource(clusterCatalogGVR).Create(ctx, cc, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		log.Printf("ClusterCatalog %s already exists", name)
		return
	}
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	createdCatalog = true
	log.Printf("Created ClusterCatalog %s with image %s", name, image)
}

func waitForClusterCatalogServing(ctx context.Context, name string) {
	gomega.Eventually(func() bool {
		obj, err := dynamicClient.Resource(clusterCatalogGVR).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			log.Printf("Error getting ClusterCatalog %s: %v", name, err)
			return false
		}
		conditions, found, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
		if !found {
			return false
		}
		for _, c := range conditions {
			cond, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			condType, _ := cond["type"].(string)
			status, _ := cond["status"].(string)
			reason, _ := cond["reason"].(string)
			message, _ := cond["message"].(string)
			switch condType {
			case "Serving":
				log.Printf("ClusterCatalog %s Serving: status=%s reason=%s", name, status, reason)
				if status != "True" && message != "" {
					log.Printf("  message: %s", message)
				}
				return status == "True"
			case "Progressing":
				if reason == "Failed" || status == "False" {
					imageRef, _, _ := unstructured.NestedString(obj.Object, "spec", "source", "image", "ref")
					log.Printf("ClusterCatalog %s Progressing: status=%s reason=%s (image: %s)", name, status, reason, imageRef)
					if message != "" {
						log.Printf("  message: %s", message)
					}
				}
			}
		}
		return false
	}, 5*time.Minute, 10*time.Second).Should(gomega.BeTrue(),
		"ClusterCatalog %s should be Serving — if using ttl.sh, the catalog image may have expired", name)
}

func deleteClusterCatalog(ctx context.Context, name string) {
	err := dynamicClient.Resource(clusterCatalogGVR).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		log.Printf("Warning: failed to delete ClusterCatalog %s: %v", name, err)
	}
}
