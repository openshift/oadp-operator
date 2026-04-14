package release

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

func parseImageReferences(data []byte) (*imageReferencesFile, error) {
	var ir imageReferencesFile
	if err := yaml.Unmarshal(data, &ir); err != nil {
		return nil, fmt.Errorf("failed to parse image-references: %w", err)
	}
	if len(ir.Spec.Tags) == 0 {
		return nil, fmt.Errorf("image-references has no tags")
	}
	return &ir, nil
}

func parseCSV(data []byte) (*csv, error) {
	var c csv
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("failed to parse CSV: %w", err)
	}
	return &c, nil
}

// csvImages extracts RELATED_IMAGE_* env var values and container images
// from the CSV's deployment specs.
func csvImages(c *csv) (relatedImages, containerImages map[string]bool) {
	relatedImages = make(map[string]bool)
	containerImages = make(map[string]bool)
	for _, dep := range c.Spec.Install.Spec.Deployments {
		for _, container := range dep.Spec.Template.Spec.Containers {
			containerImages[container.Image] = true
			for _, env := range container.Env {
				if strings.HasPrefix(env.Name, relatedImagePrefix) {
					relatedImages[env.Value] = true
				}
			}
		}
	}
	return
}

// irImages extracts the set of image references from image-references tags.
func irImages(ir *imageReferencesFile) map[string]bool {
	images := make(map[string]bool, len(ir.Spec.Tags))
	for _, tag := range ir.Spec.Tags {
		images[tag.From.Name] = true
	}
	return images
}

// ValidateImageReferencesTagVersion checks that every image tag uses the
// expected release version (e.g. ":oadp-1.6"), preventing accidental use
// of tags from a different release stream.
func ValidateImageReferencesTagVersion(imageRefsData []byte, expectedVersion string, exceptions []string) []error {
	ir, err := parseImageReferences(imageRefsData)
	if err != nil {
		return []error{err}
	}

	skip := make(map[string]bool, len(exceptions))
	for _, name := range exceptions {
		skip[name] = true
	}

	expectedSuffix := ":" + expectedVersion
	var errs []error
	for _, tag := range ir.Spec.Tags {
		if skip[tag.Name] {
			continue
		}
		if !strings.HasSuffix(tag.From.Name, expectedSuffix) {
			errs = append(errs, fmt.Errorf("image-references tag %q has image %q which does not end with %q", tag.Name, tag.From.Name, expectedSuffix))
		}
	}
	return errs
}

// ValidateImageReferencesMatchCSV ensures every image in image-references has
// a corresponding RELATED_IMAGE_* env var (or is a container image) in the CSV.
// This catches images added to image-references but not wired into the operator.
func ValidateImageReferencesMatchCSV(imageRefsData, csvData []byte) []error {
	ir, err := parseImageReferences(imageRefsData)
	if err != nil {
		return []error{err}
	}

	c, err := parseCSV(csvData)
	if err != nil {
		return []error{err}
	}

	relatedImages, containerImages := csvImages(c)

	var errs []error
	for _, tag := range ir.Spec.Tags {
		dockerImage := tag.From.Name
		if containerImages[dockerImage] {
			continue
		}
		if !relatedImages[dockerImage] {
			errs = append(errs, fmt.Errorf("image-references tag %q has image %q which is not in CSV RELATED_IMAGE_* env vars", tag.Name, dockerImage))
		}
	}
	return errs
}

// ValidateCSVMatchImageReferences ensures every RELATED_IMAGE_* env var in the
// CSV has a corresponding entry in image-references. This catches orphaned env
// vars that reference images no longer tracked in image-references.
func ValidateCSVMatchImageReferences(imageRefsData, csvData []byte) []error {
	ir, err := parseImageReferences(imageRefsData)
	if err != nil {
		return []error{err}
	}

	c, err := parseCSV(csvData)
	if err != nil {
		return []error{err}
	}

	known := irImages(ir)
	relatedImages, _ := csvImages(c)

	var errs []error
	for image := range relatedImages {
		if !known[image] {
			errs = append(errs, fmt.Errorf("CSV RELATED_IMAGE_* has image %q which is not in image-references", image))
		}
	}
	return errs
}
