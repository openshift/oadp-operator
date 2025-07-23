package lib

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	velero "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

// CreateBackupForNamespacesViaCLI creates a backup using the OADP CLI
func CreateBackupForNamespacesViaCLI(backupName string, namespaces []string, defaultVolumesToFsBackup bool, snapshotMoveData bool) error {
	var options []string

	// Add included namespaces (comma-separated)
	if len(namespaces) > 0 {
		options = append(options, "--include-namespaces", strings.Join(namespaces, ","))
	}

	// Add volume backup options
	if defaultVolumesToFsBackup {
		options = append(options, "--default-volumes-to-fs-backup")
	}

	// Add snapshot move data option
	if snapshotMoveData {
		options = append(options, "--snapshot-move-data")
	}

	// Execute CLI command
	cmd := &CLICommand{
		Resource: "backup",
		Action:   "create",
		Name:     backupName,
		Options:  options,
	}
	output, err := cmd.Execute()
	if err != nil {
		return fmt.Errorf("failed to create backup via CLI: %v, output: %s", err, string(output))
	}

	log.Printf("Backup created via CLI: %s", backupName)
	return nil
}

// CreateCustomBackupForNamespacesViaCLI creates a custom backup using the OADP CLI
func CreateCustomBackupForNamespacesViaCLI(backupName string, namespaces []string, includedResources, excludedResources []string, defaultVolumesToFsBackup bool, snapshotMoveData bool) error {
	var options []string

	// Add included namespaces (comma-separated)
	if len(namespaces) > 0 {
		options = append(options, "--include-namespaces", strings.Join(namespaces, ","))
	}

	// Add included resources
	if len(includedResources) > 0 {
		options = append(options, "--include-resources", strings.Join(includedResources, ","))
	}

	// Add excluded resources
	if len(excludedResources) > 0 {
		options = append(options, "--exclude-resources", strings.Join(excludedResources, ","))
	}

	// Add volume backup options
	if defaultVolumesToFsBackup {
		options = append(options, "--default-volumes-to-fs-backup")
	}

	// Add snapshot move data option
	if snapshotMoveData {
		options = append(options, "--snapshot-move-data")
	}

	// Execute CLI command
	cmd := &CLICommand{
		Resource: "backup",
		Action:   "create",
		Name:     backupName,
		Options:  options,
	}
	output, err := cmd.Execute()
	if err != nil {
		return fmt.Errorf("failed to create custom backup via CLI: %v, output: %s", err, string(output))
	}

	log.Printf("Custom backup created via CLI: %s", backupName)
	return nil
}

// GetBackupViaCLI gets backup details using the OADP CLI
func GetBackupViaCLI(name string) (*velero.Backup, error) {
	if name == "" {
		return nil, fmt.Errorf("backup name cannot be empty")
	}

	// Use CLI to get backup details in JSON format
	cmd := &CLICommand{
		Resource: "backup",
		Action:   "get",
		Name:     name,
		Options:  []string{"-o", "json"},
	}
	output, err := cmd.ExecuteOutput()
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
func IsBackupDoneViaCLI(name string) wait.ConditionFunc {
	return func() (bool, error) {
		// Use CLI to get backup status
		cmd := &CLICommand{
			Resource: "backup",
			Action:   "get",
			Name:     name,
			Options:  []string{"-o", "yaml"},
		}
		output, err := cmd.ExecuteOutput()
		if err != nil {
			return false, fmt.Errorf("failed to get backup status via CLI: %v", err)
		}

		// Parse phase from YAML output
		phase := ParsePhaseFromYAML(string(output))

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
func IsBackupCompletedSuccessfullyViaCLI(name string) (bool, error) {
	// Use CLI to get backup status
	cmd := &CLICommand{
		Resource: "backup",
		Action:   "get",
		Name:     name,
		Options:  []string{"-o", "yaml"},
	}
	output, err := cmd.ExecuteOutput()
	if err != nil {
		return false, fmt.Errorf("failed to get backup status via CLI: %v", err)
	}

	// Parse phase from YAML output
	phase := ParsePhaseFromYAML(string(output))

	if phase == string(velero.BackupPhaseCompleted) {
		return true, nil
	}

	// Get additional failure information using CLI
	backupLogs, logsErr := BackupLogsViaCLI(name)
	if logsErr != nil {
		backupLogs = fmt.Sprintf("Failed to get logs: %v", logsErr)
	}

	return false, fmt.Errorf(
		"backup phase is: %s; expected: %s\nvelero failure logs: %v",
		phase, string(velero.BackupPhaseCompleted), backupLogs,
	)
}

// DescribeBackupViaCLI describes backup using the OADP CLI
func DescribeBackupViaCLI(name string) (backupDescription string) {
	// Use CLI to describe backup
	cmd := &CLICommand{
		Resource: "backup",
		Action:   "describe",
		Name:     name,
		Options:  []string{"--details"},
	}
	output, err := cmd.Execute()
	if err != nil {
		return fmt.Sprintf("could not describe backup via CLI: %v, output: %s", err, string(output))
	}

	return string(output)
}

// BackupLogsViaCLI gets backup logs using the OADP CLI
func BackupLogsViaCLI(name string) (backupLogs string, err error) {
	if name == "" {
		return "", fmt.Errorf("backup name cannot be empty")
	}

	// Use CLI to get backup logs
	cmd := &CLICommand{
		Resource: "backup",
		Action:   "logs",
		Name:     name,
		Options:  []string{},
	}
	output, cmdErr := cmd.ExecuteOutput()
	if cmdErr != nil {
		return "", fmt.Errorf("failed to get backup logs via CLI: %v", cmdErr)
	}

	return string(output), nil
}

// BackupErrorLogsViaCLI gets backup error logs using the OADP CLI
func BackupErrorLogsViaCLI(name string) []string {
	bl, err := BackupLogsViaCLI(name)
	if err != nil {
		return []string{err.Error()}
	}
	return errorLogsExcludingIgnored(bl)
}

// DeleteBackupViaCLI deletes a backup using the OADP CLI
func DeleteBackupViaCLI(name string) error {
	if name == "" {
		return fmt.Errorf("backup name cannot be empty")
	}

	// Use CLI to delete backup
	cmd := &CLICommand{
		Resource: "backup",
		Action:   "delete",
		Name:     name,
		Options:  []string{},
	}
	output, err := cmd.Execute()
	if err != nil {
		return fmt.Errorf("failed to delete backup via CLI: %v, output: %s", err, string(output))
	}

	log.Printf("Backup deleted via CLI: %s", name)
	return nil
}
