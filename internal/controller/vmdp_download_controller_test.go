package controller

import (
	"context"
	"testing"

	consolev1 "github.com/openshift/api/console/v1"
	routev1 "github.com/openshift/api/route/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestVMDPDownloadSetup_StartSkipsWhenConsoleNotAvailable(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, routev1.Install(scheme))
	require.NoError(t, consolev1.Install(scheme))

	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{
		appsv1.SchemeGroupVersion,
		corev1.SchemeGroupVersion,
		routev1.GroupVersion,
	})
	mapper.Add(appsv1.SchemeGroupVersion.WithKind("Deployment"), meta.RESTScopeNamespace)
	mapper.Add(corev1.SchemeGroupVersion.WithKind("Service"), meta.RESTScopeNamespace)
	mapper.Add(routev1.GroupVersion.WithKind("Route"), meta.RESTScopeNamespace)

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRESTMapper(mapper).
		Build()

	setup := &VMDPDownloadSetup{
		Client:            client,
		Namespace:         "openshift-adp",
		OperatorName:      "openshift-adp-controller-manager",
		OperatorNamespace: "openshift-adp",
	}

	err := setup.Start(context.Background())
	require.NoError(t, err)

	deploy := &appsv1.Deployment{}
	err = client.Get(context.Background(), types.NamespacedName{
		Name:      vmdpServerDeploymentName,
		Namespace: "openshift-adp",
	}, deploy)
	assert.Error(t, err, "deployment should not exist")

	svc := &corev1.Service{}
	err = client.Get(context.Background(), types.NamespacedName{
		Name:      vmdpServerServiceName,
		Namespace: "openshift-adp",
	}, svc)
	assert.Error(t, err, "service should not exist")
}
