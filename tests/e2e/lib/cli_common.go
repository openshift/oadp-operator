package lib

import (
	"os/exec"
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

	cmd := exec.Command("kubectl", args...)
	return cmd.CombinedOutput()
}

func (c *CLICommand) ExecuteOutput() ([]byte, error) {
	args := []string{"oadp", c.Resource, c.Action}
	if c.Name != "" {
		args = append(args, c.Name)
	}
	args = append(args, c.Options...)

	cmd := exec.Command("kubectl", args...)
	return cmd.Output()
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
