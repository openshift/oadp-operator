package hcp

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"
	"time"

	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// InstallRequiredOperators installs the required operators and returns a new HCHandler
func InstallRequiredOperators(ctx context.Context, c client.Client, reqOperators []RequiredOperator) (*HCHandler, error) {
	for _, op := range reqOperators {
		log.Printf("Installing operator %s", op.Name)
		err := op.InstallOperator(ctx, c)
		if err != nil {
			return nil, fmt.Errorf("failed to install operator %s: %v", op.Name, err)
		}
	}

	return &HCHandler{
		Ctx:            ctx,
		Client:         c,
		HCOCPTestImage: HCOCPTestImage,
	}, nil
}

// InstallOperator installs a specific operator
func (op *RequiredOperator) InstallOperator(ctx context.Context, c client.Client) error {
	log.Printf("Getting PackageManifest for operator %s", op.Name)

	// Create an unstructured object for the PackageManifest
	pkg := &unstructured.Unstructured{}
	pkg.SetGroupVersionKind(packageManifestGVR.GroupVersion().WithKind("PackageManifest"))
	pkg.SetName(op.Name)
	pkg.SetNamespace(op.Namespace)

	err := c.Get(ctx, types.NamespacedName{Name: op.Name, Namespace: op.Namespace}, pkg)
	if err != nil {
		return fmt.Errorf("failed to get PackageManifest for operator %s: %v", op.Name, err)
	}

	log.Printf("Checking namespace for operator %s", op.Name)
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: op.Namespace,
		},
	}

	tempNS := &corev1.Namespace{}
	if err := c.Get(ctx, types.NamespacedName{Name: op.Namespace}, tempNS); err != nil {
		if apierrors.IsNotFound(err) {
			log.Printf("Creating namespace for operator %s", op.Name)
			// Create the namespace if it doesn't exist
			err = c.Create(ctx, ns)
			if err != nil {
				return fmt.Errorf("failed to create namespace %s: %v", op.Namespace, err)
			}
		} else {
			return fmt.Errorf("failed to get namespace %s: %v", op.Namespace, err)
		}
	}

	opGroup := &operatorsv1.OperatorGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      op.OperatorGroup,
			Namespace: op.Namespace,
		},
		Spec: operatorsv1.OperatorGroupSpec{
			TargetNamespaces: []string{op.Namespace},
		},
	}

	log.Printf("Checking operator group for operator %s", op.Name)
	tempOpGroup := &operatorsv1.OperatorGroup{}
	if err := c.Get(ctx, types.NamespacedName{Name: op.OperatorGroup, Namespace: op.Namespace}, tempOpGroup); err != nil {
		if apierrors.IsNotFound(err) {
			log.Printf("Creating operator group for operator %s", op.Name)
			// Create the operator group
			err = c.Create(ctx, opGroup)
			if err != nil {
				return fmt.Errorf("failed to create operator group %s: %v", op.OperatorGroup, err)
			}
		} else {
			return fmt.Errorf("failed to get operator group %s: %v", op.OperatorGroup, err)
		}
	}

	// Create the subscription
	subscription := &operatorsv1alpha1.Subscription{
		ObjectMeta: metav1.ObjectMeta{
			Name:      op.Name,
			Namespace: op.Namespace,
		},
		Spec: &operatorsv1alpha1.SubscriptionSpec{
			CatalogSource:          RHOperatorsNamespace,
			CatalogSourceNamespace: OCPMarketplaceNamespace,
			Package:                op.Name,
			InstallPlanApproval:    operatorsv1alpha1.ApprovalAutomatic,
		},
	}

	// If a channel is specified, use it
	if op.Channel != "" {
		subscription.Spec.Channel = op.Channel
	} else {
		// Get the default channel from the PackageManifest
		defaultChannel, ok, err := unstructured.NestedString(pkg.UnstructuredContent(), "status", "defaultChannel")
		if err != nil {
			return fmt.Errorf("failed to get default channel from PackageManifest: %v", err)
		}
		if !ok || defaultChannel == "" {
			return fmt.Errorf("no default channel found in PackageManifest for operator %s", op.Name)
		}
		subscription.Spec.Channel = defaultChannel
	}

	// If a CSV is specified, use it
	if op.Csv != "" {
		subscription.Spec.StartingCSV = op.Csv
	}

	log.Printf("Checking subscription for operator %s", op.Name)
	tempSub := &operatorsv1alpha1.Subscription{}
	if err := c.Get(ctx, types.NamespacedName{Name: op.Name, Namespace: op.Namespace}, tempSub); err != nil {
		if apierrors.IsNotFound(err) {
			log.Printf("Creating subscription for operator %s", op.Name)
			err = c.Create(ctx, subscription)
			if err != nil {
				return fmt.Errorf("failed to create subscription for operator %s: %v", op.Name, err)
			}
		} else {
			return fmt.Errorf("failed to get subscription for operator %s: %v", op.Name, err)
		}
	}

	return nil
}

// WaitForUnstructuredObject waits for an unstructured object to be deleted
func WaitForUnstructuredObject(ctx context.Context, c client.Client, obj *unstructured.Unstructured, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(ctx, WaitForNextCheckTimeout, timeout, true, func(ctx context.Context) (bool, error) {
		log.Printf("\tWaiting for object %s in namespace %s to be deleted...", obj.GetName(), obj.GetNamespace())
		newObj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"kind":       obj.GetKind(),
				"apiVersion": obj.GetAPIVersion(),
				"metadata": map[string]interface{}{
					"name":      obj.GetName(),
					"namespace": obj.GetNamespace(),
				},
			},
		}
		err := c.Get(ctx, types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}, newObj)
		log.Printf("\tObject %s exists in namespace %s: %v", obj.GetName(), obj.GetNamespace(), err)
		return apierrors.IsNotFound(err), nil
	})
}

// ApplyYAMLTemplate reads a YAML template file, renders it with the given data, and applies it using the client
func ApplyYAMLTemplate(ctx context.Context, c client.Client, manifestPath string, override bool, data interface{}) error {
	// Read the manifest
	log.Printf("\tReading YAML template %s", filepath.Base(manifestPath))
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to read manifest from %s: %v", manifestPath, err)
	}

	// Parse the manifest
	log.Printf("\tParsing manifest %s", filepath.Base(manifestPath))
	tmpl, err := template.New("manifest").Parse(string(manifest))
	if err != nil {
		return fmt.Errorf("failed to parse manifest from %s: %v", manifestPath, err)
	}

	// Execute the manifest
	log.Printf("\tExecuting manifest %s", filepath.Base(manifestPath))
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("failed to execute manifest from %s: %v", manifestPath, err)
	}

	// Create a decoder for YAML
	decoder := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)

	// Decode the YAML into an unstructured object
	log.Printf("\tDecoding YAML %s", filepath.Base(manifestPath))
	obj := &unstructured.Unstructured{}
	_, _, err = decoder.Decode(buf.Bytes(), nil, obj)
	if err != nil {
		return fmt.Errorf("failed to decode YAML from %s: %v", manifestPath, err)
	}

	// Apply the object using the client
	log.Printf("\tApplying object %s", filepath.Base(manifestPath))
	err = c.Create(ctx, obj)
	if err != nil {
		if override && apierrors.IsAlreadyExists(err) {
			log.Printf("\tObject already exists, overriding...")
			err = c.Update(ctx, obj)
			if err != nil {
				return fmt.Errorf("failed to update object from %s: %v", manifestPath, err)
			}
		} else {
			return fmt.Errorf("failed to create object from %s: %v", manifestPath, err)
		}
	}

	log.Printf("\tObject applied successfully")

	return nil
}

// getPullSecret gets the pull secret from the openshift-config namespace
func getPullSecret(ctx context.Context, c client.Client) (string, error) {
	secret := &corev1.Secret{}
	err := c.Get(ctx, types.NamespacedName{Name: "pull-secret", Namespace: "openshift-config"}, secret)
	if err != nil {
		return "", fmt.Errorf("failed to get pull secret: %v", err)
	}
	if secret.Data == nil || len(secret.Data) == 0 {
		return "", fmt.Errorf("pull secret data is empty")
	}
	dockerConfig, ok := secret.Data[".dockerconfigjson"]
	if !ok {
		return "", fmt.Errorf("pull secret does not contain .dockerconfigjson key")
	}
	return string(dockerConfig), nil
}

// FilterErrorLogs filters out error logs based on predefined patterns
func FilterErrorLogs(logs []string) []string {
	filteredLogs := []string{}
	for _, logEntry := range logs {
		shouldInclude := true
		for _, pattern := range HCPErrorIgnorePatterns {
			if strings.Contains(logEntry, pattern) {
				shouldInclude = false
				break
			}
		}
		if shouldInclude {
			filteredLogs = append(filteredLogs, logEntry)
		}
	}
	return filteredLogs
}

// deleteResource deletes a Kubernetes resource with grace period 0
func (h *HCHandler) deleteResource(obj client.Object) error {
	deleteOptions := &client.DeleteOptions{
		GracePeriodSeconds: ptr.To(int64(0)),
	}
	return h.Client.Delete(h.Ctx, obj, deleteOptions)
}

// getProjectRoot returns the absolute path to the project root
func getProjectRoot() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filename)))))
}
