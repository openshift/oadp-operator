package lib

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// realMustGatherSummaryUnsupportedOverridesOnly is the actual "## Errors"
// section (and the section immediately following it) captured from a real
// CI failure: openshift/oadp-operator PR#2404's ci/prow/5.1-e2e-test-cli-aws
// run (2093596791407644672), which failed with "expected no errors in
// must-gather Errors section" purely because the shared dpaCR carried a
// spec.unsupportedOverrides entry (a test-time kdm-controller image
// override, unrelated to the CLI suite that failed).
const realMustGatherSummaryUnsupportedOverridesOnly = `## Errors

⚠️ DataProtectionApplication **ts-velero-test** in **openshift-adp** namespace is using **unsupportedOverrides**



## Cluster information

| Cluster ID | OpenShift version | Cloud provider | Architecture |
| ---------- | ----------------- | -------------- | ------------ |
| 65162e20 | 5.1.0-0.nightly-2026-08-27-012048 | AWS | linux/amd64 |
`

func Test_mustGatherErrorsAreOnlyKnownBenignWarnings_realUnsupportedOverridesCapture(t *testing.T) {
	if !mustGatherErrorsAreOnlyKnownBenignWarnings(realMustGatherSummaryUnsupportedOverridesOnly) {
		t.Fatalf("expected the real captured unsupportedOverrides-only Errors section to be treated as benign")
	}
}

func Test_mustGatherErrorsAreOnlyKnownBenignWarnings_realErrorStillFails(t *testing.T) {
	summary := `## Errors

⚠️ DataProtectionApplication **ts-velero-test** in **openshift-adp** namespace is using **unsupportedOverrides**
❌ Velero pod is in CrashLoopBackOff

## Cluster information
`
	if mustGatherErrorsAreOnlyKnownBenignWarnings(summary) {
		t.Fatalf("expected a genuine error line alongside the benign warning to still be treated as a real error")
	}
}

func Test_mustGatherErrorsAreOnlyKnownBenignWarnings_noErrorsSectionFailsSafe(t *testing.T) {
	summary := `## Cluster information

| Cluster ID | OpenShift version |
|`
	if mustGatherErrorsAreOnlyKnownBenignWarnings(summary) {
		t.Fatalf("expected a missing Errors section to fail safe (treated as a real error)")
	}
}

func Test_shouldRetryRouteEndpointError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "503", err: errors.New("HTTP request failed with status code: 503"), want: true},
		{name: "connection refused", err: errors.New("connect: connection refused"), want: true},
		{name: "timeout", err: errors.New("dial tcp: i/o timeout"), want: true},
		{name: "route not found", err: errors.New("Service route not found"), want: false},
		{name: "nil", err: nil, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldRetryRouteEndpointError(tc.err); got != tc.want {
				t.Fatalf("shouldRetryRouteEndpointError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func Test_getAppEndpointURLAndProxyParams_proxyErrorIncludesRouteError(t *testing.T) {
	oldGetRoute := getRouteEndpointURLForApp
	oldGetPod := getFirstPodByLabelForApp
	oldSleep := sleepForAppRouteRetry
	oldMax := appRouteRetryMaxAttempts
	defer func() {
		getRouteEndpointURLForApp = oldGetRoute
		getFirstPodByLabelForApp = oldGetPod
		sleepForAppRouteRetry = oldSleep
		appRouteRetryMaxAttempts = oldMax
	}()

	appRouteRetryMaxAttempts = 1
	sleepForAppRouteRetry = func(_ context.Context, _ time.Duration) error { return nil }
	getRouteEndpointURLForApp = func(_ client.Client, _ string, _ string, _ time.Duration) (string, error) {
		return "", errors.New("HTTP request failed with status code: 503")
	}
	getFirstPodByLabelForApp = func(_ *kubernetes.Clientset, _ string, _ string) (*corev1.Pod, error) {
		return nil, errors.New("no Pod found")
	}

	_, _, err := getAppEndpointURLAndProxyParams(nil, nil, nil, "ns", "svc", "route")
	if err == nil {
		t.Fatalf("expected error when both route and proxy fallback fail")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "route endpoint remained unavailable") || !strings.Contains(errMsg, "no Pod found") {
		t.Fatalf("expected composed error to include both route and proxy causes, got: %v", err)
	}
}

// Test_getAppEndpointURLAndProxyParams_retriesTransient503ThenSucceeds
// exercises the actual retry loop (max attempts > 1): a transient 503 on the
// first call followed by success on the second must return the direct route
// result with no proxy fallback, and the route function/sleep must each be
// called exactly once for the one recovered failure.
func Test_getAppEndpointURLAndProxyParams_retriesTransient503ThenSucceeds(t *testing.T) {
	oldGetRoute := getRouteEndpointURLForApp
	oldGetPod := getFirstPodByLabelForApp
	oldSleep := sleepForAppRouteRetry
	oldMax := appRouteRetryMaxAttempts
	defer func() {
		getRouteEndpointURLForApp = oldGetRoute
		getFirstPodByLabelForApp = oldGetPod
		sleepForAppRouteRetry = oldSleep
		appRouteRetryMaxAttempts = oldMax
	}()

	appRouteRetryMaxAttempts = 3
	routeCalls := 0
	sleepCalls := 0
	sleepForAppRouteRetry = func(_ context.Context, _ time.Duration) error {
		sleepCalls++
		return nil
	}
	getRouteEndpointURLForApp = func(_ client.Client, _ string, _ string, _ time.Duration) (string, error) {
		routeCalls++
		if routeCalls == 1 {
			return "", errors.New("HTTP request failed with status code: 503")
		}
		return "http://recovered-route.example.com", nil
	}
	getFirstPodByLabelForApp = func(_ *kubernetes.Clientset, _ string, _ string) (*corev1.Pod, error) {
		t.Fatalf("proxy fallback must not be attempted when the route recovers within the retry budget")
		return nil, nil
	}

	url, proxyParams, err := getAppEndpointURLAndProxyParams(nil, nil, nil, "ns", "svc", "route")
	if err != nil {
		t.Fatalf("expected success after transient 503 recovers, got err=%v", err)
	}
	if url != "http://recovered-route.example.com" {
		t.Fatalf("expected the direct route URL to be returned, got %q", url)
	}
	if proxyParams != nil {
		t.Fatalf("expected no proxy params when the direct route succeeds, got %+v", proxyParams)
	}
	if routeCalls != 2 {
		t.Fatalf("expected exactly 2 route attempts (1 failure + 1 success), got %d", routeCalls)
	}
	if sleepCalls != 1 {
		t.Fatalf("expected exactly 1 backoff sleep between the failed and successful attempt, got %d", sleepCalls)
	}
}

// Test_getAppEndpointURLAndProxyParams_exhaustsRetriesThenFallsBackToProxy
// exercises the bounded-exhaustion path: persistent transient 503s across
// every attempt must exhaust appRouteRetryMaxAttempts, then fall back to the
// proxy pod and return its params.
func Test_getAppEndpointURLAndProxyParams_exhaustsRetriesThenFallsBackToProxy(t *testing.T) {
	oldGetRoute := getRouteEndpointURLForApp
	oldGetPod := getFirstPodByLabelForApp
	oldSleep := sleepForAppRouteRetry
	oldMax := appRouteRetryMaxAttempts
	defer func() {
		getRouteEndpointURLForApp = oldGetRoute
		getFirstPodByLabelForApp = oldGetPod
		sleepForAppRouteRetry = oldSleep
		appRouteRetryMaxAttempts = oldMax
	}()

	appRouteRetryMaxAttempts = 3
	routeCalls := 0
	sleepCalls := 0
	sleepForAppRouteRetry = func(_ context.Context, _ time.Duration) error {
		sleepCalls++
		return nil
	}
	getRouteEndpointURLForApp = func(_ client.Client, _ string, _ string, _ time.Duration) (string, error) {
		routeCalls++
		return "", errors.New("HTTP request failed with status code: 503")
	}
	getFirstPodByLabelForApp = func(kubeClient *kubernetes.Clientset, namespace string, labelSelector string) (*corev1.Pod, error) {
		return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "curl-tool-pod"}}, nil
	}

	url, proxyParams, err := getAppEndpointURLAndProxyParams(nil, nil, nil, "ns", "svc", "route")
	if err != nil {
		t.Fatalf("expected proxy fallback to succeed after exhausted retries, got err=%v", err)
	}
	if proxyParams == nil || proxyParams.PodName != "curl-tool-pod" {
		t.Fatalf("expected proxy params referencing the fallback pod, got %+v", proxyParams)
	}
	if url == "" {
		t.Fatalf("expected a non-empty internal service endpoint URL for the proxy path")
	}
	if routeCalls != appRouteRetryMaxAttempts {
		t.Fatalf("expected exactly %d route attempts (fully exhausted), got %d", appRouteRetryMaxAttempts, routeCalls)
	}
	if sleepCalls != appRouteRetryMaxAttempts-1 {
		t.Fatalf("expected %d backoff sleeps (one between each attempt, none after the last), got %d", appRouteRetryMaxAttempts-1, sleepCalls)
	}
}
