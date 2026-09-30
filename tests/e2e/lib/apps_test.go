package lib

import (
	"errors"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
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
	sleepForAppRouteRetry = func(_ time.Duration) {}
	getRouteEndpointURLForApp = func(_ client.Client, _ string, _ string) (string, error) {
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
