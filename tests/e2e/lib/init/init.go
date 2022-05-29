package init

import (
	"flag"
	"os"
	"time"
)

// Common vars obtained from flags passed in ginkgo.
var credFile, namespace, credSecretRef, instanceName, provider, ci_cred_file, settings, artifact_dir, oc_cli string
var timeoutMultiplier time.Duration

func init() {
	flag.StringVar(&credFile, "credentials", "", "Cloud Credentials file path location")
	flag.StringVar(&namespace, "velero_namespace", "velero", "Velero Namespace")
	flag.StringVar(&settings, "settings", "./templates/default_settings.json", "Settings of the velero instance")
	flag.StringVar(&instanceName, "velero_instance_name", "example-velero", "Velero Instance Name")
	flag.StringVar(&credSecretRef, "creds_secret_ref", "cloud-credentials", "Credential secret ref for backup storage location")
	flag.StringVar(&provider, "provider", "aws", "Cloud provider")
	flag.StringVar(&ci_cred_file, "ci_cred_file", credFile, "CI Cloud Cred File")
	flag.StringVar(&artifact_dir, "artifact_dir", "/tmp", "Directory for storing must gather")
	flag.StringVar(&oc_cli, "oc_cli", "oc", "OC CLI Client")

	// helps with launching debug sessions from IDE
	if os.Getenv("E2E_USE_ENV_FLAGS") == "true" {
		if os.Getenv("CLOUD_CREDENTIALS") != "" {
			credFile = os.Getenv("CLOUD_CREDENTIALS")
		}
		if os.Getenv("VELERO_NAMESPACE") != "" {
			namespace = os.Getenv("VELERO_NAMESPACE")
		}
		if os.Getenv("SETTINGS") != "" {
			settings = os.Getenv("SETTINGS")
		}
		if os.Getenv("VELERO_INSTANCE_NAME") != "" {
			instanceName = os.Getenv("VELERO_INSTANCE_NAME")
		}
		if os.Getenv("CREDS_SECRET_REF") != "" {
			credSecretRef = os.Getenv("CREDS_SECRET_REF")
		}
		if os.Getenv("PROVIDER") != "" {
			provider = os.Getenv("PROVIDER")
		}
		if os.Getenv("CI_CRED_FILE") != "" {
			ci_cred_file = os.Getenv("CI_CRED_FILE")
		} else {
			ci_cred_file = credFile
		}
		if os.Getenv("ARTIFACT_DIR") != "" {
			artifact_dir = os.Getenv("ARTIFACT_DIR")
		}
		if os.Getenv("OC_CLI") != "" {
			oc_cli = os.Getenv("OC_CLI")
		}
	}

	timeoutMultiplierInput := flag.Int64("timeout_multiplier", 1, "Customize timeout multiplier from default (1)")
	timeoutMultiplier = 1
	if timeoutMultiplierInput != nil && *timeoutMultiplierInput >= 1 {
		timeoutMultiplier = time.Duration(*timeoutMultiplierInput)
	}
}

func GetTestSuiteInstanceName() string {
	return "ts-" + instanceName
}

func GetNamespace() string {
	return namespace
}

func GetCredfile() string {
	return credFile
}

func GetCredsecretref() string {
	return credSecretRef
}
func GetInstancename() string {
	return instanceName
}
func GetProvider() string {
	return provider
}
func GetCi_Cred_File() string {
	return ci_cred_file
}
func GetSettings() string {
	return settings
}
func GetArtifact_Dir() string {
	return artifact_dir
}
func GetOc_Cli() string {
	return oc_cli
}

func GetTimeoutMultiplier() time.Duration {
	return timeoutMultiplier
}

func GetBslSecretName() string {
	return "bsl-secret-" + instanceName
}
