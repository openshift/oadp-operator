package lib

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/utils/ptr"
)

const (
	MinioDeploymentName    = "minio-cacert-test"
	MinioServiceName       = "minio-cacert-test"
	MinioTLSSecretName     = "minio-tls-cacert-test"
	MinioBucketName        = "cacert-test-bucket"
	MinioAccessKey         = "minioadmin"
	MinioSecretKey         = "minioadmin"
	minioPort              = int32(9000)
	minioNetworkPolicyName = "minio-cacert-test-network-policy"

	// minioImage pins a specific minio release. Update this when bumping minio.
	// Use a digest or immutable tag to keep test runs deterministic.
	minioImage = "docker.io/minio/minio:RELEASE.2025-04-22T22-12-26Z"
)

// GenerateSelfSignedCA creates a CA certificate and private key.
func GenerateSelfSignedCA() (caPEM, caKeyPEM []byte, err error) {
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generating CA key: %w", err)
	}

	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName:   "OADP E2E Test CA",
			Organization: []string{"OADP E2E"},
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}

	caBytes, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, fmt.Errorf("creating CA cert: %w", err)
	}

	caPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caBytes})

	caKeyBytes, err := x509.MarshalECPrivateKey(caKey)
	if err != nil {
		return nil, nil, fmt.Errorf("marshaling CA key: %w", err)
	}
	caKeyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: caKeyBytes})

	return caPEM, caKeyPEM, nil
}

// GenerateServerCert creates a TLS server certificate signed by the given CA.
func GenerateServerCert(caPEM, caKeyPEM []byte, dnsNames []string) (certPEM, keyPEM []byte, err error) {
	if len(dnsNames) == 0 {
		return nil, nil, fmt.Errorf("dnsNames must not be empty")
	}
	caBlock, _ := pem.Decode(caPEM)
	if caBlock == nil {
		return nil, nil, fmt.Errorf("failed to decode CA PEM")
	}
	caCert, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing CA cert: %w", err)
	}

	caKeyBlock, _ := pem.Decode(caKeyPEM)
	if caKeyBlock == nil {
		return nil, nil, fmt.Errorf("failed to decode CA key PEM")
	}
	caKey, err := x509.ParseECPrivateKey(caKeyBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing CA key: %w", err)
	}

	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generating server key: %w", err)
	}

	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: dnsNames[0]},
		DNSNames:     dnsNames,
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, fmt.Errorf("creating server cert: %w", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certBytes})

	keyBytes, err := x509.MarshalECPrivateKey(serverKey)
	if err != nil {
		return nil, nil, fmt.Errorf("marshaling server key: %w", err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})

	return certPEM, keyPEM, nil
}

// DeployMinioWithTLS deploys minio with TLS in the given namespace and waits for it to be ready.
// Returns the in-cluster HTTPS service URL.
func DeployMinioWithTLS(ctx context.Context, c *kubernetes.Clientset, namespace string, certPEM, keyPEM []byte) (string, error) {
	// Delete any leftover TLS secret from a previous interrupted run before creating
	// a fresh one; a stale secret would cause Velero to fail TLS validation because
	// the serving cert would not match the newly generated CA.
	if err := c.CoreV1().Secrets(namespace).Delete(ctx, MinioTLSSecretName, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return "", fmt.Errorf("deleting stale minio TLS secret: %w", err)
	}
	// Minio expects filenames public.crt and private.key in --certs-dir
	tlsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MinioTLSSecretName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"public.crt":  certPEM,
			"private.key": keyPEM,
		},
	}
	if _, err := c.CoreV1().Secrets(namespace).Create(ctx, tlsSecret, metav1.CreateOptions{}); err != nil {
		return "", fmt.Errorf("creating minio TLS secret: %w", err)
	}

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MinioDeploymentName,
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(1)),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": MinioDeploymentName},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": MinioDeploymentName},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "minio",
							Image: minioImage,
							Args:  []string{"server", "/data", "--certs-dir", "/certs"},
							Env: []corev1.EnvVar{
								{Name: "MINIO_ROOT_USER", Value: MinioAccessKey},
								{Name: "MINIO_ROOT_PASSWORD", Value: MinioSecretKey},
							},
							Ports: []corev1.ContainerPort{
								{ContainerPort: minioPort, Protocol: corev1.ProtocolTCP},
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "certs", MountPath: "/certs", ReadOnly: true},
								{Name: "data", MountPath: "/data"},
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									TCPSocket: &corev1.TCPSocketAction{
										Port: intstr.FromInt32(minioPort),
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       5,
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "certs",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: MinioTLSSecretName,
								},
							},
						},
						{
							Name: "data",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
	}
	if _, err := c.AppsV1().Deployments(namespace).Create(ctx, dep, metav1.CreateOptions{}); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return "", fmt.Errorf("creating minio deployment: %w", err)
		}
		// Deployment exists from a previous run. The TLS Secret was already replaced
		// above, but the running pod won't pick up the new cert without a restart.
		// Force a rollout by annotating the pod template (same as kubectl rollout restart).
		patch := fmt.Sprintf(`{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":%q}}}}}`,
			time.Now().UTC().Format(time.RFC3339))
		if _, err := c.AppsV1().Deployments(namespace).Patch(ctx, MinioDeploymentName,
			types.MergePatchType, []byte(patch), metav1.PatchOptions{}); err != nil {
			return "", fmt.Errorf("restarting minio deployment: %w", err)
		}
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MinioServiceName,
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": MinioDeploymentName},
			Ports: []corev1.ServicePort{
				{Port: minioPort, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromInt32(minioPort)},
			},
		},
	}
	if _, err := c.CoreV1().Services(namespace).Create(ctx, svc, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return "", fmt.Errorf("creating minio service: %w", err)
	}

	// The OADP operator's NetworkPolicy suite installs a default-deny policy on this
	// namespace (see internal/controller/networkpolicy.go), which only allow-lists
	// OADP-managed workloads. This minio pod is a test-only fixture unknown to those
	// policies, so it needs its own explicit allow rule for Velero to reach it over TLS.
	protoTCP := corev1.ProtocolTCP
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      minioNetworkPolicyName,
			Namespace: namespace,
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"app": MinioDeploymentName},
			},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					Ports: []networkingv1.NetworkPolicyPort{
						{Protocol: &protoTCP, Port: &intstr.IntOrString{Type: intstr.Int, IntVal: minioPort}},
					},
				},
			},
		},
	}
	if _, err := c.NetworkingV1().NetworkPolicies(namespace).Create(ctx, np, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return "", fmt.Errorf("creating minio NetworkPolicy: %w", err)
	}

	if err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 3*time.Minute, true, func(ctx context.Context) (bool, error) {
		d, err := c.AppsV1().Deployments(namespace).Get(ctx, MinioDeploymentName, metav1.GetOptions{})
		if err != nil {
			return false, nil
		}
		return d.Status.ReadyReplicas >= 1, nil
	}); err != nil {
		return "", fmt.Errorf("waiting for minio to be ready: %w", err)
	}

	return fmt.Sprintf("https://%s.%s.svc:%d", MinioServiceName, namespace, minioPort), nil
}

// CreateMinioBucket execs into the running minio pod and uses the bundled mc binary to
// create the bucket. Using --insecure here is intentional — TLS validation is the job of
// Velero (the system under test), not of the bucket-creation helper.
func CreateMinioBucket(ctx context.Context, c *kubernetes.Clientset, cfg *rest.Config, namespace, bucket string) error {
	// Wait until a minio pod is Running
	var podName string
	if err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
		pods, err := c.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "app=" + MinioDeploymentName,
			FieldSelector: "status.phase=Running",
		})
		if err != nil || len(pods.Items) == 0 {
			return false, nil
		}
		for _, p := range pods.Items {
			if p.Status.Phase == corev1.PodRunning {
				podName = p.Name
				return true, nil
			}
		}
		return false, nil
	}); err != nil {
		return fmt.Errorf("waiting for minio pod to be Running: %w", err)
	}

	// --insecure is a global mc flag; pass it before each subcommand so both
	// alias-set and mb skip TLS verification when connecting to localhost.
	cmd := []string{"sh", "-c", fmt.Sprintf(
		"mc --insecure alias set local https://localhost:%d %s %s && mc --insecure mb --ignore-existing local/%s",
		minioPort, MinioAccessKey, MinioSecretKey, bucket,
	)}

	req := c.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Command: cmd,
			Stdout:  true,
			Stderr:  true,
		}, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(cfg, "POST", req.URL())
	if err != nil {
		return fmt.Errorf("creating executor: %w", err)
	}

	var stdout, stderr bytes.Buffer
	if err := executor.StreamWithContext(ctx, remotecommand.StreamOptions{Stdout: &stdout, Stderr: &stderr}); err != nil {
		return fmt.Errorf("exec mc mb: %w (stdout: %s, stderr: %s)", err, stdout.String(), stderr.String())
	}
	return nil
}

// DeleteMinioResources removes the minio deployment, service, TLS secret, and NetworkPolicy.
func DeleteMinioResources(ctx context.Context, c *kubernetes.Clientset, namespace string) {
	background := metav1.DeletePropagationBackground
	deleteOpts := metav1.DeleteOptions{PropagationPolicy: &background}
	_ = c.AppsV1().Deployments(namespace).Delete(ctx, MinioDeploymentName, deleteOpts)
	_ = c.CoreV1().Services(namespace).Delete(ctx, MinioServiceName, metav1.DeleteOptions{})
	_ = c.CoreV1().Secrets(namespace).Delete(ctx, MinioTLSSecretName, metav1.DeleteOptions{})
	_ = c.NetworkingV1().NetworkPolicies(namespace).Delete(ctx, minioNetworkPolicyName, metav1.DeleteOptions{})
}
