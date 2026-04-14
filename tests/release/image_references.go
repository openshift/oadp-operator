package release

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

type imageReferencesFile struct {
	Spec struct {
		Tags []struct {
			Name string `yaml:"name"`
			From struct {
				Kind string `yaml:"kind"`
				Name string `yaml:"name"`
			} `yaml:"from"`
		} `yaml:"tags"`
	} `yaml:"spec"`
}

type csv struct {
	Spec struct {
		Install struct {
			Spec struct {
				Deployments []struct {
					Spec struct {
						Template struct {
							Spec struct {
								Containers []struct {
									Image string `yaml:"image"`
									Env   []struct {
										Name  string `yaml:"name"`
										Value string `yaml:"value"`
									} `yaml:"env"`
								} `yaml:"containers"`
							} `yaml:"spec"`
						} `yaml:"template"`
					} `yaml:"spec"`
				} `yaml:"deployments"`
			} `yaml:"spec"`
		} `yaml:"install"`
	} `yaml:"spec"`
}

// ValidateImageReferencesTagVersion checks that every image in image-references
// uses the expected release version tag (e.g. ":oadp-1.6" on the oadp-1.6 branch).
func ValidateImageReferencesTagVersion(imageRefsData []byte, expectedVersion string, exceptions []string) []error {
	var ir imageReferencesFile
	if err := yaml.Unmarshal(imageRefsData, &ir); err != nil {
		return []error{fmt.Errorf("failed to parse image-references: %w", err)}
	}
	if len(ir.Spec.Tags) == 0 {
		return []error{fmt.Errorf("image-references has no tags")}
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

// ValidateImageReferencesMatchCSV checks that every image in image-references
// is present as a RELATED_IMAGE_* env var (or container image) in the CSV.
func ValidateImageReferencesMatchCSV(imageRefsData, csvData []byte) []error {
	var ir imageReferencesFile
	if err := yaml.Unmarshal(imageRefsData, &ir); err != nil {
		return []error{fmt.Errorf("failed to parse image-references: %w", err)}
	}
	if len(ir.Spec.Tags) == 0 {
		return []error{fmt.Errorf("image-references has no tags")}
	}

	var c csv
	if err := yaml.Unmarshal(csvData, &c); err != nil {
		return []error{fmt.Errorf("failed to parse CSV: %w", err)}
	}

	relatedImages := make(map[string]bool)
	containerImages := make(map[string]bool)
	for _, dep := range c.Spec.Install.Spec.Deployments {
		for _, container := range dep.Spec.Template.Spec.Containers {
			containerImages[container.Image] = true
			for _, env := range container.Env {
				if strings.HasPrefix(env.Name, "RELATED_IMAGE_") {
					relatedImages[env.Value] = true
				}
			}
		}
	}

	var errs []error
	for _, tag := range ir.Spec.Tags {
		dockerImage := tag.From.Name
		// Skip the operator image itself — it appears as the deployment container image, not a RELATED_IMAGE
		if containerImages[dockerImage] {
			continue
		}
		if !relatedImages[dockerImage] {
			errs = append(errs, fmt.Errorf("image-references tag %q has image %q which is not in CSV RELATED_IMAGE_* env vars", tag.Name, dockerImage))
		}
	}
	return errs
}
