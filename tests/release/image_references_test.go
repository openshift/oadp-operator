package release

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to determine test file path")
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(filename)))
}

// branchVersion returns the oadp-X.Y version from the current branch.
// Checks PULL_BASE_REF first (set by Prow to the PR target branch),
// then falls back to the local git branch name.
// Returns empty string if neither matches the oadp-X.Y pattern.
func branchVersion(t *testing.T) string {
	t.Helper()
	if ref := os.Getenv("PULL_BASE_REF"); strings.HasPrefix(ref, "oadp-") {
		return ref
	}
	out, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		t.Fatalf("failed to get current branch: %v", err)
	}
	branch := strings.TrimSpace(string(out))
	if !strings.HasPrefix(branch, "oadp-") {
		return ""
	}
	return branch
}

// TestImageReferencesTagVersion is the Prow-facing entry point that validates
// image tags match the release branch version.
func TestImageReferencesTagVersion(t *testing.T) {
	version := branchVersion(t)
	if version == "" {
		t.Skip("not on an oadp-X.Y release branch")
	}

	root := repoRoot(t)
	imageRefsPath := filepath.Join(root, "bundle", "image-references")
	if _, err := os.Stat(imageRefsPath); os.IsNotExist(err) {
		t.Skip("bundle/image-references does not exist (only present on release branches)")
	}

	irData, err := os.ReadFile(imageRefsPath)
	if err != nil {
		t.Fatalf("failed to read image-references: %v", err)
	}

	// Add tag names here for images that use their own versioning scheme
	// (e.g. external dependencies not built from this repo).
	exceptions := []string{}

	if errs := ValidateImageReferencesTagVersion(irData, version, exceptions); len(errs) > 0 {
		for _, err := range errs {
			t.Error(err)
		}
	}
}

// TestImageReferencesMatchCSV is the Prow-facing entry point that validates
// real bundle files on release branches.
func TestImageReferencesMatchCSV(t *testing.T) {
	root := repoRoot(t)

	imageRefsPath := filepath.Join(root, "bundle", "image-references")
	if _, err := os.Stat(imageRefsPath); os.IsNotExist(err) {
		t.Skip("bundle/image-references does not exist (only present on release branches)")
	}

	irData, err := os.ReadFile(imageRefsPath)
	if err != nil {
		t.Fatalf("failed to read image-references: %v", err)
	}

	csvPath := filepath.Join(root, "bundle", "manifests", "oadp-operator.clusterserviceversion.yaml")
	csvData, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatalf("failed to read CSV: %v", err)
	}

	if errs := ValidateImageReferencesMatchCSV(irData, csvData); len(errs) > 0 {
		for _, err := range errs {
			t.Error(err)
		}
	}
}

func TestValidateImageReferencesMatchCSV(t *testing.T) {
	tests := []struct {
		name      string
		imageRefs string
		csv       string
		wantErrs  int
		wantMsg   string
	}{
		{
			name: "all images present as RELATED_IMAGE",
			imageRefs: `spec:
  tags:
  - name: velero
    from:
      kind: DockerImage
      name: registry.example.com/velero:latest
  - name: aws-plugin
    from:
      kind: DockerImage
      name: registry.example.com/aws-plugin:latest`,
			csv: `spec:
  install:
    spec:
      deployments:
      - spec:
          template:
            spec:
              containers:
              - image: registry.example.com/operator:latest
                env:
                - name: RELATED_IMAGE_VELERO
                  value: registry.example.com/velero:latest
                - name: RELATED_IMAGE_AWS_PLUGIN
                  value: registry.example.com/aws-plugin:latest`,
			wantErrs: 0,
		},
		{
			name: "missing image produces error",
			imageRefs: `spec:
  tags:
  - name: velero
    from:
      kind: DockerImage
      name: registry.example.com/velero:latest
  - name: missing-plugin
    from:
      kind: DockerImage
      name: registry.example.com/missing:latest`,
			csv: `spec:
  install:
    spec:
      deployments:
      - spec:
          template:
            spec:
              containers:
              - image: registry.example.com/operator:latest
                env:
                - name: RELATED_IMAGE_VELERO
                  value: registry.example.com/velero:latest`,
			wantErrs: 1,
			wantMsg:  "missing-plugin",
		},
		{
			name: "operator container image is skipped",
			imageRefs: `spec:
  tags:
  - name: oadp-operator
    from:
      kind: DockerImage
      name: registry.example.com/operator:latest`,
			csv: `spec:
  install:
    spec:
      deployments:
      - spec:
          template:
            spec:
              containers:
              - image: registry.example.com/operator:latest
                env: []`,
			wantErrs: 0,
		},
		{
			name:      "empty tags produces error",
			imageRefs: `spec: { tags: [] }`,
			csv:       `spec: {}`,
			wantErrs:  1,
			wantMsg:   "no tags",
		},
		{
			name:      "invalid image-references YAML produces error",
			imageRefs: `{{{`,
			csv:       `spec: {}`,
			wantErrs:  1,
			wantMsg:   "failed to parse image-references",
		},
		{
			name: "invalid CSV YAML produces error",
			imageRefs: `spec:
  tags:
  - name: velero
    from:
      kind: DockerImage
      name: registry.example.com/velero:latest`,
			csv:      `{{{`,
			wantErrs: 1,
			wantMsg:  "failed to parse CSV",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateImageReferencesMatchCSV([]byte(tt.imageRefs), []byte(tt.csv))
			if len(errs) != tt.wantErrs {
				t.Fatalf("got %d errors, want %d: %v", len(errs), tt.wantErrs, errs)
			}
			if tt.wantMsg != "" {
				found := false
				for _, err := range errs {
					if strings.Contains(err.Error(), tt.wantMsg) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected error containing %q, got %v", tt.wantMsg, errs)
				}
			}
		})
	}
}

func TestValidateImageReferencesTagVersion(t *testing.T) {
	tests := []struct {
		name       string
		imageRefs  string
		version    string
		exceptions []string
		wantErrs   int
		wantMsg    string
	}{
		{
			name: "all tags match version",
			imageRefs: `spec:
  tags:
  - name: oadp-rhel9-operator
    from:
      kind: DockerImage
      name: quay.io/konveyor/oadp-operator:oadp-1.6
  - name: oadp-velero-rhel9
    from:
      kind: DockerImage
      name: quay.io/konveyor/velero:oadp-1.6`,
			version:  "oadp-1.6",
			wantErrs: 0,
		},
		{
			name: "wrong version tag produces error",
			imageRefs: `spec:
  tags:
  - name: oadp-rhel9-operator
    from:
      kind: DockerImage
      name: quay.io/konveyor/oadp-operator:oadp-1.6
  - name: oadp-velero-rhel9
    from:
      kind: DockerImage
      name: quay.io/konveyor/velero:oadp-1.5`,
			version:  "oadp-1.6",
			wantErrs: 1,
			wantMsg:  "oadp-velero-rhel9",
		},
		{
			name: "non-release tag produces error",
			imageRefs: `spec:
  tags:
  - name: oadp-kubevirt-velero-plugin-rhel9
    from:
      kind: DockerImage
      name: quay.io/konveyor/kubevirt-velero-plugin:v0.7.0`,
			version:  "oadp-1.6",
			wantErrs: 1,
			wantMsg:  "v0.7.0",
		},
		{
			name: "excepted tag is skipped",
			imageRefs: `spec:
  tags:
  - name: oadp-rhel9-operator
    from:
      kind: DockerImage
      name: quay.io/konveyor/oadp-operator:oadp-1.6
  - name: oadp-kubevirt-velero-plugin-rhel9
    from:
      kind: DockerImage
      name: quay.io/konveyor/kubevirt-velero-plugin:v0.7.0`,
			version:    "oadp-1.6",
			exceptions: []string{"oadp-kubevirt-velero-plugin-rhel9"},
			wantErrs:   0,
		},
		{
			name: "only non-excepted mismatches produce errors",
			imageRefs: `spec:
  tags:
  - name: oadp-velero-rhel9
    from:
      kind: DockerImage
      name: quay.io/konveyor/velero:oadp-1.5
  - name: oadp-kubevirt-velero-plugin-rhel9
    from:
      kind: DockerImage
      name: quay.io/konveyor/kubevirt-velero-plugin:v0.7.0`,
			version:    "oadp-1.6",
			exceptions: []string{"oadp-kubevirt-velero-plugin-rhel9"},
			wantErrs:   1,
			wantMsg:    "oadp-velero-rhel9",
		},
		{
			name: "multiple mismatches",
			imageRefs: `spec:
  tags:
  - name: image-a
    from:
      kind: DockerImage
      name: registry.example.com/a:wrong
  - name: image-b
    from:
      kind: DockerImage
      name: registry.example.com/b:also-wrong`,
			version:  "oadp-1.6",
			wantErrs: 2,
		},
		{
			name:      "empty tags produces error",
			imageRefs: `spec: { tags: [] }`,
			version:   "oadp-1.6",
			wantErrs:  1,
			wantMsg:   "no tags",
		},
		{
			name:      "invalid YAML produces error",
			imageRefs: `{{{`,
			version:   "oadp-1.6",
			wantErrs:  1,
			wantMsg:   "failed to parse image-references",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateImageReferencesTagVersion([]byte(tt.imageRefs), tt.version, tt.exceptions)
			if len(errs) != tt.wantErrs {
				t.Fatalf("got %d errors, want %d: %v", len(errs), tt.wantErrs, errs)
			}
			if tt.wantMsg != "" {
				found := false
				for _, err := range errs {
					if strings.Contains(err.Error(), tt.wantMsg) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected error containing %q, got %v", tt.wantMsg, errs)
				}
			}
		})
	}
}
