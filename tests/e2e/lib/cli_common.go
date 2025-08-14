package lib

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type CLICommand struct {
	Resource string // "backup" or "restore"
	Action   string // "create", "get", "delete", etc.
	Name     string
	Options  []string
}

func (c *CLICommand) Execute() ([]byte, error) {
	args := []string{"oadp", c.Resource, c.Action}
	if c.Name != "" {
		args = append(args, c.Name)
	}
	args = append(args, c.Options...)

	c.LogCLICommand()
	cmd := exec.Command("kubectl", args...)
	return cmd.CombinedOutput()
}

func (c *CLICommand) ExecuteOutput() ([]byte, error) {
	args := []string{"oadp", c.Resource, c.Action}
	if c.Name != "" {
		args = append(args, c.Name)
	}
	args = append(args, c.Options...)

	c.LogCLICommand()
	cmd := exec.Command("kubectl", args...)
	return cmd.Output()
}

func (c *CLICommand) LogCLICommand() {
	args := []string{"kubectl", "oadp", c.Resource, c.Action}
	if c.Name != "" {
		args = append(args, c.Name)
	}
	args = append(args, c.Options...)
	log.Printf("Executing CLI command: %s", strings.Join(args, " "))
}

func ParsePhaseFromYAML(yamlOutput string) string {
	lines := strings.Split(yamlOutput, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "phase:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "phase:"))
		}
	}
	return ""
}

type CLISetup struct {
	repoURL     string
	installArgs []string
	namespace   string
}

func NewOADPCLISetup() *CLISetup {
	return &CLISetup{
		repoURL:     "https://github.com/migtools/oadp-cli.git",
		installArgs: []string{"build"},
		namespace:   "openshift-adp",
	}
}

func (c *CLISetup) Install() error {
	tmpDir, err := c.createTempDir()
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	cloneDir := filepath.Join(tmpDir, "oadp-cli")

	steps := []struct {
		name string
		fn   func() error
	}{
		{"Cloning repository", func() error { return c.cloneRepo(cloneDir) }},
		{"Building and installing", func() error { return c.buildAndInstall(cloneDir) }},
		{"Verifying installation", func() error { return c.verifyInstallation() }},
		{"Configuring namespace", func() error { return c.configureNamespace() }},
	}

	for _, step := range steps {
		log.Printf("OADP CLI Setup: %s...", step.name)
		if err := step.fn(); err != nil {
			return fmt.Errorf("%s failed: %w", step.name, err)
		}
	}

	log.Print("OADP CLI setup completed successfully")
	return nil
}

func (c *CLISetup) createTempDir() (string, error) {
	tmpDir, err := os.MkdirTemp("", "oadp-cli-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}
	return tmpDir, nil
}

func (c *CLISetup) cloneRepo(cloneDir string) error {
	return runCommand("git", []string{"clone", c.repoURL, cloneDir}, "")
}

func (c *CLISetup) buildAndInstall(cloneDir string) error {
	// Build the binary
	log.Print("Building OADP CLI...")
	if err := runCommand("make", c.installArgs, cloneDir); err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	// Verify the binary was created
	binaryPath := filepath.Join(cloneDir, "kubectl-oadp")
	if _, err := os.Stat(binaryPath); err != nil {
		return fmt.Errorf("kubectl-oadp binary not found at %s: %w", binaryPath, err)
	}

	// Try multiple target locations, starting with user-writable paths
	targetPaths := []string{
		fmt.Sprintf("%s/bin/kubectl-oadp", os.Getenv("HOME")),
		"/usr/local/bin/kubectl-oadp",
	}

	var targetPath string
	var moveErr error

	for _, tp := range targetPaths {
		targetPath = tp
		log.Printf("Attempting to move binary from %s to %s", binaryPath, targetPath)

		// Create directory if it doesn't exist (for ~/bin)
		if targetPath == fmt.Sprintf("%s/bin/kubectl-oadp", os.Getenv("HOME")) {
			binDir := filepath.Dir(targetPath)
			if err := os.MkdirAll(binDir, 0755); err != nil {
				log.Printf("Failed to create directory %s: %v", binDir, err)
				continue
			}
		}

		if err := runCommand("mv", []string{binaryPath, targetPath}, ""); err != nil {
			log.Printf("Failed to move to %s: %v", targetPath, err)
			moveErr = err
			continue
		}

		// Success!
		moveErr = nil
		break
	}

	if moveErr != nil {
		return fmt.Errorf("failed to move binary to any location: %w", moveErr)
	}

	// Make it executable
	if err := runCommand("chmod", []string{"+x", targetPath}, ""); err != nil {
		return fmt.Errorf("failed to make binary executable: %w", err)
	}

	// If we installed to ~/bin, ensure it's in PATH
	if targetPath == fmt.Sprintf("%s/bin/kubectl-oadp", os.Getenv("HOME")) {
		homeBin := fmt.Sprintf("%s/bin", os.Getenv("HOME"))
		currentPath := os.Getenv("PATH")
		if !strings.Contains(currentPath, homeBin) {
			newPath := fmt.Sprintf("%s:%s", homeBin, currentPath)
			os.Setenv("PATH", newPath)
			log.Printf("Added %s to PATH", homeBin)
		}
	}

	log.Printf("Successfully installed kubectl-oadp to %s", targetPath)
	return nil
}

func (c *CLISetup) verifyInstallation() error {
	// Check current PATH
	cmd := exec.Command("bash", "-c", "echo $PATH")
	if output, err := cmd.CombinedOutput(); err == nil {
		log.Printf("Current PATH: %s", string(output))
	}

	// Try to find kubectl-oadp binary
	cmd = exec.Command("which", "kubectl-oadp")
	if output, err := cmd.CombinedOutput(); err == nil {
		log.Printf("kubectl-oadp found at: %s", string(output))
	} else {
		log.Printf("kubectl-oadp not found in PATH: %v", err)
	}

	// List kubectl plugins
	cmd = exec.Command("kubectl", "plugin", "list")
	if output, err := cmd.CombinedOutput(); err == nil {
		log.Printf("Available kubectl plugins: %s", string(output))
	}

	return runCommand("kubectl", []string{"oadp", "version"}, "")
}

func (c *CLISetup) configureNamespace() error {
	return runCommand("kubectl", []string{"oadp", "client", "config", "set", fmt.Sprintf("namespace=%s", c.namespace)}, "")
}

func runCommand(name string, args []string, dir string) error {
	cmd := exec.Command(name, args...)
	if dir != "" {
		cmd.Dir = dir
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("command '%s %s' failed: %v\nOutput: %s",
			name, strings.Join(args, " "), err, string(output))
	}
	return nil
}
