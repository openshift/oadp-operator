package hcp

import (
	"context"
	"testing"
	"time"

	"github.com/onsi/gomega"
	hypershiftv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openshift/oadp-operator/tests/e2e/lib"
)

func TestRemoveHCP(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	tests := []struct {
		name           string
		hc             *hypershiftv1.HostedCluster
		hcp            *hypershiftv1.HostedControlPlane
		namespace      *corev1.Namespace
		secrets        []*corev1.Secret
		expectedResult bool
	}{
		{
			name: "All resources exist",
			hc: &hypershiftv1.HostedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hc",
					Namespace: "clusters",
				},
			},
			hcp: &hypershiftv1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hc",
					Namespace: "clusters-test-hc",
				},
			},
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "clusters-test-hc",
				},
			},
			secrets: []*corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-hc-pull-secret",
						Namespace: "clusters-test-hc",
					},
				},
			},
			expectedResult: true,
		},
		{
			name: "Only HC exists",
			hc: &hypershiftv1.HostedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hc",
					Namespace: "clusters",
				},
			},
			expectedResult: true,
		},
		{
			name: "Only HCP exists",
			hcp: &hypershiftv1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hc",
					Namespace: "clusters-test-hc",
				},
			},
			expectedResult: true,
		},
		{
			name: "Only namespace exists",
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "clusters-test-hc",
				},
			},
			expectedResult: true,
		},
		{
			name:           "No resources exist",
			expectedResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new client with the correct scheme
			client := fake.NewClientBuilder().WithScheme(lib.Scheme).Build()

			// Create the handler
			h := &HCHandler{
				Ctx:           context.Background(),
				Client:        client,
				HostedCluster: tt.hc,
			}

			// Set HCPNamespace based on either namespace or HCP
			if tt.namespace != nil {
				h.HCPNamespace = tt.namespace.Name
			} else if tt.hcp != nil {
				h.HCPNamespace = tt.hcp.Namespace
			}

			// Create resources if they exist in the test case
			if tt.hc != nil {
				err := client.Create(context.Background(), tt.hc)
				g.Expect(err).NotTo(gomega.HaveOccurred())
			}

			if tt.hcp != nil {
				err := client.Create(context.Background(), tt.hcp)
				g.Expect(err).NotTo(gomega.HaveOccurred())
			}

			if tt.namespace != nil {
				err := client.Create(context.Background(), tt.namespace)
				g.Expect(err).NotTo(gomega.HaveOccurred())
			}

			for _, secret := range tt.secrets {
				err := client.Create(context.Background(), secret)
				g.Expect(err).NotTo(gomega.HaveOccurred())
			}

			// Call RemoveHCP
			err := h.RemoveHCP(Wait10Min)
			g.Expect(err).NotTo(gomega.HaveOccurred())

			// Verify resources are deleted
			if tt.hc != nil {
				hc := &hypershiftv1.HostedCluster{}
				err = client.Get(context.Background(), types.NamespacedName{Name: tt.hc.Name, Namespace: tt.hc.Namespace}, hc)
				g.Expect(apierrors.IsNotFound(err)).To(gomega.BeTrue())
			}

			if tt.hcp != nil {
				hcp := &hypershiftv1.HostedControlPlane{}
				err = client.Get(context.Background(), types.NamespacedName{Name: tt.hcp.Name, Namespace: tt.hcp.Namespace}, hcp)
				g.Expect(apierrors.IsNotFound(err)).To(gomega.BeTrue())
			}

			if tt.namespace != nil {
				ns := &corev1.Namespace{}
				err = client.Get(context.Background(), types.NamespacedName{Name: tt.namespace.Name}, ns)
				g.Expect(apierrors.IsNotFound(err)).To(gomega.BeTrue())
			}

			for _, secret := range tt.secrets {
				s := &corev1.Secret{}
				err = client.Get(context.Background(), types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, s)
				g.Expect(apierrors.IsNotFound(err)).To(gomega.BeTrue())
			}
		})
	}
}

func TestIsHCPDeleted(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	tests := []struct {
		name           string
		hcp            *hypershiftv1.HostedControlPlane
		expectedResult bool
	}{
		{
			name: "HCP exists",
			hcp: &hypershiftv1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hc",
					Namespace: "clusters-test-hc",
				},
			},
			expectedResult: false,
		},
		{
			name:           "HCP is nil",
			expectedResult: true,
		},
		{
			name: "HCP does not exist",
			hcp: &hypershiftv1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "non-existent",
					Namespace: "non-existent",
				},
			},
			expectedResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new client with the correct scheme
			client := fake.NewClientBuilder().WithScheme(lib.Scheme).Build()

			// Create the handler
			h := &HCHandler{
				Ctx:    context.Background(),
				Client: client,
			}

			// Create HCP if it exists in the test case
			if tt.hcp != nil && tt.name != "HCP does not exist" {
				err := client.Create(context.Background(), tt.hcp)
				g.Expect(err).NotTo(gomega.HaveOccurred())
			}

			// Call IsHCPDeleted
			result := IsHCPDeleted(h, tt.hcp)
			g.Expect(result).To(gomega.Equal(tt.expectedResult))
		})
	}
}

func TestIsHCDeleted(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	tests := []struct {
		name           string
		hc             *hypershiftv1.HostedCluster
		expectedResult bool
	}{
		{
			name: "HC exists",
			hc: &hypershiftv1.HostedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hc",
					Namespace: "clusters",
				},
			},
			expectedResult: false,
		},
		{
			name:           "HC is nil",
			expectedResult: true,
		},
		{
			name: "HC does not exist",
			hc: &hypershiftv1.HostedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "non-existent",
					Namespace: "non-existent",
				},
			},
			expectedResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new client with the correct scheme
			client := fake.NewClientBuilder().WithScheme(lib.Scheme).Build()

			// Create the handler
			h := &HCHandler{
				Ctx:    context.Background(),
				Client: client,
			}

			// Create HC if it exists in the test case
			if tt.hc != nil && tt.name != "HC does not exist" {
				h.HostedCluster = tt.hc
				err := client.Create(context.Background(), tt.hc)
				g.Expect(err).NotTo(gomega.HaveOccurred())
			}

			// Call IsHCDeleted
			result := IsHCDeleted(h)
			g.Expect(result).To(gomega.Equal(tt.expectedResult))
		})
	}
}

func TestValidateHCP(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	hostedClusterName := "test-hc"
	hcpNamespace := GetHCPNamespace(hostedClusterName, ClustersNamespace)

	// Define test cases
	tests := []struct {
		name          string
		deployments   []*appsv1.Deployment
		statefulsets  []*appsv1.StatefulSet
		expectedError bool
	}{
		{
			name: "All required deployments ready",
			deployments: []*appsv1.Deployment{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-apiserver",
						Namespace: hcpNamespace,
					},
					Status: appsv1.DeploymentStatus{
						AvailableReplicas: 1,
						Replicas:          1,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-controller-manager",
						Namespace: hcpNamespace,
					},
					Status: appsv1.DeploymentStatus{
						AvailableReplicas: 1,
						Replicas:          1,
					},
				},
			},
			statefulsets: []*appsv1.StatefulSet{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "etcd",
						Namespace: hcpNamespace,
					},
					Status: appsv1.StatefulSetStatus{
						ReadyReplicas: 1,
						Replicas:      1,
					},
				},
			},
			expectedError: false,
		},
		{
			name: "Required deployment not ready",
			deployments: []*appsv1.Deployment{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-apiserver",
						Namespace: hcpNamespace,
					},
					Status: appsv1.DeploymentStatus{
						AvailableReplicas: 0,
						Replicas:          1,
					},
				},
			},
			statefulsets: []*appsv1.StatefulSet{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "etcd",
						Namespace: hcpNamespace,
					},
					Status: appsv1.StatefulSetStatus{
						ReadyReplicas: 1,
						Replicas:      1,
					},
				},
			},
			expectedError: true,
		},
		{
			name:        "ETCD not ready",
			deployments: []*appsv1.Deployment{},
			statefulsets: []*appsv1.StatefulSet{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "etcd",
						Namespace: hcpNamespace,
					},
					Status: appsv1.StatefulSetStatus{
						ReadyReplicas: 0,
						Replicas:      1,
					},
				},
			},
			expectedError: true,
		},
		{
			name:        "Deployment not found, accessing the handleDeploymentValidationFailure function",
			deployments: []*appsv1.Deployment{},
			statefulsets: []*appsv1.StatefulSet{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "etcd",
						Namespace: hcpNamespace,
					},
					Status: appsv1.StatefulSetStatus{
						ReadyReplicas: 1,
						Replicas:      1,
					},
				},
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new scheme and client for each test case
			scheme := runtime.NewScheme()
			err := appsv1.AddToScheme(scheme)
			g.Expect(err).NotTo(gomega.HaveOccurred())
			err = corev1.AddToScheme(scheme)
			g.Expect(err).NotTo(gomega.HaveOccurred())

			// Combine deployments and statefulsets into a single slice of objects
			objects := make([]client.Object, 0)
			for _, deployment := range tt.deployments {
				objects = append(objects, deployment)
			}
			for _, statefulset := range tt.statefulsets {
				objects = append(objects, statefulset)
			}

			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()

			// Run the validation function with both timeouts set to 5 seconds for testing
			validateFunc := ValidateHCP(5*time.Second, 5*time.Second, []string{"kube-apiserver", "kube-controller-manager"}, hcpNamespace)
			err = validateFunc(client, "")

			// Check results
			if tt.expectedError {
				g.Expect(err).To(gomega.HaveOccurred())
			} else {
				g.Expect(err).NotTo(gomega.HaveOccurred())
			}
		})
	}
}

func TestWaitForHCPDeletion(t *testing.T) {
	// Register Gomega fail handler
	gomega.RegisterTestingT(t)
	g := gomega.NewGomegaWithT(t)

	tests := []struct {
		name          string
		hcp           *hypershiftv1.HostedControlPlane
		createObj     bool
		deleteObj     bool
		timeout       time.Duration
		deleteDelay   time.Duration
		expectedError bool
		errorContains string
	}{
		{
			name: "HCP already deleted",
			hcp: &hypershiftv1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hc",
					Namespace: "clusters-test-hc",
				},
			},
			createObj:     false,
			deleteObj:     false,
			timeout:       Wait10Min,
			deleteDelay:   0,
			expectedError: false,
		},
		{
			name: "HCP deleted during wait",
			hcp: &hypershiftv1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hc",
					Namespace: "clusters-test-hc",
				},
			},
			createObj:     true,
			deleteObj:     true,
			timeout:       Wait10Min,
			deleteDelay:   WaitForNextCheckTimeout,
			expectedError: false,
		},
		{
			name: "HCP not deleted within timeout",
			hcp: &hypershiftv1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hc",
					Namespace: "clusters-test-hc",
				},
			},
			createObj:     true,
			deleteObj:     false,
			timeout:       time.Second * 2,
			deleteDelay:   0,
			expectedError: true,
		},
		{
			name: "HCP with finalizers not deleted",
			hcp: &hypershiftv1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-hc",
					Namespace:  "clusters-test-hc",
					Finalizers: []string{"test-finalizer"},
				},
			},
			createObj:     true,
			deleteObj:     true,
			timeout:       time.Second * 2,
			deleteDelay:   WaitForNextCheckTimeout,
			expectedError: true,
		},
		{
			name:          "HCP is nil",
			hcp:           nil,
			createObj:     false,
			deleteObj:     false,
			timeout:       Wait10Min,
			deleteDelay:   0,
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new scheme with HostedControlPlane registered
			scheme := runtime.NewScheme()
			err := hypershiftv1.AddToScheme(scheme)
			g.Expect(err).NotTo(gomega.HaveOccurred())

			// Create a new client with the scheme
			client := fake.NewClientBuilder().WithScheme(scheme).Build()

			// Create a context with timeout
			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout)
			defer cancel()

			// Create the handler
			h := &HCHandler{
				Ctx:    ctx,
				Client: client,
			}

			// Create HCP if needed
			if tt.createObj {
				err := client.Create(context.Background(), tt.hcp)
				g.Expect(err).NotTo(gomega.HaveOccurred())
			}

			// Start deletion in background if needed
			if tt.deleteObj {
				go func() {
					time.Sleep(tt.deleteDelay)
					err := client.Delete(context.Background(), tt.hcp)
					g.Expect(err).NotTo(gomega.HaveOccurred())
				}()
			}

			// Call WaitForHCPDeletion
			err = h.WaitForHCPDeletion(tt.hcp)

			// Check results
			if tt.expectedError {
				g.Expect(err).To(gomega.HaveOccurred())
			} else {
				g.Expect(err).NotTo(gomega.HaveOccurred())
			}
		})
	}
}

func TestHandleDeploymentValidationFailure(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	namespace := "test-namespace"
	deployments := []string{"test-deployment"}
	timeout := 5 * time.Second

	tests := []struct {
		name          string
		pods          []*corev1.Pod
		deployments   []*appsv1.Deployment
		expectedError bool
	}{
		{
			name: "Non-running pods are deleted and deployments become ready",
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "stuck-pod",
						Namespace: namespace,
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodPending,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "running-pod",
						Namespace: namespace,
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
					},
				},
			},
			deployments: []*appsv1.Deployment{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-deployment",
						Namespace: namespace,
					},
					Status: appsv1.DeploymentStatus{
						Replicas:          1,
						AvailableReplicas: 1,
					},
				},
			},
			expectedError: false,
		},
		{
			name: "Deployment not ready after timeout",
			pods: []*corev1.Pod{},
			deployments: []*appsv1.Deployment{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-deployment",
						Namespace: namespace,
					},
					Status: appsv1.DeploymentStatus{
						Replicas:          1,
						AvailableReplicas: 0,
					},
				},
			},
			expectedError: true,
		},
		{
			name:          "No pods or deployments found",
			pods:          []*corev1.Pod{},
			deployments:   []*appsv1.Deployment{},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new scheme and client for each test case
			scheme := runtime.NewScheme()
			err := corev1.AddToScheme(scheme)
			g.Expect(err).NotTo(gomega.HaveOccurred())
			err = appsv1.AddToScheme(scheme)
			g.Expect(err).NotTo(gomega.HaveOccurred())

			// Create objects for the fake client
			objects := []client.Object{}
			for _, pod := range tt.pods {
				objects = append(objects, pod)
			}
			for _, deployment := range tt.deployments {
				objects = append(objects, deployment)
			}

			// Create fake client with objects
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objects...).
				Build()

			// Create context with timeout
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			// Call the function
			err = handleDeploymentValidationFailure(ctx, fakeClient, namespace, deployments, timeout)

			// Check results
			if tt.expectedError {
				g.Expect(err).To(gomega.HaveOccurred())
			} else {
				g.Expect(err).NotTo(gomega.HaveOccurred())
			}

			// Verify pods were deleted if necessary
			for _, pod := range tt.pods {
				if pod.Status.Phase != corev1.PodRunning {
					err := fakeClient.Get(ctx, types.NamespacedName{
						Name:      pod.Name,
						Namespace: pod.Namespace,
					}, &corev1.Pod{})
					g.Expect(apierrors.IsNotFound(err)).To(gomega.BeTrue())
				}
			}
		})
	}
}

func TestValidateETCD(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	hostedClusterName := "test-hc"
	hcpNamespace := GetHCPNamespace(hostedClusterName, ClustersNamespace)
	timeout := 5 * time.Second

	// Define test cases
	tests := []struct {
		name          string
		statefulsets  []*appsv1.StatefulSet
		expectedError bool
	}{
		{
			name: "ETCD ready",
			statefulsets: []*appsv1.StatefulSet{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "etcd",
						Namespace: hcpNamespace,
					},
					Status: appsv1.StatefulSetStatus{
						ReadyReplicas: 1,
						Replicas:      1,
					},
				},
			},
			expectedError: false,
		},
		{
			name: "ETCD not ready",
			statefulsets: []*appsv1.StatefulSet{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "etcd",
						Namespace: hcpNamespace,
					},
					Status: appsv1.StatefulSetStatus{
						ReadyReplicas: 0,
						Replicas:      1,
					},
				},
			},
			expectedError: true,
		},
		{
			name:          "ETCD not found",
			statefulsets:  []*appsv1.StatefulSet{},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new scheme and client for each test case
			scheme := runtime.NewScheme()
			err := appsv1.AddToScheme(scheme)
			g.Expect(err).NotTo(gomega.HaveOccurred())

			// Create objects for the fake client
			objects := []client.Object{}
			for _, statefulset := range tt.statefulsets {
				objects = append(objects, statefulset)
			}

			// Create fake client with objects
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objects...).
				Build()

			// Create context with timeout
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			// Call the function
			err = ValidateETCD(ctx, fakeClient, hcpNamespace, timeout)

			// Check results
			if tt.expectedError {
				g.Expect(err).To(gomega.HaveOccurred())
			} else {
				g.Expect(err).NotTo(gomega.HaveOccurred())
			}
		})
	}
}

func TestValidateDeployments(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	namespace := "test-namespace"
	deployments := []string{"test-deployment"}
	timeout := 5 * time.Second

	tests := []struct {
		name          string
		deployments   []*appsv1.Deployment
		expectedError bool
	}{
		{
			name: "All deployments ready",
			deployments: []*appsv1.Deployment{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-deployment",
						Namespace: namespace,
					},
					Status: appsv1.DeploymentStatus{
						AvailableReplicas: 1,
						Replicas:          1,
					},
				},
			},
			expectedError: false,
		},
		{
			name: "Deployment not ready",
			deployments: []*appsv1.Deployment{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-deployment",
						Namespace: namespace,
					},
					Status: appsv1.DeploymentStatus{
						AvailableReplicas: 0,
						Replicas:          1,
					},
				},
			},
			expectedError: true,
		},
		{
			name:          "Deployment not found",
			deployments:   []*appsv1.Deployment{},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new scheme and client for each test case
			scheme := runtime.NewScheme()
			err := appsv1.AddToScheme(scheme)
			g.Expect(err).NotTo(gomega.HaveOccurred())
			err = corev1.AddToScheme(scheme)
			g.Expect(err).NotTo(gomega.HaveOccurred())

			// Create objects for the fake client
			objects := []client.Object{}
			for _, deployment := range tt.deployments {
				objects = append(objects, deployment)
			}

			// Create fake client with objects
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objects...).
				Build()

			// Create context with timeout
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			// Call the function
			err = ValidateDeployments(ctx, fakeClient, namespace, deployments, timeout)

			// Check results
			if tt.expectedError {
				g.Expect(err).To(gomega.HaveOccurred())
			} else {
				g.Expect(err).NotTo(gomega.HaveOccurred())
			}
		})
	}
}
