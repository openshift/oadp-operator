package lib

import (
	"reflect"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
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
`
	if mustGatherErrorsAreOnlyKnownBenignWarnings(summary) {
		t.Fatalf("expected a missing Errors section to fail safe (treated as a real error)")
	}
}

func Test_GetNamespaceEventMessagesSince_scopesToWindowAndFormats(t *testing.T) {
	// Timestamps anchored to the scenario this filtering exists for: the
	// "since" instant is a backup's creation time; a known-flake event from
	// an EARLIER backup in the same VM namespace (Kubernetes event TTL ~1h)
	// predates it and must be filtered out, while events from the current
	// backup's own lifecycle must come through in "Reason: Message" form --
	// the exact shape CheckIfFlakeOccurred's patterns are matched against.
	// since carries a fractional second on purpose: callers pass time.Now(),
	// and the boundary event below was emitted at :58.750 -- Kubernetes
	// serialized its LastTimestamp at whole-second precision as :58, which a
	// naive fractional cutoff would wrongly filter out.
	since := time.Date(2026, 11, 10, 1, 9, 58, 900000000, time.UTC) // backup created
	earlier := since.Add(-2 * time.Minute)                          // previous backup's #18957 event
	boundary := since.Truncate(time.Second)                         // emitted at :58.750, serialized as :58
	skewed := since.Add(-1500 * time.Millisecond)                   // in-window, but the runner's clock runs ~1.5s ahead of the apiserver
	inWindow := since.Add(25 * time.Second)                         // this backup's #18957 event
	clientset := fake.NewSimpleClientset(
		&corev1.Event{
			ObjectMeta:    metav1.ObjectMeta{Name: "stale-18957", Namespace: "cirros-test"},
			Reason:        "VirtualMachineBackupFailed",
			Message:       "VMI backup status was lost",
			LastTimestamp: metav1.Time{Time: earlier},
		},
		&corev1.Event{
			ObjectMeta:    metav1.ObjectMeta{Name: "boundary-18957", Namespace: "cirros-test"},
			Reason:        "VirtualMachineBackupFailed",
			Message:       "VMI backup status was lost",
			LastTimestamp: metav1.Time{Time: boundary},
		},
		&corev1.Event{
			ObjectMeta:    metav1.ObjectMeta{Name: "skewed-18957", Namespace: "cirros-test"},
			Reason:        "VirtualMachineBackupFailed",
			Message:       "VMI backup status was lost",
			LastTimestamp: metav1.Time{Time: skewed},
		},
		&corev1.Event{
			ObjectMeta:    metav1.ObjectMeta{Name: "current-18957", Namespace: "cirros-test"},
			Reason:        "VirtualMachineBackupFailed",
			Message:       "VMI backup status was lost",
			LastTimestamp: metav1.Time{Time: inWindow},
		},
		&corev1.Event{
			// EventTime-only (no LastTimestamp) exercises the fallback path.
			ObjectMeta: metav1.ObjectMeta{Name: "current-healthy", Namespace: "cirros-test"},
			Reason:     "VirtualMachineBackupCompletedSuccessfully",
			Message:    "Successfully completed VirtualMachineBackup",
			EventTime:  metav1.MicroTime{Time: inWindow},
		},
	)

	got := GetNamespaceEventMessagesSince(clientset.CoreV1(), "cirros-test", since)

	want := []string{
		// API list order (object-name order): boundary-18957, current-18957,
		// current-healthy, skewed-18957.
		"VirtualMachineBackupFailed: VMI backup status was lost",
		"VirtualMachineBackupFailed: VMI backup status was lost",
		"VirtualMachineBackupCompletedSuccessfully: Successfully completed VirtualMachineBackup",
		"VirtualMachineBackupFailed: VMI backup status was lost",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected only in-window events, formatted \"Reason: Message\", got:\n%q", got)
	}

	// The filtered output must be sufficient on its own for CheckIfFlakeOccurred
	// to match the known #18957 signature -- the wiring this whole scoping
	// exists to protect.
	if !CheckIfFlakeOccurred(got) {
		t.Fatalf("expected in-window event texts to match the registered kubevirt/kubevirt#18957 pattern")
	}
}
