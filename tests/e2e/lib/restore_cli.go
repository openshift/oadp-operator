package lib

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	velero "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

// CreateRestoreFromBackupViaCLI creates a restore using the OADP CLI
func CreateRestoreFromBackupViaCLI(backupName, restoreName string) error {
	options := []string{"--from-backup", backupName}

	// Execute CLI command
	cmd := &CLICommand{
		Resource: "restore",
		Action:   "create",
		Name:     restoreName,
		Options:  options,
	}
	output, err := cmd.Execute()
	if err != nil {
		return fmt.Errorf("failed to create restore via CLI: %v, output: %s", err, string(output))
	}

	log.Printf("Restore created via CLI: %s", restoreName)
	return nil
}

// CreateRestoreFromBackupWithOptionsViaCLI creates a restore with options using the OADP CLI
func CreateRestoreFromBackupWithOptionsViaCLI(backupName, restoreName string, restoreVolumes bool, includeNamespaces, excludeNamespaces []string) error {
	options := []string{"--from-backup", backupName}

	// Add restore volumes option
	if restoreVolumes {
		options = append(options, "--restore-volumes")
	}

	// Add included namespaces
	if len(includeNamespaces) > 0 {
		options = append(options, "--include-namespaces", strings.Join(includeNamespaces, ","))
	}

	// Add excluded namespaces
	if len(excludeNamespaces) > 0 {
		options = append(options, "--exclude-namespaces", strings.Join(excludeNamespaces, ","))
	}

	// Execute CLI command
	cmd := &CLICommand{
		Resource: "restore",
		Action:   "create",
		Name:     restoreName,
		Options:  options,
	}
	output, err := cmd.Execute()
	if err != nil {
		return fmt.Errorf("failed to create restore with options via CLI: %v, output: %s", err, string(output))
	}

	log.Printf("Restore with options created via CLI: %s", restoreName)
	return nil
}

// GetRestoreViaCLI gets restore details using the OADP CLI
func GetRestoreViaCLI(name string) (*velero.Restore, error) {
	if name == "" {
		return nil, fmt.Errorf("restore name cannot be empty")
	}

	// Use CLI to get restore details in JSON format
	cmd := &CLICommand{
		Resource: "restore",
		Action:   "get",
		Name:     name,
		Options:  []string{"-o", "json"},
	}
	output, err := cmd.ExecuteOutput()
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
func IsRestoreDoneViaCLI(name string) wait.ConditionFunc {
	return func() (bool, error) {
		cmd := &CLICommand{
			Resource: "restore",
			Action:   "get",
			Name:     name,
			Options:  []string{"-o", "yaml"},
		}
		output, err := cmd.ExecuteOutput()
		if err != nil {
			return false, fmt.Errorf("failed to get restore status via CLI: %v", err)
		}

		phase := ParsePhaseFromYAML(string(output))

		if len(phase) > 0 {
			log.Printf("restore phase: %s", phase)
		}

		return !IsRestorePhaseNotDone(phase), nil
	}
}

// IsRestoreCompletedSuccessfullyViaCLI checks if restore completed successfully using the OADP CLI
func IsRestoreCompletedSuccessfullyViaCLI(name string) (bool, error) {
	// Use CLI to get restore status
	cmd := &CLICommand{
		Resource: "restore",
		Action:   "get",
		Name:     name,
		Options:  []string{"-o", "yaml"},
	}
	output, err := cmd.ExecuteOutput()
	if err != nil {
		return false, fmt.Errorf("failed to get restore status via CLI: %v", err)
	}

	// Parse phase from YAML output
	phase := ParsePhaseFromYAML(string(output))

	if phase == string(velero.RestorePhaseCompleted) {
		return true, nil
	}

	// Get additional failure information using CLI
	restoreLogs, logsErr := RestoreLogsViaCLI(name)
	if logsErr != nil {
		restoreLogs = fmt.Sprintf("Failed to get logs: %v", logsErr)
	}

	return false, fmt.Errorf(
		"restore phase is: %s; expected: %s\nvelero failure logs: %v",
		phase, string(velero.RestorePhaseCompleted), restoreLogs,
	)
}

// DescribeRestoreViaCLI describes restore using the OADP CLI with default timeout and retry.
// The timeout prevents the command from hanging when retrieving restore details from object storage.
func DescribeRestoreViaCLI(name string) string {
	return DescribeRestoreViaCLIWithOptions(name, DefaultCLITimeout, DefaultCLIRetries, DefaultCLIRetryDelay)
}

// DescribeRestoreViaCLIWithOptions describes restore using the OADP CLI with specified timeout and retry options.
func DescribeRestoreViaCLIWithOptions(name string, timeout time.Duration, maxRetries int, retryDelay time.Duration) string {
	// Use CLI to describe restore
	cmd := &CLICommand{
		Resource: "restore",
		Action:   "describe",
		Name:     name,
		Options:  []string{"--details"},
	}

	var output []byte
	var err error

	if maxRetries > 1 {
		output, err = cmd.ExecuteWithTimeoutAndRetry(timeout, maxRetries, retryDelay)
	} else {
		output, err = cmd.ExecuteWithTimeout(timeout)
	}

	if err != nil {
		return fmt.Sprintf("could not describe restore via CLI: %v, output: %s", err, string(output))
	}

	return string(output)
}

// RestoreLogsViaCLI gets restore logs using the OADP CLI with default timeout and retry.
// The timeout prevents the command from hanging indefinitely when streaming logs from object storage.
// Retry logic helps handle transient network issues.
func RestoreLogsViaCLI(name string) (restoreLogs string, err error) {
	return RestoreLogsViaCLIWithOptions(name, DefaultCLITimeout, DefaultCLIRetries, DefaultCLIRetryDelay)
}

// RestoreLogsViaCLIWithTimeout gets restore logs using the OADP CLI with a specified timeout (no retry).
func RestoreLogsViaCLIWithTimeout(name string, timeout time.Duration) (restoreLogs string, err error) {
	return RestoreLogsViaCLIWithOptions(name, timeout, 1, 0)
}

// RestoreLogsViaCLIWithOptions gets restore logs using the OADP CLI with specified timeout and retry options.
func RestoreLogsViaCLIWithOptions(name string, timeout time.Duration, maxRetries int, retryDelay time.Duration) (restoreLogs string, err error) {
	if name == "" {
		return "", fmt.Errorf("restore name cannot be empty")
	}

	// Use CLI to get restore logs with timeout and retry to prevent hanging
	cmd := &CLICommand{
		Resource: "restore",
		Action:   "logs",
		Name:     name,
		Options:  []string{},
	}

	var output []byte
	var cmdErr error

	if maxRetries > 1 {
		output, cmdErr = cmd.ExecuteOutputWithTimeoutAndRetry(timeout, maxRetries, retryDelay)
	} else {
		output, cmdErr = cmd.ExecuteOutputWithTimeout(timeout)
	}

	if cmdErr != nil {
		return "", fmt.Errorf("failed to get restore logs via CLI: %v", cmdErr)
	}

	return string(output), nil
}

// RestoreErrorLogsViaCLI gets restore error logs using the OADP CLI
func RestoreErrorLogsViaCLI(name string) []string {
	rl, err := RestoreLogsViaCLI(name)
	if err != nil {
		return []string{err.Error()}
	}
	return errorLogsExcludingIgnored(rl)
}

// DeleteRestoreViaCLI deletes a restore using the OADP CLI
func DeleteRestoreViaCLI(name string) error {
	if name == "" {
		return fmt.Errorf("restore name cannot be empty")
	}

	// Use CLI to delete restore
	cmd := &CLICommand{
		Resource: "restore",
		Action:   "delete",
		Name:     name,
		Options:  []string{},
	}
	output, err := cmd.Execute()
	if err != nil {
		return fmt.Errorf("failed to delete restore via CLI: %v, output: %s", err, string(output))
	}

	log.Printf("Restore deleted via CLI: %s", name)
	return nil
}

// ListRestoresViaCLI lists all restores using the OADP CLI
func ListRestoresViaCLI() ([]string, error) {
	// Use CLI to list restores
	cmd := &CLICommand{
		Resource: "restore",
		Action:   "list",
		Name:     "", // list commands don't require a name
		Options:  []string{"-o", "name"},
	}
	output, err := cmd.ExecuteOutput()
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
