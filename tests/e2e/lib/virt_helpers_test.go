package lib

import (
	"testing"
)

func TestParseKubeVersion(t *testing.T) {
	tests := []struct {
		name      string
		major     string
		minor     string
		wantMajor int
		wantMinor int
		wantOK    bool
	}{
		{name: "plain", major: "1", minor: "35", wantMajor: 1, wantMinor: 35, wantOK: true},
		{name: "plus suffix", major: "1", minor: "35+", wantMajor: 1, wantMinor: 35, wantOK: true},
		{name: "git suffix", major: "1", minor: "36+gc1234ab", wantMajor: 1, wantMinor: 36, wantOK: true},
		{name: "bad major", major: "v1", minor: "35", wantOK: false},
		{name: "bad minor", major: "1", minor: "x", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			major, minor, ok := parseKubeVersion(tt.major, tt.minor)
			if ok != tt.wantOK {
				t.Fatalf("parseKubeVersion(%q, %q) ok = %v, want %v", tt.major, tt.minor, ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if major != tt.wantMajor || minor != tt.wantMinor {
				t.Fatalf("parseKubeVersion(%q, %q) = %d.%d, want %d.%d", tt.major, tt.minor, major, minor, tt.wantMajor, tt.wantMinor)
			}
		})
	}
}

func TestResolveHcoIndexTagForKubeVersion(t *testing.T) {
	tests := []struct {
		name     string
		indexTag string
		major    int
		minor    int
		want     string
	}{
		// OpenShift 4.22 = kube 1.35, below HCO 1.20's minKubeVersion 1.36:
		// nightly must fall back to a pinned installable release.
		{name: "nightly on kube 1.35 falls back", indexTag: "nightly", major: 1, minor: 35, want: hcoNightlyFallbackTag},
		// OpenShift 4.23 = kube 1.36 satisfies minKubeVersion: keep nightly.
		{name: "nightly on kube 1.36 kept", indexTag: "nightly", major: 1, minor: 36, want: "nightly"},
		{name: "nightly on kube 1.37 kept", indexTag: "nightly", major: 1, minor: 37, want: "nightly"},
		// Explicit pins are user intent and must never be rewritten.
		{name: "pinned tag untouched on old kube", indexTag: "1.18.0", major: 1, minor: 35, want: "1.18.0"},
		{name: "pinned tag untouched on new kube", indexTag: "1.19.0", major: 1, minor: 37, want: "1.19.0"},
		{name: "empty tag untouched", indexTag: "", major: 1, minor: 35, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveHcoIndexTagForKubeVersion(tt.indexTag, tt.major, tt.minor)
			if got != tt.want {
				t.Fatalf("resolveHcoIndexTagForKubeVersion(%q, %d.%d) = %q, want %q", tt.indexTag, tt.major, tt.minor, got, tt.want)
			}
		})
	}
}
