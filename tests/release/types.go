package release

const (
	relatedImagePrefix = "RELATED_IMAGE_"
	oadpBranchPrefix   = "oadp-"

	imageRefsRelPath = "bundle/image-references"
	csvRelPath       = "bundle/manifests/oadp-operator.clusterserviceversion.yaml"
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
