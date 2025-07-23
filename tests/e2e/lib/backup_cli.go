package lib

import (
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strings"

	velero "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Helper function to create kubectl oadp commands
func createKubectlOADPCommand(args ...string) *exec.Cmd {
	return exec.Command("kubectl", args...)
}

// CreateBackupForNamespacesViaCLI creates a backup using the OADP CLI
func CreateBackupForNamespacesViaCLI(ocClient client.Client, veleroNamespace, backupName string, namespaces []string, defaultVolumesToFsBackup bool, snapshotMoveData bool) error {
	args := []string{"oadp", "backup", "create", backupName}

	// Add included namespaces (comma-separated)
	if len(namespaces) > 0 {
		args = append(args, "--include-namespaces", strings.Join(namespaces, ","))
	}

	// Add volume backup options
	if defaultVolumesToFsBackup {
		args = append(args, "--default-volumes-to-fs-backup")
	}

	// Add snapshot move data option
	if snapshotMoveData {
		args = append(args, "--snapshot-move-data")
	}

	// Execute CLI command
	cmd := createKubectlOADPCommand(args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create backup via CLI: %v, output: %s", err, string(output))
	}

	log.Printf("Backup created via CLI: %s", backupName)
	return nil
}

// CreateCustomBackupForNamespacesViaCLI creates a custom backup using the OADP CLI
func CreateCustomBackupForNamespacesViaCLI(ocClient client.Client, veleroNamespace, backupName string, namespaces []string, includedResources, excludedResources []string, defaultVolumesToFsBackup bool, snapshotMoveData bool) error {
	args := []string{"oadp", "backup", "create", backupName}

	// Add included namespaces (comma-separated)
	if len(namespaces) > 0 {
		args = append(args, "--include-namespaces", strings.Join(namespaces, ","))
	}

	// Add included resources
	if len(includedResources) > 0 {
		args = append(args, "--include-resources", strings.Join(includedResources, ","))
	}

	// Add excluded resources
	if len(excludedResources) > 0 {
		args = append(args, "--exclude-resources", strings.Join(excludedResources, ","))
	}

	// Add volume backup options
	if defaultVolumesToFsBackup {
		args = append(args, "--default-volumes-to-fs-backup")
	}

	// Add snapshot move data option
	if snapshotMoveData {
		args = append(args, "--snapshot-move-data")
	}

	// Execute CLI command
	cmd := createKubectlOADPCommand(args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create custom backup via CLI: %v, output: %s", err, string(output))
	}

	log.Printf("Custom backup created via CLI: %s", backupName)
	return nil
}

// GetBackupViaCLI gets backup details using the OADP CLI
func GetBackupViaCLI(c client.Client, namespace string, name string) (*velero.Backup, error) {
	// Use CLI to get backup details in JSON format
	cmd := createKubectlOADPCommand("oadp", "backup", "get", name, "-o", "json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get backup via CLI: %v", err)
	}

	// Parse the JSON output back to velero.Backup struct
	var backup velero.Backup
	if err := json.Unmarshal(output, &backup); err != nil {
		return nil, fmt.Errorf("failed to parse backup JSON: %v", err)
	}

	return &backup, nil
}

// IsBackupDoneViaCLI checks if backup is done using the OADP CLI
func IsBackupDoneViaCLI(ocClient client.Client, veleroNamespace, name string) wait.ConditionFunc {
	return func() (bool, error) {
		// Use CLI to get backup status
		cmd := createKubectlOADPCommand("oadp", "backup", "get", name, "-o", "yaml")
		output, err := cmd.Output()
		if err != nil {
			return false, fmt.Errorf("failed to get backup status via CLI: %v", err)
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
			log.Printf("backup phase: %s", phase)
		}

		var phasesNotDone = []string{
			string(velero.BackupPhaseNew),
			string(velero.BackupPhaseInProgress),
			string(velero.BackupPhaseWaitingForPluginOperations),
			string(velero.BackupPhaseWaitingForPluginOperationsPartiallyFailed),
			string(velero.BackupPhaseFinalizing),
			string(velero.BackupPhaseFinalizingPartiallyFailed),
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

// IsBackupCompletedSuccessfullyViaCLI checks if backup completed successfully using the OADP CLI
func IsBackupCompletedSuccessfullyViaCLI(c *kubernetes.Clientset, ocClient client.Client, namespace string, name string) (bool, error) {
	// Use CLI to get backup status
	cmd := createKubectlOADPCommand("oadp", "backup", "get", name, "-o", "yaml")
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to get backup status via CLI: %v", err)
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

	if phase == string(velero.BackupPhaseCompleted) {
		return true, nil
	}

	// Get additional failure information using CLI
	backupLogs, logsErr := BackupLogsViaCLI(c, ocClient, namespace, name)
	if logsErr != nil {
		backupLogs = fmt.Sprintf("Failed to get logs: %v", logsErr)
	}

	return false, fmt.Errorf(
		"backup phase is: %s; expected: %s\nvelero failure logs: %v",
		phase, string(velero.BackupPhaseCompleted), backupLogs,
	)
}

// DescribeBackupViaCLI describes backup using the OADP CLI
func DescribeBackupViaCLI(ocClient client.Client, namespace string, name string) (backupDescription string) {
	// Use CLI to describe backup
	cmd := createKubectlOADPCommand("oadp", "backup", "describe", name, "--details")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("could not describe backup via CLI: %v, output: %s", err, string(output))
	}

	return string(output)
}

// BackupLogsViaCLI gets backup logs using the OADP CLI
func BackupLogsViaCLI(c *kubernetes.Clientset, ocClient client.Client, namespace string, name string) (backupLogs string, err error) {
	// Use CLI to get backup logs
	cmd := createKubectlOADPCommand("oadp", "backup", "logs", name)
	output, cmdErr := cmd.Output()
	if cmdErr != nil {
		return "", fmt.Errorf("failed to get backup logs via CLI: %v", cmdErr)
	}

	return string(output), nil
}

// BackupErrorLogsViaCLI gets backup error logs using the OADP CLI
func BackupErrorLogsViaCLI(c *kubernetes.Clientset, ocClient client.Client, namespace string, name string) []string {
	bl, err := BackupLogsViaCLI(c, ocClient, namespace, name)
	if err != nil {
		return []string{err.Error()}
	}
	return errorLogsExcludingIgnored(bl)
}

// DeleteBackupViaCLI deletes a backup using the OADP CLI
func DeleteBackupViaCLI(namespace string, name string) error {
	// Use CLI to delete backup
	cmd := createKubectlOADPCommand("oadp", "backup", "delete", name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete backup via CLI: %v, output: %s", err, string(output))
	}

	log.Printf("Backup deleted via CLI: %s", name)
	return nil
}
