package release

import (
	"testing"
)

func TestImageReferencesTagVersion(t *testing.T) {
	version := branchVersion(t)
	if version == "" {
		t.Skip("not on an oadp-X.Y release branch")
	}

	root := repoRoot(t)
	irData := readBundleFile(t, root, imageRefsRelPath)

	// Add tag names here for images that use their own versioning scheme
	// (e.g. external dependencies not built from this repo).
	exceptions := []string{}

	reportErrors(t, ValidateImageReferencesTagVersion(irData, version, exceptions))
}

func TestImageReferencesMatchCSV(t *testing.T) {
	root := repoRoot(t)
	irData := readBundleFile(t, root, imageRefsRelPath)
	csvData := readBundleFile(t, root, csvRelPath)

	reportErrors(t, ValidateImageReferencesMatchCSV(irData, csvData))
}

func TestCSVMatchImageReferences(t *testing.T) {
	root := repoRoot(t)
	irData := readBundleFile(t, root, imageRefsRelPath)
	csvData := readBundleFile(t, root, csvRelPath)

	reportErrors(t, ValidateCSVMatchImageReferences(irData, csvData))
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
			assertErrors(t, errs, tt.wantErrs, tt.wantMsg)
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
			assertErrors(t, errs, tt.wantErrs, tt.wantMsg)
		})
	}
}

func TestValidateCSVMatchImageReferences(t *testing.T) {
	tests := []struct {
		name      string
		imageRefs string
		csv       string
		wantErrs  int
		wantMsg   string
	}{
		{
			name: "all RELATED_IMAGEs have matching image-references entry",
			imageRefs: `spec:
  tags:
  - name: velero
    from:
      kind: DockerImage
      name: registry.example.com/velero:latest`,
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
			wantErrs: 0,
		},
		{
			name: "orphaned RELATED_IMAGE produces error",
			imageRefs: `spec:
  tags:
  - name: velero
    from:
      kind: DockerImage
      name: registry.example.com/velero:latest`,
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
                - name: RELATED_IMAGE_ORPHAN
                  value: registry.example.com/orphan:latest`,
			wantErrs: 1,
			wantMsg:  "registry.example.com/orphan:latest",
		},
		{
			name: "non-RELATED_IMAGE env vars are ignored",
			imageRefs: `spec:
  tags:
  - name: operator
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
                env:
                - name: WATCH_NAMESPACE
                  value: openshift-adp`,
			wantErrs: 0,
		},
		{
			name: "multiple orphaned RELATED_IMAGEs",
			imageRefs: `spec:
  tags:
  - name: unrelated
    from:
      kind: DockerImage
      name: registry.example.com/unrelated:latest`,
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
                - name: RELATED_IMAGE_A
                  value: registry.example.com/a:latest
                - name: RELATED_IMAGE_B
                  value: registry.example.com/b:latest`,
			wantErrs: 2,
		},
		{
			name:      "empty tags produces error",
			imageRefs: `spec: { tags: [] }`,
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
                - name: RELATED_IMAGE_X
                  value: registry.example.com/x:latest`,
			wantErrs: 1,
			wantMsg:  "no tags",
		},
		{
			name:      "invalid image-references YAML produces error",
			imageRefs: `{{{`,
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
			wantMsg:  "failed to parse image-references",
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
			errs := ValidateCSVMatchImageReferences([]byte(tt.imageRefs), []byte(tt.csv))
			assertErrors(t, errs, tt.wantErrs, tt.wantMsg)
		})
	}
}
