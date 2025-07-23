package lib

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	velero "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CreateRestoreFromBackupViaCLI creates a restore using the OADP CLI
func CreateRestoreFromBackupViaCLI(ocClient client.Client, veleroNamespace, backupName, restoreName string) error {
	args := []string{"oadp", "restore", "create", restoreName, "--from-backup", backupName}

	// Execute CLI command
	cmd := createKubectlOADPCommand(args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create restore via CLI: %v, output: %s", err, string(output))
	}

	log.Printf("Restore created via CLI: %s", restoreName)
	return nil
}

// CreateRestoreFromBackupWithOptionsViaCLI creates a restore with options using the OADP CLI
func CreateRestoreFromBackupWithOptionsViaCLI(ocClient client.Client, veleroNamespace, backupName, restoreName string, restoreVolumes bool, includeNamespaces, excludeNamespaces []string) error {
	args := []string{"oadp", "restore", "create", restoreName, "--from-backup", backupName}

	// Add restore volumes option
	if restoreVolumes {
		args = append(args, "--restore-volumes")
	}

	// Add included namespaces
	if len(includeNamespaces) > 0 {
		args = append(args, "--include-namespaces", strings.Join(includeNamespaces, ","))
	}

	// Add excluded namespaces
	if len(excludeNamespaces) > 0 {
		args = append(args, "--exclude-namespaces", strings.Join(excludeNamespaces, ","))
	}

	// Execute CLI command
	cmd := createKubectlOADPCommand(args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create restore with options via CLI: %v, output: %s", err, string(output))
	}

	log.Printf("Restore with options created via CLI: %s", restoreName)
	return nil
}

// GetRestoreViaCLI gets restore details using the OADP CLI
func GetRestoreViaCLI(c client.Client, namespace string, name string) (*velero.Restore, error) {
	// Use CLI to get restore details in JSON format
	cmd := createKubectlOADPCommand("oadp", "restore", "get", name, "-o", "json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get restore via CLI: %v", err)
	}

	// Parse the JSON output back to velero.Restore struct
	var restore velero.Restore
	if err := json.Unmarshal(output, &restore); err != nil {
		return nil, fmt.Errorf("failed to parse restore JSON: %v", err)
	}

	return &restore, nil
}

// IsRestoreDoneViaCLI checks if restore is done using the OADP CLI
func IsRestoreDoneViaCLI(ocClient client.Client, veleroNamespace, name string) wait.ConditionFunc {
	return func() (bool, error) {
		// Use CLI to get restore status
		cmd := createKubectlOADPCommand("oadp", "restore", "get", name, "-o", "yaml")
		output, err := cmd.Output()
		if err != nil {
			return false, fmt.Errorf("failed to get restore status via CLI: %v", err)
		}

		// Parse phase from YAML output
		yamlOutput := string(output)
		lines := strings.Split(yamlOutput, "\n")
		var phase string
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "phase:") {
				phase = strings.TrimSpace(strings.TrimPrefix(line, "phase:"))
				break
			}
		}

		if len(phase) > 0 {
			log.Printf("restore phase: %s", phase)
		}

		var phasesNotDone = []string{
			string(velero.RestorePhaseNew),
			string(velero.RestorePhaseInProgress),
			string(velero.RestorePhaseWaitingForPluginOperations),
			string(velero.RestorePhaseWaitingForPluginOperationsPartiallyFailed),
			string(velero.RestorePhaseFinalizing),
			string(velero.RestorePhaseFinalizingPartiallyFailed),
			"",
		}

		for _, notDonePhase := range phasesNotDone {
			if phase == notDonePhase {
				return false, nil
			}
		}
		return true, nil
	}
}

// IsRestoreCompletedSuccessfullyViaCLI checks if restore completed successfully using the OADP CLI
func IsRestoreCompletedSuccessfullyViaCLI(c *kubernetes.Clientset, ocClient client.Client, veleroNamespace, name string) (bool, error) {
	// Use CLI to get restore status
	cmd := createKubectlOADPCommand("oadp", "restore", "get", name, "-o", "yaml")
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to get restore status via CLI: %v", err)
	}

	// Parse phase from YAML output
	yamlOutput := string(output)
	lines := strings.Split(yamlOutput, "\n")
	var phase string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "phase:") {
			phase = strings.TrimSpace(strings.TrimPrefix(line, "phase:"))
			break
		}
	}

	if phase == string(velero.RestorePhaseCompleted) {
		return true, nil
	}

	// Get additional failure information using CLI
	restoreLogs, logsErr := RestoreLogsViaCLI(c, ocClient, veleroNamespace, name)
	if logsErr != nil {
		restoreLogs = fmt.Sprintf("Failed to get logs: %v", logsErr)
	}

	return false, fmt.Errorf(
		"restore phase is: %s; expected: %s\nvelero failure logs: %v",
		phase, string(velero.RestorePhaseCompleted), restoreLogs,
	)
}

// DescribeRestoreViaCLI describes restore using the OADP CLI
func DescribeRestoreViaCLI(ocClient client.Client, namespace string, name string) string {
	// Use CLI to describe restore
	cmd := createKubectlOADPCommand("oadp", "restore", "describe", name, "--details")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("could not describe restore via CLI: %v, output: %s", err, string(output))
	}

	return string(output)
}

// RestoreLogsViaCLI gets restore logs using the OADP CLI
func RestoreLogsViaCLI(c *kubernetes.Clientset, ocClient client.Client, namespace string, name string) (restoreLogs string, err error) {
	// Use CLI to get restore logs
	cmd := createKubectlOADPCommand("oadp", "restore", "logs", name)
	output, cmdErr := cmd.Output()
	if cmdErr != nil {
		return "", fmt.Errorf("failed to get restore logs via CLI: %v", cmdErr)
	}

	return string(output), nil
}

// RestoreErrorLogsViaCLI gets restore error logs using the OADP CLI
func RestoreErrorLogsViaCLI(c *kubernetes.Clientset, ocClient client.Client, namespace string, name string) []string {
	rl, err := RestoreLogsViaCLI(c, ocClient, namespace, name)
	if err != nil {
		return []string{err.Error()}
	}
	return errorLogsExcludingIgnored(rl)
}

// DeleteRestoreViaCLI deletes a restore using the OADP CLI
func DeleteRestoreViaCLI(namespace string, name string) error {
	// Use CLI to delete restore
	cmd := createKubectlOADPCommand("oadp", "restore", "delete", name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete restore via CLI: %v, output: %s", err, string(output))
	}

	log.Printf("Restore deleted via CLI: %s", name)
	return nil
}

// ListRestoresViaCLI lists all restores using the OADP CLI
func ListRestoresViaCLI(namespace string) ([]string, error) {
	// Use CLI to list restores
	cmd := createKubectlOADPCommand("oadp", "restore", "list", "-o", "name")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list restores via CLI: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var restores []string
	for _, line := range lines {
		if line != "" {
			// Remove "restore.velero.io/" prefix if present
			restore := strings.TrimPrefix(line, "restore.velero.io/")
			restores = append(restores, restore)
		}
	}

	return restores, nil
}
