package main

import (
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/mateusoliveira43/oadp-must-gather/pkg"
)

// study ref https://github.com/openshift/oadp-operator/pull/1104

func init() {
	pkg.CLI.Flags().DurationVarP(&pkg.LogsSince, "logs-since", "l", 1*time.Hour, "TODO if zero, all")
	pkg.CLI.Flags().DurationVarP(&pkg.Timeout, "timeout", "t", 0, "TODO if zero, no timeout")
	pkg.CLI.Flags().BoolVarP(&pkg.SkipTLS, "skip-tls", "s", false, "TODO")
	// TODO pkg.CLI.Flags().BoolVarP(&essentialOnly, "essential-only", "e", false, "TODO")
	pkg.CLI.Flags().BoolP("help", "h", false, "Show OADP Must-gather help message.")
	// TODO JSON output in the future?

	pkg.CLI.SetHelpCommand(&cobra.Command{Hidden: true, Use: "mateus"})
}

func main() {
	err := pkg.CLI.Execute()
	if err != nil {
		os.Exit(1)
	}
}
